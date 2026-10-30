package relay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
)

// claudeAdaptiveSupport is what an upstream reported about adaptive thinking
// for one (channel, upstream model) pair.
type claudeAdaptiveSupport uint8

const (
	claudeAdaptiveRequired claudeAdaptiveSupport = iota + 1
	claudeAdaptiveUnsupported
)

const (
	claudeAdaptiveSupportTTL      = 6 * time.Hour
	claudeAdaptiveSupportCapacity = 4096

	policyReasonClaudeAdaptiveRequired    = "claude_adaptive_thinking_required"
	policyReasonClaudeAdaptiveUnsupported = "claude_adaptive_thinking_unsupported"
)

// claudeAdaptiveSupportCache remembers upstream answers so later requests are
// shaped up front instead of paying the failed round trip again. It is
// per-process memory: every gateway node learns on its own, which costs at
// most one extra upstream 400 per node, channel and model per TTL.
var claudeAdaptiveSupportCache = hot.NewHotCache[string, claudeAdaptiveSupport](hot.LRU, claudeAdaptiveSupportCapacity).
	WithTTL(claudeAdaptiveSupportTTL).
	Build()

func claudeAdaptiveSupportKey(channelID int, model string) string {
	return fmt.Sprintf("%d|%s", channelID, model)
}

// isClaudeAdaptiveThinkingRequiredMessage reports whether an upstream 400
// says the model accepts only adaptive thinking. Matched wordings:
//   - "<model> requires adaptive thinking; omit thinking or use
//     thinking.type=adaptive and output_config.effort" (returned by
//     Claude-compatible upstreams for claude-opus-5-5);
//   - "thinking.type: omit thinking or use type adaptive" (the short form
//     some Claude-compatible upstreams return for claude-opus-5-5 and
//     claude-haiku-5-5);
//   - Anthropic's wording for adaptive-only models, which names the rejected
//     manual mode and the adaptive replacement together:
//     "thinking.type.enabled" is not supported for this model. Use
//     "thinking.type.adaptive" and "output_config.effort" ...
//     New API upstreams relay it with the dotted field paths masked as
//     domains ("***.***.enabled" ... "***.***.adaptive"), so the masked
//     paths match too.
func isClaudeAdaptiveThinkingRequiredMessage(message string) bool {
	message = strings.ToLower(message)
	if strings.Contains(message, "requires adaptive thinking") ||
		strings.Contains(message, "omit thinking or use type adaptive") {
		return true
	}
	return (containsThinkingTypePath(message, "enabled") || containsThinkingTypePath(message, "disabled")) &&
		strings.Contains(message, "not supported") &&
		containsThinkingTypePath(message, "adaptive")
}

func containsThinkingTypePath(message, thinkingType string) bool {
	return strings.Contains(message, "thinking.type."+thinkingType) ||
		strings.Contains(message, "***.***."+thinkingType)
}

// isClaudeAdaptiveThinkingRejectedMessage reports whether an upstream 400 for
// a request that the gateway rewrote to adaptive thinking says the upstream
// does not accept adaptive thinking. It is consulted only for rewritten
// requests, so the client's original controls are known to be the only
// difference. The message must name the adaptive mode together with an
// explicit rejection, for example a schema validator that only knows the
// manual modes ("Input tag 'adaptive' found using 'type' does not match any
// of the expected tags: 'disabled', 'enabled'") or "adaptive thinking is not
// supported for this model".
func isClaudeAdaptiveThinkingRejectedMessage(message string) bool {
	if isClaudeAdaptiveThinkingRequiredMessage(message) {
		return false
	}
	message = strings.ToLower(message)
	if !strings.Contains(message, "adaptive") {
		return false
	}
	for _, rejection := range []string{
		"not supported",
		"unsupported",
		"does not support",
		"does not match any of the expected tags",
		"not allowed",
		"invalid",
	} {
		if strings.Contains(message, rejection) {
			return true
		}
	}
	return false
}

// claudeThinkingControls are the request fields the adaptive rewrite can
// change. The conversion replaces them (it never mutates through the
// pointers), so a shallow copy restores the client's original controls.
type claudeThinkingControls struct {
	thinking     *dto.Thinking
	outputConfig json.RawMessage
	maxTokens    *uint
	temperature  *float64
	topP         *float64
	topK         *int
}

func captureClaudeThinkingControls(request *dto.ClaudeRequest) claudeThinkingControls {
	return claudeThinkingControls{
		thinking:     request.Thinking,
		outputConfig: request.OutputConfig,
		maxTokens:    request.MaxTokens,
		temperature:  request.Temperature,
		topP:         request.TopP,
		topK:         request.TopK,
	}
}

func (controls claudeThinkingControls) restore(request *dto.ClaudeRequest) {
	request.Thinking = controls.thinking
	request.OutputConfig = controls.outputConfig
	request.MaxTokens = controls.maxTokens
	request.Temperature = controls.temperature
	request.TopP = controls.topP
	request.TopK = controls.topK
}

// claudeThinkingCompat implements the channel's Claude thinking compatibility
// mode for one native Claude Messages attempt whose client request carries
// manual (enabled/disabled) thinking controls:
//
//   - "auto" sends the request as-is unless the upstream already reported
//     that the model requires adaptive thinking. When an attempt fails with a
//     400 saying so, the controls are rewritten to adaptive thinking and the
//     request is retried once on the same channel and key.
//   - "always" rewrites known adaptive-only models up front.
//   - both modes fall back once to the client's controls when a rewritten
//     request fails with a 400 rejecting adaptive thinking.
//
// The retry happens before any response bytes reach the client: upstream
// errors arrive as a non-200 status before the response handler runs.
// Billing is unaffected because pre-consume ran once before the relay
// attempt, failed upstream 400s are not settled, and only the final response
// is settled.
type claudeThinkingCompat struct {
	key      string
	original claudeThinkingControls
	// sentThinkingType is the thinking type of the last converted native
	// Claude request; empty when the outgoing request is not native Claude.
	sentThinkingType string
	retried          bool
	// failedThinkingType, retryStatus and retryReason describe the attempt
	// that triggered the retry.
	failedThinkingType string
	retryStatus        int
	retryReason        string
}

// newClaudeThinkingCompat returns nil when the compatibility handling cannot
// apply to the request: the channel mode is "off" or the client did not send
// manual thinking controls.
func newClaudeThinkingCompat(info *relaycommon.RelayInfo, request *dto.ClaudeRequest) *claudeThinkingCompat {
	if info.ClaudeThinkingCompatMode() == dto.ClaudeThinkingCompatModeOff {
		return nil
	}
	if request.Thinking == nil || request.Thinking.Type != "enabled" && request.Thinking.Type != "disabled" {
		return nil
	}
	compat := &claudeThinkingCompat{
		key:      claudeAdaptiveSupportKey(info.ChannelId, request.Model),
		original: captureClaudeThinkingControls(request),
	}
	if support, found, _ := claudeAdaptiveSupportCache.Get(compat.key); found {
		switch support {
		case claudeAdaptiveRequired:
			info.SetClaudeAdaptiveThinking(relaycommon.ClaudeAdaptiveThinkingRewrite)
		case claudeAdaptiveUnsupported:
			info.SetClaudeAdaptiveThinking(relaycommon.ClaudeAdaptiveThinkingKeep)
		}
	}
	return compat
}

// observe records the thinking controls of the request that is about to be
// sent. Only native Claude Messages requests are eligible for a retry.
func (t *claudeThinkingCompat) observe(converted any) {
	if t == nil {
		return
	}
	t.sentThinkingType = ""
	if request, ok := converted.(*dto.ClaudeRequest); ok && request.Thinking != nil {
		t.sentThinkingType = request.Thinking.Type
	}
}

// prepareRetry decides whether a failed attempt is retried on the same
// channel with the other form of thinking controls. On a retry it records
// the upstream's answer, restores the client's controls and sets the
// attempt's rewrite decision for the next conversion.
func (t *claudeThinkingCompat) prepareRetry(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest, status int, apiErr *types.NewAPIError) bool {
	if t == nil || t.retried || status != http.StatusBadRequest || apiErr == nil || c.Writer.Written() {
		return false
	}
	message := apiErr.Error()
	var (
		support  claudeAdaptiveSupport
		decision relaycommon.ClaudeAdaptiveThinking
		reason   string
	)
	switch t.sentThinkingType {
	case "enabled", "disabled":
		if !isClaudeAdaptiveThinkingRequiredMessage(message) {
			return false
		}
		support, decision, reason = claudeAdaptiveRequired, relaycommon.ClaudeAdaptiveThinkingRewrite, policyReasonClaudeAdaptiveRequired
	case "adaptive":
		// The client sent manual controls, so adaptive means the gateway
		// rewrote them.
		if !isClaudeAdaptiveThinkingRejectedMessage(message) {
			return false
		}
		support, decision, reason = claudeAdaptiveUnsupported, relaycommon.ClaudeAdaptiveThinkingKeep, policyReasonClaudeAdaptiveUnsupported
	default:
		return false
	}

	claudeAdaptiveSupportCache.Set(t.key, support)
	t.retried = true
	t.failedThinkingType, t.retryStatus, t.retryReason = t.sentThinkingType, status, reason
	t.original.restore(request)
	info.SetClaudeAdaptiveThinking(decision)
	return true
}

// retryUnchanged reports, after the retry request was converted, whether it
// would resend the thinking type that just failed. That happens when the
// model's capability profile has no adaptive rendering for the client's
// controls; the caller then returns the original error instead of resending.
func (t *claudeThinkingCompat) retryUnchanged() bool {
	return t != nil && t.retried && t.sentThinkingType == t.failedThinkingType
}

// recordRetry makes the same-channel retry visible in the request policy
// decisions of the log details and in the backend log.
func (t *claudeThinkingCompat) recordRetry(c *gin.Context, info *relaycommon.RelayInfo) {
	service.RecordSameChannelRetry(c, info.ChannelId, t.retryStatus, t.retryReason)
	logger.LogWarn(c, "claude thinking compatibility: channel #%d returned %d (%s); retrying once on the same channel with thinking.type=%s",
		info.ChannelId, t.retryStatus, t.retryReason, t.sentThinkingType)
}

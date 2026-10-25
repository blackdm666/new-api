package gemini

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// shouldForceUpstreamStream reports whether a non-streaming text request goes
// upstream as streamGenerateContent because the channel enables
// force_upstream_stream. Image, speech and embedding models keep their own
// endpoints, and Responses requests keep their existing conversion path.
func shouldForceUpstreamStream(info *relaycommon.RelayInfo) bool {
	if info == nil || info.ChannelMeta == nil || info.IsStream || !info.ChannelSetting.ForceUpstreamStream {
		return false
	}
	// Adaptors that embed this one (New API, advanced custom) build their own
	// upstream requests.
	if info.ChannelType != appconstant.ChannelTypeGemini {
		return false
	}
	model := strings.ToLower(info.UpstreamModelName)
	if strings.HasPrefix(model, "imagen") || strings.Contains(model, "embedding") ||
		strings.Contains(model, "image") || strings.Contains(model, "banana") || strings.Contains(model, "tts") {
		return false
	}
	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		return info.RelayMode == constant.RelayModeChatCompletions
	case types.RelayFormatClaude:
		return true
	case types.RelayFormatGemini:
		return info.RelayMode == constant.RelayModeGemini && strings.Contains(info.RequestURLPath, ":generateContent")
	default:
		return false
	}
}

// collectForcedUpstreamStream reads the upstream SSE stream to the end and
// returns a response that looks like the generateContent call the client
// made: a JSON body on success, or the upstream's own error with its status
// code when the stream carries an error frame. Nothing reaches the client
// until the stream is complete, so a cut stream fails as a whole and can be
// retried on another channel instead of returning partial text.
func collectForcedUpstreamStream(c *gin.Context, resp *http.Response) (*http.Response, error) {
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return resp, nil
	}
	defer service.CloseResponseBodyGracefully(resp)

	// A non-streaming call may wait for the upstream headers up to
	// RELAY_RESPONSE_HEADER_TIMEOUT, so the gap between two frames gets the
	// same allowance instead of STREAMING_TIMEOUT (which would cut silent
	// thinking that the non-streaming call used to survive).
	var idleTimedOut atomic.Bool
	if idle := time.Duration(common.RelayResponseHeaderTimeout) * time.Second; idle > 0 {
		timer := time.AfterFunc(idle, func() {
			idleTimedOut.Store(true)
			_ = resp.Body.Close()
		})
		defer timer.Stop()
		resp.Body = &idleResetReader{reader: resp.Body, timer: timer, idle: idle}
	}
	stopOnClientGone := context.AfterFunc(c.Request.Context(), func() { _ = resp.Body.Close() })
	defer stopOnClientGone()

	collector := &geminiStreamCollector{}
	scanner := helper.NewStreamScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data:")
		data = strings.TrimSpace(data)
		if !ok || data == "" || data == "[DONE]" {
			continue
		}
		upstreamErr, err := collector.addFrame([]byte(data))
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		}
		if upstreamErr != nil {
			return upstreamErrorResponse(resp, upstreamErr)
		}
	}
	if err := scanner.Err(); err != nil || idleTimedOut.Load() {
		switch {
		case c.Request.Context().Err() != nil:
			return nil, c.Request.Context().Err()
		case idleTimedOut.Load():
			return nil, types.NewOpenAIError(fmt.Errorf("upstream stream idle for more than %d seconds", common.RelayResponseHeaderTimeout), types.ErrorCodeBadResponse, http.StatusGatewayTimeout)
		default:
			return nil, types.NewOpenAIError(fmt.Errorf("read upstream stream: %w", err), types.ErrorCodeBadResponse, http.StatusBadGateway)
		}
	}
	if !collector.completed() {
		logger.LogWarn(c, common.LogText("forced upstream stream ended before completion after %d frames", collector.frames))
		return nil, types.NewOpenAIError(errors.New("upstream stream ended before completion"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	body, err := collector.response()
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	return rebuiltResponse(resp, http.StatusOK, body), nil
}

// idleResetReader pushes the idle deadline back on every successful read.
type idleResetReader struct {
	reader io.ReadCloser
	timer  *time.Timer
	idle   time.Duration
}

func (r *idleResetReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.timer.Reset(r.idle)
	}
	return n, err
}

func (r *idleResetReader) Close() error {
	return r.reader.Close()
}

func upstreamErrorResponse(resp *http.Response, upstreamErr any) (*http.Response, error) {
	statusCode := http.StatusInternalServerError
	if errMap, ok := upstreamErr.(map[string]any); ok {
		if code, ok := errMap["code"].(float64); ok && code >= 400 && code <= 599 {
			statusCode = int(code)
		}
	}
	body, err := common.Marshal(map[string]any{"error": upstreamErr})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	return rebuiltResponse(resp, statusCode, body), nil
}

func rebuiltResponse(resp *http.Response, statusCode int, body []byte) *http.Response {
	header := resp.Header.Clone()
	header.Set("Content-Type", "application/json")
	header.Del("Content-Length")
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		StatusCode:    statusCode,
		Proto:         resp.Proto,
		ProtoMajor:    resp.ProtoMajor,
		ProtoMinor:    resp.ProtoMinor,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       resp.Request,
	}
}

// geminiStreamCollector merges streamGenerateContent frames into the body a
// generateContent call returns. Frames are kept as generic JSON so fields the
// DTOs do not model (citationMetadata, responseId, ...) survive for native
// Gemini clients.
type geminiStreamCollector struct {
	frames     int
	top        map[string]any
	candidates []map[string]any
	byIndex    map[float64]map[string]any
	usage      *dto.GeminiUsageMetadata
}

// addFrame merges one SSE data payload. It returns the frame's "error" value
// when the upstream reported an error inside the stream.
func (s *geminiStreamCollector) addFrame(data []byte) (any, error) {
	var frame map[string]any
	if err := common.Unmarshal(data, &frame); err != nil {
		return nil, fmt.Errorf("unmarshal Gemini stream frame: %w", err)
	}
	if upstreamErr, ok := frame["error"]; ok {
		return upstreamErr, nil
	}
	s.frames++
	if s.top == nil {
		s.top = map[string]any{}
		s.byIndex = map[float64]map[string]any{}
	}

	// usageMetadata is a cumulative snapshot per frame; merge it with the
	// same rule the streaming handler bills with.
	var frameUsage struct {
		UsageMetadata *dto.GeminiUsageMetadata `json:"usageMetadata"`
	}
	if err := common.Unmarshal(data, &frameUsage); err != nil {
		return nil, fmt.Errorf("unmarshal Gemini stream usage: %w", err)
	}
	if dto.HasGeminiUsageMetadataTokens(frameUsage.UsageMetadata) {
		s.usage = dto.MergeGeminiUsageMetadataNonZero(s.usage, frameUsage.UsageMetadata)
	}

	for key, value := range frame {
		if key == "candidates" {
			continue
		}
		s.top[key] = value
	}
	candidates, _ := frame["candidates"].([]any)
	for _, raw := range candidates {
		candidate, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		s.addCandidate(candidate)
	}
	return nil, nil
}

func (s *geminiStreamCollector) addCandidate(candidate map[string]any) {
	index, _ := candidate["index"].(float64)
	merged, ok := s.byIndex[index]
	if !ok {
		merged = map[string]any{}
		s.byIndex[index] = merged
		s.candidates = append(s.candidates, merged)
	}
	for key, value := range candidate {
		if key == "content" {
			continue
		}
		merged[key] = value
	}
	content, ok := candidate["content"].(map[string]any)
	if !ok {
		return
	}
	mergedContent, _ := merged["content"].(map[string]any)
	if mergedContent == nil {
		mergedContent = map[string]any{}
		merged["content"] = mergedContent
	}
	parts, _ := mergedContent["parts"].([]any)
	for key, value := range content {
		if key != "parts" {
			mergedContent[key] = value
		}
	}
	incoming, _ := content["parts"].([]any)
	for _, raw := range incoming {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		parts = appendGeminiStreamPart(parts, part)
	}
	mergedContent["parts"] = parts
}

// appendGeminiStreamPart folds streamed text back into the parts a
// generateContent response has. A stream splits one text part across frames
// and delivers its thoughtSignature on a trailing empty-text part, while
// generateContent returns the whole text with the signature on the same part.
// So consecutive text parts with the same thought flag are concatenated until
// one of them carries a signature; empty text without a signature is dropped.
// Function calls and other parts are kept as they are, with their signatures.
func appendGeminiStreamPart(parts []any, part map[string]any) []any {
	text, isText := plainGeminiTextPart(part)
	if !isText {
		return append(parts, part)
	}
	signature, hasSignature := part["thoughtSignature"]
	if text == "" && !hasSignature {
		return parts
	}
	if len(parts) > 0 {
		last, _ := parts[len(parts)-1].(map[string]any)
		lastText, lastIsText := plainGeminiTextPart(last)
		_, lastHasSignature := last["thoughtSignature"]
		if lastIsText && !lastHasSignature && last["thought"] == part["thought"] {
			last["text"] = lastText + text
			if hasSignature {
				last["thoughtSignature"] = signature
			}
			return parts
		}
	}
	return append(parts, part)
}

// plainGeminiTextPart reports whether part holds only text, its thought flag
// and its signature, and returns the text.
func plainGeminiTextPart(part map[string]any) (string, bool) {
	text, ok := part["text"].(string)
	if !ok {
		return "", false
	}
	for key := range part {
		switch key {
		case "text", "thought", "thoughtSignature":
		default:
			return "", false
		}
	}
	return text, true
}

// completed reports whether the stream reached its end: a candidate with a
// finish reason, or a prompt blocked before any candidate.
func (s *geminiStreamCollector) completed() bool {
	if s.frames == 0 {
		return false
	}
	for _, candidate := range s.candidates {
		if reason, _ := candidate["finishReason"].(string); reason != "" && reason != "FINISH_REASON_UNSPECIFIED" {
			return true
		}
	}
	feedback, _ := s.top["promptFeedback"].(map[string]any)
	reason, _ := feedback["blockReason"].(string)
	return len(s.candidates) == 0 && reason != ""
}

func (s *geminiStreamCollector) response() ([]byte, error) {
	out := make(map[string]any, len(s.top)+1)
	for key, value := range s.top {
		out[key] = value
	}
	candidates := make([]any, 0, len(s.candidates))
	for _, candidate := range s.candidates {
		candidates = append(candidates, candidate)
	}
	out["candidates"] = candidates
	if s.usage != nil {
		usageJSON, err := common.Marshal(s.usage)
		if err != nil {
			return nil, err
		}
		var usage map[string]any
		if err := common.Unmarshal(usageJSON, &usage); err != nil {
			return nil, err
		}
		raw, _ := out["usageMetadata"].(map[string]any)
		if raw == nil {
			raw = map[string]any{}
		}
		for key, value := range usage {
			// Unreported detail lists marshal as null or []; generateContent omits them.
			if list, isList := value.([]any); value == nil || (isList && len(list) == 0) {
				continue
			}
			raw[key] = value
		}
		out["usageMetadata"] = raw
	}
	return common.Marshal(out)
}

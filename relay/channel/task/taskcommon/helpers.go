package taskcommon

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// UnmarshalMetadata converts a map[string]any metadata to a typed struct via JSON round-trip.
// This replaces the repeated pattern: json.Marshal(metadata) → json.Unmarshal(bytes, &target).
func UnmarshalMetadata(metadata map[string]any, target any) error {
	if metadata == nil {
		return nil
	}
	// Prevent metadata from overriding model fields to avoid billing bypass.
	delete(metadata, "model")
	metaBytes, err := common.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata failed: %w", err)
	}
	if err := common.Unmarshal(metaBytes, target); err != nil {
		return fmt.Errorf("unmarshal metadata failed: %w", err)
	}
	return nil
}

// DefaultString returns val if non-empty, otherwise fallback.
func DefaultString(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

// ResolvedModelName returns the model name that an adaptor must use for
// upstream capability checks and request construction. Task relays resolve
// channel model mappings globally before adaptor validation, so a populated
// UpstreamModelName always takes precedence over the public request model.
func ResolvedModelName(info *relaycommon.RelayInfo, requestModel string) string {
	if info != nil && info.ChannelMeta != nil {
		if modelName := strings.TrimSpace(info.UpstreamModelName); modelName != "" {
			return modelName
		}
	}
	return strings.TrimSpace(requestModel)
}

// ModelConfigCandidates returns model names in sales-profile order. Adaptors
// with public model variants (for example a fixed 1080p SKU) can preserve the
// public profile when it is known, while arbitrary aliases fall back to the
// globally resolved upstream model.
func ModelConfigCandidates(info *relaycommon.RelayInfo, requestModel string) []string {
	candidates := make([]string, 0, 3)
	seen := make(map[string]struct{}, 3)
	add := func(modelName string) {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			return
		}
		if _, exists := seen[modelName]; exists {
			return
		}
		seen[modelName] = struct{}{}
		candidates = append(candidates, modelName)
	}
	if info != nil {
		add(info.OriginModelName)
	}
	add(requestModel)
	if info != nil && info.ChannelMeta != nil {
		add(info.UpstreamModelName)
	}
	return candidates
}

// DefaultInt returns val if non-zero, otherwise fallback.
func DefaultInt(val, fallback int) int {
	if val == 0 {
		return fallback
	}
	return val
}

// EncodeLocalTaskID encodes an upstream operation name to a URL-safe base64 string.
// Used by Gemini/Vertex to store upstream names as task IDs.
func EncodeLocalTaskID(name string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(name))
}

// DecodeLocalTaskID decodes a base64-encoded upstream operation name.
func DecodeLocalTaskID(id string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// BuildProxyURL constructs the video proxy URL using the public task ID.
// e.g., "https://your-server.com/v1/videos/task_xxxx/content"
func BuildProxyURL(taskID string) string {
	return fmt.Sprintf("%s/v1/videos/%s/content", system_setting.ServerAddress, taskID)
}

// Status-to-progress mapping constants for polling updates.
const (
	ProgressSubmitted  = "10%"
	ProgressQueued     = "20%"
	ProgressInProgress = "30%"
	ProgressComplete   = "100%"
)

// ---------------------------------------------------------------------------
// BaseBilling — embeddable no-op implementations for TaskAdaptor billing methods.
// Adaptors that do not need custom billing can embed this struct directly.
// ---------------------------------------------------------------------------

type BaseBilling struct{}

// EstimateBilling returns nil (no extra ratios; use base model price).
func (BaseBilling) EstimateBilling(_ *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	return nil
}

// AdjustBillingOnSubmit returns nil (no submit-time adjustment).
func (BaseBilling) AdjustBillingOnSubmit(_ *relaycommon.RelayInfo, _ []byte) map[string]float64 {
	return nil
}

// AdjustBillingOnComplete returns 0 (keep pre-charged amount).
func (BaseBilling) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}

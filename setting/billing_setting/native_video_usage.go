package billing_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
)

// NativeVideoUsageSchema describes the quantities exposed by the existing Go
// video adapters. It does not register routes or change any model's price.
func NativeVideoUsageSchema(model string) map[string]jsplugin.UsageFieldSchema {
	name := strings.ToLower(strings.TrimSpace(model))
	veo := strings.HasPrefix(name, "veo-")
	if !veo && !strings.HasPrefix(name, "grok-imagine-video") && !strings.HasPrefix(name, "gemini-omni-flash") {
		return nil
	}
	schema := map[string]jsplugin.UsageFieldSchema{
		"seconds": {Type: "number", Unit: "second", Description: jsplugin.LocalizedText{"en": "Video generation unit price", "zh": "视频生成单价"}},
	}
	if veo {
		schema["resolution"] = jsplugin.UsageFieldSchema{Enum: []string{"720p", "1080p", "4k"},
			Description: jsplugin.LocalizedText{"en": "Output video resolution", "zh": "输出视频分辨率"}}
	}
	return schema
}

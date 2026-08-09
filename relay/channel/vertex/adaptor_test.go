package vertex

import (
	"encoding/json"
	"fmt"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertClaudeRequestToGeminiForGoogleModel(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3.6-flash",
		RelayFormat:     types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3.6-flash",
		},
	}
	adaptor.Init(info)
	require.Equal(t, RequestModeGemini, adaptor.RequestMode)

	maxTokens := uint(64)
	converted, err := adaptor.ConvertClaudeRequest(nil, info, &dto.ClaudeRequest{
		Model:     "gemini-3.6-flash",
		System:    "Follow the system instruction.",
		MaxTokens: &maxTokens,
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	})
	require.NoError(t, err)

	geminiRequest, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, geminiRequest.Contents, 1)
	assert.Equal(t, "user", geminiRequest.Contents[0].Role)
	require.Len(t, geminiRequest.Contents[0].Parts, 1)
	assert.Equal(t, "hello", geminiRequest.Contents[0].Parts[0].Text)
	require.NotNil(t, geminiRequest.SystemInstructions)
	require.Len(t, geminiRequest.SystemInstructions.Parts, 1)
	assert.Equal(t, "Follow the system instruction.", geminiRequest.SystemInstructions.Parts[0].Text)
	assert.Equal(t, maxTokens, *geminiRequest.GenerationConfig.MaxOutputTokens)
	assert.Equal(t, []types.RelayFormat{types.RelayFormatGemini}, info.RequestConversionChain)

	payload, err := json.Marshal(geminiRequest)
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"contents"`)
	assert.NotContains(t, string(payload), `"anthropic_version"`)
	assert.NotContains(t, string(payload), `"messages"`)
	assert.NotContains(t, string(payload), `"max_tokens"`)
}

func TestConvertClaudeImageRequestToGeminiPreserves4KConfig(t *testing.T) {
	tests := []struct {
		name        string
		aspectRatio string
		imageSize   string
	}{
		{name: "square_1k", aspectRatio: "1:1", imageSize: "1K"},
		{name: "portrait_2k", aspectRatio: "9:16", imageSize: "2K"},
		{name: "landscape_4k", aspectRatio: "16:9", imageSize: "4K"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adaptor := &Adaptor{}
			info := &relaycommon.RelayInfo{
				OriginModelName: "gemini-3-pro-image-preview",
				RelayFormat:     types.RelayFormatClaude,
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "gemini-3-pro-image-preview",
				},
			}
			adaptor.Init(info)
			require.Equal(t, RequestModeGemini, adaptor.RequestMode)

			body := fmt.Sprintf(`{
				"model":"gemini-3-pro-image-preview",
				"max_tokens":1024,
				"messages":[{"role":"user","content":"Generate an image."}],
				"extra_body":{"google":{"image_config":{"aspect_ratio":%q,"image_size":%q}}}
			}`, tt.aspectRatio, tt.imageSize)
			var request dto.ClaudeRequest
			require.NoError(t, json.Unmarshal([]byte(body), &request))

			converted, err := adaptor.ConvertClaudeRequest(nil, info, &request)
			require.NoError(t, err)
			require.IsType(t, &dto.GeminiChatRequest{}, converted)
			geminiRequest := converted.(*dto.GeminiChatRequest)

			expectedConfig := fmt.Sprintf(`{"aspectRatio":%q,"imageSize":%q}`, tt.aspectRatio, tt.imageSize)
			assert.JSONEq(t, expectedConfig, string(geminiRequest.GenerationConfig.ImageConfig))
			payload, err := json.Marshal(geminiRequest)
			require.NoError(t, err)
			assert.NotContains(t, string(payload), `"extra_body"`)
			assert.Equal(t, []types.RelayFormat{types.RelayFormatGemini}, info.RequestConversionChain)
		})
	}
}

package vertex

import (
	"encoding/json"
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
	assert.Equal(t, []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatGemini}, info.RequestConversionChain)

	payload, err := json.Marshal(geminiRequest)
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"contents"`)
	assert.NotContains(t, string(payload), `"anthropic_version"`)
	assert.NotContains(t, string(payload), `"messages"`)
	assert.NotContains(t, string(payload), `"max_tokens"`)
}

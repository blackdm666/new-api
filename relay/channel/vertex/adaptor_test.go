package vertex

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
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

func TestConvertAudioSpeechRequestToVertexGeminiTTS(t *testing.T) {
	speed := 1.25
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeAudioSpeech,
		OriginModelName: "gemini-2.5-flash-tts",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-2.5-flash-tts",
		},
	}
	adaptor.Init(info)

	body, err := adaptor.ConvertAudioRequest(nil, info, dto.AudioRequest{
		Model:        "gemini-2.5-flash-tts",
		Input:        "Hello from NewAPI.",
		Voice:        "Kore",
		Instructions: "Speak warmly",
		Speed:        &speed,
	})
	require.NoError(t, err)
	require.Equal(t, "wav", adaptor.AudioResponseFormat)

	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	var converted dto.GeminiChatRequest
	require.NoError(t, json.Unmarshal(payload, &converted))
	require.Len(t, converted.Contents, 1)
	require.Len(t, converted.Contents[0].Parts, 1)
	assert.Equal(t, "Speak warmly. Speak at 1.25x normal speed: Hello from NewAPI.", converted.Contents[0].Parts[0].Text)
	assert.Equal(t, []string{"AUDIO"}, converted.GenerationConfig.ResponseModalities)
	assert.JSONEq(t, `{"voiceConfig":{"prebuiltVoiceConfig":{"voiceName":"Kore"}}}`, string(converted.GenerationConfig.SpeechConfig))
}

func TestConvertAudioSpeechRequestRejectsUnsupportedFormat(t *testing.T) {
	adaptor := &Adaptor{RequestMode: RequestModeGemini}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech}

	_, err := adaptor.ConvertAudioRequest(nil, info, dto.AudioRequest{
		Model:          "gemini-2.5-flash-tts",
		Input:          "Hello",
		Voice:          "Kore",
		ResponseFormat: "mp3",
	})
	require.EqualError(t, err, `Vertex AI audio speech supports response_format pcm or wav, got "mp3"`)
}

func TestVertexAudioSpeechUsesV1Beta1GenerateContentURL(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeAudioSpeech,
		OriginModelName: "gemini-2.5-flash-tts",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiVersion:        "us-central1",
			ApiKey:            "test-key",
			UpstreamModelName: "gemini-2.5-flash-tts",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VertexKeyType: dto.VertexKeyTypeAPIKey,
			},
		},
	}
	adaptor.Init(info)

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://us-central1-aiplatform.googleapis.com/v1beta1/publishers/google/models/gemini-2.5-flash-tts:generateContent?key=test-key", requestURL)
}

func TestVertexTTSResponseDecodesPCMAndWrapsWAV(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pcm := []byte{0x01, 0x02, 0x03, 0x04}
	responseJSON := fmt.Sprintf(`{
		"candidates":[{"content":{"role":"model","parts":[{"inlineData":{"mimeType":"audio/L16;codec=pcm;rate=24000","data":%q}}]}}],
		"usageMetadata":{
			"promptTokenCount":3,
			"candidatesTokenCount":20,
			"totalTokenCount":23,
			"promptTokensDetails":[{"modality":"TEXT","tokenCount":3}],
			"candidatesTokensDetails":[{"modality":"AUDIO","tokenCount":20}]
		}
	}`, base64.StdEncoding.EncodeToString(pcm))

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(responseJSON)),
	}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-2.5-flash-tts",
		},
	}
	adaptor := &Adaptor{RequestMode: RequestModeGemini, AudioResponseFormat: "wav"}

	usageValue, apiErr := adaptor.DoResponse(context, resp, info)
	require.Nil(t, apiErr)
	usage, ok := usageValue.(*dto.Usage)
	require.True(t, ok)
	assert.Equal(t, 3, usage.PromptTokens)
	assert.Equal(t, 20, usage.CompletionTokens)
	assert.Equal(t, 20, usage.CompletionTokenDetails.AudioTokens)
	assert.Equal(t, "audio/wav", recorder.Header().Get("Content-Type"))
	require.Len(t, recorder.Body.Bytes(), 44+len(pcm))
	assert.Equal(t, "RIFF", recorder.Body.String()[:4])
	assert.Equal(t, "WAVE", recorder.Body.String()[8:12])
	assert.Equal(t, pcm, recorder.Body.Bytes()[44:])
}

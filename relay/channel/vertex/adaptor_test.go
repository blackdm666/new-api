package vertex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/mewkiz/flac"
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
		Voice:        "kore",
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

// Clients often send OpenAI defaults (mp3, alloy) or call the TTS model through
// chat. Every client mistake is a 400 that names the fix in the client's language.
func TestVertexTTSClientErrorsExplainTheFix(t *testing.T) {
	require.NoError(t, i18n.Init())
	common.DefaultLanguage = i18n.LangZhCN
	t.Cleanup(func() { common.DefaultLanguage = "" })

	speed := 5.0
	tests := []struct {
		name    string
		request dto.AudioRequest
		stream  bool
		want    string
	}{
		{name: "format", request: dto.AudioRequest{Input: "你好", Voice: "Kore", ResponseFormat: "opus"},
			want: "gemini-3.1-tts 不支持 response_format「opus」，可选 mp3、wav、flac、pcm，不传默认 wav"},
		{name: "openai voice", request: dto.AudioRequest{Input: "你好", Voice: "alloy"},
			want: "gemini-3.1-tts 不支持音色「alloy」，OpenAI 的 alloy、nova 等音色名不能使用。可用音色：" + strings.Join(geminiTTSVoices, ", ")},
		{name: "missing voice", request: dto.AudioRequest{Input: "你好"},
			want: "缺少 voice：请指定 Gemini 预设音色，例如 Kore。可用音色：" + strings.Join(geminiTTSVoices, ", ")},
		{name: "missing input", request: dto.AudioRequest{Input: "  ", Voice: "Kore"},
			want: "缺少 input：请传入要合成的文本"},
		{name: "speed", request: dto.AudioRequest{Input: "你好", Voice: "Kore", Speed: &speed},
			want: "speed 取值范围为 0.25–4，当前为 5"},
		{name: "too long", request: dto.AudioRequest{Input: strings.Repeat("a", 8001), Voice: "Kore"},
			want: "input、instructions 和 speed 生成的语速指令合计不能超过 8000 字节（中文约 2600 字），当前 8001 字节，请分段合成"},
		{name: "stream", request: dto.AudioRequest{Input: "你好", Voice: "Kore", StreamFormat: "sse"},
			want: "gemini-3.1-tts 不支持流式输出，请去掉 stream 或 stream_format=sse，接口会一次性返回完整音频"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adaptor := &Adaptor{RequestMode: RequestModeGemini}
			// By conversion time the request carries the mapped upstream model;
			// messages must name the model the client asked for.
			info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech, IsStream: tt.stream, OriginModelName: "gemini-3.1-tts"}
			tt.request.Model = "gemini-3.1-flash-tts-preview"

			_, err := adaptor.ConvertAudioRequest(nil, info, tt.request)
			require.EqualError(t, err, tt.want)
			var apiErr *types.NewAPIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.Equal(t, types.ErrorCodeInvalidRequest, apiErr.GetErrorCode())
		})
	}

	t.Run("chat endpoint", func(t *testing.T) {
		adaptor := &Adaptor{RequestMode: RequestModeGemini}
		info := &relaycommon.RelayInfo{
			OriginModelName: "gemini-3.1-tts",
			ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-tts-preview"},
		}
		_, err := adaptor.ConvertOpenAIRequest(nil, info, &dto.GeneralOpenAIRequest{Model: "gemini-3.1-tts"})
		require.EqualError(t, err, "gemini-3.1-tts 是语音合成模型，不能用对话接口调用，请改用 POST /v1/audio/speech（OpenAI 兼容）")
		var apiErr *types.NewAPIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	})

	t.Run("no audio", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"OTHER"}]}`)),
		}
		info := &relaycommon.RelayInfo{
			RelayMode:   relayconstant.RelayModeAudioSpeech,
			ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-tts-preview"},
		}
		adaptor := &Adaptor{RequestMode: RequestModeGemini, AudioResponseFormat: "mp3"}

		_, apiErr := adaptor.DoResponse(context, resp, info)
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
		assert.Equal(t, "模型没有返回音频（finishReason=OTHER），通常是偶发情况，请稍后重试；如果反复出现，请检查文本是否过短或只有符号", apiErr.Error())
	})
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

func TestVertexTTSResponseEncodesRequestedFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 1.5 s of a ramp signal; no candidatesTokenCount, so audio tokens come from
	// the PCM length: ceil(1.5 s) = 2 s -> 2/60*1000 = 33 tokens for every format.
	sampleCount := vertexTTSSampleRate * 3 / 2
	pcm := make([]byte, sampleCount*2)
	for i := range sampleCount {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(i%2000-1000)))
	}
	responseJSON := fmt.Sprintf(`{
		"candidates":[{"content":{"role":"model","parts":[{"inlineData":{"mimeType":"audio/L16;codec=pcm;rate=24000","data":%q}}]}}],
		"usageMetadata":{"promptTokenCount":3,"totalTokenCount":3}
	}`, base64.StdEncoding.EncodeToString(pcm))

	tests := []struct {
		format      string
		contentType string
		check       func(t *testing.T, body []byte)
	}{
		{format: "mp3", contentType: "audio/mpeg", check: func(t *testing.T, body []byte) {
			duration, err := common.GetAudioDuration(context.Background(), bytes.NewReader(body), ".mp3")
			require.NoError(t, err)
			// Whole 576-sample frames plus the delay-flush frames: never shorter
			// than the speech, at most 72 ms longer.
			assert.GreaterOrEqual(t, duration, 1.5)
			assert.LessOrEqual(t, duration, 1.572)
		}},
		{format: "flac", contentType: "audio/flac", check: func(t *testing.T, body []byte) {
			stream, err := flac.New(bytes.NewReader(body))
			require.NoError(t, err)
			assert.Equal(t, uint32(vertexTTSSampleRate), stream.Info.SampleRate)
			assert.Equal(t, uint8(vertexTTSChannels), stream.Info.NChannels)
			assert.Equal(t, uint64(sampleCount), stream.Info.NSamples)
			decoded := make([]byte, 0, len(pcm))
			for {
				audioFrame, err := stream.ParseNext()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				for _, sample := range audioFrame.Subframes[0].Samples {
					decoded = binary.LittleEndian.AppendUint16(decoded, uint16(int16(sample)))
				}
			}
			assert.Equal(t, pcm, decoded)
		}},
		{format: "wav", contentType: "audio/wav", check: func(t *testing.T, body []byte) {
			assert.Equal(t, pcm, body[44:])
		}},
		{format: "pcm", contentType: "audio/pcm", check: func(t *testing.T, body []byte) {
			assert.Equal(t, pcm, body)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(responseJSON)),
			}
			info := &relaycommon.RelayInfo{
				RelayMode:   relayconstant.RelayModeAudioSpeech,
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-tts"},
			}
			adaptor := &Adaptor{RequestMode: RequestModeGemini, AudioResponseFormat: tt.format}

			usageValue, apiErr := adaptor.DoResponse(context, resp, info)
			require.Nil(t, apiErr)
			usage, ok := usageValue.(*dto.Usage)
			require.True(t, ok)
			assert.Equal(t, 33, usage.CompletionTokens)
			assert.Equal(t, 33, usage.CompletionTokenDetails.AudioTokens)
			assert.Equal(t, tt.contentType, recorder.Header().Get("Content-Type"))
			tt.check(t, recorder.Body.Bytes())
		})
	}
}

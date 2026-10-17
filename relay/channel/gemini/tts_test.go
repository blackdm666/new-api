package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
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

// A Gemini channel (for example one in front of an account pool) answers
// OpenAI /v1/audio/speech through generateContent with AUDIO output.
func TestGeminiChannelServesAudioSpeech(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeAudioSpeech,
		OriginModelName: "gemini-3.1-tts",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "http://sub2api:8080",
			ApiKey:            "pool-key",
			UpstreamModelName: "gemini-3.1-flash-tts-preview",
		},
	}

	body, err := adaptor.ConvertAudioRequest(nil, info, dto.AudioRequest{
		Model: "gemini-3.1-flash-tts-preview",
		Input: "你好",
		Voice: "kore",
	})
	require.NoError(t, err)
	assert.Equal(t, "wav", adaptor.AudioResponseFormat)
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"contents":[{"role":"user","parts":[{"text":"你好"}]}],
		"generationConfig":{"responseModalities":["AUDIO"],"speechConfig":{"voiceConfig":{"prebuiltVoiceConfig":{"voiceName":"Kore"}}}}
	}`, string(payload))

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "http://sub2api:8080/v1beta/models/gemini-3.1-flash-tts-preview:generateContent", requestURL)

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	header := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(context, &header, info))
	assert.Equal(t, "pool-key", header.Get("x-goog-api-key"))
}

// Clients often send OpenAI defaults (mp3, alloy) or call the TTS model through
// chat. Every client mistake is a 400 that names the fix in the client's language.
func TestSpeechClientErrorsExplainTheFix(t *testing.T) {
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
			want: "gemini-3.1-tts 不支持音色「alloy」，OpenAI 的 alloy、nova 等音色名不能使用。可用音色：" + strings.Join(ttsVoices, ", ")},
		{name: "missing voice", request: dto.AudioRequest{Input: "你好"},
			want: "缺少 voice：请指定 Gemini 预设音色，例如 Kore。可用音色：" + strings.Join(ttsVoices, ", ")},
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
			adaptor := &Adaptor{}
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
		adaptor := &Adaptor{}
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
		adaptor := &Adaptor{AudioResponseFormat: "mp3"}

		_, apiErr := adaptor.DoResponse(context, resp, info)
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
		assert.Equal(t, "模型没有返回音频（finishReason=OTHER），通常是偶发情况，请稍后重试；如果反复出现，请检查文本是否过短或只有符号", apiErr.Error())
	})
}

func TestSpeechHandlerEncodesRequestedFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 1.5 s of a ramp signal; no candidatesTokenCount, so audio tokens come from
	// the PCM length: ceil(1.5 s) = 2 s -> 2/60*1000 = 33 tokens for every format.
	sampleCount := ttsSampleRate * 3 / 2
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
			assert.Equal(t, uint32(ttsSampleRate), stream.Info.SampleRate)
			assert.Equal(t, uint8(ttsChannels), stream.Info.NChannels)
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
			adaptor := &Adaptor{AudioResponseFormat: tt.format}

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

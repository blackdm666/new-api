package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/mewkiz/flac"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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

// transcriptionContext is a multipart /v1/audio/transcriptions request as an
// OpenAI SDK sends it.
func transcriptionContext(t *testing.T, path string, fields map[string]string, filename string, audio []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}
	if filename != "" {
		part, err := writer.CreateFormFile("file", filename)
		require.NoError(t, err)
		_, err = part.Write(audio)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, path, &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
	return ctx, recorder
}

// A Gemini channel answers OpenAI /v1/audio/transcriptions through
// generateContent with the audio inline; language and prompt become hints.
func TestGeminiChannelServesAudioTranscription(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wav, err := wrapPCMAsWAV(make([]byte, ttsSampleRate*3)) // 1.5 s
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeAudioTranscription,
		OriginModelName: "gemini-3-stt",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "http://antigravity:8045",
			ApiKey:            "pool-key",
			UpstreamModelName: "gemini-3-flash",
		},
	}

	t.Run("json", func(t *testing.T) {
		ctx, _ := transcriptionContext(t, "/v1/audio/transcriptions",
			map[string]string{"model": "gemini-3-stt", "language": "zh", "prompt": "88API, Gemini"}, "voice.bin", wav)
		adaptor := &Adaptor{}
		body, err := adaptor.ConvertAudioRequest(ctx, info, dto.AudioRequest{Model: "gemini-3-flash", ResponseFormat: "json"})
		require.NoError(t, err)
		assert.Equal(t, &Transcription{ResponseFormat: "json", Language: "zh"}, adaptor.Transcription)
		payload, err := io.ReadAll(body)
		require.NoError(t, err)
		assert.JSONEq(t, fmt.Sprintf(`{
			"contents":[{"role":"user","parts":[
				{"inlineData":{"mimeType":"audio/wav","data":%q}},
				{"text":"Transcribe the speech in this audio into text, in the language it is spoken. Write it with the normal spacing and punctuation of that language. Do not translate or summarize, and do not add timestamps, speaker labels or notes. If there is no speech, the transcript is empty.\nThe audio language is \"zh\" (ISO-639-1).\nContext for spelling and style only, it is not part of the audio: 88API, Gemini\nReturn JSON: text is the transcript, language is the spoken language as a lowercase English name such as english or chinese."}
			]}],
			"generationConfig":{"responseMimeType":"application/json","responseSchema":{"type":"OBJECT","properties":{"language":{"type":"STRING"},"text":{"type":"STRING"}},"required":["language","text"]}}
		}`, base64.StdEncoding.EncodeToString(wav)), string(payload))

		requestURL, err := adaptor.GetRequestURL(info)
		require.NoError(t, err)
		assert.Equal(t, "http://antigravity:8045/v1beta/models/gemini-3-flash:generateContent", requestURL)
		header := http.Header{}
		require.NoError(t, adaptor.SetupRequestHeader(ctx, &header, info))
		assert.Equal(t, "application/json", header.Get("Content-Type"))
		assert.Equal(t, "pool-key", header.Get("x-goog-api-key"))
	})

	// Regression: as plain text Gemini returned Chinese as "欢 迎 使 用 8 8 A P I …"
	// for json and text, while verbose_json (structured output) stayed clean.
	// Every format must ask for the same structured output.
	for _, format := range []string{"json", "text", "verbose_json"} {
		t.Run(format+" asks for structured output", func(t *testing.T) {
			ctx, _ := transcriptionContext(t, "/v1/audio/transcriptions", map[string]string{"model": "gemini-3-stt"}, "voice.wav", wav)
			adaptor := &Adaptor{}
			body, err := adaptor.ConvertAudioRequest(ctx, info, dto.AudioRequest{ResponseFormat: format})
			require.NoError(t, err)
			payload, err := io.ReadAll(body)
			require.NoError(t, err)
			assert.JSONEq(t, `{"responseMimeType":"application/json","responseSchema":{"type":"OBJECT","properties":{"language":{"type":"STRING"},"text":{"type":"STRING"}},"required":["language","text"]}}`,
				gjson.GetBytes(payload, "generationConfig").Raw)
			assert.NotContains(t, gjson.GetBytes(payload, "contents.0.parts.1.text").String(), "verbatim")
			if format == "verbose_json" {
				assert.Equal(t, &Transcription{ResponseFormat: "verbose_json", Duration: 1.5}, adaptor.Transcription)
			}
		})
	}
}

func TestTranscriptionAudioMIME(t *testing.T) {
	tests := []struct {
		name     string
		head     []byte
		filename string
		want     string
	}{
		{name: "wav", head: []byte("RIFF\x24\x00\x00\x00WAVEfmt "), filename: "a.mp3", want: "audio/wav"},
		{name: "mp3 with id3", head: []byte("ID3\x04\x00"), filename: "a", want: "audio/mp3"},
		{name: "mp3 frame", head: []byte{0xFF, 0xF3, 0x64, 0xC4}, want: "audio/mp3"},
		{name: "adts aac", head: []byte{0xFF, 0xF1, 0x50, 0x80}, want: "audio/aac"},
		{name: "ogg opus", head: []byte("OggS\x00\x02"), filename: "a.opus", want: "audio/ogg"},
		{name: "flac", head: []byte("fLaC\x00"), want: "audio/flac"},
		{name: "m4a", head: []byte("\x00\x00\x00\x20ftypM4A "), want: "audio/mp4"},
		{name: "webm", head: []byte{0x1A, 0x45, 0xDF, 0xA3, 0x9F}, want: "audio/webm"},
		{name: "aiff", head: []byte("FORM\x00\x00\x00\x00AIFF"), want: "audio/aiff"},
		{name: "unknown header, known extension", head: []byte("????"), filename: "Voice.M4A", want: "audio/mp4"},
		{name: "unknown", head: []byte("????"), filename: "notes.txt", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, transcriptionAudioMIME(tt.head, tt.filename))
		})
	}
}

// The transcript is returned in the requested OpenAI format; thought parts are
// not part of it, and Gemini's timing-free output never invents segments.
func TestTranscriptionHandlerFormats(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name          string
		transcription Transcription
		modelText     string
		contentType   string
		want          string
	}{
		{name: "json", transcription: Transcription{ResponseFormat: "json"}, modelText: `{"language":"chinese","text":" 大家好，会议三点半开始。\n"}`,
			contentType: "application/json", want: `{"text":"大家好，会议三点半开始。"}`},
		{name: "text", transcription: Transcription{ResponseFormat: "text"}, modelText: `{"language":"english","text":"Good morning everyone."}`,
			contentType: "text/plain; charset=utf-8", want: "Good morning everyone."},
		{name: "text keeps a plain-text answer", transcription: Transcription{ResponseFormat: "text"}, modelText: "Good morning everyone.\n",
			contentType: "text/plain; charset=utf-8", want: "Good morning everyone."},
		{name: "verbose_json", transcription: Transcription{ResponseFormat: "verbose_json", Duration: 11.46},
			modelText:   `{"language":"Chinese","text":"大家好，会议三点半开始。"}`,
			contentType: "application/json", want: `{"task":"transcribe","language":"chinese","duration":11.46,"text":"大家好，会议三点半开始。"}`},
		{name: "verbose_json keeps the client language when the model returns plain text", transcription: Transcription{ResponseFormat: "verbose_json", Language: "en"},
			modelText:   "Good morning everyone.",
			contentType: "application/json", want: `{"task":"transcribe","language":"en","text":"Good morning everyone."}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", nil)
			parts, err := common.Marshal([]map[string]any{{"text": "Thinking about the audio", "thought": true}, {"text": tt.modelText}})
			require.NoError(t, err)
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
				`{"candidates":[{"content":{"role":"model","parts":%s},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":299,"candidatesTokenCount":29,"totalTokenCount":328}}`, parts)))}
			info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioTranscription, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3-flash"}}
			transcription := tt.transcription
			adaptor := &Adaptor{Transcription: &transcription}

			_, apiErr := adaptor.DoResponse(ctx, resp, info)
			require.Nil(t, apiErr)
			assert.Equal(t, tt.contentType, recorder.Header().Get("Content-Type"))
			if tt.contentType == "application/json" {
				assert.JSONEq(t, tt.want, recorder.Body.String())
			} else {
				assert.Equal(t, tt.want, recorder.Body.String())
			}
		})
	}
}

// Transcriptions settle on Gemini's usageMetadata like chat requests to the
// model: audio input is reported under the audio details, implicit cache hits
// are billed once at the cache price, and without metadata the pre-consume
// audio estimate stands in for the prompt.
func TestTranscriptionUsageSettlesOnGeminiTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const expr = `tier("base", p * 3.5 + c * 21 + cr * 0.35)` // gemini-3-stt
	tests := []struct {
		name          string
		usageMetadata string
		estimate      int
		wantUsage     func(t *testing.T, usage *dto.Usage)
		wantParams    billingexpr.TokenParams
	}{
		{
			name:          "usage metadata with cache and audio details",
			usageMetadata: `{"promptTokenCount":19824,"candidatesTokenCount":40,"totalTokenCount":19864,"cachedContentTokenCount":14380,"promptTokensDetails":[{"modality":"AUDIO","tokenCount":19800},{"modality":"TEXT","tokenCount":24}]}`,
			estimate:      13000,
			wantUsage: func(t *testing.T, usage *dto.Usage) {
				assert.Equal(t, 19824, usage.PromptTokens)
				assert.Equal(t, 40, usage.CompletionTokens)
				assert.Equal(t, 19800, usage.PromptTokensDetails.AudioTokens)
				assert.Equal(t, 24, usage.PromptTokensDetails.TextTokens)
				assert.Equal(t, 14380, usage.PromptTokensDetails.CachedTokens)
			},
			wantParams: billingexpr.TokenParams{P: 19824 - 14380, C: 40, Len: 19824, CR: 14380, AI: 19800},
		},
		{
			name:          "usage metadata without modality details",
			usageMetadata: `{"promptTokenCount":324,"candidatesTokenCount":38,"totalTokenCount":362}`,
			estimate:      217,
			wantUsage: func(t *testing.T, usage *dto.Usage) {
				assert.Equal(t, 324, usage.PromptTokens)
				assert.Equal(t, 38, usage.CompletionTokens)
			},
			wantParams: billingexpr.TokenParams{P: 324, C: 38, Len: 324},
		},
		{
			name:     "no usage metadata",
			estimate: 217,
			wantUsage: func(t *testing.T, usage *dto.Usage) {
				assert.Equal(t, 217, usage.PromptTokens)
				assert.Positive(t, usage.CompletionTokens)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", nil)
			responseJSON := `{"candidates":[{"content":{"role":"model","parts":[{"text":"Good morning everyone."}]},"finishReason":"STOP"}]`
			if tt.usageMetadata != "" {
				responseJSON += `,"usageMetadata":` + tt.usageMetadata
			}
			responseJSON += `}`
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(responseJSON))}
			info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioTranscription, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3-flash"}}
			info.SetEstimatePromptTokens(tt.estimate)
			adaptor := &Adaptor{Transcription: &Transcription{ResponseFormat: "json"}}

			usageValue, apiErr := adaptor.DoResponse(ctx, resp, info)
			require.Nil(t, apiErr)
			usage, ok := usageValue.(*dto.Usage)
			require.True(t, ok)
			tt.wantUsage(t, usage)
			if tt.usageMetadata == "" {
				return
			}

			params := service.BuildTieredTokenParams(usage, false, billingexpr.UsedVars(expr))
			assert.Equal(t, tt.wantParams, params)
			cost, _, err := billingexpr.RunExpr(expr, params)
			require.NoError(t, err)
			assert.InDelta(t, tt.wantParams.P*3.5+tt.wantParams.C*21+tt.wantParams.CR*0.35, cost, 1e-6)
		})
	}
}

// Client mistakes are 400s that name the fix in the client's language.
func TestTranscriptionClientErrorsExplainTheFix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	common.DefaultLanguage = i18n.LangZhCN
	t.Cleanup(func() { common.DefaultLanguage = "" })
	mp3 := []byte("ID3\x04\x00\x00\x00\x00\x00\x00")

	tests := []struct {
		name        string
		translation bool
		format      string
		fields      map[string]string
		filename    string
		audio       []byte
		want        string
	}{
		{name: "srt", format: "srt", filename: "a.mp3", audio: mp3,
			want: "gemini-3-stt 不支持 response_format「srt」，可选 json、text、verbose_json，不传默认 json；srt、vtt 字幕格式暂不支持"},
		{name: "vtt", format: "vtt", filename: "a.mp3", audio: mp3,
			want: "gemini-3-stt 不支持 response_format「vtt」，可选 json、text、verbose_json，不传默认 json；srt、vtt 字幕格式暂不支持"},
		{name: "translation", translation: true, filename: "a.mp3", audio: mp3,
			want: "gemini-3-stt 不支持 /v1/audio/translations（翻译成英文），请改用 POST /v1/audio/transcriptions 转写原文"},
		{name: "stream", fields: map[string]string{"stream": "true"}, filename: "a.mp3", audio: mp3,
			want: "gemini-3-stt 不支持流式转写，请去掉 stream 参数，接口会一次性返回完整结果"},
		{name: "missing file",
			want: "缺少 file：请用 multipart/form-data 上传音频文件，字段名为 file"},
		{name: "too large", filename: "long.wav", audio: make([]byte, transcriptionMaxFileBytes+1),
			want: "音频文件 25.0 MB，超过 25 MB 上限，请压缩（例如转成 16 kHz 单声道 mp3）或分段后再转写"},
		{name: "unknown format", filename: "notes.txt", audio: []byte("hello"),
			want: "无法识别上传音频的格式，支持 mp3、wav、ogg/opus、flac、m4a/mp4、aac、webm、aiff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, relayMode := "/v1/audio/transcriptions", relayconstant.RelayModeAudioTranscription
			if tt.translation {
				path, relayMode = "/v1/audio/translations", relayconstant.RelayModeAudioTranslation
			}
			ctx, _ := transcriptionContext(t, path, tt.fields, tt.filename, tt.audio)
			info := &relaycommon.RelayInfo{RelayMode: relayMode, OriginModelName: "gemini-3-stt"}

			_, err := (&Adaptor{}).ConvertAudioRequest(ctx, info, dto.AudioRequest{Model: "gemini-3-flash", ResponseFormat: tt.format})
			require.EqualError(t, err, tt.want)
			var apiErr *types.NewAPIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.Equal(t, types.ErrorCodeInvalidRequest, apiErr.GetErrorCode())
		})
	}
}

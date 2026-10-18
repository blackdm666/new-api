package xai

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newAudioTestContext(t *testing.T, body []byte, contentType string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", contentType)
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	return c, recorder
}

func newAudioRelayInfo(mode int, request *dto.AudioRequest) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode: mode,
		Request:   request,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "http://sub2api:8080",
		},
	}
}

func TestXAIAudioRequestURL(t *testing.T) {
	adaptor := &Adaptor{}
	for mode, want := range map[int]string{
		constant.RelayModeAudioSpeech:        "http://sub2api:8080/v1/tts",
		constant.RelayModeAudioTranscription: "http://sub2api:8080/v1/stt",
	} {
		url, err := adaptor.GetRequestURL(newAudioRelayInfo(mode, nil))
		require.NoError(t, err)
		assert.Equal(t, want, url)
	}
}

func TestXAIConvertTTSRequest(t *testing.T) {
	speed := 1.2
	body := []byte(`{"model":"grok-tts","input":"你好","voice":"ara","response_format":"wav","speed":1.2,"language":"zh","sample_rate":44100,"text_normalization":true,"instructions":"ignored"}`)
	c, _ := newAudioTestContext(t, body, "application/json")
	adaptor := &Adaptor{}
	request := dto.AudioRequest{Model: "grok-tts", Input: "你好", Voice: "ara", ResponseFormat: "wav", Speed: &speed, Language: []byte(`"zh"`)}

	reader, err := adaptor.ConvertAudioRequest(c, newAudioRelayInfo(constant.RelayModeAudioSpeech, &request), request)
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)

	assert.JSONEq(t, `{"text":"你好","voice_id":"ara","language":"zh","speed":1.2,"text_normalization":true,"output_format":{"codec":"wav","sample_rate":44100}}`, string(data))
	assert.Equal(t, "wav", adaptor.audioCodec)
	assert.Equal(t, 44100, adaptor.audioSampleRate)
}

func TestXAIConvertTTSRequestDefaultsAndRejects(t *testing.T) {
	c, _ := newAudioTestContext(t, []byte(`{"model":"grok-tts","input":"hi"}`), "application/json")
	request := dto.AudioRequest{Model: "grok-tts", Input: "hi"}
	reader, err := (&Adaptor{}).ConvertAudioRequest(c, newAudioRelayInfo(constant.RelayModeAudioSpeech, &request), request)
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"text":"hi","voice_id":"eve","language":"auto","output_format":{"codec":"mp3"}}`, string(data))

	tests := []struct {
		name    string
		body    string
		request dto.AudioRequest
		message string
	}{
		{name: "opus", body: `{}`, request: dto.AudioRequest{Input: "hi", ResponseFormat: "opus"}, message: "response_format"},
		{name: "empty_input", body: `{}`, request: dto.AudioRequest{Input: " "}, message: "input is required"},
		{name: "stream", body: `{}`, request: dto.AudioRequest{Input: "hi", StreamFormat: "sse"}, message: "streaming"},
		{name: "sample_rate", body: `{"sample_rate":999999999}`, request: dto.AudioRequest{Input: "hi"}, message: "sample_rate"},
		{name: "openai_voice", body: `{}`, request: dto.AudioRequest{Input: "hi", Voice: "Alloy"}, message: `voice "Alloy" is an OpenAI voice and is not available for grok tts, use altair, ara, atlas, aurora, carina, castor, celeste, cosmo, eve, helios, helix, iris, kepler, leo, liora, lumen, luna, lux, naksh, orion, perseus, rex, rigel, sal, sirius, ursa, zagan, zenith or a custom voice id`},
		{name: "unknown_voice", body: `{}`, request: dto.AudioRequest{Input: "hi", Voice: "storyteller"}, message: `voice "storyteller" is not available for grok tts, use altair, ara,`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newAudioTestContext(t, []byte(tt.body), "application/json")
			_, err := (&Adaptor{}).ConvertAudioRequest(c, newAudioRelayInfo(constant.RelayModeAudioSpeech, &tt.request), tt.request)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.message)
		})
	}
}

func TestXAITTSResponseBillsInputCharacters(t *testing.T) {
	audio := bytes.Repeat([]byte{0, 1}, 24000) // pcm 16-bit 24kHz = 1s
	// xAI 的 wav 头把 RIFF/data 长度写成占位值 0xFFFFFFFF。
	wav := append([]byte("RIFF\xff\xff\xff\xffWAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\xc0\x5d\x00\x00\x80\xbb\x00\x00\x02\x00\x10\x00data\xff\xff\xff\xff"), audio...)
	tests := []struct {
		name        string
		codec       string
		contentType string
		body        []byte
		want        []byte
	}{
		{name: "binary", codec: "pcm", contentType: "audio/pcm", body: audio, want: audio},
		{name: "json_base64", codec: "pcm", contentType: "application/json", body: []byte(`{"audio":"` + base64.StdEncoding.EncodeToString(audio) + `","content_type":"audio/pcm","duration":1}`), want: audio},
		{name: "wav_placeholder_size", codec: "wav", contentType: "audio/pcm", body: wav, want: wav},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newAudioTestContext(t, nil, "application/json")
			request := &dto.AudioRequest{Model: "grok-tts", Input: "你好 world"}
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {tt.contentType}}, Body: io.NopCloser(bytes.NewReader(tt.body))}

			usage, apiErr := handleXAITTSResponse(c, resp, newAudioRelayInfo(constant.RelayModeAudioSpeech, request), tt.codec, 0)
			require.Nil(t, apiErr)
			assert.Equal(t, tt.want, recorder.Body.Bytes())
			assert.Equal(t, "audio/pcm", recorder.Header().Get("Content-Type"))
			assert.Equal(t, 8, usage.PromptTokens)
			assert.Equal(t, 17, usage.CompletionTokenDetails.AudioTokens)
		})
	}
}

func TestXAIConvertSTTRequestKeepsXAIFieldsAndPutsFileLast(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "grok-stt"))
	require.NoError(t, writer.WriteField("language", "zh"))
	require.NoError(t, writer.WriteField("diarize", "true"))
	require.NoError(t, writer.WriteField("prompt", "openai only"))
	part, err := writer.CreateFormFile("file", "a.mp3")
	require.NoError(t, err)
	_, err = part.Write([]byte("audio-bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("response_format", "verbose_json"))
	require.NoError(t, writer.Close())

	c, _ := newAudioTestContext(t, body.Bytes(), writer.FormDataContentType())
	request := dto.AudioRequest{Model: "grok-stt", ResponseFormat: "verbose_json"}
	adaptor := &Adaptor{}
	reader, err := adaptor.ConvertAudioRequest(c, newAudioRelayInfo(constant.RelayModeAudioTranscription, &request), request)
	require.NoError(t, err)

	_, params, err := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	require.NoError(t, err)
	mr := multipart.NewReader(reader, params["boundary"])
	var names []string
	fields := map[string]string{}
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		value, err := io.ReadAll(p)
		require.NoError(t, err)
		names = append(names, p.FormName())
		fields[p.FormName()] = string(value)
	}
	assert.Equal(t, "file", names[len(names)-1])
	assert.Equal(t, map[string]string{"language": "zh", "diarize": "true", "file": "audio-bytes"}, fields)
	assert.Equal(t, "verbose_json", adaptor.audioResponseFormat)
}

func TestXAISTTResponseFormats(t *testing.T) {
	// diarize=true 时 xAI 在每个词上带 speaker；第二个词带 0 号说话人，第一个没带（未开 diarize 的形状）。
	upstream := `{"text":"你好","language":"zh-cn","duration":61.2,"words":[{"text":"你","start":0.1,"end":0.2},{"text":"好","start":0.3,"end":0.5,"speaker":0}]}`
	tests := []struct {
		format      string
		contentType string
		check       func(t *testing.T, body string)
	}{
		{format: "json", contentType: "application/json", check: func(t *testing.T, body string) {
			assert.JSONEq(t, `{"text":"你好"}`, body)
		}},
		{format: "text", contentType: "text/plain; charset=utf-8", check: func(t *testing.T, body string) {
			assert.Equal(t, "你好", body)
		}},
		{format: "verbose_json", contentType: "application/json", check: func(t *testing.T, body string) {
			assert.Equal(t, "transcribe", gjson.Get(body, "task").String())
			assert.Equal(t, 61.2, gjson.Get(body, "duration").Float())
			assert.Equal(t, "好", gjson.Get(body, "words.1.word").String())
			assert.False(t, gjson.Get(body, "words.0.speaker").Exists(), "speaker is omitted when upstream did not diarize")
			assert.Equal(t, gjson.Number, gjson.Get(body, "words.1.speaker").Type, "speaker 0 must survive omitempty")
			assert.Equal(t, int64(0), gjson.Get(body, "words.1.speaker").Int())
		}},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			c, recorder := newAudioTestContext(t, nil, "multipart/form-data")
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(upstream))}
			usage, apiErr := handleXAISTTResponse(c, resp, newAudioRelayInfo(constant.RelayModeAudioTranscription, &dto.AudioRequest{}), tt.format)
			require.Nil(t, apiErr)
			assert.Equal(t, tt.contentType, recorder.Header().Get("Content-Type"))
			tt.check(t, recorder.Body.String())
			// 62 秒按每分钟 1000 tokens 计
			assert.Equal(t, 1033, usage.PromptTokens)
		})
	}
}

func TestXAIConvertSTTRequestRejectsSubtitleFormats(t *testing.T) {
	c, _ := newAudioTestContext(t, nil, "multipart/form-data")
	request := dto.AudioRequest{Model: "grok-stt", ResponseFormat: "srt"}
	_, err := (&Adaptor{}).ConvertAudioRequest(c, newAudioRelayInfo(constant.RelayModeAudioTranscription, &request), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "response_format")
}

func TestXAIRejectsVoiceModelsOnChatEndpoints(t *testing.T) {
	require.NoError(t, i18n.Init())
	tests := []struct {
		model   string
		wantErr string
	}{
		{model: "grok-tts", wantErr: "POST /v1/audio/speech"},
		{model: "grok-stt", wantErr: "POST /v1/audio/transcriptions"},
		{model: "grok-voice-latest", wantErr: "/v1/realtime"},
		{model: "grok-4.7"},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			c, _ := newAudioTestContext(t, nil, "application/json")
			info := &relaycommon.RelayInfo{
				OriginModelName: tt.model,
				ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: tt.model},
			}
			adaptor := &Adaptor{}
			_, chatErr := adaptor.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: tt.model})
			_, responsesErr := adaptor.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{Model: tt.model})
			_, claudeErr := adaptor.ConvertClaudeRequest(c, info, &dto.ClaudeRequest{Model: tt.model})
			if tt.wantErr == "" {
				require.NoError(t, chatErr)
				require.NoError(t, responsesErr)
				// xAI 渠道本来就不支持 Claude 格式，普通模型保持原来的 not available。
				assert.EqualError(t, claudeErr, "not available")
				return
			}
			for _, err := range []error{chatErr, responsesErr, claudeErr} {
				var apiErr *types.NewAPIError
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				assert.Equal(t, types.ErrorCodeInvalidRequest, apiErr.GetErrorCode())
				assert.Contains(t, err.Error(), tt.model)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestXAIRealtimeModelsRejectedOnAudioEndpoints(t *testing.T) {
	require.NoError(t, i18n.Init())
	for _, mode := range []int{constant.RelayModeAudioSpeech, constant.RelayModeAudioTranscription} {
		c, _ := newAudioTestContext(t, []byte(`{"model":"grok-voice-latest","input":"hi"}`), "application/json")
		request := dto.AudioRequest{Model: "grok-voice-latest", Input: "hi"}
		info := newAudioRelayInfo(mode, &request)
		info.OriginModelName = "grok-voice-latest"
		info.UpstreamModelName = "grok-voice-latest"

		_, err := (&Adaptor{}).ConvertAudioRequest(c, info, request)
		var apiErr *types.NewAPIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
		assert.Contains(t, err.Error(), "/v1/realtime")
	}
}

func TestXAIWebSocketURL(t *testing.T) {
	tests := []struct {
		name    string
		mode    int
		model   string
		path    string
		want    string
		wantErr string
	}{
		{
			name:  "realtime forwards mapped model and session params only",
			mode:  constant.RelayModeRealtime,
			model: "grok-voice-latest",
			path:  "/v1/realtime?model=my-voice&conversation_id=conv_1&reasoning.effort=none&key=sk-secret",
			want:  "http://sub2api:8080/v1/realtime?conversation_id=conv_1&model=grok-voice-latest&reasoning.effort=none",
		},
		{
			name:    "realtime rejects cheaper models",
			mode:    constant.RelayModeRealtime,
			model:   "grok-tts",
			path:    "/v1/realtime?model=grok-tts",
			wantErr: "does not support realtime",
		},
		{
			name:  "streaming tts forwards synthesis params and defaults language",
			mode:  constant.RelayModeAudioSpeechStream,
			model: "grok-tts",
			path:  "/v1/tts?voice=ara&codec=pcm&sample_rate=16000&model=grok-tts&api_key=sk-secret",
			want:  "http://sub2api:8080/v1/tts?codec=pcm&language=auto&sample_rate=16000&voice=ara",
		},
		{
			name:    "streaming tts rejects OpenAI voices",
			mode:    constant.RelayModeAudioSpeechStream,
			model:   "grok-tts",
			path:    "/v1/tts?voice=alloy",
			wantErr: "OpenAI voice",
		},
		{
			name:    "streaming tts rejects invalid synthesis params",
			mode:    constant.RelayModeAudioSpeechStream,
			model:   "grok-tts",
			path:    "/v1/tts?voice=eve&speed=2.0",
			wantErr: "speed must be between 0.7 and 1.5",
		},
		{
			name:    "streaming tts rejects other models",
			mode:    constant.RelayModeAudioSpeechStream,
			model:   "grok-voice-latest",
			path:    "/v1/tts",
			wantErr: "does not support streaming tts",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := newAudioRelayInfo(tt.mode, nil)
			info.RequestURLPath = tt.path
			info.OriginModelName = tt.model
			info.UpstreamModelName = tt.model
			got, err := (&Adaptor{}).GetRequestURL(info)
			if tt.wantErr != "" {
				var apiErr *types.NewAPIError
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// 取值以 xAI 文档为准，并与线上上游实测一致：codec 区分大小写、sample_rate 只认整数、speed 0.7–1.5。
func TestValidateTTSStreamQuery(t *testing.T) {
	for _, query := range []string{
		"",
		"voice=Rex&codec=ulaw&sample_rate=48000&bit_rate=192000&speed=1.5&with_timestamps=true",
		"voice=xai_rex&speed=0.7&text_normalization=false&optimize_streaming_latency=3",
		"voice=abc123xy&codec=alaw&sample_rate=8000",
	} {
		values, err := url.ParseQuery(query)
		require.NoError(t, err)
		assert.NoError(t, ValidateTTSStreamQuery(values), query)
	}

	tests := []struct {
		query   string
		message string
	}{
		{"speed=2.0", "speed must be between 0.7 and 1.5"},
		{"speed=0.69", "speed must be between 0.7 and 1.5"},
		{"speed=NaN", "speed must be between 0.7 and 1.5"},
		{"sample_rate=12345", "sample_rate must be one of 8000, 16000, 22050, 24000, 44100, 48000"},
		{"sample_rate=24000.0", "sample_rate must be one of"},
		{"codec=aac", `codec "aac" is not supported by grok tts, use mp3, wav, pcm, mulaw, ulaw, alaw`},
		{"codec=flac", `codec "flac" is not supported`},
		{"codec=MP3", `codec "MP3" is not supported`},
		{"bit_rate=12345", "bit_rate must be one of 32000, 64000, 96000, 128000, 192000"},
		{"with_timestamps=1", "with_timestamps must be true or false"},
		{"voice=alloy", `voice "alloy" is an OpenAI voice and is not available for grok tts`},
		{"voice=nonexistent", `voice "nonexistent" is not available for grok tts, use altair,`},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			values, err := url.ParseQuery(tt.query)
			require.NoError(t, err)
			err = ValidateTTSStreamQuery(values)
			var apiErr *types.NewAPIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.True(t, types.IsSkipRetryError(apiErr))
			assert.Contains(t, err.Error(), tt.message)
		})
	}
}

// 上游握手失败时，客户收到的报错不能带上游地址或查询串；请求本身被拒（4xx）时不换渠道重试。
func TestXAIWebSocketHandshakeErrorHidesUpstreamAddress(t *testing.T) {
	tests := []struct {
		status    int
		skipRetry bool
	}{
		{http.StatusBadRequest, true},
		{http.StatusNotFound, true},
		{http.StatusUnauthorized, false},
		{http.StatusTooManyRequests, false},
		{http.StatusBadGateway, false},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"error":{"message":"internal detail"}}`))
			}))
			t.Cleanup(upstream.Close)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/tts?voice=eve", nil)
			info := newAudioRelayInfo(constant.RelayModeAudioSpeechStream, nil)
			info.ChannelBaseUrl = upstream.URL
			info.RequestURLPath = "/v1/tts?voice=eve&codec=mp3"
			info.UpstreamModelName = xaiTTSModel

			_, err := (&Adaptor{}).DoRequest(c, info, nil)
			var apiErr *types.NewAPIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, fmt.Sprintf("upstream websocket handshake failed with status %d", tt.status), err.Error())
			assert.Equal(t, tt.status, apiErr.StatusCode)
			assert.Equal(t, tt.skipRetry, types.IsSkipRetryError(apiErr))
		})
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/tts", nil)
	info := newAudioRelayInfo(constant.RelayModeAudioSpeechStream, nil)
	info.ChannelBaseUrl = "http://127.0.0.1:1"
	info.RequestURLPath = "/v1/tts?voice=eve"
	info.UpstreamModelName = xaiTTSModel
	_, err := (&Adaptor{}).DoRequest(c, info, nil)
	require.Error(t, err)
	assert.Equal(t, "failed to connect to upstream websocket", err.Error())
}

// fakeWssBilling records reservation targets and refuses targets above limit.
type fakeWssBilling struct {
	mu      sync.Mutex
	limit   int
	targets []int
}

func (b *fakeWssBilling) Settle(int) error         { return nil }
func (b *fakeWssBilling) Refund(*gin.Context)      {}
func (b *fakeWssBilling) NeedsRefund() bool        { return false }
func (b *fakeWssBilling) GetPreConsumedQuota() int { return 0 }
func (b *fakeWssBilling) Reserve(target int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.targets = append(b.targets, target)
	if target > b.limit {
		return errors.New("insufficient quota")
	}
	return nil
}

type wsFrame struct {
	messageType int
	data        []byte
}

// runXAIWebSocketSession wires client <-> xaiWebSocketHandler <-> fake upstream and
// returns the client connection, the frames upstream received and the session usage.
func runXAIWebSocketSession(t *testing.T, mode int, model, expr string, billing *fakeWssBilling, upstream func(*websocket.Conn)) (*websocket.Conn, <-chan []wsFrame, <-chan *dto.RealtimeUsage) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	upgrader := websocket.Upgrader{}
	received := make(chan []wsFrame, 1)
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		upstream(conn)
		var frames []wsFrame
		for {
			messageType, data, err := conn.ReadMessage()
			if err != nil {
				break
			}
			frames = append(frames, wsFrame{messageType, data})
		}
		received <- frames
	}))
	t.Cleanup(upstreamServer.Close)

	result := make(chan *dto.RealtimeUsage, 1)
	gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientWs, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer clientWs.Close()
		targetWs, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(upstreamServer.URL, "http"), nil)
		if err != nil {
			result <- nil
			return
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r
		info := &relaycommon.RelayInfo{
			RelayMode:     mode,
			ClientWs:      clientWs,
			TargetWs:      targetWs,
			Billing:       billing,
			BillingSource: "subscription",
			ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: model},
			TieredBillingSnapshot: &billingexpr.BillingSnapshot{
				BillingMode:  "tiered_expr",
				ExprString:   expr,
				ExprHash:     billingexpr.ExprHashString(expr),
				GroupRatio:   0.3,
				QuotaPerUnit: 500000,
			},
		}
		usage, apiErr := xaiWebSocketHandler(c, info)
		targetWs.Close()
		if apiErr != nil {
			usage = nil
		}
		result <- usage
	}))
	t.Cleanup(gatewayServer.Close)

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(gatewayServer.URL, "http"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	return client, received, result
}

func closeClient(t *testing.T, client *websocket.Conn) {
	t.Helper()
	require.NoError(t, client.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")))
}

func TestXAIRealtimeSessionRelaysFramesAndBillsDuration(t *testing.T) {
	billing := &fakeWssBilling{limit: math.MaxInt32}
	client, received, result := runXAIWebSocketSession(t, constant.RelayModeRealtime, "grok-voice-latest", `tier("base", p * 560)`, billing, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.done","response":{"usage":{}}}`))
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3})
	})

	userText := []byte(`{"type":"conversation.item.create","item":{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}}`)
	seedText := []byte(`{"type":"conversation.item.create","item":{"type":"message","role":"assistant","content":[{"type":"text","text":"earlier"}]}}`)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, userText))
	require.NoError(t, client.WriteMessage(websocket.TextMessage, seedText))
	require.NoError(t, client.WriteMessage(websocket.BinaryMessage, []byte{9, 9}))

	messageType, data, err := client.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, messageType)
	assert.Equal(t, "response.done", gjson.GetBytes(data, "type").String())
	messageType, data, err = client.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.BinaryMessage, messageType)
	assert.Equal(t, []byte{1, 2, 3}, data)
	closeClient(t, client)

	usage := <-result
	require.NotNil(t, usage)
	// 一条用户文字输入 50 tokens；不到 1 秒的会话按 1 秒计（17 tokens）。
	assert.Equal(t, 50, usage.InputTokenDetails.TextTokens)
	assert.Equal(t, 17, usage.InputTokenDetails.AudioTokens)
	assert.Equal(t, 67, usage.InputTokens)

	frames := <-received
	require.Len(t, frames, 3)
	assert.Equal(t, userText, frames[0].data)
	assert.Equal(t, seedText, frames[1].data)
	assert.Equal(t, wsFrame{websocket.BinaryMessage, []byte{9, 9}}, frames[2])

	// 文字输入转发前按「已用 + 1 分钟 + 50 tokens」续留，每 token $560/1M × 500000 × 组倍率 0.3 = 0.084。
	// 已用时长不足 1 秒按 1 秒计（1017 tokens）；时钟精度下已用为 0 时恰好 1 分钟（1000 tokens）。
	billing.mu.Lock()
	defer billing.mu.Unlock()
	require.NotEmpty(t, billing.targets)
	assert.Contains(t, []int{(1000 + 50) * 84, (1017 + 50) * 84}, billing.targets[0])
}

func TestXAIRealtimeSessionStopsBeforeUnaffordableTextInput(t *testing.T) {
	billing := &fakeWssBilling{limit: 0}
	client, received, result := runXAIWebSocketSession(t, constant.RelayModeRealtime, "grok-voice-latest", `tier("base", p * 560)`, billing, func(*websocket.Conn) {})

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"conversation.item.create","item":{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}}`)))

	_, data, err := client.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "error", gjson.GetBytes(data, "type").String())
	assert.Equal(t, string(types.ErrorCodeInsufficientUserQuota), gjson.GetBytes(data, "error.code").String())
	_, _, err = client.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	assert.Equal(t, websocket.ClosePolicyViolation, closeErr.Code)

	usage := <-result
	require.NotNil(t, usage)
	assert.Zero(t, usage.InputTokenDetails.TextTokens, "a rejected text input is not billed")
	assert.Empty(t, <-received, "a rejected text input never reaches upstream")
}

func TestXAITTSStreamBillsTextDeltaCharacters(t *testing.T) {
	billing := &fakeWssBilling{limit: math.MaxInt32}
	client, received, result := runXAIWebSocketSession(t, constant.RelayModeAudioSpeechStream, "grok-tts", `tier("base", p * 105)`, billing, func(conn *websocket.Conn) {
		for range 2 {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"audio.delta","delta":"AAAA"}`))
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"audio.done"}`))
	})

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"text.delta","delta":"你好，world"}`)))
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"text.done"}`)))
	for _, want := range []string{"audio.delta", "audio.done"} {
		_, data, err := client.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, want, gjson.GetBytes(data, "type").String())
	}
	closeClient(t, client)

	usage := <-result
	require.NotNil(t, usage)
	assert.Equal(t, 8, usage.InputTokens)
	assert.Equal(t, 8, usage.InputTokenDetails.TextTokens)
	<-received

	// 8 字符 × $105/1M × 500000 × 0.3 = 126，在转发 text.delta 之前预留。
	billing.mu.Lock()
	defer billing.mu.Unlock()
	assert.Equal(t, []int{126}, billing.targets)
}

// 上游按段合成：产出音频的段落计费；音色不存在等原因失败、没有音频的段落不计费，但发送前仍按全部字符预留。
func TestXAITTSStreamBillsOnlySynthesizedUtterances(t *testing.T) {
	billing := &fakeWssBilling{limit: math.MaxInt32}
	client, received, result := runXAIWebSocketSession(t, constant.RelayModeAudioSpeechStream, "grok-tts", `tier("base", p * 105)`, billing, func(conn *websocket.Conn) {
		// 第一段先出音频、客户端再补一句，补的字符也要计费。
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"audio.delta","delta":"AAAA"}`))
		for range 2 {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"audio.done"}`))
		for range 2 {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"TTS synthesis failed: Voice 'abc12345' not found"}`))
	})

	send := func(event string) {
		require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(event)))
	}
	send(`{"type":"text.delta","delta":"Hello"}`)
	_, data, err := client.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "audio.delta", gjson.GetBytes(data, "type").String())
	send(`{"type":"text.delta","delta":" world"}`)
	send(`{"type":"text.done"}`)
	_, data, err = client.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "audio.done", gjson.GetBytes(data, "type").String())
	send(`{"type":"text.delta","delta":"失败的一段"}`)
	send(`{"type":"text.done"}`)
	_, data, err = client.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "error", gjson.GetBytes(data, "type").String())
	closeClient(t, client)

	usage := <-result
	require.NotNil(t, usage)
	assert.Equal(t, 11, usage.InputTokens, "only the synthesized utterance (Hello world) is billed")
	assert.Equal(t, 11, usage.InputTokenDetails.TextTokens)
	<-received

	// 每字符 $105/1M × 500000 × 0.3 = 15.75：按累计 5、11、16 字符预留（四舍五入）。
	billing.mu.Lock()
	defer billing.mu.Unlock()
	assert.Equal(t, []int{79, 173, 252}, billing.targets)
}

// 实时语音的 session.update 带 OpenAI 或不存在的音色时回 error、不转发；合法音色照常转发。
func TestXAIRealtimeSessionRejectsUnknownVoices(t *testing.T) {
	billing := &fakeWssBilling{limit: math.MaxInt32}
	client, received, result := runXAIWebSocketSession(t, constant.RelayModeRealtime, "grok-voice-latest", `tier("base", p * 560)`, billing, func(*websocket.Conn) {})

	for _, voice := range []string{"alloy", "nonexistent"} {
		require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"session.update","session":{"voice":"`+voice+`","instructions":"hi"}}`)))
		_, data, err := client.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, "error", gjson.GetBytes(data, "type").String())
		assert.Equal(t, "invalid_request_error", gjson.GetBytes(data, "error.type").String())
		assert.Equal(t, "session.voice", gjson.GetBytes(data, "error.param").String())
		assert.Contains(t, gjson.GetBytes(data, "error.message").String(), `voice "`+voice+`" is`)
		assert.Contains(t, gjson.GetBytes(data, "error.message").String(), "not available for grok voice, use altair, ara,")
	}
	valid := []byte(`{"type":"session.update","session":{"voice":"rex"}}`)
	noVoice := []byte(`{"type":"session.update","session":{"instructions":"be brief"}}`)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, valid))
	require.NoError(t, client.WriteMessage(websocket.TextMessage, noVoice))
	closeClient(t, client)

	require.NotNil(t, <-result)
	frames := <-received
	require.Len(t, frames, 2, "rejected session.update events never reach upstream")
	assert.Equal(t, valid, frames[0].data)
	assert.Equal(t, noVoice, frames[1].data)
}

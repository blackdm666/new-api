package xai

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
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
	upstream := `{"text":"你好","language":"zh-cn","duration":61.2,"words":[{"text":"你","start":0.1,"end":0.2},{"text":"好","start":0.3,"end":0.5}]}`
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

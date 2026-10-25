package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Frame shapes below follow streamGenerateContent bodies captured from a
// Vertex Gemini channel on 2026-10-25 (signatures shortened).
const (
	forcedStreamFinalUsage = `"usageMetadata":{"promptTokenCount":46,"candidatesTokenCount":92,"totalTokenCount":138,"trafficType":"ON_DEMAND"},"modelVersion":"gemini-4-argon","responseId":"resp-1"`
	forcedStreamEmptyUsage = `"usageMetadata":{"trafficType":"ON_DEMAND"},"modelVersion":"gemini-4-argon","responseId":"resp-1"`
)

func forcedStreamFrame(parts string, finishReason string, usage string) string {
	candidate := `{"content":{"role":"model","parts":[` + parts + `]}`
	if finishReason != "" {
		candidate += `,"finishReason":"` + finishReason + `"`
	}
	return "data: {\"candidates\":[" + candidate + "}]," + usage + "}\n\n"
}

func forcedStreamResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func forcedStreamContext(path string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	return c
}

func TestCollectForcedUpstreamStreamRebuildsGenerateContentParts(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		parts string
	}{
		{
			name: "trailing signature joins the streamed text",
			body: forcedStreamFrame(`{"text":"1. 太阳"}`, "", forcedStreamEmptyUsage) +
				forcedStreamFrame(`{"text":"光看起来是白色的。"}`, "", forcedStreamEmptyUsage) +
				forcedStreamFrame(`{"text":"","thoughtSignature":"AY89a19VpxaM"}`, "STOP", forcedStreamFinalUsage),
			parts: `[{"text":"1. 太阳光看起来是白色的。","thoughtSignature":"AY89a19VpxaM"}]`,
		},
		{
			name: "function calls keep their own parts and the empty text is dropped",
			body: forcedStreamFrame(`{"functionCall":{"name":"get_weather","args":{"city":"上海"},"id":"call_1"},"thoughtSignature":"AY89a1+/NZLP"}`, "", forcedStreamEmptyUsage) +
				forcedStreamFrame(`{"functionCall":{"name":"get_weather","args":{"city":"北京"},"id":"call_2"}}`, "", forcedStreamEmptyUsage) +
				forcedStreamFrame(`{"text":""}`, "STOP", forcedStreamFinalUsage),
			parts: `[{"functionCall":{"name":"get_weather","args":{"city":"上海"},"id":"call_1"},"thoughtSignature":"AY89a1+/NZLP"},` +
				`{"functionCall":{"name":"get_weather","args":{"city":"北京"},"id":"call_2"}}]`,
		},
		{
			name: "thought summaries and the answer stay separate parts",
			body: forcedStreamFrame(`{"text":"先想","thought":true}`, "", forcedStreamEmptyUsage) +
				forcedStreamFrame(`{"text":"一想","thought":true}`, "", forcedStreamEmptyUsage) +
				forcedStreamFrame(`{"text":"答"}`, "", forcedStreamEmptyUsage) +
				forcedStreamFrame(`{"text":"案"},{"text":"","thoughtSignature":"SIG"}`, "STOP", forcedStreamFinalUsage),
			parts: `[{"text":"先想一想","thought":true},{"text":"答案","thoughtSignature":"SIG"}]`,
		},
		{
			name: "text after a signed part starts a new part",
			body: forcedStreamFrame(`{"text":"a","thoughtSignature":"SIG1"}`, "", forcedStreamEmptyUsage) +
				forcedStreamFrame(`{"text":"b"}`, "STOP", forcedStreamFinalUsage),
			parts: `[{"text":"a","thoughtSignature":"SIG1"},{"text":"b"}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := forcedStreamContext("/v1beta/models/gemini-4-argon:generateContent")

			resp, err := collectForcedUpstreamStream(c, forcedStreamResponse(tt.body))
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.JSONEq(t, tt.parts, gjson.GetBytes(body, "candidates.0.content.parts").Raw)
			assert.Equal(t, "model", gjson.GetBytes(body, "candidates.0.content.role").Str)
			assert.Equal(t, "STOP", gjson.GetBytes(body, "candidates.0.finishReason").Str)
			assert.Equal(t, int64(1), gjson.GetBytes(body, "candidates.#").Int())
			assert.Equal(t, "resp-1", gjson.GetBytes(body, "responseId").Str)
			assert.Equal(t, int64(92), gjson.GetBytes(body, "usageMetadata.candidatesTokenCount").Int())
			assert.Equal(t, "ON_DEMAND", gjson.GetBytes(body, "usageMetadata.trafficType").Str)
			assert.False(t, gjson.GetBytes(body, "usageMetadata.toolUsePromptTokensDetails").Exists())
		})
	}
}

func TestCollectForcedUpstreamStreamFailsWithoutPartialText(t *testing.T) {
	text := forcedStreamFrame(`{"text":"partial"}`, "", forcedStreamEmptyUsage)
	tests := []struct {
		name       string
		body       string
		statusCode int
		message    string
	}{
		{
			name:       "error frame keeps the upstream status and message",
			body:       text + "data: {\"error\":{\"code\":429,\"message\":\"Resource exhausted\",\"status\":\"RESOURCE_EXHAUSTED\"}}\n\n",
			statusCode: http.StatusTooManyRequests,
			message:    "Resource exhausted",
		},
		{
			name:       "stream cut before a finish reason",
			body:       text,
			statusCode: http.StatusBadGateway,
			message:    "upstream stream ended before completion",
		},
		{
			name:       "empty stream",
			body:       ": keep-alive\n\n",
			statusCode: http.StatusBadGateway,
			message:    "upstream stream ended before completion",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := forcedStreamContext("/v1/chat/completions")

			resp, err := collectForcedUpstreamStream(c, forcedStreamResponse(tt.body))
			var newAPIErr *types.NewAPIError
			if err == nil {
				// An upstream error frame comes back as an ordinary upstream
				// error response, so retry and status code mapping apply.
				require.NotNil(t, resp)
				newAPIErr = service.RelayErrorHandler(c.Request.Context(), resp, false)
			} else {
				newAPIErr = types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
			}
			require.NotNil(t, newAPIErr)
			assert.Equal(t, tt.statusCode, newAPIErr.StatusCode)
			assert.Contains(t, newAPIErr.Error(), tt.message)
		})
	}
}

func TestCollectForcedUpstreamStreamIdleTimeout(t *testing.T) {
	oldTimeout := common.RelayResponseHeaderTimeout
	common.RelayResponseHeaderTimeout = 1
	t.Cleanup(func() { common.RelayResponseHeaderTimeout = oldTimeout })

	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	resp := forcedStreamResponse("")
	resp.Body = reader

	_, err := collectForcedUpstreamStream(forcedStreamContext("/v1/chat/completions"), resp)
	require.Error(t, err)
	newAPIErr := types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	assert.Equal(t, http.StatusGatewayTimeout, newAPIErr.StatusCode)
}

func TestCollectForcedUpstreamStreamKeepsNonStreamResponses(t *testing.T) {
	tests := []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`))},
		{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(`{}`))},
	}
	for _, upstream := range tests {
		resp, err := collectForcedUpstreamStream(forcedStreamContext("/v1/chat/completions"), upstream)
		require.NoError(t, err)
		assert.Same(t, upstream, resp)
	}
}

func TestCollectForcedUpstreamStreamCompletesBlockedPrompt(t *testing.T) {
	body := "data: {\"promptFeedback\":{\"blockReason\":\"PROHIBITED_CONTENT\"},\"usageMetadata\":{\"promptTokenCount\":12,\"totalTokenCount\":12}}\n\n"

	resp, err := collectForcedUpstreamStream(forcedStreamContext("/v1/chat/completions"), forcedStreamResponse(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	rebuilt, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "PROHIBITED_CONTENT", gjson.GetBytes(rebuilt, "promptFeedback.blockReason").Str)
	assert.Equal(t, int64(0), gjson.GetBytes(rebuilt, "candidates.#").Int())
}

// The rebuilt response must bill exactly what the same stream bills when the
// client asks for streaming, for native and converted output alike.
func TestForcedUpstreamStreamBillsLikeStreaming(t *testing.T) {
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() { constant.StreamingTimeout = oldStreamingTimeout })

	stream := "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"hello \"}]}}],\"usageMetadata\":{\"promptTokenCount\":151,\"toolUsePromptTokenCount\":18329}}\n\n" +
		forcedStreamFrame(`{"text":"world"},{"text":"","thoughtSignature":"SIG"}`, "STOP",
			`"usageMetadata":{"promptTokenCount":151,"toolUsePromptTokenCount":18329,"candidatesTokenCount":1089,"thoughtsTokenCount":1120,"totalTokenCount":20689}`)
	newInfo := func(format types.RelayFormat) *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			RelayFormat:     format,
			OriginModelName: "gemini-3.8-flash",
			ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.8-flash"},
		}
	}

	streamed, streamErr := geminiStreamHandler(forcedStreamContext("/v1/chat/completions"), newInfo(types.RelayFormatOpenAI),
		forcedStreamResponse(stream), func(_ string, _ *dto.GeminiChatResponse) bool { return true })
	require.Nil(t, streamErr)

	tests := []struct {
		name    string
		path    string
		format  types.RelayFormat
		handler func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{name: "native", path: "/v1beta/models/gemini-3.8-flash:generateContent", format: types.RelayFormatGemini, handler: GeminiTextGenerationHandler},
		{name: "openai", path: "/v1/chat/completions", format: types.RelayFormatOpenAI, handler: GeminiChatHandler},
		{name: "claude", path: "/v1/messages", format: types.RelayFormatClaude, handler: GeminiChatHandler},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := forcedStreamContext(tt.path)
			info := newInfo(tt.format)
			if tt.format == types.RelayFormatClaude {
				info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{}
			}

			resp, err := collectForcedUpstreamStream(c, forcedStreamResponse(stream))
			require.NoError(t, err)
			usage, newAPIErr := tt.handler(c, info, resp)
			require.Nil(t, newAPIErr)

			assert.Equal(t, streamed.PromptTokens, usage.PromptTokens)
			assert.Equal(t, streamed.CompletionTokens, usage.CompletionTokens)
			assert.Equal(t, streamed.TotalTokens, usage.TotalTokens)
			assert.Equal(t, streamed.CompletionTokenDetails.ReasoningTokens, usage.CompletionTokenDetails.ReasoningTokens)
			assert.Equal(t, http.StatusOK, c.Writer.Status())
		})
	}
}

func TestShouldForceUpstreamStreamScope(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		stream      bool
		channelType int
		format      types.RelayFormat
		mode        int
		path        string
		model       string
		forced      bool
	}{
		{name: "New API channel embedding this adaptor", enabled: true, channelType: constant.ChannelTypeNewAPI, format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, path: "/v1/chat/completions", model: "gemini-4-argon"},
		{name: "openai chat", enabled: true, format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, path: "/v1/chat/completions", model: "gemini-4-argon", forced: true},
		{name: "claude messages", enabled: true, format: types.RelayFormatClaude, path: "/v1/messages", model: "gemini-4-argon", forced: true},
		{name: "native generateContent", enabled: true, format: types.RelayFormatGemini, mode: relayconstant.RelayModeGemini, path: "/v1beta/models/gemini-4-argon:generateContent", model: "gemini-4-argon", forced: true},
		{name: "setting off", format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, path: "/v1/chat/completions", model: "gemini-4-argon"},
		{name: "client already streams", enabled: true, stream: true, format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, path: "/v1/chat/completions", model: "gemini-4-argon"},
		{name: "responses keep their path", enabled: true, format: types.RelayFormatOpenAIResponses, mode: relayconstant.RelayModeResponses, path: "/v1/responses", model: "gemini-4-argon"},
		{name: "native embedding", enabled: true, format: types.RelayFormatGemini, mode: relayconstant.RelayModeGemini, path: "/v1beta/models/gemini-embedding-001:embedContent", model: "gemini-embedding-001"},
		{name: "image model", enabled: true, format: types.RelayFormatGemini, mode: relayconstant.RelayModeGemini, path: "/v1beta/models/gemini-3-pro-image:generateContent", model: "gemini-3-pro-image"},
		{name: "speech model", enabled: true, format: types.RelayFormatGemini, mode: relayconstant.RelayModeGemini, path: "/v1beta/models/gemini-3.1-flash-tts:generateContent", model: "gemini-3.1-flash-tts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channelType := tt.channelType
			if channelType == 0 {
				channelType = constant.ChannelTypeGemini
			}
			info := &relaycommon.RelayInfo{
				IsStream:       tt.stream,
				RelayFormat:    tt.format,
				RelayMode:      tt.mode,
				RequestURLPath: tt.path,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:       channelType,
					ChannelBaseUrl:    "https://upstream.example",
					UpstreamModelName: tt.model,
					ChannelSetting:    dto.ChannelSettings{ForceUpstreamStream: tt.enabled},
				},
			}
			adaptor := &Adaptor{}
			adaptor.Init(info)

			assert.Equal(t, tt.forced, adaptor.forceUpstreamStream)
			assert.Equal(t, tt.stream, info.IsStream)
			if tt.forced {
				url, err := adaptor.GetRequestURL(info)
				require.NoError(t, err)
				assert.Equal(t, "https://upstream.example/v1beta/models/"+tt.model+":streamGenerateContent?alt=sse", url)
				assert.False(t, info.DisablePing)
			}
		})
	}
}

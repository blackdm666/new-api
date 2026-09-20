package perfmetrics_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixture struct {
	name, body string
	format     types.RelayFormat
	handler    func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
	stream     bool
	terminal   string
}

type observation struct {
	outcome perfmetrics.Outcome
	close   bool
	ctxErr  error
	stream  relaycommon.StreamOutcome
	err     error
}

func TestCompletedRelaySurvivesProxyClose(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.ReleaseMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	previousTimeout := constant.StreamingTimeout
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	constant.StreamingTimeout = 10
	fixtures := []fixture{
		{"images", `{"created":1789917600,"data":[{"url":"https://example.invalid/image.png"}]}`, types.RelayFormatOpenAI, openai.OpenaiImageHandler, false, ""},
		{"chat", `{"id":"mock","model":"mock","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`, types.RelayFormatOpenAI, openai.OpenaiHandler, false, ""},
		{"claude", `{"id":"mock","type":"message","model":"mock","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}`, types.RelayFormatClaude,
			func(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
				return claude.ClaudeHandler(c, resp, info)
			}, false, ""},
		{"responses-json", `{"id":"mock","object":"response","model":"mock","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`, types.RelayFormatOpenAIResponses, openai.OaiResponsesHandler, false, ""},
		{"responses-sse", "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"mock\",\"model\":\"mock\",\"status\":\"completed\",\"usage\":{\"input_tokens\":2,\"output_tokens\":1,\"total_tokens\":3}}}\n\n", types.RelayFormatOpenAIResponses, openai.OaiResponsesStreamHandler, true, "response.completed"},
		{"chat-sse", "data: {\"id\":\"mock\",\"model\":\"mock\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\ndata: [DONE]\n\n", types.RelayFormatOpenAI, openai.OaiStreamHandler, true, "[DONE]"},
		{"claude-sse", "data: {\"type\":\"message_stop\"}\n\n", types.RelayFormatClaude,
			func(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
				return claude.ClaudeStreamHandler(c, resp, info)
			}, true, "message_stop"},
	}
	for _, f := range fixtures {
		for _, closeConnection := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/close=%t", f.name, closeConnection), func(t *testing.T) { runDeliveryFixture(t, f, closeConnection) })
		}
	}
}

// Faults are injected only at the network-writer boundary, below Gin. Exercise
// the real copy/stream helpers, including the errors their void APIs conceal.
type faultWriter struct {
	*httptest.ResponseRecorder
	writeErr error
	flushErr error
	short    bool
}

func (w *faultWriter) Write(p []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if w.short {
		return len(p) - 1, nil
	}
	return w.ResponseRecorder.Write(p)
}

func (w *faultWriter) FlushError() error {
	if w.flushErr != nil {
		return w.flushErr
	}
	w.ResponseRecorder.Flush()
	return nil
}

func TestDeliveryEvidenceGuards(t *testing.T) {
	broken := errors.New("downstream connection broken")
	for _, tc := range []struct {
		name     string
		writeErr error
		flushErr error
		short    bool
		mode     string
		cancel   bool
		want     perfmetrics.Outcome
	}{
		{name: "fully copied JSON then cancellation", mode: "json", cancel: true, want: perfmetrics.OutcomeSuccess},
		{name: "JSON write fails", mode: "json", writeErr: broken, want: perfmetrics.OutcomeIgnored},
		{name: "JSON short write", mode: "json", short: true, want: perfmetrics.OutcomeIgnored},
		{name: "JSON flush fails inside Gin", mode: "json", flushErr: broken, want: perfmetrics.OutcomeIgnored},
		{name: "HTTP 200 with no body proves nothing", mode: "headers", cancel: true, want: perfmetrics.OutcomeIgnored},
		{name: "partial body flushed", mode: "partial", cancel: true, want: perfmetrics.OutcomeIgnored},
		{name: "complete body not flushed", mode: "buffered", cancel: true, want: perfmetrics.OutcomeIgnored},
		{name: "completed SSE then cancellation", mode: "completed", cancel: true, want: perfmetrics.OutcomeSuccess},
		{name: "terminal parsed but skipped after cancellation", mode: "skipped", want: perfmetrics.OutcomeIgnored},
		{name: "terminal write fails", mode: "completed", writeErr: broken, want: perfmetrics.OutcomeIgnored},
		{name: "terminal flush fails", mode: "completed", flushErr: broken, want: perfmetrics.OutcomeIgnored},
		{name: "SSE midstream cancellation", mode: "midstream", cancel: true, want: perfmetrics.OutcomeIgnored},
		{name: "upstream failure then normal close", mode: "failure", cancel: true, want: perfmetrics.OutcomeFailure},
		{name: "upstream content refusal then normal close", mode: "refusal", cancel: true, want: perfmetrics.OutcomeIgnored},
		{name: "output limit then normal close", mode: "limit", cancel: true, want: perfmetrics.OutcomeSuccess},
		{name: "business rejection after full JSON", mode: "business", cancel: true, want: perfmetrics.OutcomeIgnored},
		{name: "deadline without terminal", mode: "timeout", want: perfmetrics.OutcomeFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writer := &faultWriter{ResponseRecorder: httptest.NewRecorder(), writeErr: tc.writeErr, flushErr: tc.flushErr, short: tc.short}
			engine := gin.New()
			var outcome perfmetrics.Outcome
			engine.GET("/", func(c *gin.Context) {
				info := &relaycommon.RelayInfo{}
				switch tc.mode {
				case "json", "business":
					service.IOCopyBytesGracefully(c, nil, []byte(`{"ok":true}`))
					info.PerformanceBusinessRejection = tc.mode == "business"
				case "headers", "partial", "buffered":
					c.Header("Content-Length", "10")
					c.Status(http.StatusOK)
					if tc.mode == "partial" {
						_, err := c.Writer.Write([]byte("part"))
						require.NoError(t, err)
						c.Writer.Flush()
					} else if tc.mode == "buffered" {
						_, err := c.Writer.Write([]byte("0123456789"))
						require.NoError(t, err)
					}
				default:
					info.IsStream = true
					info.StreamStatus = relaycommon.NewStreamStatus()
					info.StreamStatus.RequireTerminal()
					switch tc.mode {
					case "completed", "skipped":
						info.StreamStatus.MarkCompleted()
					case "failure":
						info.StreamStatus.MarkFailed("server_error", "", 502)
					case "refusal":
						info.StreamStatus.MarkIncomplete("content_filter")
					case "limit":
						info.StreamStatus.MarkIncomplete("max_output_tokens")
					case "timeout":
						info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, context.DeadlineExceeded)
					}
					if tc.mode == "skipped" {
						require.NoError(t, helper.StringData(c, `{"type":"delta"}`))
						cancel()
					}
					_ = helper.StringData(c, `{"type":"test-event"}`)
					if tc.mode != "timeout" {
						info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
					}
				}
				if tc.cancel {
					cancel()
				}
				outcome = perfmetrics.ClassifyRelayOutcome(c.Request.Context(), info, nil)
			})
			common.TrackResponseDelivery(engine).ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))
			assert.Equal(t, tc.want, outcome)
		})
	}
}

func TestDeliveryTrackingPreservesWebSocketUpgrade(t *testing.T) {
	server := httptest.NewServer(common.TrackResponseDelivery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		defer conn.Close()
		assert.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("ready")))
	})))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, message, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "ready", string(message))
}

func runDeliveryFixture(t *testing.T, f fixture, closeConnection bool) {
	t.Helper()
	results := make(chan observation, 1)
	clientReceived := make(chan struct{})
	handlerFinished := make(chan struct{})
	engine := gin.New()
	engine.POST("/", func(c *gin.Context) {
		info := &relaycommon.RelayInfo{
			OriginModelName: "mock", UsingGroup: "mock", StartTime: time.Now(),
			RelayFormat: f.format, IsStream: f.stream, DisablePing: true,
			TokenCountMeta: relaycommon.TokenCountMeta{},
			ChannelMeta:    &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "mock"},
		}
		ct := "application/json"
		if f.stream {
			ct = "text/event-stream"
		}
		resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {ct}}, Body: io.NopCloser(strings.NewReader(f.body))}
		_, apiErr := f.handler(c, info, resp)
		close(handlerFinished)
		if apiErr != nil {
			results <- observation{err: apiErr}
			return
		}
		// A deterministic barrier stands in for the real settlement window.
		// No call to context.CancelFunc is used anywhere in this experiment.
		if closeConnection {
			select {
			case <-c.Request.Context().Done():
			case <-time.After(5 * time.Second):
				results <- observation{err: fmt.Errorf("connection did not close")}
				return
			}
		} else {
			select {
			case <-clientReceived:
			case <-time.After(5 * time.Second):
				results <- observation{err: fmt.Errorf("client did not receive body")}
				return
			}
		}
		results <- observation{
			outcome: perfmetrics.ClassifyRelayOutcome(c.Request.Context(), info, nil),
			close:   c.Request.Close, ctxErr: c.Request.Context().Err(),
			stream: info.StreamStatus.OutcomeSnapshot(),
		}
	})
	backend := httptest.NewServer(common.TrackResponseDelivery(engine))
	defer backend.Close()
	target, err := url.Parse(backend.URL)
	require.NoError(t, err)
	proxyHandler := httputil.NewSingleHostReverseProxy(target)
	// Same backend connection semantics as production's no-Upgrade -> close map.
	transport := &http.Transport{DisableKeepAlives: closeConnection}
	defer transport.CloseIdleConnections()
	proxyHandler.Transport = transport
	proxyHandler.FlushInterval = -1
	proxy := httptest.NewServer(proxyHandler)
	defer proxy.Close()
	request, err := http.NewRequest(http.MethodPost, proxy.URL, nil)
	require.NoError(t, err)
	// The downstream client does NOT request Connection: close.
	client := proxy.Client()
	client.Timeout = 8 * time.Second
	response, err := client.Do(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var received string
	if f.stream {
		reader := bufio.NewReader(response.Body)
		for {
			line, readErr := reader.ReadString('\n')
			require.NoError(t, readErr)
			received += line
			if line == "\n" && strings.Contains(received, f.terminal) {
				break
			}
		}
		// Ensure the real adaptor has processed the terminal before the
		// client ends its completed SSE subscription.
		select {
		case <-handlerFinished:
		case <-time.After(8 * time.Second):
			t.Fatal("handler did not finish")
		}
		if closeConnection {
			response.Body.Close()
		}
	} else {
		data, readErr := io.ReadAll(response.Body)
		require.NoError(t, readErr)
		require.Equal(t, f.body, string(data))
		received = string(data)
		response.Body.Close()
	}
	close(clientReceived)
	var r observation
	select {
	case r = <-results:
	case <-time.After(8 * time.Second):
		t.Fatal("relay did not finish")
	}
	response.Body.Close()
	require.NoError(t, r.err)
	require.Equal(t, perfmetrics.OutcomeSuccess, r.outcome)
	if f.stream && r.stream.Response != relaycommon.ResponseOutcomeCompleted {
		t.Fatal("missing real stream completed evidence")
	}
	t.Logf("%s close=%t backend_request_close=%t received=%d outcome=%s ctx=%v terminal=%s end=%s\n",
		f.name, closeConnection, r.close, len(received), r.outcome, r.ctxErr, r.stream.Response, r.stream.EndReason)
}

package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOaiResponsesStreamHandlerStopsAtCompletedEventWithoutUpstreamEOF(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 1
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	for _, eventType := range []string{"response.completed", "response.done"} {
		t.Run(eventType, func(t *testing.T) {
			reader, writer := io.Pipe()
			t.Cleanup(func() {
				_ = reader.Close()
				_ = writer.Close()
			})
			go func() {
				_, _ = io.WriteString(writer, fmt.Sprintf(`data: {"type":%q,"response":{"status":"completed","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`+"\n\n", eventType))
			}()

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Set(common.RequestIdKey, "responses-completed-test")
			info := &relaycommon.RelayInfo{
				OriginModelName: "gpt-test",
				DisablePing:     true,
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "gpt-test",
				},
			}
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       reader,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			}

			usage, apiErr := OaiResponsesStreamHandler(c, info, resp)

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 2, usage.PromptTokens)
			assert.Equal(t, 3, usage.CompletionTokens)
			require.NotNil(t, info.StreamStatus)
			assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
			assert.Contains(t, recorder.Body.String(), "event: "+eventType)
		})
	}
}

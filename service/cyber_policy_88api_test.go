package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecideRelayRetryStopsOnSub2APICyberSessionBlock(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	blocked := types.WithOpenAIError(types.OpenAIError{
		Message: "该会话已被网络安全策略屏蔽，请开启新会话 / This session is blocked by cyber-security policy, please start a new session",
		Type:    "permission_error",
		Code:    "session_blocked_by_cyber_policy",
	}, http.StatusForbidden)
	decision := DecideRelayRetry(c, blocked, 2)
	assert.Equal(t, "stop", decision.Action)
	assert.Equal(t, "system_retry_exclusion", decision.Reason)

	// Other 403s keep the global retry policy.
	other := types.WithOpenAIError(types.OpenAIError{Message: "forbidden", Type: "permission_error", Code: "forbidden"}, http.StatusForbidden)
	assert.Equal(t, "retry", DecideRelayRetry(c, other, 2).Action)
}

func TestAppendStreamStatusRecordsFailureCode(t *testing.T) {
	ss := relaycommon.NewStreamStatus()
	ss.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
	ss.MarkFailed("cyber_policy", "invalid_request_error", 0)
	other := model.NewLogOther()

	appendStreamStatus(&relaycommon.RelayInfo{IsStream: true, StreamStatus: ss}, other)

	var decoded map[string]any
	require.NoError(t, common.UnmarshalJsonStr(other.JSONString(), &decoded))
	streamInfo, ok := decoded["stream_status"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "error", streamInfo["status"])
	assert.Equal(t, "failed", streamInfo["response_status"])
	assert.Equal(t, "cyber_policy", streamInfo["error_code"])
	assert.Equal(t, "invalid_request_error", streamInfo["error_type"])
}

func TestAppendStreamStatusOmitsEmptyFailureCode(t *testing.T) {
	ss := relaycommon.NewStreamStatus()
	ss.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	other := model.NewLogOther()

	appendStreamStatus(&relaycommon.RelayInfo{IsStream: true, StreamStatus: ss}, other)

	var decoded map[string]any
	require.NoError(t, common.UnmarshalJsonStr(other.JSONString(), &decoded))
	streamInfo := decoded["stream_status"].(map[string]any)
	assert.NotContains(t, streamInfo, "error_code")
	assert.NotContains(t, streamInfo, "error_type")
}

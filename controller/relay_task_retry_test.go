package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldRetryTaskRelayDoesNotReplaceLocalBillingError(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	apiErr := types.NewErrorWithStatusCode(
		fmt.Errorf("预扣费额度失败, 用户剩余额度: 1, 需要预扣费额度: 4"),
		types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden,
		types.ErrOptionWithSkipRetry(),
	)
	taskErr := service.TaskErrorFromAPIError(apiErr)

	require.False(t, shouldRetryTaskRelay(c, 55, taskErr, 3))
}

func TestShouldRetryTaskRelayNeverRetriesLocalErrors(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	for _, statusCode := range []int{
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
	} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			taskErr := &taskdto.TaskError{
				StatusCode: statusCode,
				LocalError: true,
			}

			require.False(t, shouldRetryTaskRelay(c, 55, taskErr, 3))
		})
	}
}

func TestShouldRetryTaskRelayUsesConfiguredStatusCodes(t *testing.T) {
	originalRanges := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() {
		operation_setting.AutomaticRetryStatusCodeRanges = originalRanges
	})
	require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("429,500-503"))

	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	require.False(t, shouldRetryTaskRelay(c, 55, &taskdto.TaskError{StatusCode: http.StatusForbidden}, 3))
	require.True(t, shouldRetryTaskRelay(c, 55, &taskdto.TaskError{StatusCode: http.StatusTooManyRequests}, 3))
	require.True(t, shouldRetryTaskRelay(c, 55, &taskdto.TaskError{StatusCode: http.StatusInternalServerError}, 3))
	require.False(t, shouldRetryTaskRelay(c, 55, &taskdto.TaskError{StatusCode: http.StatusGatewayTimeout}, 3))
}

func TestRespondTaskErrorPreservesLocalRateLimitMessage(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	taskErr := &taskdto.TaskError{
		Message:    "本地限流，请稍后再试",
		StatusCode: http.StatusTooManyRequests,
		LocalError: true,
	}

	respondTaskError(c, taskErr)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "本地限流，请稍后再试", taskErr.Message)
}

func TestRespondTaskErrorRewritesUpstreamRateLimitMessage(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	taskErr := &taskdto.TaskError{
		Message:    "upstream rate limit",
		StatusCode: http.StatusTooManyRequests,
	}

	respondTaskError(c, taskErr)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "当前分组上游负载已饱和，请稍后再试", taskErr.Message)
}

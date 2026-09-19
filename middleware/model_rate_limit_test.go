package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveModelRequestRateLimitFallsBackFromTokenGroupToUserGroup(t *testing.T) {
	originalTotal := setting.ModelRequestRateLimitCount
	originalSuccess := setting.ModelRequestRateLimitSuccessCount
	originalGroups := setting.ModelRequestRateLimitGroup2JSONString()
	setting.ModelRequestRateLimitCount = 60
	setting.ModelRequestRateLimitSuccessCount = 60
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(`{
		"vip":[120,120],
		"代理商":[600,600],
		"Claude 官转 可蒸馏":[2000,2000]
	}`))
	t.Cleanup(func() {
		setting.ModelRequestRateLimitCount = originalTotal
		setting.ModelRequestRateLimitSuccessCount = originalSuccess
		require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(originalGroups))
	})

	tests := []struct {
		name        string
		tokenGroup  string
		userGroup   string
		expectTotal int
		expectOK    int
	}{
		{name: "default user and unconfigured model group use global limits", tokenGroup: "GPT-Pro", userGroup: "default", expectTotal: 60, expectOK: 60},
		{name: "vip user and unconfigured model group use identity limits", tokenGroup: "GPT-Pro", userGroup: "vip", expectTotal: 120, expectOK: 120},
		{name: "configured model group overrides vip identity limits", tokenGroup: "Claude 官转 可蒸馏", userGroup: "vip", expectTotal: 2000, expectOK: 2000},
		{name: "agent user and unconfigured model group use identity limits", tokenGroup: "GPT-Pro", userGroup: "代理商", expectTotal: 600, expectOK: 600},
		{name: "empty token group uses vip identity limits", userGroup: "vip", expectTotal: 120, expectOK: 120},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			total, success := resolveModelRequestRateLimit(test.tokenGroup, test.userGroup)
			assert.Equal(t, test.expectTotal, total)
			assert.Equal(t, test.expectOK, success)
		})
	}
}

func TestModelRequestRateLimitMessagesGuideUsersToSupport(t *testing.T) {
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		language string
		key      string
		expected string
	}{
		{name: "Chinese successful request limit", language: "zh-CN", key: i18n.MsgRateLimitReached, expected: "您的请求频率过快；当前用户组1分钟内最多允许60次成功请求，请稍后重试。如需更高的请求频率或并发额度，请前往https://88api.ai 控制台内联系客服申请提升。"},
		{name: "Chinese total request limit", language: "zh-CN", key: i18n.MsgRateLimitTotalReached, expected: "您的请求频率过快；当前用户组1分钟内最多允许60次请求（失败请求也会计入），请检查请求参数并稍后重试。如需更高的请求频率或并发额度，请前往https://88api.ai 控制台内联系客服申请提升。"},
		{name: "English successful request limit", language: "en", key: i18n.MsgRateLimitReached, expected: "Your request rate is too high. The current user group allows up to 60 successful requests within a 1-minute window. Please try again later. To request a higher rate or concurrency limit, visit the https://88api.ai console and contact support."},
		{name: "English total request limit", language: "en", key: i18n.MsgRateLimitTotalReached, expected: "Your request rate is too high. The current user group allows up to 60 requests within a 1-minute window, and failed requests also count. Check your request parameters and try again later. To request a higher rate or concurrency limit, visit the https://88api.ai console and contact support."},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("GET", "/v1/chat/completions", nil)
			ctx.Request.Header.Set("Accept-Language", test.language)
			message := common.TranslateMessage(ctx, test.key, map[string]any{"Minutes": 1, "Max": 60})
			assert.Equal(t, test.expected, message)
		})
	}
}

func TestModelRedisRateLimitUsesUTCRegardlessOfLocalTimezone(t *testing.T) {
	redisServer, redisClient := useRateLimitMiniRedis(t)
	previousLocation := time.Local
	time.Local = time.FixedZone("test-utc-plus-eight", 8*60*60)
	t.Cleanup(func() { time.Local = previousLocation })

	ctx := context.Background()
	recordKey := "rateLimit:model-utc-record"
	recordRedisRequest(ctx, redisClient, recordKey, 2)
	recorded, err := redisClient.LIndex(ctx, recordKey, 0).Result()
	require.NoError(t, err)
	recordedAt, err := time.Parse(modelRateLimitTimeFormat, recorded)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC(), recordedAt, 2*time.Second)

	checkKey := "rateLimit:model-utc-check"
	withinWindow := time.Now().UTC().Add(-30 * time.Second).Format(modelRateLimitTimeFormat)
	_, err = redisServer.Push(checkKey, withinWindow, withinWindow)
	require.NoError(t, err)
	allowed, err := checkRedisRateLimit(ctx, redisClient, checkKey, 2, 60)
	require.NoError(t, err)
	assert.False(t, allowed, "an existing UTC timestamp inside the window must remain limited on a non-UTC host")
}

var modelRateLimitTestUsers atomic.Int64

func TestModelRateLimitStreamFailuresDoNotConsumeSuccessLimit(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		for _, totalLimit := range []int{0, 2} {
			t.Run(fmt.Sprintf("%s/total=%d", backend, totalLimit), func(t *testing.T) {
				userID := 7200000 + int(modelRateLimitTestUsers.Add(1))
				handler := memoryRateLimitHandler(60, totalLimit, 1)
				if backend == "redis" {
					useRateLimitMiniRedis(t)
					handler = redisRateLimitHandler(60, totalLimit, 1)
				}
				router := gin.New()
				router.GET("/:outcome", func(c *gin.Context) { c.Set("id", userID) }, handler, func(c *gin.Context) {
					status := relaycommon.NewStreamStatus()
					if c.Param("outcome") == "failed" {
						status.MarkFailed("server_error", "", 0)
					} else {
						status.MarkCompleted()
					}
					common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, status)
					c.Status(http.StatusOK)
				})
				assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/failed", "127.0.0.1:1000").Code)
				if totalLimit > 0 {
					assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/failed", "127.0.0.1:1000").Code)
				} else {
					assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
				}
				assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
			})
		}
	}
}

func TestModelMemoryRateLimitReservesConcurrentSuccessAdmission(t *testing.T) {
	userID := 7200000 + int(modelRateLimitTestUsers.Add(1))
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	router := gin.New()
	router.GET("/:outcome", func(c *gin.Context) { c.Set("id", userID) }, memoryRateLimitHandler(60, 0, 1), func(c *gin.Context) {
		if c.Param("outcome") == "failed" {
			close(entered)
			<-release
			status := relaycommon.NewStreamStatus()
			status.MarkFailed("server_error", "", 0)
			common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, status)
		}
		c.Status(http.StatusOK)
	})
	go func() {
		defer close(finished)
		assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/failed", "127.0.0.1:1000").Code)
	}()
	<-entered
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
	close(release)
	<-finished
	assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
}

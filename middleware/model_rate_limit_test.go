package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting"
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

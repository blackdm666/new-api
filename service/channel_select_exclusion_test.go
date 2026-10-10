package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExcludedFailedChannelKeepsAutoRetryInItsGroup(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-groups-exclusion-model"
	createChannelSelectAutoGroupsChannel(t, db, 2201, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2202, "default", modelName)
	model.InitChannelCache()
	gin.SetMode(gin.TestMode)

	for _, crossGroupRetry := range []bool{false, true} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
		common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, crossGroupRetry)
		retry := 0
		param := &RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: modelName, RequestPath: "/v1/videos", Retry: &retry}

		first, group, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, first)
		assert.Equal(t, 2201, first.Id)
		assert.Equal(t, "vip", group)

		ExcludeChannelFromRetry(ctx, first.Id)
		param.IncreaseRetry()
		second, _, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		if crossGroupRetry {
			require.NotNil(t, second)
			assert.Equal(t, 2202, second.Id, "cross-group retry still reaches the next group")
		} else {
			assert.Nil(t, second, "without cross-group retry the request stays in its group")
		}
	}
}

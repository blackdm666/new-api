package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSelectAutoGroupsTest(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRetryTimes := common.RetryTimes
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalMaxTokenAutoGroups := setting.GetMaxTokenAutoGroups()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.MemoryCacheEnabled = true
	common.RetryTimes = 0

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))

	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RetryTimes = originalRetryTimes
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(fmt.Sprintf("%d", originalMaxTokenAutoGroups)))

		if originalMemoryCacheEnabled && originalDB != nil &&
			originalDB.Migrator().HasTable(&model.Channel{}) && originalDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	return db
}

func createChannelSelectAutoGroupsChannel(t *testing.T, db *gorm.DB, id int, group, modelName string) {
	t.Helper()
	priority := int64(0)
	weight := uint(100)
	require.NoError(t, db.Create(&model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      fmt.Sprintf("key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", id),
		Weight:   &weight,
		Models:   modelName,
		Group:    group,
		Priority: &priority,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

func TestCacheGetRandomSatisfiedChannelUsesTokenAutoGroupsWhenGlobalAutoIsEmpty(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-groups-runtime-model"
	createChannelSelectAutoGroupsChannel(t, db, 2101, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2102, "default", modelName)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}

	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2101, first.Id)
	assert.Equal(t, "vip", selectedGroup)
	assert.Equal(t, "vip", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
	assert.Empty(t, setting.GetAutoGroups(), "the selection must not depend on the global Auto list")

	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2102, second.Id)
	assert.Equal(t, "default", selectedGroup)
	assert.Equal(t, "default", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

// A token group that does not list the model is the token's configuration, not
// an outage: 400 with the groups the user may switch to. A group that lists the
// model with disabled channels only keeps the retryable 503.
func TestSelectChannelForRequestTellsGroupWithoutModelFromUnavailableChannels(t *testing.T) {
	require.NoError(t, appI18n.Init())
	db := setupChannelSelectAutoGroupsTest(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP","video":"Video"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2,"video":1,"private":1}`))
	createChannelSelectAutoGroupsChannel(t, db, 2201, "video", "video-model")
	createChannelSelectAutoGroupsChannel(t, db, 2202, "vip", "video-model")
	// Not selectable by the user, so never suggested.
	createChannelSelectAutoGroupsChannel(t, db, 2203, "private", "video-model")
	createChannelSelectAutoGroupsChannel(t, db, 2204, "default", "text-model")
	priority := int64(0)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "paused-model", ChannelId: 2205, Enabled: false, Priority: &priority}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "vip", Model: "paused-model", ChannelId: 2206, Enabled: false, Priority: &priority}).Error)
	model.InitChannelCache()

	for _, tc := range []struct {
		name, group, model string
		autoGroups         []string
		wantStatus         int
		wantCode           types.ErrorCode
		wantMessageID      string
		wantGroups         any
	}{
		{name: "group without the model", group: "default", model: "video-model", wantStatus: http.StatusBadRequest, wantCode: types.ErrorCodeModelNotInGroup, wantMessageID: appI18n.MsgDistributorModelNotInGroupSuggest, wantGroups: "video, vip"},
		{name: "model no group offers", group: "default", model: "missing-model", wantStatus: http.StatusBadRequest, wantCode: types.ErrorCodeModelNotInGroup, wantMessageID: appI18n.MsgDistributorModelNotInGroup},
		{name: "disabled channels are not suggested", group: "video", model: "paused-model", wantStatus: http.StatusBadRequest, wantCode: types.ErrorCodeModelNotInGroup, wantMessageID: appI18n.MsgDistributorModelNotInGroup},
		{name: "group with disabled channels only", group: "default", model: "paused-model", wantStatus: http.StatusServiceUnavailable, wantCode: types.ErrorCodeModelNotFound, wantMessageID: appI18n.MsgDistributorNoAvailableChannel},
		{name: "auto without the model in any candidate", group: "auto", model: "video-model", autoGroups: []string{"default"}, wantStatus: http.StatusBadRequest, wantCode: types.ErrorCodeModelNotInGroup, wantMessageID: appI18n.MsgDistributorModelNotInGroupSuggest, wantGroups: "video, vip"},
		{name: "auto with a candidate listing the model", group: "auto", model: "paused-model", autoGroups: []string{"default", "vip"}, wantStatus: http.StatusServiceUnavailable, wantCode: types.ErrorCodeModelNotFound, wantMessageID: appI18n.MsgDistributorNoAvailableChannel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(ctx, constant.ContextKeyUsingGroup, tc.group)
			if tc.autoGroups != nil {
				common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, tc.autoGroups)
			}
			retry := 0
			channel, _, selectErr := SelectChannelForRequest(ctx, tc.model, &RetryParam{
				Ctx: ctx, TokenGroup: tc.group, ModelName: tc.model, RequestPath: "/v1/videos", Retry: &retry,
			})
			require.Nil(t, channel)
			require.NotNil(t, selectErr)
			assert.Equal(t, tc.wantStatus, selectErr.StatusCode)
			assert.Equal(t, tc.wantCode, selectErr.Code)
			assert.Equal(t, tc.wantMessageID, selectErr.MessageID)
			assert.Equal(t, tc.wantGroups, selectErr.Params["Groups"])
			assert.Equal(t, tc.wantStatus == http.StatusServiceUnavailable, selectErr.NoAvailableChannel)
		})
	}

	params := map[string]any{"Group": "Minimax官转", "Model": "minimax-h3", "Groups": "视频模型"}
	assert.Equal(t, "令牌分组「Minimax官转」不提供模型「minimax-h3」。请在控制台「API 密钥」页编辑该令牌，改用包含该模型的分组：视频模型。",
		appI18n.Translate(appI18n.LangZhCN, appI18n.MsgDistributorModelNotInGroupSuggest, params))
	assert.Equal(t, `Token group "Minimax官转" does not offer model "minimax-h3". Edit the token on the "API Keys" page of the console and switch it to a group that offers this model: 视频模型.`,
		appI18n.Translate(appI18n.LangEn, appI18n.MsgDistributorModelNotInGroupSuggest, params))
}

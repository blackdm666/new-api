package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureTokenGroupAuthorizationTest(
	t *testing.T,
	usableGroups string,
	groupRatios string,
	specialGroups map[string]map[string]string,
) {
	t.Helper()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	specialSetting := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	originalSpecialGroups := specialSetting.ReadAll()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(usableGroups))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groupRatios))
	specialSetting.Clear()
	specialSetting.AddAll(specialGroups)
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		specialSetting.Clear()
		specialSetting.AddAll(originalSpecialGroups)
	})
}

func tokenGroupAuthorizationRequest(name string, group string) map[string]any {
	return map[string]any{
		"name":                 name,
		"expired_time":         -1,
		"remain_quota":         100,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                group,
		"cross_group_retry":    false,
	}
}

func tokenGroupAuthorizationContext(
	t *testing.T,
	method string,
	body any,
	userID int,
	userGroup string,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	ctx, recorder := newAuthenticatedContext(t, method, "/api/token/", body, userID)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, userGroup)
	return ctx, recorder
}

func TestAddTokenRejectsUnauthorizedExplicitGroup(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	configureTokenGroupAuthorizationTest(
		t,
		`{"default":"Default"}`,
		`{"default":1,"内部自用":1}`,
		nil,
	)
	request := tokenGroupAuthorizationRequest("forbidden-internal", "内部自用")
	ctx, recorder := tokenGroupAuthorizationContext(t, http.MethodPost, request, 101, "default")

	AddToken(ctx)

	assert.False(t, decodeAPIResponse(t, recorder).Success)
	var count int64
	require.NoError(t, db.Model(&model.Token{}).Where("user_id = ?", 101).Count(&count).Error)
	assert.Zero(t, count)
}

func TestAddTokenRejectsGroupWithoutPricingRatio(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	configureTokenGroupAuthorizationTest(
		t,
		`{"default":"Default","retired":"Retired"}`,
		`{"default":1}`,
		nil,
	)
	request := tokenGroupAuthorizationRequest("missing-ratio", "retired")
	ctx, recorder := tokenGroupAuthorizationContext(t, http.MethodPost, request, 102, "default")

	AddToken(ctx)

	assert.False(t, decodeAPIResponse(t, recorder).Success)
	var count int64
	require.NoError(t, db.Model(&model.Token{}).Where("user_id = ?", 102).Count(&count).Error)
	assert.Zero(t, count)
}

func TestAddTokenRejectsSpeciallyRemovedGroup(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	configureTokenGroupAuthorizationTest(
		t,
		`{"default":"Default","private":"Private"}`,
		`{"default":1,"private":1}`,
		map[string]map[string]string{"restricted": {"-:private": ""}},
	)
	request := tokenGroupAuthorizationRequest("special-removal", "private")
	ctx, recorder := tokenGroupAuthorizationContext(t, http.MethodPost, request, 103, "restricted")

	AddToken(ctx)

	assert.False(t, decodeAPIResponse(t, recorder).Success)
	var count int64
	require.NoError(t, db.Model(&model.Token{}).Where("user_id = ?", 103).Count(&count).Error)
	assert.Zero(t, count)
}

func TestAddTokenAllowsSpeciallyGrantedGroup(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	configureTokenGroupAuthorizationTest(
		t,
		`{"default":"Default"}`,
		`{"default":1,"private":1}`,
		map[string]map[string]string{"enterprise": {"+:private": "Private"}},
	)
	request := tokenGroupAuthorizationRequest("special-grant", "private")
	ctx, recorder := tokenGroupAuthorizationContext(t, http.MethodPost, request, 103, "enterprise")

	AddToken(ctx)

	require.True(t, decodeAPIResponse(t, recorder).Success)
	var stored model.Token
	require.NoError(t, db.Where("user_id = ?", 103).First(&stored).Error)
	assert.Equal(t, "private", stored.Group)
}

func TestAddTokenPreservesEmptyGroupInheritance(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	configureTokenGroupAuthorizationTest(t, `{"default":"Default"}`, `{"default":1}`, nil)
	request := tokenGroupAuthorizationRequest("inherited-group", "")
	ctx, recorder := tokenGroupAuthorizationContext(t, http.MethodPost, request, 104, "default")

	AddToken(ctx)

	require.True(t, decodeAPIResponse(t, recorder).Success)
	var stored model.Token
	require.NoError(t, db.Where("user_id = ?", 104).First(&stored).Error)
	assert.Empty(t, stored.Group)
}

func TestUpdateTokenRejectsUnauthorizedGroupChange(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	configureTokenGroupAuthorizationTest(
		t,
		`{"default":"Default"}`,
		`{"default":1,"内部自用":1}`,
		nil,
	)
	token := seedToken(t, db, 105, "owned", "owned-token-key")
	request := tokenGroupAuthorizationRequest("owned", "内部自用")
	request["id"] = token.Id
	ctx, recorder := tokenGroupAuthorizationContext(t, http.MethodPut, request, 105, "default")

	UpdateToken(ctx)

	assert.False(t, decodeAPIResponse(t, recorder).Success)
	var stored model.Token
	require.NoError(t, db.First(&stored, token.Id).Error)
	assert.Equal(t, "default", stored.Group)
}

func TestUpdateTokenAllowsUnchangedLegacyGroup(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	configureTokenGroupAuthorizationTest(t, `{"default":"Default"}`, `{"default":1}`, nil)
	token := seedToken(t, db, 106, "legacy", "legacy-token-key")
	require.NoError(t, db.Model(token).Update("group", "retired").Error)
	request := tokenGroupAuthorizationRequest("renamed-legacy", "retired")
	request["id"] = token.Id
	ctx, recorder := tokenGroupAuthorizationContext(t, http.MethodPut, request, 106, "default")

	UpdateToken(ctx)

	require.True(t, decodeAPIResponse(t, recorder).Success)
	var stored model.Token
	require.NoError(t, db.First(&stored, token.Id).Error)
	assert.Equal(t, "renamed-legacy", stored.Name)
	assert.Equal(t, "retired", stored.Group)
}

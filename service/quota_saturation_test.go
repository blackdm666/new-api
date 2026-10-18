package service

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestAttachQuotaSaturationNestsUnderAdminInfo verifies the saturation marker
// is nested under other.admin_info.quota_saturation so it is admin-only (the
// log formatter strips admin_info for non-admin viewers).
func TestAttachQuotaSaturationNestsUnderAdminInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	relayInfo := &relaycommon.RelayInfo{
		UserId:          7,
		OriginModelName: "gpt-image-1",
		QuotaClamp: &common.QuotaClamp{
			Op:       "QuotaFromDecimal",
			Kind:     common.QuotaClampOverflow,
			Original: 1.8e19,
			Clamped:  common.MaxQuota,
		},
	}

	other := model.NewLogOther()
	other.SetPublic("model_price", 0.004)
	attachQuotaSaturation(ctx, relayInfo, other)

	adminInfo, ok := other.Snapshot()["admin_info"].(map[string]any)
	require.True(t, ok, "admin_info should be created")
	sat, ok := adminInfo["quota_saturation"].(map[string]any)
	require.True(t, ok, "quota_saturation should be nested under admin_info")
	require.Equal(t, "QuotaFromDecimal", sat["op"])
	require.Equal(t, common.QuotaClampOverflow, sat["kind"])
	require.Equal(t, common.MaxQuota, sat["clamped"])
}

func TestCalcViolationFeeQuotaSaturates(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	require.Equal(t, common.MaxQuota, calcViolationFeeQuota(1e20, 1))
}

func TestCalcOpenRouterCacheCreateTokensDoesNotWrap(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	got := CalcOpenRouterCacheCreateTokens(dto.Usage{Cost: math.Inf(1)}, hosttypes.PriceData{
		ModelRatio:         1,
		CacheCreationRatio: 2,
		CacheRatio:         1,
		CompletionRatio:    1,
	})
	require.Equal(t, -1, got)
}

// TestAttachQuotaSaturationPreservesExistingAdminInfo verifies the marker is
// merged into a pre-existing admin_info map without clobbering it.
func TestAttachQuotaSaturationPreservesExistingAdminInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	relayInfo := &relaycommon.RelayInfo{
		QuotaClamp: &common.QuotaClamp{Op: "QuotaFromFloat", Kind: common.QuotaClampUnderflow, Clamped: common.MinQuota},
	}
	other := model.NewLogOther()
	other.SetAdmin("admin_username", "root")
	attachQuotaSaturation(ctx, relayInfo, other)

	adminInfo := other.Snapshot()["admin_info"].(map[string]any)
	require.Equal(t, "root", adminInfo["admin_username"], "existing admin_info fields preserved")
	require.NotNil(t, adminInfo["quota_saturation"])
}

// TestAttachQuotaSaturationNoClampNoMarker verifies the common case (no
// saturation) leaves the log untouched.
func TestAttachQuotaSaturationNoClampNoMarker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	relayInfo := &relaycommon.RelayInfo{QuotaClamp: nil}
	other := model.NewLogOther()
	other.SetPublic("model_price", 0.004)
	attachQuotaSaturation(ctx, relayInfo, other)

	_, hasAdmin := other.Snapshot()["admin_info"]
	require.False(t, hasAdmin, "no admin_info should be added when there is no clamp")
}

func TestPreConsumeBillingRejectsSaturatedQuotaBeforeDeduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{
		QuotaClamp: &common.QuotaClamp{
			Op:       "QuotaFromFloat",
			Kind:     common.QuotaClampOverflow,
			Original: 1e30,
			Clamped:  common.MaxQuota,
		},
	}

	apiErr := PreConsumeBilling(c, common.MaxQuota, info)

	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeModelPriceError, apiErr.GetErrorCode())
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Same(t, info.QuotaClamp, apiErr.Err)
	var clamp *common.QuotaClamp
	require.ErrorAs(t, apiErr, &clamp)
	require.Same(t, info.QuotaClamp, clamp)
	require.Nil(t, info.Billing)
}

func TestPreConsumeBillingRejectsNegativeQuotaBeforeDeduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{}

	apiErr := PreConsumeBilling(c, -1, info)

	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeModelPriceError, apiErr.GetErrorCode())
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Nil(t, info.Billing)
}

// Balance and token quota rejections read as customer messages in the site
// currency with two decimals, while the error codes and the 403 that clients
// branch on stay as they were, and nothing is reserved.
func TestPreConsumeBillingExplainsInsufficientBalanceAndTokenQuota(t *testing.T) {
	require.NoError(t, appI18n.Init())
	gin.SetMode(gin.TestMode)
	originalDB, originalRedis := model.DB, common.RedisEnabled
	originalDisplay, originalRate := operation_setting.GetGeneralSetting().QuotaDisplayType, operation_setting.USDExchangeRate
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	model.DB, common.RedisEnabled = db, false
	// InitLogDB runs the column-name setup that InitDB would, so the token
	// remaining-quota lookup by key works on this database.
	originalLogDB, originalMaster := model.LOG_DB, common.IsMasterNode
	common.IsMasterNode = false
	require.NoError(t, model.InitLogDB())
	model.LOG_DB, common.IsMasterNode = originalLogDB, originalMaster
	// ¥1 = 250000 quota.
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	operation_setting.USDExchangeRate = 2
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = originalDB, originalRedis
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplay
		operation_setting.USDExchangeRate = originalRate
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	const required = 3211088 // ¥12.844352
	for _, tc := range []struct {
		name                  string
		userQuota, tokenQuota int
		lang                  string
		wantCode              types.ErrorCode
		wantMessage           string
	}{
		{"negative balance", -125000, 0, "zh-CN", types.ErrorCodeInsufficientUserQuota,
			"余额不足：当前余额 ¥-0.50，请充值后重试。"},
		{"balance below the reserve", 1250000, 0, "zh-CN", types.ErrorCodeInsufficientUserQuota,
			"余额不足：当前余额 ¥5.00，本次需要预留 ¥12.84（按请求参数预估，完成后按实际用量多退少补）。请充值后重试。"},
		{"balance below the reserve in English", 1250000, 0, "en", types.ErrorCodeInsufficientUserQuota,
			"Insufficient balance: your balance is ¥5.00, and this request needs ¥12.84 reserved (estimated from the request parameters; after completion you pay for actual usage and the difference is refunded or charged). Please top up and try again."},
		{"token quota below the reserve", 25000000, 500000, "zh-CN", types.ErrorCodePreConsumeTokenQuotaFailed,
			"令牌额度不足：该令牌剩余 ¥2.00，本次需要预留 ¥12.84。请在控制台「API 密钥」页提高该令牌的额度或设为无限配额。"},
		{"token quota below the reserve in English", 25000000, 500000, "en", types.ErrorCodePreConsumeTokenQuotaFailed,
			`Insufficient token quota: this token has ¥2.00 left, and this request needs ¥12.84 reserved. Raise the token quota or set it to Unlimited Quota on the "API Keys" page of the console.`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := model.User{Username: "quota-message-" + common.GetRandomString(6), Password: "unused-password-hash",
				Status: common.UserStatusEnabled, Group: "default", Quota: tc.userQuota, AffCode: common.GetRandomString(10)}
			require.NoError(t, db.Create(&user).Error)
			token := model.Token{UserId: user.Id, Key: "quota-message-" + common.GetRandomString(10), Name: "quota-message",
				Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: tc.tokenQuota, UnlimitedQuota: tc.tokenQuota == 0}
			require.NoError(t, db.Create(&token).Error)

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
			c.Request.Header.Set("Accept-Language", tc.lang)
			info := &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, TokenUnlimited: token.UnlimitedQuota,
				ForcePreConsume: true, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}

			apiErr := PreConsumeBilling(c, required, info)

			require.NotNil(t, apiErr)
			assert.Equal(t, tc.wantCode, apiErr.GetErrorCode())
			assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
			assert.Equal(t, tc.wantMessage, apiErr.Error())
			require.NoError(t, db.First(&user, user.Id).Error)
			require.NoError(t, db.First(&token, token.Id).Error)
			assert.Equal(t, tc.userQuota, user.Quota)
			assert.Equal(t, tc.tokenQuota, token.RemainQuota)
		})
	}
}

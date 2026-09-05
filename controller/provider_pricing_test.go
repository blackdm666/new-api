package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func providerPricingTestSignature(secret, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestValidProviderPricingSignature(t *testing.T) {
	secret := "test-provider-pricing-secret"
	now := time.Unix(1_747_886_400, 0)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	signature := providerPricingTestSignature(secret, timestamp)

	assert.True(t, validProviderPricingSignature(secret, timestamp, signature, now))
	assert.True(t, validProviderPricingSignature(secret, strconv.FormatInt(now.Unix()-60, 10), providerPricingTestSignature(secret, strconv.FormatInt(now.Unix()-60, 10)), now))
	assert.True(t, validProviderPricingSignature(secret, strconv.FormatInt(now.Unix()+60, 10), providerPricingTestSignature(secret, strconv.FormatInt(now.Unix()+60, 10)), now))
	assert.False(t, validProviderPricingSignature(secret, timestamp, "", now))
	assert.False(t, validProviderPricingSignature(secret, "not-a-timestamp", signature, now))
	assert.False(t, validProviderPricingSignature(secret, strconv.FormatInt(now.Unix()-61, 10), signature, now))
	assert.False(t, validProviderPricingSignature(secret, strconv.FormatInt(now.Unix()+61, 10), signature, now))
	assert.False(t, validProviderPricingSignature(secret, timestamp, providerPricingTestSignature("wrong-secret", timestamp), now))
}

func TestGetProviderPricingFailsClosedWithoutValidSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("secret not configured", func(t *testing.T) {
		t.Setenv(providerPricingAuthSecretEnv, "")
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Request = httptest.NewRequest("GET", "/api/provider/pricing", nil)

		GetProviderPricing(context)

		assert.Equal(t, 503, response.Code)
		assert.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
		assert.Contains(t, response.Body.String(), `"success":false`)
	})

	t.Run("invalid signature", func(t *testing.T) {
		t.Setenv(providerPricingAuthSecretEnv, "configured-secret")
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Request = httptest.NewRequest("GET", "/api/provider/pricing", nil)
		context.Request.Header.Set("X-Hvoy-Ts", strconv.FormatInt(time.Now().Unix(), 10))
		context.Request.Header.Set("X-Hvoy-Sign", "invalid")

		GetProviderPricing(context)

		assert.Equal(t, 401, response.Code)
		assert.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
		assert.Contains(t, response.Body.String(), `"message":"unauthorized"`)
	})
}

func TestBuildProviderPricingModels(t *testing.T) {
	cacheRatio := 0.1
	cacheCreateRatio := 1.25
	pricing := []model.Pricing{
		{
			ModelName:        "claude-test",
			QuotaType:        0,
			ModelRatio:       2,
			CompletionRatio:  3,
			CacheRatio:       &cacheRatio,
			CreateCacheRatio: &cacheCreateRatio,
			EnableGroup:      []string{"all"},
		},
		{
			ModelName:   "image-test",
			QuotaType:   1,
			ModelPrice:  0.2,
			BillingUnit: billing_setting.BillingUnitRequest,
			EnableGroup: []string{"standard"},
		},
		{
			ModelName:   "video-per-second",
			QuotaType:   1,
			ModelPrice:  1.8,
			BillingUnit: billing_setting.BillingUnitSecond,
			EnableGroup: []string{"standard"},
		},
		{
			ModelName:   "dynamic-test",
			QuotaType:   0,
			BillingMode: billing_setting.BillingModeTieredExpr,
			BillingExpr: `len <= 200000 ? tier("standard", p * 3 + c * 9 + cr * 0.3 + cc * 3.75 + cc1h * 6) : tier("long", p * 6 + c * 18)`,
			EnableGroup: []string{"all"},
		},
	}

	models := buildProviderPricingModels(pricing, map[string]float64{
		"standard": 0.5,
		"premium":  1.5,
		"auto":     1,
	}, 7)

	require.Len(t, models, 5)
	assert.Equal(t, "claude-test", models[0].ModelName)
	assert.Equal(t, "premium", models[0].GroupName)
	assert.InDelta(t, 42, *models[0].InputPrice, 1e-9)
	assert.InDelta(t, 126, *models[0].OutputPrice, 1e-9)
	assert.InDelta(t, 4.2, *models[0].CacheInputPrice, 1e-9)
	assert.InDelta(t, 52.5, *models[0].CacheCreatePrice, 1e-9)
	assert.InDelta(t, 84, *models[0].CacheCreatePrice1h, 1e-9)

	assert.Equal(t, "standard", models[1].GroupName)
	assert.InDelta(t, 14, *models[1].InputPrice, 1e-9)

	assert.Equal(t, "dynamic-test", models[2].ModelName)
	assert.Equal(t, "premium", models[2].GroupName)
	assert.InDelta(t, 31.5, *models[2].InputPrice, 1e-9)
	assert.InDelta(t, 94.5, *models[2].OutputPrice, 1e-9)
	assert.InDelta(t, 3.15, *models[2].CacheInputPrice, 1e-9)
	assert.InDelta(t, 39.375, *models[2].CacheCreatePrice, 1e-9)
	assert.InDelta(t, 63, *models[2].CacheCreatePrice1h, 1e-9)
	assert.Equal(t, "动态计价；展示最低标准档：standard；实际按请求时段/上下文结算", models[2].Note)

	assert.Equal(t, "standard", models[3].GroupName)
	assert.InDelta(t, 10.5, *models[3].InputPrice, 1e-9)

	assert.Equal(t, "image-test", models[4].ModelName)
	assert.Equal(t, providerPricingCallUnit, models[4].PriceUnit)
	assert.InDelta(t, 0.7, *models[4].UnitPrice, 1e-9)
}

func TestProviderPricingGroupsExpandsAllAndRemovesDuplicates(t *testing.T) {
	groups := providerPricingGroups(
		[]string{"all", "standard", "missing", "standard"},
		map[string]float64{"standard": 1, "premium": 0.5, "auto": 1, "all": 1},
	)

	assert.Equal(t, []string{"premium", "standard"}, groups)
}

func TestBuildProviderPricingModelsSkipsInvalidAndZeroCallPrices(t *testing.T) {
	models := buildProviderPricingModels([]model.Pricing{
		{ModelName: "free-call", QuotaType: 1, ModelPrice: 1, EnableGroup: []string{"free"}},
		{ModelName: "bad-ratio", QuotaType: 0, ModelRatio: -1, EnableGroup: []string{"paid"}},
	}, map[string]float64{"free": 0, "paid": 1}, 1)

	assert.Empty(t, models)
	assert.Empty(t, buildProviderPricingModels(nil, nil, 0))
}

func TestProviderTieredTokenPricesRejectsUnsupportedExpressions(t *testing.T) {
	t.Run("fixed fee", func(t *testing.T) {
		_, ok := providerTieredTokenPrices(`tier("base", 1 + p * 2)`, 1, 1)
		assert.False(t, ok)
	})

	t.Run("non linear", func(t *testing.T) {
		_, ok := providerTieredTokenPrices(`tier("base", p * p)`, 1, 1)
		assert.False(t, ok)
	})

	t.Run("task usage", func(t *testing.T) {
		_, ok := providerTieredTokenPrices(`tier("base", u("seconds") * 2)`, 1, 1)
		assert.False(t, ok)
	})
}

func TestProviderTieredTokenPricesMatchesProductionStyleStandardTier(t *testing.T) {
	prices, ok := providerTieredTokenPrices(
		`len <= 272000 ? tier("标准上下文", p * 35 + c * 210 + cr * 3.5 + cc * 43.75) : tier("长上下文", p * 70 + c * 315 + cr * 7 + cc * 87.5)`,
		0.1,
		1,
	)

	require.True(t, ok)
	require.NotNil(t, prices.InputPrice)
	require.NotNil(t, prices.OutputPrice)
	require.NotNil(t, prices.CacheInputPrice)
	require.NotNil(t, prices.CacheCreatePrice)
	assert.Nil(t, prices.CacheCreatePrice1h)
	assert.Equal(t, "标准上下文", prices.Tier)
	assert.InDelta(t, 3.5, *prices.InputPrice, 1e-9)
	assert.InDelta(t, 21, *prices.OutputPrice, 1e-9)
	assert.InDelta(t, 0.35, *prices.CacheInputPrice, 1e-9)
	assert.InDelta(t, 4.375, *prices.CacheCreatePrice, 1e-9)
}

func TestProviderTieredTokenPricesChoosesLowestTimeTier(t *testing.T) {
	prices, ok := providerTieredTokenPrices(
		`(weekday("Asia/Shanghai") >= 1 && weekday("Asia/Shanghai") <= 5 && hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 18) ? tier("高峰", p * 9 + c * 27 + cr * 0.3) : tier("空闲", p * 4.5 + c * 13.5 + cr * 0.15)`,
		0.7,
		1,
	)

	require.True(t, ok)
	assert.Equal(t, "空闲", prices.Tier)
	assert.InDelta(t, 3.15, *prices.InputPrice, 1e-9)
	assert.InDelta(t, 9.45, *prices.OutputPrice, 1e-9)
	assert.InDelta(t, 0.105, *prices.CacheInputPrice, 1e-9)
}

func TestProviderPricingSiteDomain(t *testing.T) {
	assert.Equal(t, "88api.ai", providerPricingSiteDomain("https://88api.ai/"))
	assert.Equal(t, "localhost", providerPricingSiteDomain("http://localhost:3000"))
	assert.Equal(t, "", providerPricingSiteDomain(""))
}

func TestProviderPricingJSONUsesNumbersAndModelLevelCallUnit(t *testing.T) {
	inputPrice := 1.25
	unitPrice := 0.2
	payload := providerPricingResponse{
		SchemaVersion: providerPricingSchemaVersion,
		Success:       true,
		Message:       "",
		Data: &providerPricingData{
			Currency:  "CNY",
			PriceUnit: providerPricingTokenUnit,
			UpdatedAt: "2026-09-04T12:00:00Z",
			Models: []providerPricingModel{
				{ModelName: "token-model", GroupName: "standard", InputPrice: &inputPrice, Enabled: true},
				{ModelName: "call-model", GroupName: "image", PriceUnit: providerPricingCallUnit, UnitPrice: &unitPrice, Enabled: true},
			},
		},
	}

	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	data := decoded["data"].(map[string]any)
	models := data["models"].([]any)
	tokenModel := models[0].(map[string]any)
	callModel := models[1].(map[string]any)

	assert.IsType(t, float64(0), tokenModel["input_price"])
	assert.NotContains(t, tokenModel, "price_unit")
	assert.NotContains(t, tokenModel, "unit_price")
	assert.Equal(t, providerPricingCallUnit, callModel["price_unit"])
	assert.IsType(t, float64(0), callModel["unit_price"])
	assert.NotContains(t, callModel, "input_price")
}

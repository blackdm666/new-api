package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetProviderPricingPublicDeepSeekGroups(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldRedis := common.RedisEnabled
	db := setupModelListControllerTestDB(t)
	oldGroups := setting.UserUsableGroups2JSONString()
	oldRatios := ratio_setting.GroupRatio2JSONString()
	oldExchange := operation_setting.USDExchangeRate
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.RedisEnabled = oldRedis
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		operation_setting.USDExchangeRate = oldExchange
		model.InvalidatePricingCache()
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"Deepseek官转":"official","Deepseek开源":"open source","auto":"automatic"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"Deepseek官转":0.7,"Deepseek开源":0.5,"内部自用":1,"auto":1}`))
	operation_setting.USDExchangeRate = 1
	withTieredBillingConfig(t,
		map[string]string{"deepseek-v4-flash": billing_setting.BillingModeTieredExpr, "deepseek-v4-pro": billing_setting.BillingModeTieredExpr},
		map[string]string{
			"deepseek-v4-flash": `hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 18 ? tier("高峰", p * 3 + c * 9 + cr * 0.1) : tier("空闲", p * 1.5 + c * 4.5 + cr * 0.05)`,
			"deepseek-v4-pro":   `tier("空闲", p * 4.5 + c * 13.5 + cr * 0.15)`,
		})
	require.NoError(t, db.Create(&[]model.Ability{
		{Model: "deepseek-v4-flash", Group: "Deepseek官转", ChannelId: 1, Enabled: true},
		{Model: "deepseek-v4-flash", Group: "Deepseek开源", ChannelId: 1, Enabled: true},
		{Model: "deepseek-v4-flash", Group: "内部自用", ChannelId: 1, Enabled: true},
		{Model: "deepseek-v4-pro", Group: "all", ChannelId: 1, Enabled: true},
		{Model: "private-model", Group: "内部自用", ChannelId: 1, Enabled: true},
	}).Error)
	model.InvalidatePricingCache()

	for _, tc := range []struct {
		name, secret string
		headers      bool
	}{
		{"anonymous", "", false},
		{"legacy secret does not require signature", "obsolete-secret", false},
		{"test page signature ignored", "obsolete-secret", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HVOY_PROVIDER_PRICING_AUTH_SECRET", tc.secret)
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			ctx.Request = httptest.NewRequest("GET", "/api/provider/pricing", nil)
			if tc.headers {
				ctx.Request.Header.Set("X-Hvoy-Ts", "1747886400")
				ctx.Request.Header.Set("X-Hvoy-Sign", "obsolete-signature")
			}
			GetProviderPricing(ctx)
			require.Equal(t, 200, response.Code)
			var payload providerPricingResponse
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
			require.True(t, payload.Success)
			require.NotNil(t, payload.Data)
			require.Len(t, payload.Data.Models, 4)
			assert.Equal(t, "CNY", payload.Data.Currency)
			assert.Equal(t, "per_1m_tokens", payload.Data.PriceUnit)
			expected := [][3]float64{{1.05, 3.15, 0.035}, {0.75, 2.25, 0.025}, {3.15, 9.45, 0.105}, {2.25, 6.75, 0.075}}
			for i, entry := range payload.Data.Models {
				assert.NotEqual(t, "内部自用", entry.GroupName)
				assert.NotEqual(t, "auto", entry.GroupName)
				require.NotNil(t, entry.InputPrice)
				require.NotNil(t, entry.OutputPrice)
				require.NotNil(t, entry.CacheInputPrice)
				assert.InDelta(t, expected[i][0], *entry.InputPrice, 1e-9)
				assert.InDelta(t, expected[i][1], *entry.OutputPrice, 1e-9)
				assert.InDelta(t, expected[i][2], *entry.CacheInputPrice, 1e-9)
			}
		})
	}
	assert.Contains(t, model.GetModelEnableGroups("deepseek-v4-flash"), "内部自用", "public feed must not mutate the shared cache")

	t.Run("extra groups publish an account-only group", func(t *testing.T) {
		t.Setenv("HVOY_PROVIDER_PRICING_EXTRA_GROUPS", " 内部自用 , auto, missing")
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest("GET", "/api/provider/pricing", nil)
		GetProviderPricing(ctx)
		require.Equal(t, 200, response.Code)
		var payload providerPricingResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
		require.NotNil(t, payload.Data)

		var internal *providerPricingModel
		for i, entry := range payload.Data.Models {
			assert.NotEqual(t, "auto", entry.GroupName)
			if entry.ModelName == "deepseek-v4-flash" && entry.GroupName == "内部自用" {
				internal = &payload.Data.Models[i]
			}
		}
		require.NotNil(t, internal)
		assert.InDelta(t, 1.5, *internal.InputPrice, 1e-9)
		assert.InDelta(t, 4.5, *internal.OutputPrice, 1e-9)
		assert.InDelta(t, 0.05, *internal.CacheInputPrice, 1e-9)
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

	require.Len(t, models, 6)
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

	assert.Equal(t, "video-per-second", models[5].ModelName)
	assert.Equal(t, providerPricingCallUnit, models[5].PriceUnit)
	assert.InDelta(t, 6.3, *models[5].UnitPrice, 1e-9)
	assert.Equal(t, providerPricingPerSecondNote, models[5].Note)
}

func TestBuildProviderPricingModelsPublishesUsageExpressionBasePrice(t *testing.T) {
	resolution := jsplugin.UsageFieldSchema{Enum: []string{"480p", "720p", "1080p"}}
	seconds := jsplugin.UsageFieldSchema{Type: "number", Unit: "second"}
	tests := []struct {
		name      string
		pricing   model.Pricing
		priceUnit string
		price     float64
		note      string
	}{
		{
			name:      "fixed fee per image",
			pricing:   model.Pricing{BillingExpr: `tier("image", fixed(0.2)) * image_count`},
			priceUnit: providerPricingCallUnit,
			price:     0.1,
			note:      "按张计费：unit_price 为单张价格",
		},
		{
			name:      "fixed fee per request",
			pricing:   model.Pricing{BillingExpr: `tier("request", fixed(0.6))`},
			priceUnit: providerPricingCallUnit,
			price:     0.3,
			note:      "按次计费",
		},
		{
			name: "per second at the first resolution",
			pricing: model.Pricing{
				BillingExpr:        `u("resolution") == "480p" ? tier("480p", u("seconds") * 0.075) : u("resolution") == "720p" ? tier("720p", u("seconds") * 0.15) : tier("1080p", u("seconds") * 0.2)`,
				BillingUsageSchema: map[string]jsplugin.UsageFieldSchema{"resolution": resolution, "seconds": seconds},
			},
			priceUnit: providerPricingCallUnit,
			price:     0.0375,
			note:      "按秒计费：unit_price 为每秒价格；展示基础档：480p",
		},
		{
			name: "upstream video tokens per million",
			pricing: model.Pricing{
				BillingExpr: `u("upstreamUnits") != nil ? (u("video_input") == "present" ? tier("with_video", u("upstreamUnits") * 42 / 1000000) : tier("without_video", u("upstreamUnits") * 70 / 1000000)) : tier("estimate", 0)`,
				BillingUsageSchema: map[string]jsplugin.UsageFieldSchema{
					"video_input":   {Enum: []string{"none", "present"}},
					"upstreamUnits": {Type: "number", Unit: "token"},
				},
			},
			// Token rows inherit the feed-level per_1m_tokens unit.
			priceUnit: "",
			price:     35,
			note:      "按上游视频 Token 计费；展示基础档：without_video",
		},
		{
			name: "counted images at the first quality",
			pricing: model.Pricing{
				BillingExpr: `u("quality") == "2K" ? tier("2K", u("image_count") * 0.6) : tier("1K", u("image_count") * 0.3)`,
				BillingUsageSchema: map[string]jsplugin.UsageFieldSchema{
					"quality":     {Enum: []string{"1K", "2K"}},
					"image_count": {Type: "number", Unit: "count"},
				},
			},
			priceUnit: providerPricingCallUnit,
			price:     0.15,
			note:      "按张计费：unit_price 为单张价格；展示基础档：1K",
		},
		{
			name: "tiered expression wins over a legacy fixed model price",
			pricing: model.Pricing{
				QuotaType:          1,
				ModelPrice:         9,
				BillingExpr:        `tier("720p", u("seconds") * 0.2)`,
				BillingUsageSchema: map[string]jsplugin.UsageFieldSchema{"seconds": seconds},
			},
			priceUnit: providerPricingCallUnit,
			price:     0.1,
			note:      "按秒计费：unit_price 为每秒价格；展示基础档：720p",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.pricing.ModelName = "usage-model"
			tt.pricing.BillingMode = billing_setting.BillingModeTieredExpr
			tt.pricing.EnableGroup = []string{"standard"}
			models := buildProviderPricingModels([]model.Pricing{tt.pricing}, map[string]float64{"standard": 0.5}, 1)

			require.Len(t, models, 1)
			assert.Equal(t, tt.priceUnit, models[0].PriceUnit)
			assert.Equal(t, tt.note, models[0].Note)
			if tt.priceUnit == "" {
				assert.Nil(t, models[0].UnitPrice)
				assert.InDelta(t, tt.price, *models[0].InputPrice, 1e-9)
				assert.InDelta(t, tt.price, *models[0].OutputPrice, 1e-9)
				return
			}
			assert.InDelta(t, tt.price, *models[0].UnitPrice, 1e-9)
		})
	}
}

func TestBuildProviderPricingModelsOmitsUnpriceableUsageExpressions(t *testing.T) {
	seconds := jsplugin.UsageFieldSchema{Type: "number", Unit: "second"}
	for name, pricing := range map[string]model.Pricing{
		"selector missing from usage schema": {
			BillingExpr:        `u("resolution") == "480p" ? tier("480p", u("seconds") * 0.1) : tier("1080p", u("seconds") * 0.3)`,
			BillingUsageSchema: map[string]jsplugin.UsageFieldSchema{"seconds": seconds},
		},
		"token and usage dimensions mixed": {
			BillingExpr:        `tier("base", p * 2 + u("seconds") * 0.1)`,
			BillingUsageSchema: map[string]jsplugin.UsageFieldSchema{"seconds": seconds},
		},
		"constant without fixed fee or usage": {BillingExpr: `tier("base", 0.5)`},
	} {
		t.Run(name, func(t *testing.T) {
			pricing.ModelName = "usage-model"
			pricing.BillingMode = billing_setting.BillingModeTieredExpr
			pricing.EnableGroup = []string{"standard"}
			assert.Empty(t, buildProviderPricingModels([]model.Pricing{pricing}, map[string]float64{"standard": 1}, 1))
		})
	}
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

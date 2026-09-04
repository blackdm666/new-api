package controller

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			ModelRatio:  1,
			BillingMode: billing_setting.BillingModeTieredExpr,
			EnableGroup: []string{"standard"},
		},
	}

	models := buildProviderPricingModels(pricing, map[string]float64{
		"standard": 0.5,
		"premium":  1.5,
		"auto":     1,
	}, 7)

	require.Len(t, models, 3)
	assert.Equal(t, "claude-test", models[0].ModelName)
	assert.Equal(t, "premium", models[0].GroupName)
	assert.InDelta(t, 42, *models[0].InputPrice, 1e-9)
	assert.InDelta(t, 126, *models[0].OutputPrice, 1e-9)
	assert.InDelta(t, 4.2, *models[0].CacheInputPrice, 1e-9)
	assert.InDelta(t, 52.5, *models[0].CacheCreatePrice, 1e-9)
	assert.InDelta(t, 84, *models[0].CacheCreatePrice1h, 1e-9)

	assert.Equal(t, "standard", models[1].GroupName)
	assert.InDelta(t, 14, *models[1].InputPrice, 1e-9)

	assert.Equal(t, "image-test", models[2].ModelName)
	assert.Equal(t, providerPricingCallUnit, models[2].PriceUnit)
	assert.InDelta(t, 0.7, *models[2].UnitPrice, 1e-9)
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

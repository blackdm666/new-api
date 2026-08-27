package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateModelPriceRejectsInvalidValuesWithoutReplacingCurrentPrices(t *testing.T) {
	savedPrices := ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateModelPriceByJSONString(savedPrices))
	})

	require.NoError(t, UpdateModelPriceByJSONString(`{"secure-model":1.25,"free-model":0}`))

	for _, invalid := range []string{
		`{"secure-model":null}`,
		`{"secure-model":"1.25"}`,
		`{"secure-model":-1}`,
		`null`,
	} {
		t.Run(invalid, func(t *testing.T) {
			assert.Error(t, UpdateModelPriceByJSONString(invalid))
			price, ok := GetModelPrice("secure-model", false)
			assert.True(t, ok)
			assert.Equal(t, 1.25, price)
			freePrice, freeOK := GetModelPrice("free-model", false)
			assert.True(t, freeOK)
			assert.Zero(t, freePrice)
		})
	}
}

func TestValidateNumericPricingMapsAllowsZeroAndExponentButRejectsInvalidTypes(t *testing.T) {
	assert.NoError(t, ValidateNumericPricingMapJSONString("ModelRatio", `{"free":0,"small":1e-6}`))
	assert.Error(t, ValidateNumericPricingMapJSONString("ModelRatio", `{"bad":null}`))
	assert.Error(t, ValidateNumericPricingMapJSONString("ModelRatio", `{"bad":"1"}`))
	assert.Error(t, ValidateNumericPricingMapJSONString("ModelRatio", `{"bad":-0.1}`))
}

func TestUpdateModelRatioRejectsNullWithoutReplacingCurrentRatios(t *testing.T) {
	savedRatios := ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateModelRatioByJSONString(savedRatios))
	})

	require.NoError(t, UpdateModelRatioByJSONString(`{"secure-model":2}`))
	assert.Error(t, UpdateModelRatioByJSONString(`{"secure-model":null}`))
	ratio, ok, _ := GetModelRatio("secure-model")
	assert.True(t, ok)
	assert.Equal(t, 2.0, ratio)
}

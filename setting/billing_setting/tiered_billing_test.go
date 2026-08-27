package billing_setting

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withBillingModes(t *testing.T, modes string, patches []string) {
	t.Helper()

	savedModes := GetBillingModeCopy()
	savedPatches := append([]string(nil), constant.TaskPricePatches...)
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"billing_setting.billing_mode": mustJSON(t, savedModes),
		}))
		constant.TaskPricePatches = savedPatches
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": modes,
	}))
	constant.TaskPricePatches = append([]string(nil), patches...)
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}

func TestTaskBillingUnitResolution(t *testing.T) {
	withBillingModes(t, `{
		"explicit-request":"per_request",
		"explicit-second":"per_second",
		"tiered":"tiered_expr",
		"malformed":"per_minute"
	}`, []string{"explicit-second", "legacy-patch"})

	assert.True(t, IsTaskPerRequestBilling("explicit-request"))
	assert.False(t, IsTaskPerRequestBilling("explicit-second"))
	assert.True(t, IsTaskPerRequestBilling("legacy-patch"))
	assert.False(t, IsTaskPerRequestBilling("legacy-fixed"))
	assert.False(t, IsTaskPerRequestBilling("malformed"))
	assert.False(t, IsTaskPerRequestBilling("legacy-ratio"))
	assert.False(t, IsTaskPerRequestBilling("tiered"))
	assert.True(t, ShouldSkipTaskCompletionAdjustment("legacy-fixed", true))
	assert.False(t, ShouldSkipTaskCompletionAdjustment("legacy-ratio", false))
	assert.False(t, ShouldSkipTaskCompletionAdjustment("explicit-second", true))

	assert.Equal(t, BillingUnitRequest, ResolveTaskBillingUnit("explicit-request"))
	assert.Equal(t, BillingUnitSecond, ResolveTaskBillingUnit("explicit-second"))
	assert.Equal(t, BillingUnitRequest, ResolveTaskBillingUnit("legacy-patch"))
	assert.Empty(t, ResolveTaskBillingUnit("legacy-fixed"))
	assert.Empty(t, ResolveTaskBillingUnit("malformed"))
	assert.Empty(t, ResolveTaskBillingUnit("legacy-ratio"))
	assert.Empty(t, ResolveTaskBillingUnit("tiered"))
}

func TestValidateBillingModesJSONRejectsUnknownAndNullValues(t *testing.T) {
	assert.NoError(t, ValidateBillingModesJSON(`{"request":"per_request","second":"per_second","ratio":"ratio","tiered":"tiered_expr"}`))
	assert.Error(t, ValidateBillingModesJSON(`{"bad":"per_minute"}`))
	assert.Error(t, ValidateBillingModesJSON(`{"bad":null}`))
	assert.Error(t, ValidateBillingModesJSON(`null`))
}

func TestBillingModeConfigDropsInvalidEntriesButKeepsValidOnes(t *testing.T) {
	withBillingModes(t, `{
		"valid":"per_second",
		"unknown":"per_minute",
		"null-mode":null
	}`, nil)

	assert.Equal(t, BillingModePerSecond, GetBillingMode("valid"))
	assert.Equal(t, BillingModeRatio, GetBillingMode("unknown"))
	assert.Equal(t, BillingModeRatio, GetBillingMode("null-mode"))
}

func TestFixedPriceCatalogUnit(t *testing.T) {
	withBillingModes(t, `{
		"explicit-request":"per_request",
		"explicit-second":"per_second",
		"tiered":"tiered_expr"
	}`, nil)

	assert.Equal(t, BillingUnitRequest, ResolveFixedPriceBillingUnit("explicit-request"))
	assert.Equal(t, BillingUnitSecond, ResolveFixedPriceBillingUnit("explicit-second"))
	assert.Equal(t, BillingUnitRequest, ResolveFixedPriceBillingUnit("legacy-fixed"))
	assert.Empty(t, ResolveFixedPriceBillingUnit("tiered"))
}

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
		"tiered":"tiered_expr"
	}`, []string{"explicit-second", "legacy-patch"})

	assert.True(t, IsTaskPerRequestBilling("explicit-request"))
	assert.False(t, IsTaskPerRequestBilling("explicit-second"))
	assert.True(t, IsTaskPerRequestBilling("legacy-patch"))
	assert.False(t, IsTaskPerRequestBilling("tiered"))

	assert.Equal(t, BillingUnitRequest, ResolveTaskBillingUnit("explicit-request", true))
	assert.Equal(t, BillingUnitSecond, ResolveTaskBillingUnit("explicit-second", true))
	assert.Equal(t, BillingUnitRequest, ResolveTaskBillingUnit("legacy-patch", false))
	assert.Equal(t, BillingUnitRequest, ResolveTaskBillingUnit("legacy-fixed", true))
	assert.Empty(t, ResolveTaskBillingUnit("legacy-ratio", false))
	assert.Empty(t, ResolveTaskBillingUnit("tiered", true))
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

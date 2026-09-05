package billing_setting

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
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

func TestSmokeTestTaskExprValidatesDeclaredUsageVectors(t *testing.T) {
	videoSchema := map[string]jsplugin.UsageFieldSchema{
		"seconds": {Type: "number", Unit: "second"},
		"mode":    {Enum: []string{"std", "pro"}},
		"quality": {Enum: []string{"sd", "hd"}},
	}

	tests := []struct {
		name          string
		schema        map[string]jsplugin.UsageFieldSchema
		expression    string
		expectedError string
	}{
		{
			name:       "declared numeric and enum facts",
			schema:     videoSchema,
			expression: `u("mode") == "pro" ? tier("pro", u("seconds") * 0.8) : tier("std", u("seconds") * 0.4)`,
		},
		{
			name:          "undeclared literal key",
			schema:        videoSchema,
			expression:    `tier("base", u("clips") * 0.1)`,
			expectedError: `usage key "clips" is not declared`,
		},
		{
			name:          "negative duration boundary",
			schema:        videoSchema,
			expression:    fmt.Sprintf(`u("seconds") == %d ? -1 : 0`, relaycommon.MaxTaskDurationSeconds),
			expectedError: "result must be finite and non-negative",
		},
		{
			name:          "negative count boundary",
			schema:        map[string]jsplugin.UsageFieldSchema{"clips": {Type: "number", Unit: "count"}},
			expression:    fmt.Sprintf(`u("clips") == %d ? -1 : 0`, dto.MaxImageN),
			expectedError: "result must be finite and non-negative",
		},
		{
			name:          "negative token boundary",
			schema:        map[string]jsplugin.UsageFieldSchema{"tokens": {Type: "number", Unit: "token"}},
			expression:    fmt.Sprintf(`u("tokens") == %d ? -1 : 0`, common.MaxQuota),
			expectedError: "result must be finite and non-negative",
		},
		{
			name:          "negative credit boundary",
			schema:        map[string]jsplugin.UsageFieldSchema{"units": {Type: "number", Unit: "credit"}},
			expression:    fmt.Sprintf(`u("units") == %d ? -1 : 0`, common.MaxQuota),
			expectedError: "result must be finite and non-negative",
		},
		{
			name:          "negative enum combination",
			schema:        videoSchema,
			expression:    `u("mode") == "pro" && u("quality") == "hd" ? -1 : 0`,
			expectedError: "result must be finite and non-negative",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := SmokeTestTaskExpr(testCase.expression, testCase.schema)
			if testCase.expectedError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, testCase.expectedError)
		})
	}
}

func TestSmokeTestTaskExprCapsOversizedEnumProductsAtLastCombination(t *testing.T) {
	schema := make(map[string]jsplugin.UsageFieldSchema, 7)
	condition := ""
	for index := 0; index < 7; index++ {
		schema[fmt.Sprintf("enum_%d", index)] = jsplugin.UsageFieldSchema{Enum: []string{"first", "middle", "last"}}
		if condition != "" {
			condition += " && "
		}
		condition += fmt.Sprintf(`u("enum_%d") == "last"`, index)
	}

	err := SmokeTestTaskExpr(condition+" ? -1 : 0", schema)
	require.ErrorContains(t, err, "result must be finite and non-negative")
}

func TestSmokeTestExprRejectsTaskUsageWithoutSchema(t *testing.T) {
	err := SmokeTestExpr(`u("mode") == "std" ? 1 : 2`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "mode")
	assert.ErrorContains(t, err, "no task plugin usage schema")

	require.NoError(t, SmokeTestExpr(`tier("base", p * 2 + c * 8)`))
}

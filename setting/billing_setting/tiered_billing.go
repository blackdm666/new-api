package billing_setting

import (
	"fmt"
	"math"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/samber/lo"
)

const (
	BillingModeRatio      = "ratio"
	BillingModePerRequest = "per_request"
	BillingModePerSecond  = "per_second"
	BillingModeTieredExpr = "tiered_expr"
	BillingUnitRequest    = "request"
	BillingUnitSecond     = "second"
	BillingModeField      = "billing_mode"
	BillingExprField      = "billing_expr"
	maxTaskExprSmokeTests = 64
)

// BillingSetting is managed by config.GlobalConfig.Register.
// DB keys: billing_setting.billing_mode, billing_setting.billing_expr
type BillingSetting struct {
	BillingMode BillingModeMap    `json:"billing_mode"`
	BillingExpr map[string]string `json:"billing_expr"`
}

type BillingModeMap map[string]string

func (m *BillingModeMap) UnmarshalJSON(data []byte) error {
	var rawModes map[string]*string
	if err := common.Unmarshal(data, &rawModes); err != nil {
		return err
	}
	if rawModes == nil {
		return fmt.Errorf("billing modes must be a JSON object")
	}
	validModes := make(BillingModeMap, len(rawModes))
	for model, mode := range rawModes {
		if mode == nil {
			common.SysError(fmt.Sprintf("ignored null billing mode for model %s", model))
			continue
		}
		switch *mode {
		case BillingModeRatio, BillingModePerRequest, BillingModePerSecond, BillingModeTieredExpr:
			validModes[model] = *mode
		default:
			common.SysError(fmt.Sprintf("ignored unsupported billing mode %q for model %s", *mode, model))
		}
	}
	*m = validModes
	return nil
}

var billingSetting = BillingSetting{
	BillingMode: make(BillingModeMap),
	BillingExpr: make(map[string]string),
}

func init() {
	config.GlobalConfig.Register("billing_setting", &billingSetting)
}

// ---------------------------------------------------------------------------
// Read accessors (hot path, must be fast)
// ---------------------------------------------------------------------------

func GetBillingMode(model string) string {
	if mode, ok := billingSetting.BillingMode[model]; ok {
		return mode
	}
	return BillingModeRatio
}

func GetBillingExpr(model string) (string, bool) {
	expr, ok := billingSetting.BillingExpr[model]
	return expr, ok
}

func GetBillingModeCopy() map[string]string {
	modes := make(map[string]string, len(billingSetting.BillingMode))
	for model, mode := range billingSetting.BillingMode {
		modes[model] = mode
	}
	return modes
}

func GetBillingExprCopy() map[string]string {
	return lo.Assign(billingSetting.BillingExpr)
}

func ValidateBillingModesJSON(jsonStr string) error {
	var modes map[string]*string
	if err := common.UnmarshalJsonStr(jsonStr, &modes); err != nil {
		return fmt.Errorf("invalid billing modes: %w", err)
	}
	if modes == nil {
		return fmt.Errorf("billing modes must be a JSON object")
	}
	for model, mode := range modes {
		if mode == nil {
			return fmt.Errorf("billing mode for model %s must be a string", model)
		}
		switch *mode {
		case BillingModeRatio, BillingModePerRequest, BillingModePerSecond, BillingModeTieredExpr:
		default:
			return fmt.Errorf("unsupported billing mode %q for model %s", *mode, model)
		}
	}
	return nil
}

func modelUsesTaskPricePatch(model string) bool {
	for _, patchedModel := range constant.TaskPricePatches {
		if patchedModel == model {
			return true
		}
	}
	return false
}

// ResolveFixedPriceBillingUnit returns the catalog unit for a ModelPrice.
// Legacy fixed prices remain per-request until an administrator explicitly
// chooses per-second pricing.
func ResolveFixedPriceBillingUnit(model string) string {
	switch GetBillingMode(model) {
	case BillingModePerSecond:
		return BillingUnitSecond
	case BillingModeTieredExpr:
		return ""
	default:
		return BillingUnitRequest
	}
}

// IsTaskPerRequestBilling controls whether task adapters' duration,
// resolution, and other multipliers are skipped. Explicit settings take
// precedence over the legacy TASK_PRICE_PATCH environment variable.
func IsTaskPerRequestBilling(model string) bool {
	return ResolveTaskBillingUnit(model) == BillingUnitRequest
}

// ShouldSkipTaskCompletionAdjustment preserves the historical rule that a
// legacy fixed-price task is not recalculated from an upstream total_tokens
// value, even when its submit-time adaptor ratios remain enabled.
func ShouldSkipTaskCompletionAdjustment(model string, legacyFixedPrice bool) bool {
	switch GetBillingMode(model) {
	case BillingModePerRequest:
		return true
	case BillingModePerSecond, BillingModeTieredExpr:
		return false
	default:
		return modelUsesTaskPricePatch(model) || legacyFixedPrice
	}
}

// ResolveTaskBillingUnit returns the unit stored in task logs and snapshots.
// Legacy fixed-price tasks that are not covered by TASK_PRICE_PATCH keep their
// historical adaptor-ratio arithmetic and therefore have no explicit unit.
func ResolveTaskBillingUnit(model string) string {
	switch GetBillingMode(model) {
	case BillingModePerRequest:
		return BillingUnitRequest
	case BillingModePerSecond:
		return BillingUnitSecond
	case BillingModeTieredExpr:
		return ""
	default:
		if modelUsesTaskPricePatch(model) {
			return BillingUnitRequest
		}
		return ""
	}
}

func GetPricingSyncData(base map[string]any) map[string]any {
	extra := make(map[string]any, 2)
	if modes := GetBillingModeCopy(); len(modes) > 0 {
		extra[BillingModeField] = modes
	}
	if exprs := GetBillingExprCopy(); len(exprs) > 0 {
		extra[BillingExprField] = exprs
	}
	return lo.Assign(base, extra)
}

// ---------------------------------------------------------------------------
// Smoke test (called externally for validation before save)
// ---------------------------------------------------------------------------

func SmokeTestExpr(exprStr string) error {
	return smokeTestExpr(exprStr)
}

func smokeTestExpr(exprStr string) error {
	if _, err := billingexpr.CompileFromCache(exprStr); err != nil {
		return err
	}
	usageKeys := billingexpr.UsedUsageKeys(exprStr)
	if len(usageKeys) > 0 {
		sortedKeys := make([]string, 0, len(usageKeys))
		for key := range usageKeys {
			sortedKeys = append(sortedKeys, key)
		}
		sort.Strings(sortedKeys)
		return fmt.Errorf("expression references usage keys %v but the model has no task plugin usage schema", sortedKeys)
	}

	vectors := []billingexpr.TokenParams{
		{P: 0, C: 0, Len: 0},
		{P: 1000, C: 1000, Len: 1000},
		{P: 100000, C: 100000, Len: 100000},
		{P: 1000000, C: 1000000, Len: 1000000},
	}

	for _, v := range vectors {
		for _, request := range billingExprSmokeRequests() {
			result, _, err := billingexpr.RunExprWithRequest(exprStr, v, request)
			if err != nil {
				return fmt.Errorf("vector {p=%g, c=%g}: run failed: %w", v.P, v.C, err)
			}
			if math.IsNaN(result) || math.IsInf(result, 0) || result < 0 {
				return fmt.Errorf("vector {p=%g, c=%g}: result must be finite and non-negative, got %f", v.P, v.C, result)
			}
		}
	}
	return nil
}

// SmokeTestTaskExpr validates a task usage expression against the usage facts
// declared by its plugin. Literal u() keys must be declared; dynamic calls are
// still exercised by the generated runtime vectors when possible.
func SmokeTestTaskExpr(exprStr string, schema map[string]jsplugin.UsageFieldSchema) error {
	if _, err := billingexpr.CompileFromCache(exprStr); err != nil {
		return err
	}
	for key := range billingexpr.UsedUsageKeys(exprStr) {
		if _, declared := schema[key]; !declared {
			return fmt.Errorf("usage key %q is not declared by the task plugin", key)
		}
	}

	for _, usage := range taskUsageSmokeVectors(schema) {
		for _, request := range billingExprSmokeRequests() {
			request.Usage = usage
			result, _, err := billingexpr.RunExprWithRequest(exprStr, billingexpr.TokenParams{}, request)
			if err != nil {
				return fmt.Errorf("usage vector %v: run failed: %w", usage, err)
			}
			if math.IsNaN(result) || math.IsInf(result, 0) || result < 0 {
				return fmt.Errorf("usage vector %v: result must be finite and non-negative, got %f", usage, result)
			}
		}
	}
	return nil
}

type usageSmokeDimension struct {
	name   string
	values []any
}

func taskUsageSmokeVectors(schema map[string]jsplugin.UsageFieldSchema) []map[string]any {
	names := make([]string, 0, len(schema))
	for name := range schema {
		names = append(names, name)
	}
	sort.Strings(names)

	dimensions := make([]usageSmokeDimension, 0, len(names))
	for _, name := range names {
		field := schema[name]
		if len(field.Enum) > 0 {
			values := make([]any, len(field.Enum))
			for index, value := range field.Enum {
				values[index] = value
			}
			dimensions = append(dimensions, usageSmokeDimension{name: name, values: values})
			continue
		}
		if field.Type == "boolean" {
			dimensions = append(dimensions, usageSmokeDimension{name: name, values: []any{false, true}})
			continue
		}
		limit := relaycommon.MaxTaskDurationSeconds
		if field.Unit == "count" {
			limit = dto.MaxImageN
		}
		if field.Unit == "token" || field.Unit == "credit" {
			limit = common.MaxQuota
		}
		dimensions = append(dimensions, usageSmokeDimension{
			name:   name,
			values: []any{float64(0), float64(1), float64(limit)},
		})
	}

	if usageSmokeCombinationCount(dimensions, maxTaskExprSmokeTests) > maxTaskExprSmokeTests {
		for index := range dimensions {
			field := schema[dimensions[index].name]
			if len(field.Enum) <= 2 {
				continue
			}
			dimensions[index].values = []any{field.Enum[0], field.Enum[len(field.Enum)-1]}
		}
	}

	vectors := make([]map[string]any, 0, maxTaskExprSmokeTests)
	var appendVectors func(int, map[string]any)
	appendVectors = func(index int, current map[string]any) {
		if len(vectors) >= maxTaskExprSmokeTests {
			return
		}
		if index == len(dimensions) {
			vector := make(map[string]any, len(current))
			for key, value := range current {
				vector[key] = value
			}
			vectors = append(vectors, vector)
			return
		}
		for _, value := range dimensions[index].values {
			current[dimensions[index].name] = value
			appendVectors(index+1, current)
		}
		delete(current, dimensions[index].name)
	}
	appendVectors(0, make(map[string]any, len(dimensions)))

	combinationCount := usageSmokeCombinationCount(dimensions, maxTaskExprSmokeTests)
	if combinationCount > maxTaskExprSmokeTests && len(vectors) > 0 {
		last := make(map[string]any, len(dimensions))
		for _, dimension := range dimensions {
			last[dimension.name] = dimension.values[len(dimension.values)-1]
		}
		vectors[len(vectors)-1] = last
	}
	return vectors
}

func usageSmokeCombinationCount(dimensions []usageSmokeDimension, stopAfter int) int {
	count := 1
	for _, dimension := range dimensions {
		if len(dimension.values) == 0 {
			return 0
		}
		if count > stopAfter/len(dimension.values) {
			return stopAfter + 1
		}
		count *= len(dimension.values)
	}
	return count
}

func billingExprSmokeRequests() []billingexpr.RequestInput {
	return []billingexpr.RequestInput{
		{},
		{
			Headers: map[string]string{
				"anthropic-beta": "fast-mode-2026-02-01",
			},
			Body: []byte(`{"service_tier":"fast","stream_options":{"include_usage":true},"messages":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21]}`),
		},
	}
}

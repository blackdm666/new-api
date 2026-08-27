package billing_setting

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
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
	vectors := []billingexpr.TokenParams{
		{P: 0, C: 0, Len: 0},
		{P: 1000, C: 1000, Len: 1000},
		{P: 100000, C: 100000, Len: 100000},
		{P: 1000000, C: 1000000, Len: 1000000},
	}
	requests := []billingexpr.RequestInput{
		{},
		{
			Headers: map[string]string{
				"anthropic-beta": "fast-mode-2026-02-01",
			},
			Body: []byte(`{"service_tier":"fast","stream_options":{"include_usage":true},"messages":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21]}`),
		},
	}

	for _, v := range vectors {
		for _, request := range requests {
			result, _, err := billingexpr.RunExprWithRequest(exprStr, v, request)
			if err != nil {
				return fmt.Errorf("vector {p=%g, c=%g}: run failed: %w", v.P, v.C, err)
			}
			if result < 0 {
				return fmt.Errorf("vector {p=%g, c=%g}: result %f < 0", v.P, v.C, result)
			}
		}
	}
	return nil
}

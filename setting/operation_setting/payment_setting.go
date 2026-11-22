package operation_setting

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type PaymentSetting struct {
	AmountOptions  []int           `json:"amount_options"`
	AmountDiscount map[int]float64 `json:"amount_discount"` // 充值金额档位对应的折扣，例如 100 元 0.9 表示满 100 元享受 9 折优惠
	// NoDiscountGroups lists user groups that always pay the normal price:
	// AmountDiscount is neither applied nor shown to users in these groups.
	NoDiscountGroups []string `json:"no_discount_groups"`

	ComplianceConfirmed    bool   `json:"compliance_confirmed"`
	ComplianceTermsVersion string `json:"compliance_terms_version"`
	ComplianceConfirmedAt  int64  `json:"compliance_confirmed_at"`
	ComplianceConfirmedBy  int    `json:"compliance_confirmed_by"`
	ComplianceConfirmedIP  string `json:"compliance_confirmed_ip"`
}

const CurrentComplianceTermsVersion = "v1"

// 默认配置
var paymentSetting = PaymentSetting{
	AmountOptions:    []int{10, 20, 50, 100, 200, 500},
	AmountDiscount:   map[int]float64{},
	NoDiscountGroups: []string{},
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("payment_setting", &paymentSetting)
}

func GetPaymentSetting() *PaymentSetting {
	return &paymentSetting
}

// GetAmountDiscount returns the discount for the highest configured amount
// threshold that does not exceed amount. A missing or non-positive discount
// falls back to the normal price.
func GetAmountDiscount(amount int64) float64 {
	discount := 1.0
	matchedThreshold := int64(-1)

	for threshold, candidate := range paymentSetting.AmountDiscount {
		threshold64 := int64(threshold)
		if threshold64 > amount || threshold64 <= matchedThreshold {
			continue
		}
		matchedThreshold = threshold64
		if candidate > 0 {
			discount = candidate
		} else {
			discount = 1.0
		}
	}

	return discount
}

// IsAmountDiscountExcludedGroup reports whether users in group must not
// receive the top-up amount discount.
func IsAmountDiscountExcludedGroup(group string) bool {
	group = strings.TrimSpace(group)
	if group == "" {
		return false
	}
	return slices.ContainsFunc(paymentSetting.NoDiscountGroups, func(excluded string) bool {
		return strings.TrimSpace(excluded) == group
	})
}

// GetAmountDiscountForGroup is GetAmountDiscount for a user in group. Groups
// listed in NoDiscountGroups always get the normal price (1.0).
func GetAmountDiscountForGroup(amount int64, group string) float64 {
	if IsAmountDiscountExcludedGroup(group) {
		return 1.0
	}
	return GetAmountDiscount(amount)
}

// GetAmountDiscountMapForGroup returns the discount tiers a user in group
// should see on the top-up page; excluded groups see none.
func GetAmountDiscountMapForGroup(group string) map[int]float64 {
	if IsAmountDiscountExcludedGroup(group) {
		return map[int]float64{}
	}
	return paymentSetting.AmountDiscount
}

func IsPaymentComplianceConfirmed() bool {
	return paymentSetting.ComplianceConfirmed &&
		paymentSetting.ComplianceTermsVersion == CurrentComplianceTermsVersion
}

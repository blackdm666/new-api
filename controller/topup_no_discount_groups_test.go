package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func withNoDiscountGroupsTestSettings(t *testing.T) {
	t.Helper()
	paymentSetting := operation_setting.GetPaymentSetting()
	originalDiscounts := paymentSetting.AmountDiscount
	originalExcluded := paymentSetting.NoDiscountGroups
	originalPrice := operation_setting.Price
	originalStripeUnitPrice := setting.StripeUnitPrice
	originalWaffoUnitPrice := setting.WaffoUnitPrice
	originalPancakeUnitPrice := setting.WaffoPancakeUnitPrice
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalRatios := common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		paymentSetting.AmountDiscount = originalDiscounts
		paymentSetting.NoDiscountGroups = originalExcluded
		operation_setting.Price = originalPrice
		setting.StripeUnitPrice = originalStripeUnitPrice
		setting.WaffoUnitPrice = originalWaffoUnitPrice
		setting.WaffoPancakeUnitPrice = originalPancakeUnitPrice
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalRatios))
	})

	paymentSetting.AmountDiscount = map[int]float64{100: 0.99, 500: 0.98, 1000: 0.95}
	paymentSetting.NoDiscountGroups = []string{"代理商", " 特殊代理商 "}
	operation_setting.Price = 7
	setting.StripeUnitPrice = 7
	setting.WaffoUnitPrice = 1
	setting.WaffoPancakeUnitPrice = 1
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{}`))
}

func TestNoDiscountGroupsPayNormalPriceOnEveryPaymentPath(t *testing.T) {
	withNoDiscountGroupsTestSettings(t)

	for _, group := range []string{"代理商", "特殊代理商"} {
		require.True(t, decimal.RequireFromString("7000").Equal(getPayMoneyDecimal(1000, group)), group)
		require.Equal(t, 7000.0, getPayMoney(1000, group), group)
		require.Equal(t, 7000.0, getStripePayMoney(1000, group), group)
		require.Equal(t, 1000.0, getWaffoPayMoney(1000, group), group)
		require.Equal(t, 1000.0, getWaffoPancakePayMoney(1000, group), group)
	}

	// Other groups keep the tiered discount.
	require.True(t, decimal.RequireFromString("6650").Equal(getPayMoneyDecimal(1000, "default")))
	require.InDelta(t, 6650.0, getStripePayMoney(1000, "default"), 1e-9)
	require.InDelta(t, 950.0, getWaffoPayMoney(1000, "default"), 1e-9)
	require.InDelta(t, 950.0, getWaffoPancakePayMoney(1000, "default"), 1e-9)
	require.True(t, decimal.RequireFromString("3430").Equal(getPayMoneyDecimal(500, "vip")))
}

func TestNoDiscountGroupsStillApplyTopupGroupRatio(t *testing.T) {
	withNoDiscountGroupsTestSettings(t)
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"代理商":1.1}`))

	require.True(t, decimal.RequireFromString("7700").Equal(getPayMoneyDecimal(1000, "代理商")))
}

func TestNoDiscountGroupsHideDiscountTiers(t *testing.T) {
	withNoDiscountGroupsTestSettings(t)

	require.Empty(t, operation_setting.GetAmountDiscountMapForGroup("代理商"))
	require.Empty(t, operation_setting.GetAmountDiscountMapForGroup("特殊代理商"))
	require.Len(t, operation_setting.GetAmountDiscountMapForGroup("default"), 3)
	require.Len(t, operation_setting.GetAmountDiscountMapForGroup(""), 3)
}

func TestNoDiscountGroupsEmptyByDefaultKeepsDiscount(t *testing.T) {
	withNoDiscountGroupsTestSettings(t)
	operation_setting.GetPaymentSetting().NoDiscountGroups = nil

	require.Equal(t, 0.95, operation_setting.GetAmountDiscountForGroup(1000, "代理商"))
	require.False(t, operation_setting.IsAmountDiscountExcludedGroup(""))
}

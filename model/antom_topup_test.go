package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRechargeAntomCreditsQuotaExactlyOnce(t *testing.T) {
	truncateTables(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	user := insertUserForPaymentGuardTest(t, 901, 0)
	order := &TopUp{
		UserId:          user.Id,
		Amount:          10,
		Money:           10,
		MoneyMinor:      1000,
		CreditedQuota:   5_000_000,
		Currency:        "CNY",
		TradeNo:         "ANTOM-ONCE",
		PaymentMethod:   PaymentMethodAntom,
		PaymentProvider: PaymentProviderAntom,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())

	alreadyDone, err := RechargeAntom(order.TradeNo, "ALIPAY_HK", 1000, "CNY", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	assert.Equal(t, 5_000_000, getUserQuotaForPaymentGuardTest(t, user.Id))

	reloaded := GetTopUpByTradeNo(order.TradeNo)
	require.NotNil(t, reloaded)
	assert.Equal(t, common.TopUpStatusSuccess, reloaded.Status)
	assert.Equal(t, "ALIPAY_HK", reloaded.PaymentMethod)
	assert.Equal(t, "CNY", reloaded.Currency)

	alreadyDone, err = RechargeAntom(order.TradeNo, "ALIPAY_HK", 1000, "CNY", "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, alreadyDone)
	assert.Equal(t, 5_000_000, getUserQuotaForPaymentGuardTest(t, user.Id))
}

func TestRechargeAntomLegacyOrderSupportsWalletQuotaAboveInt32(t *testing.T) {
	truncateTables(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	user := insertUserForPaymentGuardTest(t, 902, 0)
	order := &TopUp{
		UserId:          user.Id,
		Amount:          5000,
		Money:           5000,
		MoneyMinor:      500000,
		CreditedQuota:   0,
		Currency:        "USD",
		TradeNo:         "ANTOM-LEGACY-WIDE-WALLET",
		PaymentMethod:   PaymentMethodAntom,
		PaymentProvider: PaymentProviderAntom,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())

	alreadyDone, err := RechargeAntom(order.TradeNo, "ALIPAY_HK", 500000, "USD", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	assert.Equal(t, 2_500_000_000, getUserQuotaForPaymentGuardTest(t, user.Id))
	assert.Equal(t, 2_500_000_000, GetTopUpByTradeNo(order.TradeNo).CreditedQuota)
}

func TestRechargeAntomRejectsAmountCurrencyAndProviderMismatch(t *testing.T) {
	testCases := []struct {
		name          string
		provider      string
		paidMinor     int64
		currency      string
		expectedError error
	}{
		{name: "amount", provider: PaymentProviderAntom, paidMinor: 999, currency: "USD", expectedError: ErrTopUpAmountMismatch},
		{name: "currency", provider: PaymentProviderAntom, paidMinor: 1000, currency: "CNY", expectedError: ErrTopUpCurrencyMismatch},
		{name: "provider", provider: PaymentProviderStripe, paidMinor: 1000, currency: "USD", expectedError: ErrPaymentMethodMismatch},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			user := insertUserForPaymentGuardTest(t, 920+index, 0)
			order := &TopUp{
				UserId:          user.Id,
				Amount:          10,
				Money:           10,
				MoneyMinor:      1000,
				CreditedQuota:   5_000_000,
				Currency:        "USD",
				TradeNo:         "ANTOM-MISMATCH-" + testCase.name,
				PaymentMethod:   PaymentMethodAntom,
				PaymentProvider: testCase.provider,
				CreateTime:      common.GetTimestamp(),
				Status:          common.TopUpStatusPending,
			}
			require.NoError(t, order.Insert())

			_, err := RechargeAntom(order.TradeNo, "ALIPAY_CN", testCase.paidMinor, testCase.currency, "127.0.0.1")
			require.ErrorIs(t, err, testCase.expectedError)
			assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
			assert.Zero(t, getUserQuotaForPaymentGuardTest(t, user.Id))
		})
	}
}

func TestTopUpMoneyCentsUsesLockedAntomMinorAmount(t *testing.T) {
	topUp := &TopUp{
		Money:           10.01,
		MoneyMinor:      1002,
		PaymentProvider: PaymentProviderAntom,
	}
	assert.Equal(t, int64(1002), topUpMoneyCents(topUp))
}

package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripeRechargePersistsActualPaidAmountAndCreditsSnapshot(t *testing.T) {
	truncateTables(t)
	user := &User{
		Username: "stripe-actual-buyer", Email: "stripe-actual-buyer@example.com",
		AffCode: "stripe-actual-buyer-code", Status: common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(user).Error)
	topUp := &TopUp{
		UserId: user.Id, Amount: 100, Money: 90, CreditedQuota: 12345,
		TradeNo: "STRIPE-ACTUAL-PAID", PaymentMethod: PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(topUp).Error)

	require.NoError(t, Recharge(topUp.TradeNo, "cus_actual", 87.34, "USD", "127.0.0.1"))
	require.NoError(t, DB.First(topUp, topUp.Id).Error)
	assert.InDelta(t, 87.34, topUp.Money, 0.000001)
	assert.Equal(t, "USD", topUp.Currency)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 12345, user.Quota)
	assert.Equal(t, "cus_actual", user.StripeCustomer)

	require.NoError(t, Recharge(topUp.TradeNo, "cus_actual", 87.34, "USD", "127.0.0.1"))
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 12345, user.Quota)
}

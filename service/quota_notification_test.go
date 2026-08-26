package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalletQuotaWarningUsesCurrentBalanceInsteadOfRequestSnapshot(t *testing.T) {
	truncate(t)
	seedUser(t, 1547, 35_244)

	claimed, version, err := model.ClaimQuotaWarning(1547, model.QuotaNotificationSourceWallet, 0, 250_000, 35_244)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, int64(1), version)

	relayInfo := &relaycommon.RelayInfo{UserId: 1547, UserQuota: 500_000}
	claimed, _, remaining, err := claimQuotaWarningForCurrentBalance(relayInfo, model.QuotaNotificationSourceWallet, 0, 250_000)
	require.NoError(t, err)
	assert.False(t, claimed)
	assert.Equal(t, int64(35_244), remaining)

	state := &model.QuotaNotificationState{}
	require.NoError(t, model.DB.First(state, "user_id = ? AND source = ? AND source_id = ?", 1547, model.QuotaNotificationSourceWallet, 0).Error)
	assert.True(t, state.BelowThreshold)
	assert.Equal(t, int64(1), state.Version)
}

func TestSubscriptionQuotaWarningUsesCurrentSettledBalance(t *testing.T) {
	truncate(t)
	seedUser(t, 1548, 1_000_000)
	seedSubscription(t, 88, 1548, 500_000, 464_756)

	claimed, version, err := model.ClaimQuotaWarning(1548, model.QuotaNotificationSourceSubscription, 88, 250_000, 35_244)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, int64(1), version)

	relayInfo := &relaycommon.RelayInfo{
		UserId:                                1548,
		SubscriptionId:                        88,
		SubscriptionAmountTotal:               500_000,
		SubscriptionAmountUsedAfterPreConsume: 0,
		SubscriptionPostDelta:                 0,
	}
	claimed, _, remaining, err := claimQuotaWarningForCurrentBalance(relayInfo, model.QuotaNotificationSourceSubscription, 88, 250_000)
	require.NoError(t, err)
	assert.False(t, claimed)
	assert.Equal(t, int64(35_244), remaining)

	state := &model.QuotaNotificationState{}
	require.NoError(t, model.DB.First(state, "user_id = ? AND source = ? AND source_id = ?", 1548, model.QuotaNotificationSourceSubscription, 88).Error)
	assert.True(t, state.BelowThreshold)
	assert.Equal(t, int64(1), state.Version)

	assert.NotZero(t, state.NotifiedTime)
}

package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAffiliateTransferIsAtomicIdempotentAndAuditable(t *testing.T) {
	user := setupAffiliatePayoutTest(t, 120)
	user.Quota = 3 * int(common.QuotaPerUnit)
	require.NoError(t, DB.Model(user).Update("quota", user.Quota).Error)
	amountCents := int64(200)
	amountQuota := 2 * int(common.QuotaPerUnit)

	record, err := user.TransferAffiliateCentsToQuotaWithRequestId(amountCents, "transfer-request-1")
	require.NoError(t, err)
	assert.Equal(t, user.Id, record.UserId)
	assert.Equal(t, amountCents, record.AmountCents)
	assert.Equal(t, amountQuota, record.AmountQuota)
	assert.Equal(t, int64(12_000), record.BalanceCentsBefore)
	assert.Equal(t, int64(11_800), record.BalanceCentsAfter)
	assert.Equal(t, 3*int(common.QuotaPerUnit), record.QuotaBefore)
	assert.Equal(t, 5*int(common.QuotaPerUnit), record.QuotaAfter)

	duplicate, err := user.TransferAffiliateCentsToQuotaWithRequestId(amountCents, "transfer-request-1")
	require.NoError(t, err)
	assert.Equal(t, record.Id, duplicate.Id)

	var count int64
	require.NoError(t, DB.Model(&AffiliateTransfer{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	fresh, err := GetUserById(user.Id, false)
	require.NoError(t, err)
	account, err := GetAffiliateAccount(user.Id)
	require.NoError(t, err)
	assert.Equal(t, record.BalanceCentsAfter, account.AvailableCents)
	assert.Zero(t, fresh.AffQuota)
	assert.Equal(t, record.QuotaAfter, fresh.Quota)
}

func TestAffiliateTransferRejectsConflictAndPreservesBalance(t *testing.T) {
	user := setupAffiliatePayoutTest(t, 2)
	amountCents := int64(100)
	amountQuota := int(common.QuotaPerUnit)
	_, err := user.TransferAffiliateCentsToQuotaWithRequestId(amountCents, "transfer-conflict")
	require.NoError(t, err)

	_, err = user.TransferAffiliateCentsToQuotaWithRequestId(2*amountCents, "transfer-conflict")
	assert.ErrorIs(t, err, ErrAffiliateTransferRequestConflict)
	_, err = user.TransferAffiliateCentsToQuotaWithRequestId(2*amountCents, "transfer-insufficient")
	assert.ErrorIs(t, err, ErrAffiliateTransferInsufficientBalance)

	var count int64
	require.NoError(t, DB.Model(&AffiliateTransfer{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	fresh, err := GetUserById(user.Id, false)
	require.NoError(t, err)
	account, err := GetAffiliateAccount(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(100), account.AvailableCents)
	assert.Zero(t, fresh.AffQuota)
	assert.Equal(t, amountQuota, fresh.Quota)
}

func TestAffiliateTransferRejectsWalletOverflowWithoutConsumingCommission(t *testing.T) {
	user := setupAffiliatePayoutTest(t, 2)
	require.NoError(t, DB.Model(user).Update("quota", common.MaxWalletQuota-100).Error)

	_, err := user.TransferAffiliateCentsToQuotaWithRequestId(100, "transfer-wallet-overflow")
	require.ErrorIs(t, err, ErrWalletQuotaLimitExceeded)

	fresh, err := GetUserById(user.Id, false)
	require.NoError(t, err)
	account, err := GetAffiliateAccount(user.Id)
	require.NoError(t, err)
	assert.Equal(t, common.MaxWalletQuota-100, fresh.Quota)
	assert.Equal(t, int64(200), account.AvailableCents)

	var count int64
	require.NoError(t, DB.Model(&AffiliateTransfer{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestListAffiliateTransfersSupportsAdminSearch(t *testing.T) {
	user := setupAffiliatePayoutTest(t, 10)
	user.DisplayName = "Ledger User"
	require.NoError(t, DB.Model(user).Update("display_name", user.DisplayName).Error)
	_, err := user.TransferAffiliateCentsToQuotaWithRequestId(100, "transfer-searchable")
	require.NoError(t, err)

	rows, total, err := ListAffiliateTransfers(AffiliateTransferQueryOptions{Keyword: "Ledger User"}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, user.Username, rows[0].Username)
	assert.Equal(t, user.DisplayName, rows[0].DisplayName)

	rows, total, err = ListAffiliateTransfers(AffiliateTransferQueryOptions{UserId: user.Id}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, user.Id, rows[0].UserId)
}

func TestLegacyInvitationQuotaIsNotWithdrawableCommission(t *testing.T) {
	truncateTables(t)
	user := &User{
		Username: "legacy-affiliate-reward",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
		AffQuota: 30 * int(common.QuotaPerUnit),
	}
	require.NoError(t, DB.Create(user).Error)

	account, err := GetAffiliateAccount(user.Id)
	require.NoError(t, err)
	assert.Zero(t, account.AvailableCents)
	_, err = CreateAffiliatePayout(CreateAffiliatePayoutParams{
		UserId:        user.Id,
		RequestId:     "legacy-must-not-withdraw",
		AmountCents:   AffiliatePayoutMinimumCents,
		PaymentMethod: AffiliatePayoutMethodAlipay,
		AccountName:   "Legacy User",
		Account:       "legacy@example.com",
	})
	assert.ErrorIs(t, err, ErrAffiliatePayoutInsufficientBalance)

	require.NoError(t, TransferLegacyAffQuotaToQuota(user.Id, int(common.QuotaPerUnit)))
	fresh, err := GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, 29*int(common.QuotaPerUnit), fresh.AffQuota)
	assert.Equal(t, int(common.QuotaPerUnit), fresh.Quota)
	var transfers int64
	require.NoError(t, DB.Model(&AffiliateTransfer{}).Count(&transfers).Error)
	assert.Zero(t, transfers)
}

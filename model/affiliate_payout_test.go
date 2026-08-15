package model

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAffiliatePayoutTest(t *testing.T, availableLocalAmount int) *User {
	t.Helper()
	truncateTables(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		affiliatePayoutNow = time.Now
	})
	affiliatePayoutNow = func() time.Time {
		return time.Date(2026, time.September, 10, 12, 0, 0, 0, affiliatePayoutLocation())
	}
	user := &User{
		Username: "payout-user",
		Password: "password-value",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.Create(&AffiliateAccount{
		UserId:              user.Id,
		AvailableCents:      int64(availableLocalAmount * 100),
		LifetimeEarnedCents: int64(availableLocalAmount * 100),
	}).Error)
	return user
}

func TestAffiliatePayoutCreateIsIdempotentAndEncrypted(t *testing.T) {
	user := setupAffiliatePayoutTest(t, 300)
	affiliatePayoutNow = func() time.Time {
		return time.Date(2026, time.September, 5, 12, 0, 0, 0, affiliatePayoutLocation())
	}
	params := CreateAffiliatePayoutParams{
		UserId:        user.Id,
		RequestId:     "request-1",
		AmountCents:   10_000,
		PaymentMethod: AffiliatePayoutMethodAlipay,
		AccountName:   "Test User",
		Account:       "payout@example.com",
	}
	payout, err := CreateAffiliatePayout(params)
	require.NoError(t, err)
	assert.Equal(t, AffiliatePayoutStatusPending, payout.Status)
	assert.Equal(t, "payout@example.com", payout.Account)
	assert.Equal(t, int64(10_000), payout.AmountCents)
	assert.Equal(t, time.Date(2026, time.September, 10, 0, 0, 0, 0, affiliatePayoutLocation()).Unix(), payout.EligibleSettlementTime)

	var encrypted string
	require.NoError(t, DB.Raw("SELECT account_encrypted FROM affiliate_payouts WHERE id = ?", payout.Id).Scan(&encrypted).Error)
	assert.NotEmpty(t, encrypted)
	assert.NotContains(t, encrypted, params.Account)

	duplicate, err := CreateAffiliatePayout(params)
	require.NoError(t, err)
	assert.Equal(t, payout.Id, duplicate.Id)
	var count int64
	require.NoError(t, DB.Model(&AffiliatePayout{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	fresh, err := GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Zero(t, fresh.AffQuota)
	summary, err := GetAffiliatePayoutSummary(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(20_000), summary.AvailableCents)
	assert.Equal(t, int64(10_000), summary.FrozenCents)
}

func TestAffiliatePayoutRejectsReusedRequestIdWithDifferentPayload(t *testing.T) {
	user := setupAffiliatePayoutTest(t, 300)
	params := CreateAffiliatePayoutParams{
		UserId:        user.Id,
		RequestId:     "request-conflict",
		AmountCents:   10_000,
		PaymentMethod: AffiliatePayoutMethodAlipay,
		AccountName:   "Test User",
		Account:       "payout@example.com",
	}
	_, err := CreateAffiliatePayout(params)
	require.NoError(t, err)
	params.AmountCents = 20_000
	_, err = CreateAffiliatePayout(params)
	assert.ErrorIs(t, err, ErrAffiliatePayoutRequestConflict)
	params.AmountCents = 10_000
	params.Account = "another@example.com"
	_, err = CreateAffiliatePayout(params)
	assert.ErrorIs(t, err, ErrAffiliatePayoutRequestConflict)

	account, err := GetAffiliateAccount(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(20_000), account.AvailableCents)
	assert.Equal(t, int64(10_000), account.FrozenCents)
	var count int64
	require.NoError(t, DB.Model(&AffiliatePayout{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestAffiliatePayoutReviewCancelAndManualPayment(t *testing.T) {
	user := setupAffiliatePayoutTest(t, 400)
	affiliatePayoutNow = func() time.Time {
		return time.Date(2026, time.September, 5, 12, 0, 0, 0, affiliatePayoutLocation())
	}
	create := func(requestID string) *AffiliatePayout {
		payout, err := CreateAffiliatePayout(CreateAffiliatePayoutParams{
			UserId:        user.Id,
			RequestId:     requestID,
			AmountCents:   10_000,
			PaymentMethod: AffiliatePayoutMethodAlipay,
			AccountName:   "Test User",
			Account:       "payout@example.com",
		})
		require.NoError(t, err)
		return payout
	}

	cancelled := create("cancel-request")
	require.NoError(t, CancelAffiliatePayout(cancelled.Id, user.Id))
	require.NoError(t, DB.First(cancelled, cancelled.Id).Error)
	assert.Equal(t, AffiliatePayoutStatusCancelled, cancelled.Status)

	rejected := create("reject-request")
	assert.ErrorIs(t, ReviewAffiliatePayout(rejected.Id, 99, false, ""), ErrAffiliatePayoutRejectionReasonRequired)
	require.NoError(t, ReviewAffiliatePayout(rejected.Id, 99, false, "invalid account"))
	require.NoError(t, DB.First(rejected, rejected.Id).Error)
	assert.Equal(t, AffiliatePayoutStatusRejected, rejected.Status)

	approved := create("approved-request")
	require.NoError(t, ReviewAffiliatePayout(approved.Id, 99, true, ""))
	assert.ErrorIs(t, MarkAffiliatePayoutPaid(approved.Id, 99), ErrAffiliatePayoutSettlementNotDue)
	affiliatePayoutNow = func() time.Time {
		return time.Date(2026, time.September, 10, 12, 0, 0, 0, affiliatePayoutLocation())
	}
	require.NoError(t, MarkAffiliatePayoutPaid(approved.Id, 99))
	require.NoError(t, DB.First(approved, approved.Id).Error)
	assert.Equal(t, AffiliatePayoutStatusPaid, approved.Status)
	assert.Equal(t, AffiliatePayoutDisbursementManual, approved.DisbursementMode)
	assert.Equal(t, "MANUAL_CONFIRMED", approved.ProviderStatus)
	assert.Contains(t, approved.PaymentReference, "MANUAL-")
	assert.Equal(t, time.Date(2026, time.September, 10, 12, 0, 0, 0, affiliatePayoutLocation()).Unix(), approved.PaidTime)

	account, err := GetAffiliateAccount(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(30_000), account.AvailableCents)
	assert.Zero(t, account.FrozenCents)
}

func TestAffiliatePayoutOnlyAcceptsAlipayForNewApplications(t *testing.T) {
	user := setupAffiliatePayoutTest(t, 100)
	_, err := CreateAffiliatePayout(CreateAffiliatePayoutParams{
		UserId:        user.Id,
		RequestId:     "bank-request",
		AmountCents:   AffiliatePayoutMinimumCents,
		PaymentMethod: AffiliatePayoutMethodBank,
		AccountName:   "Test User",
		Account:       "6222020202020202",
	})
	assert.ErrorIs(t, err, ErrAffiliatePayoutAccountInvalid)

	var count int64
	require.NoError(t, DB.Model(&AffiliatePayout{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCommissionTransferDoesNotCreateTopUpOrReferralCommission(t *testing.T) {
	user := setupAffiliatePayoutTest(t, 120)
	transferQuota := int(common.QuotaPerUnit)
	require.NoError(t, user.TransferAffQuotaToQuota(transferQuota))

	var topUpCount int64
	var commissionCount int64
	var transferCount int64
	require.NoError(t, DB.Model(&TopUp{}).Count(&topUpCount).Error)
	require.NoError(t, DB.Model(&AffiliateCommission{}).Count(&commissionCount).Error)
	require.NoError(t, DB.Model(&AffiliateTransfer{}).Count(&transferCount).Error)
	assert.Zero(t, topUpCount)
	assert.Zero(t, commissionCount)
	assert.Equal(t, int64(1), transferCount)

	fresh, err := GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, transferQuota, fresh.Quota)
	account, err := GetAffiliateAccount(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(11_900), account.AvailableCents)
}

func TestAffiliatePayoutRejectsInvalidAmountsAndOwnership(t *testing.T) {
	user := setupAffiliatePayoutTest(t, 100)
	base := CreateAffiliatePayoutParams{
		UserId:        user.Id,
		RequestId:     "minimum-request",
		AmountCents:   AffiliatePayoutMinimumCents - 1,
		PaymentMethod: AffiliatePayoutMethodAlipay,
		AccountName:   "Test User",
		Account:       "test@example.com",
	}
	_, err := CreateAffiliatePayout(base)
	assert.ErrorIs(t, err, ErrAffiliatePayoutAmountTooSmall)
	base.AmountCents = AffiliatePayoutMinimumCents
	payout, err := CreateAffiliatePayout(base)
	require.NoError(t, err)
	assert.True(t, errors.Is(CancelAffiliatePayout(payout.Id, user.Id+1), ErrAffiliatePayoutForbidden))
	base.RequestId = "insufficient-request"
	_, err = CreateAffiliatePayout(base)
	assert.ErrorIs(t, err, ErrAffiliatePayoutInsufficientBalance)
}

func TestAffiliatePayoutDirectDisbursementReusesAmbiguousAttemptAndCompletesOnce(t *testing.T) {
	user := setupAffiliatePayoutTest(t, 150)
	payout, err := CreateAffiliatePayout(CreateAffiliatePayoutParams{
		UserId:        user.Id,
		RequestId:     "direct-payout-idempotency",
		AmountCents:   AffiliatePayoutMinimumCents,
		PaymentMethod: AffiliatePayoutMethodAlipay,
		AccountName:   "张三",
		Account:       "payout@example.com",
	})
	require.NoError(t, err)
	require.NoError(t, ReviewAffiliatePayout(payout.Id, 99, true, ""))

	started, newAttempt, err := BeginAffiliatePayoutDisbursement(payout.Id, 99)
	require.NoError(t, err)
	assert.True(t, newAttempt)
	assert.Equal(t, AffiliatePayoutStatusProcessing, started.Status)
	assert.Equal(t, 1, started.PaymentAttempt)
	assert.NotEmpty(t, started.PaymentReference)

	reused, newAttempt, err := BeginAffiliatePayoutDisbursement(payout.Id, 100)
	require.NoError(t, err)
	assert.False(t, newAttempt)
	assert.Equal(t, started.PaymentReference, reused.PaymentReference)
	assert.Equal(t, 1, reused.PaymentAttempt)

	require.NoError(t, CompleteAffiliatePayoutDisbursement(payout.Id, 99, started.PaymentReference, "alipay-order-1", "fund-order-1", "SUCCESS"))
	require.NoError(t, CompleteAffiliatePayoutDisbursement(payout.Id, 99, started.PaymentReference, "alipay-order-1", "fund-order-1", "SUCCESS"))
	completed, err := GetAffiliatePayoutById(payout.Id)
	require.NoError(t, err)
	assert.Equal(t, AffiliatePayoutStatusPaid, completed.Status)
	assert.Equal(t, "alipay-order-1", completed.ProviderOrderId)
	assert.Equal(t, "fund-order-1", completed.ProviderFundOrderId)
}

func TestAffiliatePayoutDefinitiveFailureAllowsNewAttemptReference(t *testing.T) {
	user := setupAffiliatePayoutTest(t, 150)
	payout, err := CreateAffiliatePayout(CreateAffiliatePayoutParams{
		UserId:        user.Id,
		RequestId:     "direct-payout-retry",
		AmountCents:   AffiliatePayoutMinimumCents,
		PaymentMethod: AffiliatePayoutMethodAlipay,
		AccountName:   "张三",
		Account:       "payout@example.com",
	})
	require.NoError(t, err)
	require.NoError(t, ReviewAffiliatePayout(payout.Id, 99, true, ""))

	first, _, err := BeginAffiliatePayoutDisbursement(payout.Id, 99)
	require.NoError(t, err)
	require.NoError(t, FailAffiliatePayoutDisbursement(payout.Id, first.PaymentReference, "PAYEE_NOT_EXIST", "recipient missing"))
	second, newAttempt, err := BeginAffiliatePayoutDisbursement(payout.Id, 99)
	require.NoError(t, err)
	assert.True(t, newAttempt)
	assert.Equal(t, 2, second.PaymentAttempt)
	assert.NotEqual(t, first.PaymentReference, second.PaymentReference)
}

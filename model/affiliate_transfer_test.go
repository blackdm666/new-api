package model

import (
	"strconv"
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

func TestAffiliateTransferReviewFreezesUntilApprovedOrRejected(t *testing.T) {
	user := setupAffiliatePayoutTest(t, 10)
	setAffiliateOptionForTest(t, AffiliateTransferAutoApproveOptionKey, "false")
	quotaPerUnit := int(common.QuotaPerUnit)

	pending, err := user.TransferAffiliateCentsToQuotaWithRequestId(300, "transfer-review-1")
	require.NoError(t, err)
	assert.Equal(t, AffiliateTransferStatusPending, pending.Status)
	duplicate, err := user.TransferAffiliateCentsToQuotaWithRequestId(300, "transfer-review-1")
	require.NoError(t, err)
	assert.Equal(t, pending.Id, duplicate.Id)

	account, err := GetAffiliateAccount(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(700), account.AvailableCents)
	assert.Equal(t, int64(300), account.FrozenCents)
	fresh, err := GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Zero(t, fresh.Quota)

	_, err = ReviewAffiliateTransfer(pending.Id, 1, false, " ")
	assert.ErrorIs(t, err, ErrAffiliateTransferRejectionReasonRequired)
	approved, err := ReviewAffiliateTransfer(pending.Id, 1, true, "")
	require.NoError(t, err)
	assert.Equal(t, AffiliateTransferStatusApproved, approved.Status)
	assert.Equal(t, 1, approved.OperatorId)
	assert.Zero(t, approved.QuotaBefore)
	assert.Equal(t, 3*quotaPerUnit, approved.QuotaAfter)
	_, err = ReviewAffiliateTransfer(pending.Id, 1, true, "")
	assert.ErrorIs(t, err, ErrAffiliateTransferStatusInvalid)

	rejected, err := user.TransferAffiliateCentsToQuotaWithRequestId(200, "transfer-review-2")
	require.NoError(t, err)
	assert.ErrorIs(t, CancelAffiliateTransfer(rejected.Id, user.Id+1), ErrAffiliateTransferForbidden)
	reviewed, err := ReviewAffiliateTransfer(rejected.Id, 1, false, "self referral")
	require.NoError(t, err)
	assert.Equal(t, AffiliateTransferStatusRejected, reviewed.Status)
	assert.Equal(t, "self referral", reviewed.RejectReason)

	cancelled, err := user.TransferAffiliateCentsToQuotaWithRequestId(100, "transfer-review-3")
	require.NoError(t, err)
	require.NoError(t, CancelAffiliateTransfer(cancelled.Id, user.Id))
	assert.ErrorIs(t, CancelAffiliateTransfer(cancelled.Id, user.Id), ErrAffiliateTransferStatusInvalid)

	account, err = GetAffiliateAccount(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(700), account.AvailableCents)
	assert.Zero(t, account.FrozenCents)
	fresh, err = GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, 3*quotaPerUnit, fresh.Quota)
	pendingSummary, err := GetAffiliateTransferPendingSummary(0)
	require.NoError(t, err)
	assert.Zero(t, pendingSummary.Count)

	rows, total, err := ListAffiliateTransfers(AffiliateTransferQueryOptions{Status: AffiliateTransferStatusRejected}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	assert.Equal(t, rejected.Id, rows[0].Id)
}

func TestAffiliateInviterRiskScoresSelfReferralAndHidesEvidenceFromUser(t *testing.T) {
	user := setupAffiliatePayoutTest(t, 10)
	require.NoError(t, LOG_DB.AutoMigrate(&AuditLog{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM affiliate_commissions")
		LOG_DB.Exec("DELETE FROM audit_logs")
	})
	quotaPerUnit := int(common.QuotaPerUnit)
	require.NoError(t, DB.Model(user).Updates(map[string]any{"username": "rainc", "created_at": int64(1_000_000), "used_quota": 0}).Error)
	selfAccount := &User{Username: "esun", AffCode: "risk-self", Email: "rainc0627@qq.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, InviterId: user.Id, CreatedAt: 1_000_360, UsedQuota: 90 * quotaPerUnit}
	friend := &User{Username: "friend-account", AffCode: "risk-friend", Email: "friend@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, InviterId: user.Id, CreatedAt: 9_000_000}
	require.NoError(t, DB.Create(selfAccount).Error)
	require.NoError(t, DB.Create(friend).Error)
	for i, inviteeId := range []int{selfAccount.Id, friend.Id} {
		require.NoError(t, DB.Create(&AffiliateCommission{InviterId: user.Id, InviteeId: inviteeId, TopUpId: 9000 + i, CommissionCents: 500, Status: AffiliateCommissionStatusApproved}).Error)
	}
	sessions := []struct {
		userId int
		ip     string
	}{
		{user.Id, "203.0.113.7"}, {selfAccount.Id, "203.0.113.7"},
		// A busy exit shared by more than four accounts must not count.
		{user.Id, "198.51.100.9"}, {friend.Id, "198.51.100.9"}, {9001, "198.51.100.9"}, {9002, "198.51.100.9"}, {9003, "198.51.100.9"},
		{user.Id, "10.0.0.8"}, {friend.Id, "10.0.0.8"},
	}
	for i, session := range sessions {
		require.NoError(t, DB.Create(&UserSession{SID: "risk-session-" + strconv.Itoa(i), UserID: session.userId, Status: "active", RefreshHash: "hash", LoginMethod: "password", IP: session.ip, UserAgent: "Mozilla/5.0 Test", ExpiresAt: 1, LastActiveAt: 1}).Error)
	}

	score, detail, err := EvaluateAffiliateInviterRisk(user.Id)
	require.NoError(t, err)
	// shared IP 3 + same UA 1 + registered within a day 2 + idle inviter 2 + similar name 2
	assert.Equal(t, 10, score)
	assert.Equal(t, 2, detail.InviteeCount)
	assert.False(t, detail.SingleInvitee)
	assert.Zero(t, detail.InviteeClusterCount)
	assert.Equal(t, int64(500), detail.SuspiciousCommissionCents)
	require.Len(t, detail.Pairs, 1)
	assert.Equal(t, selfAccount.Id, detail.Pairs[0].InviteeId)
	assert.Equal(t, "203.0.113.7", detail.Pairs[0].SharedIp)
	assert.Equal(t, []string{AffiliateRiskSignalSharedIP, AffiliateRiskSignalSameUserAgent, AffiliateRiskSignalRegisteredClose, AffiliateRiskSignalInviterIdle, AffiliateRiskSignalSimilarName}, detail.Pairs[0].Signals)

	setAffiliateOptionForTest(t, AffiliateTransferAutoApproveOptionKey, "false")
	transfer, err := user.TransferAffiliateCentsToQuotaWithRequestId(100, "transfer-risk")
	require.NoError(t, err)
	require.NoError(t, EvaluateAffiliateTransferRisk(transfer.Id))
	adminRows, _, err := ListAffiliateTransfers(AffiliateTransferQueryOptions{IncludeAdminRemarks: true}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.NotNil(t, adminRows[0].Risk)
	assert.Equal(t, 10, adminRows[0].RiskScore)
	userRows, _, err := ListAffiliateTransfers(AffiliateTransferQueryOptions{UserId: user.Id}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Nil(t, userRows[0].Risk)
	assert.Zero(t, userRows[0].RiskScore)
}

func TestAffiliateRiskNameSimilarity(t *testing.T) {
	cases := []struct {
		inviter affiliateRiskUser
		invitee affiliateRiskUser
		similar bool
	}{
		{affiliateRiskUser{Username: "lee-x"}, affiliateRiskUser{Username: "lee-s"}, true},
		{affiliateRiskUser{Username: "1332537578", Email: "1332537578@qq.com"}, affiliateRiskUser{Username: "13216031237", Email: "us1332537578@qq.com"}, true},
		{affiliateRiskUser{Username: "ao15185338454"}, affiliateRiskUser{Email: "al15185338454@163.com"}, true},
		{affiliateRiskUser{Username: "liu12300", Email: "647603604@qq.com"}, affiliateRiskUser{Username: "ao15185338454", Email: "al15185338454@163.com"}, false},
		{affiliateRiskUser{Username: "google_1726"}, affiliateRiskUser{Username: "google_1727"}, false},
		{affiliateRiskUser{Username: "dong_soft", Email: "xizhudong@gmail.com"}, affiliateRiskUser{Username: "dong123", Email: "342877216@qq.com"}, false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.similar, affiliateRiskNamesSimilar(affiliateRiskNameTokens(tc.inviter), affiliateRiskNameTokens(tc.invitee)), "%s / %s", tc.inviter.Username, tc.invitee.Username)
	}
}

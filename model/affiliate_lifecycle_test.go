package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAffiliateLifecycleFromInvitationThroughPaymentAndSettlement(t *testing.T) {
	truncateTables(t)

	originalQuotaPerUnit := common.QuotaPerUnit
	originalQuotaForNewUser := common.QuotaForNewUser
	originalQuotaForInvitee := common.QuotaForInvitee
	originalQuotaForInviter := common.QuotaForInviter
	common.QuotaPerUnit = 500_000
	common.QuotaForNewUser = 0
	common.QuotaForInvitee = 0
	common.QuotaForInviter = 0
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		common.QuotaForNewUser = originalQuotaForNewUser
		common.QuotaForInvitee = originalQuotaForInvitee
		common.QuotaForInviter = originalQuotaForInviter
		affiliatePayoutNow = time.Now
	})

	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateCommissionAutoApproveOptionKey, "false")
	setAffiliateOptionForTest(t, AffiliateCommissionGroupRatesOptionKey, `{"default":500,"高级推广":1000,"金牌推广":1500}`)

	promoter := &User{
		Username: "lifecycle-promoter",
		Email:    "lifecycle-promoter@example.com",
		Group:    AffiliatePromoterGroupDefault,
		AffCode:  "lifecycle-invite-link",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
		Quota:    int(common.QuotaPerUnit),
	}
	require.NoError(t, DB.Create(promoter).Error)

	resolvedInviterID, err := GetUserIdByAffCode(promoter.AffCode)
	require.NoError(t, err)
	assert.Equal(t, promoter.Id, resolvedInviterID)

	invitees := make([]*User, 0, 4)
	for index := 1; index <= 4; index++ {
		invitee := &User{
			Username:  fmt.Sprintf("lifecycle-invitee-%d", index),
			Password:  "LifecycleUser#2026",
			Email:     fmt.Sprintf("lifecycle-invitee-%d@example.com", index),
			Group:     AffiliatePromoterGroupDefault,
			InviterId: resolvedInviterID,
			Status:    common.UserStatusEnabled,
			Role:      common.RoleCommonUser,
		}
		require.NoError(t, invitee.Insert(resolvedInviterID))
		assert.Equal(t, promoter.Id, invitee.InviterId)
		invitees = append(invitees, invitee)
	}

	for index, invitee := range invitees {
		topUp := &TopUp{
			UserId:          invitee.Id,
			Amount:          3_000,
			Money:           3_000,
			TradeNo:         fmt.Sprintf("AFF-LIFECYCLE-%02d", index+1),
			PaymentMethod:   "alipay",
			PaymentProvider: PaymentProviderEpay,
			CreateTime:      common.GetTimestamp(),
			Status:          common.TopUpStatusPending,
		}
		require.NoError(t, topUp.Insert())

		alreadyDone, err := RechargeEpay(topUp.TradeNo, "alipay", topUp.Money, "127.0.0.1")
		require.NoError(t, err)
		assert.False(t, alreadyDone)
		alreadyDone, err = RechargeEpay(topUp.TradeNo, "alipay", topUp.Money, "127.0.0.1")
		require.NoError(t, err)
		assert.True(t, alreadyDone)

		storedTopUp := GetTopUpByTradeNo(topUp.TradeNo)
		require.NotNil(t, storedTopUp)
		assert.Equal(t, common.TopUpStatusSuccess, storedTopUp.Status)
		assert.Equal(t, 3_000*int(common.QuotaPerUnit), storedTopUp.CreditedQuota)
	}

	commissions, total, err := ListAffiliateCommissions(
		AffiliateCommissionQueryOptions{InviterId: promoter.Id},
		&common.PageInfo{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(4), total)
	require.Len(t, commissions, 4)
	for _, commission := range commissions {
		assert.Equal(t, AffiliateCommissionStatusPending, commission.Status)
		assert.Equal(t, int64(300_000), commission.TopUpAmountCents)
		assert.Equal(t, 500, commission.RateBasisPoints)
		assert.Equal(t, int64(15_000), commission.CommissionCents)
	}

	require.NoError(t, CompleteAffiliateCommission(commissions[0].Id, 99, false, "演示：支付账户异常"))
	for _, commission := range commissions[1:] {
		require.NoError(t, CompleteAffiliateCommission(commission.Id, 99, true, ""))
	}

	account, err := GetAffiliateAccount(promoter.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(45_000), account.AvailableCents)
	assert.Equal(t, int64(45_000), account.LifetimeEarnedCents)

	transferCents := int64(5_000)
	transferQuota := 50 * int(common.QuotaPerUnit)
	transfer, err := promoter.TransferAffiliateCentsToQuotaWithRequestId(transferCents, "affiliate-lifecycle-transfer")
	require.NoError(t, err)
	assert.Equal(t, int64(40_000), transfer.BalanceCentsAfter)
	assert.Equal(t, int(common.QuotaPerUnit)+transferQuota, transfer.QuotaAfter)

	affiliatePayoutNow = func() time.Time {
		return time.Date(2026, time.September, 5, 12, 0, 0, 0, affiliatePayoutLocation())
	}
	rejectedPayout, err := CreateAffiliatePayout(CreateAffiliatePayoutParams{
		UserId:        promoter.Id,
		RequestId:     "affiliate-lifecycle-payout-rejected",
		AmountCents:   10_000,
		PaymentMethod: AffiliatePayoutMethodAlipay,
		AccountName:   "生命周期演示用户",
		Account:       "lifecycle@example.com",
	})
	require.NoError(t, err)
	require.NoError(t, ReviewAffiliatePayout(rejectedPayout.Id, 99, false, "演示：支付宝实名信息不一致"))

	paidPayout, err := CreateAffiliatePayout(CreateAffiliatePayoutParams{
		UserId:        promoter.Id,
		RequestId:     "affiliate-lifecycle-payout-paid",
		AmountCents:   10_000,
		PaymentMethod: AffiliatePayoutMethodAlipay,
		AccountName:   "生命周期演示用户",
		Account:       "lifecycle@example.com",
	})
	require.NoError(t, err)
	require.NoError(t, ReviewAffiliatePayout(paidPayout.Id, 99, true, ""))
	assert.ErrorIs(t, MarkAffiliatePayoutPaid(paidPayout.Id, 99), ErrAffiliatePayoutSettlementNotDue)
	affiliatePayoutNow = func() time.Time {
		return time.Date(2026, time.September, 10, 12, 0, 0, 0, affiliatePayoutLocation())
	}
	require.NoError(t, MarkAffiliatePayoutPaid(paidPayout.Id, 99))

	promoter, err = GetUserById(promoter.Id, false)
	require.NoError(t, err)
	account, err = GetAffiliateAccount(promoter.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(30_000), account.AvailableCents)
	assert.Zero(t, account.FrozenCents)
	assert.Equal(t, int(common.QuotaPerUnit)+transferQuota, promoter.Quota)
	assert.Equal(t, int64(45_000), account.LifetimeEarnedCents)

	var topUpCount int64
	var commissionCount int64
	var transferCount int64
	var payoutCount int64
	require.NoError(t, DB.Model(&TopUp{}).Count(&topUpCount).Error)
	require.NoError(t, DB.Model(&AffiliateCommission{}).Count(&commissionCount).Error)
	require.NoError(t, DB.Model(&AffiliateTransfer{}).Count(&transferCount).Error)
	require.NoError(t, DB.Model(&AffiliatePayout{}).Count(&payoutCount).Error)
	assert.Equal(t, int64(4), topUpCount)
	assert.Equal(t, int64(4), commissionCount)
	assert.Equal(t, int64(1), transferCount)
	assert.Equal(t, int64(2), payoutCount)

	finalPaidPayout := &AffiliatePayout{}
	require.NoError(t, DB.First(finalPaidPayout, paidPayout.Id).Error)
	assert.Equal(t, AffiliatePayoutStatusPaid, finalPaidPayout.Status)
	assert.Contains(t, finalPaidPayout.PaymentReference, "MANUAL-")
	finalRejectedPayout := &AffiliatePayout{}
	require.NoError(t, DB.First(finalRejectedPayout, rejectedPayout.Id).Error)
	assert.Equal(t, AffiliatePayoutStatusRejected, finalRejectedPayout.Status)
	assert.Equal(t, "演示：支付宝实名信息不一致", finalRejectedPayout.RejectReason)
}

func TestAffiliateAutoApprovalCreditsEpayDuringRecharge(t *testing.T) {
	truncateTables(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateCommissionAutoApproveOptionKey, "true")

	inviter := &User{Username: "auto-epay-promoter", Email: "auto-epay-promoter@example.com", Group: AffiliatePromoterGroupDefault, AffCode: "auto-epay-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(inviter).Error)
	invitee := &User{Username: "auto-epay-invitee", Email: "auto-epay-invitee@example.com", Group: AffiliatePromoterGroupDefault, InviterId: inviter.Id, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(invitee).Error)
	topUp := &TopUp{
		UserId:          invitee.Id,
		Amount:          1_000,
		Money:           1_000,
		TradeNo:         "AFF-AUTO-EPAY-CALLBACK",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())
	alreadyDone, err := RechargeEpay(topUp.TradeNo, "alipay", topUp.Money, "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)

	record := &AffiliateCommission{}
	require.NoError(t, DB.Where("top_up_id = ?", topUp.Id).First(record).Error)
	assert.Equal(t, AffiliateCommissionStatusApproved, record.Status)
	assert.Equal(t, int64(5_000), record.CommissionCents)
	account, err := GetAffiliateAccount(inviter.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(5_000), account.AvailableCents)
	assert.Equal(t, int64(5_000), account.LifetimeEarnedCents)

	alreadyDone, err = RechargeEpay(topUp.TradeNo, "alipay", topUp.Money, "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, alreadyDone)
	account, err = GetAffiliateAccount(inviter.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(5_000), account.AvailableCents)
	var count int64
	require.NoError(t, DB.Model(&AffiliateCommission{}).Where("top_up_id = ?", topUp.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

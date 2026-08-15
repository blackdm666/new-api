package model

import (
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setAffiliateOptionForTest(t *testing.T, key string, value string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	previous, existed := common.OptionMap[key]
	common.OptionMap[key] = value
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if existed {
			common.OptionMap[key] = previous
			return
		}
		delete(common.OptionMap, key)
	})
}

func enableAffiliateForTest(t *testing.T) {
	t.Helper()
	setAffiliateOptionForTest(t, AffiliateCommissionEnabledOptionKey, "true")
	setAffiliateOptionForTest(t, AffiliateCommissionActivatedAtOptionKey, "1")
}

func createAffiliateFixture(t *testing.T, suffix string, money float64) (*User, *User, *TopUp, *AffiliateCommission) {
	t.Helper()
	inviter := &User{
		Username: "affiliate-fixture-owner-" + suffix,
		Email:    "affiliate-fixture-owner-" + suffix + "@example.com",
		Group:    AffiliatePromoterGroupDefault,
		AffCode:  "affiliate-fixture-owner-code-" + suffix,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(inviter).Error)
	invitee := &User{
		Username:  "affiliate-fixture-buyer-" + suffix,
		Email:     "affiliate-fixture-buyer-" + suffix + "@example.com",
		AffCode:   "affiliate-fixture-buyer-code-" + suffix,
		InviterId: inviter.Id,
		Status:    common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(invitee).Error)
	topUp := &TopUp{
		UserId:          invitee.Id,
		Money:           money,
		TradeNo:         "AFF-FIXTURE-" + suffix,
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CompleteTime:    common.GetTimestamp(),
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, DB.Create(topUp).Error)
	require.NoError(t, CreateAffiliateCommissionForTopUp(topUp))
	record := &AffiliateCommission{}
	require.NoError(t, DB.Where("top_up_id = ?", topUp.Id).First(record).Error)
	return inviter, invitee, topUp, record
}

func TestAffiliateCommissionCreatedFromSuccessfulTopUpAndApproved(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	originalRate := operation_setting.USDExchangeRate
	originalDisplay := operation_setting.GetGeneralSetting().QuotaDisplayType
	operation_setting.USDExchangeRate = 1
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() {
		operation_setting.USDExchangeRate = originalRate
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplay
	})

	inviter := &User{
		Username:    "affiliate-owner",
		DisplayName: "Affiliate Owner",
		Email:       "owner@example.com",
		Group:       "高级推广",
		AffCode:     "affiliate-owner-code",
		Status:      common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(inviter).Error)
	invitee := &User{
		Username:  "affiliate-buyer",
		Email:     "buyer@example.com",
		AffCode:   "affiliate-buyer-code",
		InviterId: inviter.Id,
		Status:    common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(invitee).Error)
	topUp := &TopUp{
		UserId:          invitee.Id,
		Amount:          100,
		Money:           200,
		TradeNo:         "AFF-COMMISSION-ORDER",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CompleteTime:    common.GetTimestamp(),
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, DB.Create(topUp).Error)

	require.NoError(t, CreateAffiliateCommissionForTopUp(topUp))
	require.NoError(t, CreateAffiliateCommissionForTopUp(topUp))

	var records []AffiliateCommission
	require.NoError(t, DB.Find(&records).Error)
	require.Len(t, records, 1)
	assert.Equal(t, AffiliateCommissionStatusPending, records[0].Status)
	assert.Equal(t, int64(20000), records[0].TopUpAmountCents)
	assert.Equal(t, 1000, records[0].RateBasisPoints)
	assert.Equal(t, int64(2000), records[0].CommissionCents)
	expectedQuota := common.QuotaFromFloat(20 * common.QuotaPerUnit)
	assert.Equal(t, expectedQuota, records[0].CommissionQuota)

	listed, total, err := ListAffiliateCommissions(AffiliateCommissionQueryOptions{InviterId: inviter.Id}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, listed, 1)
	assert.Equal(t, inviter.Username, listed[0].InviterUsername)
	assert.Equal(t, inviter.DisplayName, listed[0].InviterDisplayName)
	assert.Equal(t, invitee.Username, listed[0].InviteeUsername)

	summary, err := GetAffiliateSummary(inviter.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(2000), summary.PendingCommissionCents)
	assert.Equal(t, int64(1), summary.EffectiveInviteeCount)

	require.NoError(t, CompleteAffiliateCommission(records[0].Id, 99, true, ""))
	account, err := GetAffiliateAccount(inviter.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(2_000), account.AvailableCents)
	assert.Equal(t, int64(2_000), account.LifetimeEarnedCents)

	require.ErrorIs(t, CompleteAffiliateCommission(records[0].Id, 99, true, ""), ErrAffiliateCommissionStatusInvalid)
}

func TestAffiliateCommissionAutoApprovalCreditsExactlyOnce(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateCommissionAutoApproveOptionKey, "true")
	originalRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 1
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })

	inviter, _, topUp, record := createAffiliateFixture(t, "auto-approve", 100)
	assert.Equal(t, AffiliateCommissionStatusApproved, record.Status)
	assert.Zero(t, record.OperatorId)
	assert.Positive(t, record.ApprovedTime)
	account, err := GetAffiliateAccount(inviter.Id)
	require.NoError(t, err)
	assert.Equal(t, record.CommissionCents, account.AvailableCents)
	assert.Equal(t, record.CommissionCents, account.LifetimeEarnedCents)

	require.NoError(t, CreateAffiliateCommissionForTopUp(topUp))
	account, err = GetAffiliateAccount(inviter.Id)
	require.NoError(t, err)
	assert.Equal(t, record.CommissionCents, account.AvailableCents)
	assert.Equal(t, record.CommissionCents, account.LifetimeEarnedCents)

	summary, err := GetAffiliateSummary(inviter.Id)
	require.NoError(t, err)
	assert.Zero(t, summary.PendingCommissionCents)
	assert.Equal(t, record.CommissionCents, summary.ApprovedCommissionCents)
}

func TestAffiliateCommissionAutoApprovalModeSwitchOnlyAffectsNewTopUps(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateCommissionAutoApproveOptionKey, "false")
	originalRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 1
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })

	inviter, _, pendingTopUp, pendingRecord := createAffiliateFixture(t, "auto-switch-pending", 100)
	assert.Equal(t, AffiliateCommissionStatusPending, pendingRecord.Status)
	account, err := GetAffiliateAccount(inviter.Id)
	require.NoError(t, err)
	assert.Zero(t, account.AvailableCents)

	setAffiliateOptionForTest(t, AffiliateCommissionAutoApproveOptionKey, "true")
	invitee := &User{
		Username:  "affiliate-fixture-buyer-auto-switch-approved",
		Email:     "affiliate-fixture-buyer-auto-switch-approved@example.com",
		AffCode:   "affiliate-fixture-buyer-auto-switch-approved-code",
		InviterId: inviter.Id,
		Status:    common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(invitee).Error)
	approvedTopUp := &TopUp{
		UserId:          invitee.Id,
		Money:           200,
		TradeNo:         "AFF-FIXTURE-auto-switch-approved",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CompleteTime:    common.GetTimestamp(),
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, DB.Create(approvedTopUp).Error)
	require.NoError(t, CreateAffiliateCommissionForTopUp(approvedTopUp))

	approvedRecord := &AffiliateCommission{}
	require.NoError(t, DB.Where("top_up_id = ?", approvedTopUp.Id).First(approvedRecord).Error)
	assert.Equal(t, AffiliateCommissionStatusApproved, approvedRecord.Status)
	assert.Positive(t, approvedRecord.ApprovedTime)
	account, err = GetAffiliateAccount(inviter.Id)
	require.NoError(t, err)
	assert.Equal(t, approvedRecord.CommissionCents, account.AvailableCents)
	assert.Equal(t, approvedRecord.CommissionCents, account.LifetimeEarnedCents)

	// Enabling automatic approval must not rewrite the existing manual-review queue.
	require.NoError(t, DB.First(pendingRecord, pendingRecord.Id).Error)
	assert.Equal(t, AffiliateCommissionStatusPending, pendingRecord.Status)
	require.NoError(t, CreateAffiliateCommissionForTopUp(pendingTopUp))
	require.NoError(t, CreateAffiliateCommissionForTopUp(approvedTopUp))
	account, err = GetAffiliateAccount(inviter.Id)
	require.NoError(t, err)
	assert.Equal(t, approvedRecord.CommissionCents, account.AvailableCents)

	summary, err := GetAffiliateSummary(inviter.Id)
	require.NoError(t, err)
	assert.Equal(t, pendingRecord.CommissionCents, summary.PendingCommissionCents)
	assert.Equal(t, approvedRecord.CommissionCents, summary.ApprovedCommissionCents)
}

func TestAffiliateCommissionUsesConfiguredGroupRateSnapshot(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateCommissionGroupRatesOptionKey, `{"default":600,"高级推广":1200,"金牌推广":1800}`)

	inviter := &User{
		Username: "affiliate-config-owner",
		Email:    "config-owner@example.com",
		Group:    "金牌推广",
		AffCode:  "affiliate-config-owner-code",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(inviter).Error)
	invitee := &User{
		Username:  "affiliate-config-buyer",
		Email:     "config-buyer@example.com",
		AffCode:   "affiliate-config-buyer-code",
		InviterId: inviter.Id,
		Status:    common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(invitee).Error)
	topUp := &TopUp{
		UserId:          invitee.Id,
		Money:           100,
		TradeNo:         "AFF-CONFIG-RATE",
		PaymentProvider: PaymentProviderEpay,
		CompleteTime:    common.GetTimestamp(),
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, DB.Create(topUp).Error)
	require.NoError(t, CreateAffiliateCommissionForTopUp(topUp))

	record := AffiliateCommission{}
	require.NoError(t, DB.First(&record).Error)
	assert.Equal(t, 1800, record.RateBasisPoints)
	assert.Equal(t, int64(1800), record.CommissionCents)
	assert.Equal(t, "金牌推广", record.InviterGroup)
	assert.Equal(t, "金牌推广", record.TierName)

	setAffiliateOptionForTest(t, AffiliateCommissionGroupRatesOptionKey, `{"default":500,"高级推广":1000,"金牌推广":1500}`)
	require.NoError(t, DB.First(&record, record.Id).Error)
	assert.Equal(t, 1800, record.RateBasisPoints)
	assert.Equal(t, int64(1800), record.CommissionCents)
}

func TestDefaultUserGroupUsesJuniorPromoterRateAndTier(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateCommissionGroupRatesOptionKey, `{"default":600,"高级推广":1200,"金牌推广":1800}`)

	inviter, _, _, record := createAffiliateFixture(t, "default-junior", 100)
	assert.Equal(t, AffiliatePromoterGroupDefault, inviter.Group)
	assert.Equal(t, 600, record.RateBasisPoints)
	assert.Equal(t, AffiliatePromoterGroupDefault, record.InviterGroup)
	assert.Equal(t, AffiliatePromoterGroupLegacyJunior, record.TierName)

	summary, err := GetAffiliateSummary(inviter.Id)
	require.NoError(t, err)
	assert.Equal(t, 600, summary.RateBasisPoints)
	assert.Equal(t, AffiliatePromoterGroupLegacyJunior, summary.TierName)
	assert.Equal(t, int64(1), summary.InviteCount)
	assert.True(t, summary.UpgradeEligible)
}

func TestLegacyJuniorRateConfigurationIsNormalizedForDefaultUsers(t *testing.T) {
	setAffiliateOptionForTest(t, AffiliateCommissionGroupRatesOptionKey, `{"初级推广":650,"高级推广":1100,"金牌推广":1600}`)

	policy := getAffiliatePolicy()
	assert.Equal(t, 650, policy.DefaultRateBasisPoints)
	assert.Equal(t, 650, policy.GroupRates[AffiliatePromoterGroupDefault])
	_, hasLegacyGroup := policy.GroupRates[AffiliatePromoterGroupLegacyJunior]
	assert.False(t, hasLegacyGroup)
}

func TestAffiliateCommissionOnlyUsesSuccessfulWalletTopUps(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	inviter := &User{
		Username: "affiliate-status-owner",
		Email:    "status-owner@example.com",
		AffCode:  "affiliate-status-owner-code",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(inviter).Error)
	invitee := &User{
		Username:  "affiliate-status-buyer",
		Email:     "status-buyer@example.com",
		AffCode:   "affiliate-status-buyer-code",
		InviterId: inviter.Id,
		Status:    common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(invitee).Error)
	topUp := &TopUp{
		UserId:  invitee.Id,
		Money:   100,
		TradeNo: "AFF-PENDING-ORDER",
		Status:  common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(topUp).Error)
	require.NoError(t, CreateAffiliateCommissionForTopUp(topUp))

	var count int64
	require.NoError(t, DB.Model(&AffiliateCommission{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestAffiliateCommissionOnlyUsesEpayProvider(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	inviter := &User{Username: "affiliate-epay-only-owner", Email: "epay-only-owner@example.com", AffCode: "epay-only-owner-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(inviter).Error)
	invitee := &User{Username: "affiliate-epay-only-buyer", Email: "epay-only-buyer@example.com", AffCode: "epay-only-buyer-code", InviterId: inviter.Id, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(invitee).Error)

	stripeTopUp := &TopUp{
		UserId:          invitee.Id,
		Money:           100,
		TradeNo:         "AFF-STRIPE-EXCLUDED",
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe,
		CompleteTime:    common.GetTimestamp(),
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, DB.Create(stripeTopUp).Error)
	require.NoError(t, CreateAffiliateCommissionForTopUp(stripeTopUp))

	var count int64
	require.NoError(t, DB.Model(&AffiliateCommission{}).Count(&count).Error)
	assert.Zero(t, count)
	account, err := GetAffiliateAccount(inviter.Id)
	require.NoError(t, err)
	assert.Zero(t, account.AvailableCents)
}

func TestAffiliateCommissionCannotApproveInvalidatedTopUp(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	inviter := &User{
		Username: "affiliate-invalid-owner",
		Email:    "invalid-owner@example.com",
		AffCode:  "affiliate-invalid-owner-code",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(inviter).Error)
	invitee := &User{
		Username:  "affiliate-invalid-buyer",
		Email:     "invalid-buyer@example.com",
		AffCode:   "affiliate-invalid-buyer-code",
		InviterId: inviter.Id,
		Status:    common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(invitee).Error)
	topUp := &TopUp{
		UserId:          invitee.Id,
		Money:           100,
		TradeNo:         "AFF-INVALIDATED-ORDER",
		PaymentProvider: PaymentProviderEpay,
		CompleteTime:    common.GetTimestamp(),
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, DB.Create(topUp).Error)
	require.NoError(t, CreateAffiliateCommissionForTopUp(topUp))
	record := AffiliateCommission{}
	require.NoError(t, DB.First(&record).Error)

	require.NoError(t, DB.Model(topUp).Update("status", common.TopUpStatusFailed).Error)
	require.ErrorIs(t, CompleteAffiliateCommission(record.Id, 99, true, ""), ErrAffiliateTopUpInvalid)

	require.NoError(t, DB.First(&record, record.Id).Error)
	assert.Equal(t, AffiliateCommissionStatusPending, record.Status)
	account, err := GetAffiliateAccount(inviter.Id)
	require.NoError(t, err)
	assert.Zero(t, account.AvailableCents)
}

func TestAffiliateUpgradeNoticeCreatedOnceAtConfiguredThreshold(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateUpgradeInviteesThresholdOptionKey, "2")
	inviter := &User{
		Username: "affiliate-upgrade-owner",
		Email:    "upgrade-owner@example.com",
		AffCode:  "affiliate-upgrade-owner-code",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(inviter).Error)

	for index := 1; index <= 3; index++ {
		invitee := &User{
			Username:  fmt.Sprintf("affiliate-upgrade-buyer-%d", index),
			Email:     fmt.Sprintf("upgrade-buyer-%d@example.com", index),
			AffCode:   fmt.Sprintf("affiliate-upgrade-buyer-code-%d", index),
			InviterId: inviter.Id,
			Status:    common.UserStatusEnabled,
		}
		require.NoError(t, DB.Create(invitee).Error)
		topUp := &TopUp{
			UserId:          invitee.Id,
			Money:           100,
			TradeNo:         fmt.Sprintf("AFF-UPGRADE-%d", index),
			PaymentProvider: PaymentProviderEpay,
			CompleteTime:    common.GetTimestamp(),
			Status:          common.TopUpStatusSuccess,
		}
		require.NoError(t, DB.Create(topUp).Error)
		require.NoError(t, CreateAffiliateCommissionForTopUp(topUp))
	}

	var notices []AffiliateUpgradeNotice
	require.NoError(t, DB.Find(&notices).Error)
	require.Len(t, notices, 1)
	assert.Equal(t, 2, notices[0].Threshold)
	assert.Equal(t, 2, notices[0].EffectiveInviteeCount)
}

func TestAffiliateCommissionActivationCutoffPreventsHistoricalBackfill(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateCommissionActivatedAtOptionKey, "100")

	inviter := &User{Username: "activation-owner", Email: "activation-owner@example.com", AffCode: "activation-owner-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(inviter).Error)
	invitee := &User{Username: "activation-buyer", Email: "activation-buyer@example.com", AffCode: "activation-buyer-code", InviterId: inviter.Id, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(invitee).Error)

	for _, item := range []struct {
		tradeNo string
		time    int64
	}{
		{tradeNo: "AFF-BEFORE-ACTIVATION", time: 99},
		{tradeNo: "AFF-AT-ACTIVATION", time: 100},
	} {
		topUp := &TopUp{UserId: invitee.Id, Money: 100, TradeNo: item.tradeNo, PaymentProvider: PaymentProviderEpay, CompleteTime: item.time, Status: common.TopUpStatusSuccess}
		require.NoError(t, DB.Create(topUp).Error)
		require.NoError(t, CreateAffiliateCommissionForTopUp(topUp))
	}

	var records []AffiliateCommission
	require.NoError(t, DB.Find(&records).Error)
	require.Len(t, records, 1)
	assert.Equal(t, "AFF-AT-ACTIVATION", records[0].TradeNo)
}

func TestAffiliateCommissionSkipsZeroCommissionAndDeletedInviter(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateCommissionGroupRatesOptionKey, `{"default":1,"高级推广":1,"金牌推广":1}`)

	inviter := &User{Username: "skip-owner", Email: "skip-owner@example.com", Group: AffiliatePromoterGroupDefault, AffCode: "skip-owner-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(inviter).Error)
	invitee := &User{Username: "skip-buyer", Email: "skip-buyer@example.com", AffCode: "skip-buyer-code", InviterId: inviter.Id, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(invitee).Error)
	tiny := &TopUp{UserId: invitee.Id, Money: 0.01, TradeNo: "AFF-ZERO-COMMISSION", PaymentProvider: PaymentProviderEpay, CompleteTime: common.GetTimestamp(), Status: common.TopUpStatusSuccess}
	require.NoError(t, DB.Create(tiny).Error)
	require.NoError(t, CreateAffiliateCommissionForTopUp(tiny))

	require.NoError(t, DB.Delete(inviter).Error)
	normal := &TopUp{UserId: invitee.Id, Money: 100, TradeNo: "AFF-DELETED-INVITER", PaymentProvider: PaymentProviderEpay, CompleteTime: common.GetTimestamp(), Status: common.TopUpStatusSuccess}
	require.NoError(t, DB.Create(normal).Error)
	require.NoError(t, CreateAffiliateCommissionForTopUp(normal))

	var count int64
	require.NoError(t, DB.Model(&AffiliateCommission{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestEpayRechargeCompletesWhenCommissionIsZeroOrInviterDeleted(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateCommissionGroupRatesOptionKey, `{"default":1,"高级推广":1,"金牌推广":1}`)

	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	inviter := &User{Username: "epay-business-owner", Email: "epay-business-owner@example.com", Group: AffiliatePromoterGroupDefault, AffCode: "epay-business-owner-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(inviter).Error)
	invitee := &User{Username: "epay-business-buyer", Email: "epay-business-buyer@example.com", AffCode: "epay-business-buyer-code", InviterId: inviter.Id, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(invitee).Error)

	tiny := &TopUp{UserId: invitee.Id, Amount: 1, Money: 0.01, TradeNo: "EPAY-AFF-ZERO", PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay, CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending}
	require.NoError(t, DB.Create(tiny).Error)
	alreadyDone, err := RechargeEpay(tiny.TradeNo, "alipay", tiny.Money, "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	assert.Equal(t, common.TopUpStatusSuccess, GetTopUpByTradeNo(tiny.TradeNo).Status)
	assert.Equal(t, 100, getUserQuotaForPaymentGuardTest(t, invitee.Id))

	require.NoError(t, DB.Delete(inviter).Error)
	normal := &TopUp{UserId: invitee.Id, Amount: 2, Money: 100, TradeNo: "EPAY-AFF-DELETED", PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay, CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending}
	require.NoError(t, DB.Create(normal).Error)
	alreadyDone, err = RechargeEpay(normal.TradeNo, "alipay", normal.Money, "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	assert.Equal(t, common.TopUpStatusSuccess, GetTopUpByTradeNo(normal.TradeNo).Status)
	assert.Equal(t, 300, getUserQuotaForPaymentGuardTest(t, invitee.Id))

	var count int64
	require.NoError(t, DB.Model(&AffiliateCommission{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestAffiliateCommissionConcurrentApprovalCreditsExactlyOnce(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	inviter, _, _, record := createAffiliateFixture(t, "concurrent", 100)

	start := make(chan struct{})
	errorsCh := make(chan error, 2)
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func(operatorId int) {
			defer wg.Done()
			<-start
			errorsCh <- CompleteAffiliateCommission(record.Id, operatorId, true, "")
		}(index + 1)
	}
	close(start)
	wg.Wait()
	close(errorsCh)

	successes := 0
	for err := range errorsCh {
		if err == nil {
			successes++
			continue
		}
		assert.ErrorIs(t, err, ErrAffiliateCommissionStatusInvalid)
	}
	assert.Equal(t, 1, successes)
	account, err := GetAffiliateAccount(inviter.Id)
	require.NoError(t, err)
	assert.Equal(t, record.CommissionCents, account.AvailableCents)
	assert.Equal(t, record.CommissionCents, account.LifetimeEarnedCents)
}

func TestAffiliateUpgradeCandidateApprovalAndNotificationRetry(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateUpgradeInviteesThresholdOptionKey, "1")
	setAffiliateOptionForTest(t, AffiliateGoldUpgradeInviteesThresholdOptionKey, "2")
	inviter, _, _, _ := createAffiliateFixture(t, "upgrade-candidate", 100)

	candidates, total, err := ListAffiliateUpgradeCandidates(&common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, candidates, 1)
	assert.Equal(t, inviter.Id, candidates[0].InviterId)
	assert.Equal(t, "高级推广", candidates[0].NextGroup)
	approvedGroup, err := ApproveAffiliateUpgrade(inviter.Id, AffiliatePromoterGroupAdvanced)
	require.NoError(t, err)
	assert.Equal(t, AffiliatePromoterGroupAdvanced, approvedGroup)
	require.NoError(t, DB.First(inviter, inviter.Id).Error)
	assert.Equal(t, "高级推广", inviter.Group)
	_, err = ApproveAffiliateUpgrade(inviter.Id, AffiliatePromoterGroupAdvanced)
	require.ErrorIs(t, err, ErrAffiliateUpgradeNotEligible)

	notice := &AffiliateUpgradeNotice{}
	require.NoError(t, DB.Where("inviter_id = ?", inviter.Id).First(notice).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Model(notice).Updates(map[string]interface{}{"last_attempt_time": now, "next_attempt_time": now - 1}).Error)
	claimed, err := ClaimAffiliateUpgradeNotice(notice.Id, now, now-300)
	require.NoError(t, err)
	assert.True(t, claimed)
	require.NoError(t, DB.Model(notice).Update("attempt_count", 8).Error)
	require.NoError(t, RecordAffiliateUpgradeNoticeFailure(notice.Id, "smtp unavailable"))
	require.NoError(t, DB.First(notice, notice.Id).Error)
	assert.Positive(t, notice.DeadLetterTime)
	assert.Equal(t, "smtp unavailable", notice.LastError)
	require.NoError(t, RetryAffiliateUpgradeNotice(notice.Id))
	require.NoError(t, DB.First(notice, notice.Id).Error)
	assert.Zero(t, notice.AttemptCount)
	assert.Zero(t, notice.DeadLetterTime)
	assert.Empty(t, notice.LastError)
}

func TestAffiliateUpgradeDoesNotReplaceUnrelatedUserGroups(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateUpgradeInviteesThresholdOptionKey, "1")
	setAffiliateOptionForTest(t, AffiliateGoldUpgradeInviteesThresholdOptionKey, "2")
	inviter, _, _, _ := createAffiliateFixture(t, "unrelated-group", 100)
	require.NoError(t, DB.Model(inviter).Update("group", "vip").Error)

	candidates, total, err := ListAffiliateUpgradeCandidates(&common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, candidates)
	_, err = ApproveAffiliateUpgrade(inviter.Id, AffiliatePromoterGroupAdvanced)
	assert.ErrorIs(t, err, ErrAffiliateUpgradeNotEligible)

	require.NoError(t, DB.First(inviter, inviter.Id).Error)
	assert.Equal(t, "vip", inviter.Group)
}

func TestAffiliateSettingsRequireAllCoreGroupsAndRejectReason(t *testing.T) {
	assert.NoError(t, ValidateAffiliateOptionValue(AffiliateCommissionAutoApproveOptionKey, "true"))
	assert.Error(t, ValidateAffiliateOptionValue(AffiliateCommissionAutoApproveOptionKey, "automatic"))
	assert.Error(t, ValidateAffiliateOptionValue(AffiliateCommissionGroupRatesOptionKey, `{"default":500,"高级推广":1000}`))
	assert.NoError(t, ValidateAffiliateOptionValue(AffiliateCommissionGroupRatesOptionKey, `{"default":550,"高级推广":1000,"金牌推广":1500}`))
	assert.NoError(t, ValidateAffiliateUpgradeThresholds(50, 500, 200000, 2000000))
	assert.Error(t, ValidateAffiliateUpgradeThresholds(50, 50, 200000, 2000000))
	assert.Error(t, ValidateAffiliateUpgradeThresholds(500, 50, 200000, 2000000))
	assert.Error(t, ValidateAffiliateUpgradeThresholds(50, 500, 200000, 200000))
	assert.ErrorIs(t, CompleteAffiliateCommission(1, 99, false, ""), ErrAffiliateRejectReasonRequired)
}

func TestAffiliateAdvancedPromoterRequiresConfiguredThresholdBeforeGoldReview(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateUpgradeInviteesThresholdOptionKey, "1")
	setAffiliateOptionForTest(t, AffiliateGoldUpgradeInviteesThresholdOptionKey, "2")

	inviter, _, _, _ := createAffiliateFixture(t, "gold-upgrade-first", 100)
	approvedGroup, err := ApproveAffiliateUpgrade(inviter.Id, AffiliatePromoterGroupAdvanced)
	require.NoError(t, err)
	assert.Equal(t, AffiliatePromoterGroupAdvanced, approvedGroup)

	summary, err := GetAffiliateSummary(inviter.Id)
	require.NoError(t, err)
	assert.True(t, summary.UpgradeEligible)
	assert.Equal(t, AffiliatePromoterGroupGold, summary.NextTierName)
	assert.Equal(t, 2, summary.UpgradeThreshold)
	assert.Equal(t, int64(1), summary.UpgradeProgress)

	candidates, total, err := ListAffiliateUpgradeCandidates(&common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, candidates)

	invitee := &User{
		Username:  "affiliate-fixture-buyer-gold-upgrade-second",
		Email:     "affiliate-fixture-buyer-gold-upgrade-second@example.com",
		AffCode:   "affiliate-fixture-buyer-code-gold-upgrade-second",
		InviterId: inviter.Id,
		Status:    common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(invitee).Error)
	topUp := &TopUp{
		UserId:          invitee.Id,
		Money:           100,
		TradeNo:         "AFF-FIXTURE-gold-upgrade-second",
		PaymentProvider: PaymentProviderEpay,
		CompleteTime:    common.GetTimestamp(),
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, DB.Create(topUp).Error)
	require.NoError(t, CreateAffiliateCommissionForTopUp(topUp))

	candidates, total, err = ListAffiliateUpgradeCandidates(&common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, candidates, 1)
	assert.Equal(t, AffiliatePromoterGroupAdvanced, candidates[0].CurrentGroup)
	assert.Equal(t, AffiliatePromoterGroupGold, candidates[0].NextGroup)
	assert.Equal(t, 2, candidates[0].Threshold)

	approvedGroup, err = ApproveAffiliateUpgrade(inviter.Id, AffiliatePromoterGroupGold)
	require.NoError(t, err)
	assert.Equal(t, AffiliatePromoterGroupGold, approvedGroup)
	require.NoError(t, DB.First(inviter, inviter.Id).Error)
	assert.Equal(t, AffiliatePromoterGroupGold, inviter.Group)

	var notices []AffiliateUpgradeNotice
	require.NoError(t, DB.Where("inviter_id = ?", inviter.Id).Order("threshold ASC").Find(&notices).Error)
	require.Len(t, notices, 2)
	assert.Equal(t, 1, notices[0].Threshold)
	assert.Equal(t, 2, notices[1].Threshold)
}

func TestAffiliateUpgradeCanQualifyByCumulativeTopUpAmount(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateUpgradeInviteesThresholdOptionKey, "50")
	setAffiliateOptionForTest(t, AffiliateGoldUpgradeInviteesThresholdOptionKey, "500")
	setAffiliateOptionForTest(t, AffiliateUpgradeTopUpAmountThresholdOptionKey, "200000")
	setAffiliateOptionForTest(t, AffiliateGoldUpgradeTopUpAmountThresholdOptionKey, "2000000")

	inviter, _, _, _ := createAffiliateFixture(t, "amount-upgrade-advanced", 2000)
	candidates, total, err := ListAffiliateUpgradeCandidates(&common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, candidates, 1)
	assert.False(t, candidates[0].EligibleByInvitees)
	assert.True(t, candidates[0].EligibleByTopUpAmount)
	assert.Equal(t, int64(200000), candidates[0].EffectiveTopUpAmountCents)
	assert.Equal(t, int64(200000), candidates[0].TopUpAmountThresholdCents)

	approvedGroup, err := ApproveAffiliateUpgrade(inviter.Id, AffiliatePromoterGroupAdvanced)
	require.NoError(t, err)
	assert.Equal(t, AffiliatePromoterGroupAdvanced, approvedGroup)

	invitee := &User{
		Username:  "affiliate-fixture-buyer-amount-upgrade-gold",
		Email:     "affiliate-fixture-buyer-amount-upgrade-gold@example.com",
		AffCode:   "affiliate-fixture-buyer-code-amount-upgrade-gold",
		InviterId: inviter.Id,
		Status:    common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(invitee).Error)
	topUp := &TopUp{
		UserId:          invitee.Id,
		Money:           18000,
		TradeNo:         "AFF-FIXTURE-amount-upgrade-gold",
		PaymentProvider: PaymentProviderEpay,
		CompleteTime:    common.GetTimestamp(),
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, DB.Create(topUp).Error)
	require.NoError(t, CreateAffiliateCommissionForTopUp(topUp))

	candidates, total, err = ListAffiliateUpgradeCandidates(&common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, candidates, 1)
	assert.Equal(t, AffiliatePromoterGroupGold, candidates[0].NextGroup)
	assert.False(t, candidates[0].EligibleByInvitees)
	assert.True(t, candidates[0].EligibleByTopUpAmount)
	assert.Equal(t, int64(2000000), candidates[0].EffectiveTopUpAmountCents)

	approvedGroup, err = ApproveAffiliateUpgrade(inviter.Id, AffiliatePromoterGroupGold)
	require.NoError(t, err)
	assert.Equal(t, AffiliatePromoterGroupGold, approvedGroup)
}

func TestAffiliateRejectedCommissionDoesNotCountTowardAmountUpgrade(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateUpgradeInviteesThresholdOptionKey, "50")
	setAffiliateOptionForTest(t, AffiliateGoldUpgradeInviteesThresholdOptionKey, "500")
	setAffiliateOptionForTest(t, AffiliateUpgradeTopUpAmountThresholdOptionKey, "200000")
	setAffiliateOptionForTest(t, AffiliateGoldUpgradeTopUpAmountThresholdOptionKey, "2000000")

	inviter, _, _, record := createAffiliateFixture(t, "amount-upgrade-rejected", 2000)
	require.NoError(t, CompleteAffiliateCommission(record.Id, 99, false, "risk review rejected"))

	metrics, err := GetAffiliateUpgradeMetrics(inviter.Id)
	require.NoError(t, err)
	assert.Zero(t, metrics.EffectiveInviteeCount)
	assert.Zero(t, metrics.EffectiveTopUpAmountCents)
	candidates, total, err := ListAffiliateUpgradeCandidates(&common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, candidates)
	_, err = ApproveAffiliateUpgrade(inviter.Id, AffiliatePromoterGroupAdvanced)
	assert.ErrorIs(t, err, ErrAffiliateUpgradeNotEligible)
}

func TestAffiliateAdvancedPromoterSummaryUsesFiveHundredInviteeDefault(t *testing.T) {
	truncateTables(t)
	enableAffiliateForTest(t)
	setAffiliateOptionForTest(t, AffiliateGoldUpgradeInviteesThresholdOptionKey, "invalid")
	inviter := &User{
		Username: "affiliate-default-gold-threshold",
		Email:    "affiliate-default-gold-threshold@example.com",
		Group:    AffiliatePromoterGroupAdvanced,
		AffCode:  "affiliate-default-gold-threshold-code",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(inviter).Error)

	summary, err := GetAffiliateSummary(inviter.Id)
	require.NoError(t, err)
	assert.True(t, summary.UpgradeEligible)
	assert.Equal(t, AffiliatePromoterGroupGold, summary.NextTierName)
	assert.Equal(t, 500, summary.UpgradeThreshold)
	assert.Equal(t, int64(2000000), summary.UpgradeTopUpAmountThresholdCents)
}

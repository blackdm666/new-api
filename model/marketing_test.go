package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMarketingAudienceUsesOnlySuccessfulEpayTopups(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	users := []*User{
		{Username: "single_epay", Password: "password", Email: "single@example.com", AffCode: "marketing-aff-1", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: 1, CreatedAt: now - 90*86400},
		{Username: "stripe_user", Password: "password", Email: "stripe@example.com", AffCode: "marketing-aff-2", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: 1, CreatedAt: now - 90*86400},
		{Username: "trial_user", Password: "password", Email: "trial@example.com", AffCode: "marketing-aff-3", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: 1, UsedQuota: 100, CreatedAt: now - 2*86400},
	}
	for _, user := range users {
		require.NoError(t, DB.Create(user).Error)
	}
	require.NoError(t, DB.Create(&TopUp{UserId: users[0].Id, Amount: 100, Money: 100, TradeNo: "EPAY-MARKETING-1", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusSuccess, CompleteTime: now - 31*86400}).Error)
	require.NoError(t, DB.Create(&TopUp{UserId: users[1].Id, Amount: 100, Money: 100, TradeNo: "STRIPE-MARKETING-1", PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusSuccess, CompleteTime: now - 31*86400}).Error)

	one := 1
	rows, total, err := ListMarketingAudience(MarketingAudienceRule{TopUpCountMin: &one, TopUpCountMax: &one, LastTopUpBefore: now - 30*86400}, 100, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, users[0].Id, rows[0].Id)

	zero := 0
	quotaMax := 10
	rows, total, err = ListMarketingAudience(MarketingAudienceRule{TopUpCountMax: &zero, QuotaMax: &quotaMax, UsedQuotaPositive: true}, 100, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, users[2].Id, rows[0].Id)
}

func TestMarketingRecipientDedupeCooldownAndScenePriority(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	paid := &MarketingCampaign{Name: "paid", Scene: MarketingScenePaidLowBalance, Status: MarketingCampaignStatusRunning, AudienceRule: "{}", LocalizedContent: "{}", ActionPath: "/wallet", Automatic: true}
	inactive := &MarketingCampaign{Name: "inactive", Scene: MarketingSceneInactive, Status: MarketingCampaignStatusRunning, AudienceRule: "{}", LocalizedContent: "{}", ActionPath: "/dashboard/overview", Automatic: true}
	require.NoError(t, DB.Create(paid).Error)
	require.NoError(t, DB.Create(inactive).Error)
	low := &MarketingRecipient{CampaignId: paid.Id, UserId: 42, DedupeKey: "paid-42", Language: "zh-CN", RecipientMasked: "u**r@example.com", ClickTokenHash: HashMarketingToken("paid-token"), Status: MarketingRecipientStatusPending, CreatedTime: now}
	old := &MarketingRecipient{CampaignId: inactive.Id, UserId: 42, DedupeKey: "inactive-42", Language: "zh-CN", RecipientMasked: "u**r@example.com", ClickTokenHash: HashMarketingToken("inactive-token"), Status: MarketingRecipientStatusPending, CreatedTime: now}
	created, err := CreateMarketingRecipient(old)
	require.NoError(t, err)
	assert.True(t, created)
	created, err = CreateMarketingRecipient(low)
	require.NoError(t, err)
	assert.True(t, created, "the same user may have separate lifecycle stages")
	created, err = CreateMarketingRecipient(&MarketingRecipient{CampaignId: paid.Id, UserId: 42, DedupeKey: "paid-42", Language: "zh-CN", RecipientMasked: "u**r@example.com", ClickTokenHash: HashMarketingToken("duplicate-token"), Status: MarketingRecipientStatusPending})
	require.NoError(t, err)
	assert.False(t, created)

	rows, err := ListPendingMarketingRecipients(10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, low.Id, rows[0].Id)
	assert.False(t, UserMarketingCooldownActive(42, low.Id, now-7*86400))
	require.NoError(t, UpdateMarketingRecipientQueued(low.Id, 99, now))
	assert.True(t, UserMarketingCooldownActive(42, old.Id, now-7*86400))
}

func TestMarketingClickAndEpayConversionAreIdempotent(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	campaign := &MarketingCampaign{Name: "conversion", Scene: MarketingSceneCustom, Status: MarketingCampaignStatusRunning, AudienceRule: "{}", LocalizedContent: "{}", ActionPath: "/wallet"}
	require.NoError(t, DB.Create(campaign).Error)
	recipient := &MarketingRecipient{CampaignId: campaign.Id, UserId: 7, DedupeKey: "conversion-7", Language: "zh-CN", RecipientMasked: "u**r@example.com", ClickTokenHash: HashMarketingToken("click-7"), Status: MarketingRecipientStatusDelivered, DeliveredTime: now - 100}
	require.NoError(t, DB.Create(recipient).Error)
	require.NoError(t, RecordMarketingClick(recipient))
	require.NoError(t, RecordMarketingClick(recipient))
	topUp := &TopUp{UserId: 7, Money: 995, TradeNo: "EPAY-CONVERSION-1", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusSuccess, CompleteTime: now}
	require.NoError(t, DB.Create(topUp).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return AttributeMarketingConversionTx(tx, topUp) }))
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return AttributeMarketingConversionTx(tx, topUp) }))
	secondTopUp := &TopUp{UserId: 7, Money: 300, TradeNo: "EPAY-CONVERSION-2", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusSuccess, CompleteTime: now + 10}
	require.NoError(t, DB.Create(secondTopUp).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return AttributeMarketingConversionTx(tx, secondTopUp) }))
	var clickCount, conversionCount int64
	require.NoError(t, DB.Model(&MarketingEvent{}).Where("event_type = ?", MarketingEventClick).Count(&clickCount).Error)
	require.NoError(t, DB.Model(&MarketingEvent{}).Where("event_type = ?", MarketingEventConversion).Count(&conversionCount).Error)
	assert.Equal(t, int64(1), clickCount)
	assert.Equal(t, int64(1), conversionCount)
	var event MarketingEvent
	require.NoError(t, DB.Where("event_type = ?", MarketingEventConversion).First(&event).Error)
	assert.Equal(t, int64(99500), event.AmountCents)
}

func TestMarketingConversionDoesNotFallBackToAnOlderClick(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	campaign := &MarketingCampaign{Name: "last click", Scene: MarketingSceneCustom, Status: MarketingCampaignStatusRunning, AudienceRule: "{}", LocalizedContent: "{}", ActionPath: "/wallet"}
	require.NoError(t, DB.Create(campaign).Error)
	older := &MarketingRecipient{CampaignId: campaign.Id, UserId: 17, DedupeKey: "older-click", Language: "zh-CN", RecipientMasked: "u***@example.com", ClickTokenHash: HashMarketingToken("older-click"), Status: MarketingRecipientStatusDelivered, ClickedTime: now - 200}
	latest := &MarketingRecipient{CampaignId: campaign.Id, UserId: 17, DedupeKey: "latest-click", Language: "zh-CN", RecipientMasked: "u***@example.com", ClickTokenHash: HashMarketingToken("latest-click"), Status: MarketingRecipientStatusDelivered, ClickedTime: now - 100}
	require.NoError(t, DB.Create(older).Error)
	require.NoError(t, DB.Create(latest).Error)
	first := &TopUp{UserId: 17, Money: 100, TradeNo: "EPAY-LAST-CLICK-1", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusSuccess, CompleteTime: now}
	require.NoError(t, DB.Create(first).Error)
	require.NoError(t, AttributeMarketingConversion(first))
	second := &TopUp{UserId: 17, Money: 200, TradeNo: "EPAY-LAST-CLICK-2", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusSuccess, CompleteTime: now + 10}
	require.NoError(t, DB.Create(second).Error)
	require.NoError(t, AttributeMarketingConversion(second))
	var events int64
	require.NoError(t, DB.Model(&MarketingEvent{}).Where("event_type = ?", MarketingEventConversion).Count(&events).Error)
	assert.Equal(t, int64(1), events)
	require.NoError(t, DB.First(older, older.Id).Error)
	assert.Zero(t, older.ConvertedTime)
}

func TestCancellingMarketingCampaignExpiresQueuedPayload(t *testing.T) {
	truncateTables(t)
	campaign := &MarketingCampaign{Name: "cancel", Scene: MarketingSceneCustom, Status: MarketingCampaignStatusRunning, AudienceRule: "{}", LocalizedContent: "{}", ActionPath: "/wallet"}
	require.NoError(t, DB.Create(campaign).Error)
	delivery, _, err := EnqueueEmailDelivery(&EmailDelivery{DeliveryKey: "marketing-cancel", Category: "marketing_custom", RelatedId: campaign.Id, UserId: 22, Recipient: "cancel@example.com", Subject: "subject", Body: "sensitive body", Priority: EmailPriorityMarketing})
	require.NoError(t, err)
	recipient := &MarketingRecipient{CampaignId: campaign.Id, UserId: 22, DedupeKey: "cancel-22", Language: "zh-CN", RecipientMasked: "c***@example.com", ClickTokenHash: HashMarketingToken("cancel-token"), EmailDeliveryId: delivery.Id, Status: MarketingRecipientStatusQueued}
	require.NoError(t, DB.Create(recipient).Error)

	require.NoError(t, SetMarketingCampaignStatus(campaign.Id, []string{MarketingCampaignStatusRunning}, MarketingCampaignStatusCancelled, ""))
	delivery, err = GetEmailDeliveryById(delivery.Id)
	require.NoError(t, err)
	assert.NotZero(t, delivery.ExpiredTime)
	assert.Empty(t, delivery.Recipient)
	assert.Empty(t, delivery.Subject)
	assert.Empty(t, delivery.Body)
	recipient, err = GetMarketingRecipient(recipient.Id)
	require.NoError(t, err)
	assert.Equal(t, MarketingRecipientStatusSkipped, recipient.Status)
}

func TestDisablingAutomationPausesAndReenablingResumesQueuedCampaign(t *testing.T) {
	truncateTables(t)
	require.NoError(t, EnsureMarketingAutomations())
	content, err := common.Marshal(DefaultMarketingContents()[MarketingScenePaidLowBalance])
	require.NoError(t, err)
	require.NoError(t, UpdateMarketingAutomation(MarketingScenePaidLowBalance, true, true, string(content)))
	campaign := &MarketingCampaign{Name: "automatic", Scene: MarketingScenePaidLowBalance, Status: MarketingCampaignStatusRunning, AudienceRule: "{}", LocalizedContent: string(content), ActionPath: "/wallet", Automatic: true}
	require.NoError(t, DB.Create(campaign).Error)
	delivery, _, err := EnqueueEmailDelivery(&EmailDelivery{DeliveryKey: "marketing-automation-pause", Category: "marketing_paid_low_balance", RelatedId: campaign.Id, UserId: 23, Recipient: "pause@example.com", Subject: "subject", Body: "body", Priority: EmailPriorityMarketing})
	require.NoError(t, err)
	recipient := &MarketingRecipient{CampaignId: campaign.Id, UserId: 23, DedupeKey: "pause-23", Language: "zh-CN", RecipientMasked: "p***@example.com", ClickTokenHash: HashMarketingToken("pause-token"), EmailDeliveryId: delivery.Id, Status: MarketingRecipientStatusQueued}
	require.NoError(t, DB.Create(recipient).Error)

	require.NoError(t, UpdateMarketingAutomation(MarketingScenePaidLowBalance, false, false, string(content)))
	require.NoError(t, DB.First(campaign, campaign.Id).Error)
	assert.Equal(t, MarketingCampaignStatusPaused, campaign.Status)
	require.NoError(t, DeferEmailDelivery(delivery.Id, common.GetTimestamp()+3600))

	require.NoError(t, UpdateMarketingAutomation(MarketingScenePaidLowBalance, true, true, string(content)))
	require.NoError(t, DB.First(campaign, campaign.Id).Error)
	assert.Equal(t, MarketingCampaignStatusRunning, campaign.Status)
	require.NoError(t, DB.First(delivery, delivery.Id).Error)
	assert.LessOrEqual(t, delivery.NextAttemptTime, common.GetTimestamp())
}

func TestPermanentMarketingFailureCreatesSuppression(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	campaign := &MarketingCampaign{Name: "failure", Scene: MarketingSceneCustom, Status: MarketingCampaignStatusRunning, AudienceRule: "{}", LocalizedContent: "{}", ActionPath: "/wallet"}
	require.NoError(t, DB.Create(campaign).Error)
	delivery, _, err := EnqueueEmailDelivery(&EmailDelivery{DeliveryKey: "marketing-failure", Category: "marketing_custom", UserId: 9, Recipient: "gone@example.com", Subject: "subject", Body: "body", Priority: EmailPriorityMarketing})
	require.NoError(t, err)
	require.NoError(t, DB.Model(delivery).Updates(map[string]any{"attempts": EmailDeliveryMaxAttempts, "dead_letter_time": now, "last_error": "SMTP 550 5.1.1 user unknown"}).Error)
	recipient := &MarketingRecipient{CampaignId: campaign.Id, UserId: 9, DedupeKey: "failure-9", Language: "zh-CN", RecipientMasked: "g**e@example.com", ClickTokenHash: HashMarketingToken("failure-token"), EmailDeliveryId: delivery.Id, Status: MarketingRecipientStatusQueued}
	require.NoError(t, DB.Create(recipient).Error)
	require.NoError(t, ReconcileMarketingRecipients(10))
	assert.True(t, IsMarketingSuppressed(9, "gone@example.com"))
}

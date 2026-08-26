package model

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

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

func TestMarketingAutomationAudiencesUseActivationAndAffiliateSignals(t *testing.T) {
	truncateTables(t)
	setAffiliateOptionForTest(t, AffiliateCommissionEnabledOptionKey, "true")
	now := common.GetTimestamp()
	users := []*User{
		{Username: "registration_pending", Password: "password", Email: "registration-pending@example.com", AffCode: "marketing-activation-aff-1", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", RequestCount: 0, CreatedAt: now - 48*3600},
		{Username: "registration_done", Password: "password", Email: "registration-done@example.com", AffCode: "marketing-activation-aff-2", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", RequestCount: 1, CreatedAt: now - 48*3600},
		{Username: "affiliate_active", Password: "password", Email: "affiliate-active@example.com", AffCode: "marketing-activation-aff-3", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", RequestCount: 20, CreatedAt: now - 90*86400},
		{Username: "affiliate_inactive", Password: "password", Email: "affiliate-inactive@example.com", AffCode: "marketing-activation-aff-4", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", RequestCount: 20, CreatedAt: now - 90*86400},
		{Username: "affiliate_unpaid", Password: "password", Email: "affiliate-unpaid@example.com", AffCode: "marketing-activation-aff-5", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", RequestCount: 20, CreatedAt: now - 90*86400},
	}
	for _, user := range users {
		require.NoError(t, DB.Create(user).Error)
	}
	require.NoError(t, DB.Create(&Token{UserId: users[2].Id, Key: "affiliate-active-token", Name: "active", Status: common.TokenStatusEnabled, CreatedTime: now - 60*86400, AccessedTime: now - 2*86400}).Error)
	require.NoError(t, DB.Create(&Token{UserId: users[3].Id, Key: "affiliate-inactive-token", Name: "inactive", Status: common.TokenStatusEnabled, CreatedTime: now - 90*86400, AccessedTime: now - 45*86400}).Error)
	require.NoError(t, DB.Create(&Token{UserId: users[4].Id, Key: "affiliate-unpaid-token", Name: "unpaid", Status: common.TokenStatusEnabled, CreatedTime: now - 60*86400, AccessedTime: now - 2*86400}).Error)
	for index, user := range users[2:4] {
		require.NoError(t, DB.Create(&TopUp{UserId: user.Id, Amount: 100, Money: 100, TradeNo: "EPAY-AFFILIATE-ACTIVATION-" + strconv.Itoa(index+1), PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusSuccess, CompleteTime: now - 40*86400}).Error)
	}

	zero := 0
	rows, total, err := ListMarketingAudience(MarketingAudienceRule{CreatedBefore: now - 24*3600, RequestCountMax: &zero}, 100, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, users[0].Id, rows[0].Id)

	one := 1
	ten := 10
	rows, total, err = ListMarketingAudience(MarketingAudienceRule{RequestCountMin: &ten, LastAPIUseAfter: now - 30*86400, RequireAffiliateEnabled: true, TopUpCountMin: &one}, 100, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, users[2].Id, rows[0].Id)
	assert.Equal(t, int64(now-2*86400), rows[0].LastAPIUseTime)
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

func TestListMarketingRecipientsFiltersEngagementAndIncludesUsername(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	user := &User{Username: "sending_record_user", Password: "password", Email: "sending-record@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, DB.Create(user).Error)
	campaign := &MarketingCampaign{Name: "sending records", Scene: MarketingSceneCustom, Status: MarketingCampaignStatusCompleted, AudienceRule: "{}", LocalizedContent: "{}", ActionPath: "/wallet"}
	require.NoError(t, DB.Create(campaign).Error)
	recipients := []*MarketingRecipient{
		{CampaignId: campaign.Id, UserId: user.Id, DedupeKey: "sending-record-unopened", Language: "zh-CN", RecipientMasked: "s***d@example.com", ClickTokenHash: HashMarketingToken("sending-record-unopened"), Status: MarketingRecipientStatusDelivered},
		{CampaignId: campaign.Id, UserId: user.Id, DedupeKey: "sending-record-clicked", Language: "zh-CN", RecipientMasked: "s***d@example.com", ClickTokenHash: HashMarketingToken("sending-record-clicked"), Status: MarketingRecipientStatusDelivered, ClickedTime: now},
		{CampaignId: campaign.Id, UserId: user.Id, DedupeKey: "sending-record-converted", Language: "zh-CN", RecipientMasked: "s***d@example.com", ClickTokenHash: HashMarketingToken("sending-record-converted"), Status: MarketingRecipientStatusDelivered, ClickedTime: now, ConvertedTime: now},
	}
	for _, recipient := range recipients {
		require.NoError(t, DB.Create(recipient).Error)
	}

	rows, total, err := ListMarketingRecipients(campaign.Id, "", &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, rows, 3)
	assert.Equal(t, user.Username, rows[0].Username)

	rows, total, err = ListMarketingRecipients(campaign.Id, MarketingRecipientEngagementClicked, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, rows, 2)

	rows, total, err = ListMarketingRecipients(campaign.Id, MarketingRecipientEngagementConverted, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, recipients[2].Id, rows[0].Id)

	_, _, err = ListMarketingRecipients(campaign.Id, "unknown", &common.PageInfo{Page: 1, PageSize: 10})
	assert.ErrorIs(t, err, ErrMarketingInvalid)
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
	content, err := common.Marshal(DefaultMarketingContents()[MarketingSceneInactive])
	require.NoError(t, err)
	require.NoError(t, UpdateMarketingAutomation(MarketingSceneInactive, true, true, string(content), ""))
	campaign := &MarketingCampaign{Name: "automatic", Scene: MarketingSceneInactive, Status: MarketingCampaignStatusRunning, AudienceRule: "{}", LocalizedContent: string(content), ActionPath: "/dashboard/overview", Automatic: true}
	require.NoError(t, DB.Create(campaign).Error)
	delivery, _, err := EnqueueEmailDelivery(&EmailDelivery{DeliveryKey: "marketing-automation-pause", Category: "marketing_inactive", RelatedId: campaign.Id, UserId: 23, Recipient: "pause@example.com", Subject: "subject", Body: "body", Priority: EmailPriorityMarketing})
	require.NoError(t, err)
	recipient := &MarketingRecipient{CampaignId: campaign.Id, UserId: 23, DedupeKey: "pause-23", Language: "zh-CN", RecipientMasked: "p***@example.com", ClickTokenHash: HashMarketingToken("pause-token"), EmailDeliveryId: delivery.Id, Status: MarketingRecipientStatusQueued}
	require.NoError(t, DB.Create(recipient).Error)

	require.NoError(t, UpdateMarketingAutomation(MarketingSceneInactive, false, false, string(content), ""))
	require.NoError(t, DB.First(campaign, campaign.Id).Error)
	assert.Equal(t, MarketingCampaignStatusPaused, campaign.Status)
	require.NoError(t, DeferEmailDelivery(delivery.Id, common.GetTimestamp()+3600))

	require.NoError(t, UpdateMarketingAutomation(MarketingSceneInactive, true, true, string(content), ""))
	require.NoError(t, DB.First(campaign, campaign.Id).Error)
	assert.Equal(t, MarketingCampaignStatusRunning, campaign.Status)
	require.NoError(t, DB.First(delivery, delivery.Id).Error)
	assert.LessOrEqual(t, delivery.NextAttemptTime, common.GetTimestamp())
}

func TestEnsureMarketingAutomationsRetiresBalanceScenes(t *testing.T) {
	truncateTables(t)
	content := `{"zh-CN":{"subject":"legacy","body":"legacy"},"en":{"subject":"legacy","body":"legacy"}}`
	legacy := &MarketingAutomation{Scene: MarketingScenePaidLowBalance, Enabled: true, ApplyExisting: true, BaselineReady: true, LocalizedContent: content}
	require.NoError(t, DB.Create(legacy).Error)
	campaign := &MarketingCampaign{Name: "legacy balance", Scene: MarketingScenePaidLowBalance, Status: MarketingCampaignStatusRunning, AudienceRule: "{}", LocalizedContent: content, ActionPath: "/wallet", Automatic: true}
	require.NoError(t, DB.Create(campaign).Error)
	delivery, _, err := EnqueueEmailDelivery(&EmailDelivery{DeliveryKey: "legacy-balance", Category: "marketing_paid_low_balance", RelatedId: campaign.Id, UserId: 23, Recipient: "legacy@example.com", Subject: "legacy", Body: "legacy", Priority: EmailPriorityMarketing})
	require.NoError(t, err)
	recipient := &MarketingRecipient{CampaignId: campaign.Id, UserId: 23, DedupeKey: "legacy-balance", Language: "zh-CN", RecipientMasked: "l***y@example.com", ClickTokenHash: HashMarketingToken("legacy-balance"), EmailDeliveryId: delivery.Id, Status: MarketingRecipientStatusQueued}
	require.NoError(t, DB.Create(recipient).Error)

	require.NoError(t, EnsureMarketingAutomations())
	require.NoError(t, DB.First(legacy, legacy.Id).Error)
	require.NoError(t, DB.First(campaign, campaign.Id).Error)
	require.NoError(t, DB.First(delivery, delivery.Id).Error)
	require.NoError(t, DB.First(recipient, recipient.Id).Error)
	assert.False(t, legacy.Enabled)
	assert.False(t, legacy.ApplyExisting)
	assert.False(t, legacy.BaselineReady)
	assert.Equal(t, MarketingCampaignStatusCancelled, campaign.Status)
	assert.NotZero(t, delivery.ExpiredTime)
	assert.Equal(t, MarketingRecipientStatusSkipped, recipient.Status)

	automations, err := ListMarketingAutomations()
	require.NoError(t, err)
	require.Len(t, automations, 5)
	assert.Equal(t, []string{MarketingSceneRegistration, MarketingSceneSingleTopUp, MarketingSceneInactive, MarketingSceneAffiliate, MarketingSceneAnnouncement}, []string{automations[0].Scene, automations[1].Scene, automations[2].Scene, automations[3].Scene, automations[4].Scene})
	assert.ErrorIs(t, UpdateMarketingAutomation(MarketingScenePaidLowBalance, true, true, content, ""), ErrMarketingInvalid)
}

func TestEnsureMarketingAutomationsRefreshesOnlyLegacyDefaultCopy(t *testing.T) {
	truncateTables(t)
	legacyRegistration, err := common.Marshal(legacyMarketingDefaultContents()[MarketingSceneRegistration])
	require.NoError(t, err)
	customSingleTopUp := `{"zh-CN":{"subject":"自定义主题","body":"自定义正文"},"en":{"subject":"Custom subject","body":"Custom body"}}`
	previousOptimizedAffiliate := `{"zh-CN":{"subject":"上一版默认主题","body":"上一版默认正文"}}`
	originalAffiliateHash := previousOptimizedMarketingContentHashes[MarketingSceneAffiliate]
	previousOptimizedMarketingContentHashes[MarketingSceneAffiliate] = hashMarketingValue(previousOptimizedAffiliate)
	t.Cleanup(func() { previousOptimizedMarketingContentHashes[MarketingSceneAffiliate] = originalAffiliateHash })
	registration := &MarketingAutomation{Scene: MarketingSceneRegistration, LocalizedContent: string(legacyRegistration)}
	custom := &MarketingAutomation{Scene: MarketingSceneSingleTopUp, LocalizedContent: customSingleTopUp}
	affiliate := &MarketingAutomation{Scene: MarketingSceneAffiliate, LocalizedContent: previousOptimizedAffiliate}
	require.NoError(t, DB.Create(registration).Error)
	require.NoError(t, DB.Create(custom).Error)
	require.NoError(t, DB.Create(affiliate).Error)
	campaign := &MarketingCampaign{Name: "legacy registration", Scene: MarketingSceneRegistration, Status: MarketingCampaignStatusRunning, AudienceRule: "{}", LocalizedContent: string(legacyRegistration), ActionPath: "/keys", Automatic: true}
	require.NoError(t, DB.Create(campaign).Error)

	require.NoError(t, EnsureMarketingAutomations())
	require.NoError(t, DB.First(registration, registration.Id).Error)
	require.NoError(t, DB.First(custom, custom.Id).Error)
	require.NoError(t, DB.First(affiliate, affiliate.Id).Error)
	require.NoError(t, DB.First(campaign, campaign.Id).Error)
	assert.Equal(t, customSingleTopUp, custom.LocalizedContent)
	assert.NotEqual(t, previousOptimizedAffiliate, affiliate.LocalizedContent)
	assert.Equal(t, registration.LocalizedContent, campaign.LocalizedContent)

	localized := map[string]MarketingLocalizedContent{}
	require.NoError(t, common.UnmarshalJsonStr(registration.LocalizedContent, &localized))
	require.Len(t, localized, 7)
	assert.Equal(t, "一个 API 密钥，连接更多主流模型", localized["zh-CN"].Subject)
	assert.Equal(t, "One API key, more leading models", localized["en"].Subject)
}

func TestOptimizedMarketingDefaultsCoverAllRecipientLanguages(t *testing.T) {
	contents := DefaultMarketingContents()
	languages := []string{"zh-CN", "zh-TW", "en", "fr", "ja", "ru", "vi"}
	scenes := []string{MarketingSceneRegistration, MarketingSceneSingleTopUp, MarketingSceneInactive, MarketingSceneAffiliate}
	for _, scene := range scenes {
		for _, language := range languages {
			content, ok := contents[scene][language]
			assert.True(t, ok, "%s should provide %s copy", scene, language)
			assert.NotEmpty(t, strings.TrimSpace(content.Subject))
			assert.LessOrEqual(t, utf8.RuneCountInString(content.Subject), 120)
			assert.NotEmpty(t, strings.TrimSpace(content.Body))
			assert.LessOrEqual(t, utf8.RuneCountInString(content.Body), 5000)
		}
	}
}

func TestMarketingAutomationTriggerConfigUsesSafeDefaultsAndValidation(t *testing.T) {
	_, registration, err := NormalizeMarketingAutomationTriggerConfig(MarketingSceneRegistration, "")
	require.NoError(t, err)
	assert.Equal(t, 24, registration.RegistrationWaitHours)
	assert.Equal(t, 1, registration.MaxSendsPerUser)
	assert.Equal(t, 2, registration.RepeatIntervalDays)

	encoded, config, err := NormalizeMarketingAutomationTriggerConfig(MarketingSceneInactive, "")
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)
	assert.Equal(t, 30, config.MatchDays)
	assert.Equal(t, 1, config.MaxSendsPerUser)
	assert.Equal(t, 30, config.RepeatIntervalDays)

	_, config, err = NormalizeMarketingAutomationTriggerConfig(MarketingSceneInactive, `{"match_days":45,"max_sends_per_user":2,"repeat_interval_days":14}`)
	require.NoError(t, err)
	assert.Equal(t, 45, config.MatchDays)
	assert.Equal(t, 2, config.MaxSendsPerUser)
	assert.Equal(t, 14, config.RepeatIntervalDays)

	_, _, err = NormalizeMarketingAutomationTriggerConfig(MarketingSceneAnnouncement, `{"expiry_hours":0}`)
	assert.ErrorIs(t, err, ErrMarketingInvalid)

	_, affiliate, err := NormalizeMarketingAutomationTriggerConfig(MarketingSceneAffiliate, `{"active_within_days":14,"min_request_count":25,"min_topup_count":2,"max_sends_per_user":1,"repeat_interval_days":30}`)
	require.NoError(t, err)
	assert.Equal(t, 14, affiliate.ActiveWithinDays)
	assert.Equal(t, 25, affiliate.MinRequestCount)
	assert.Equal(t, 2, affiliate.MinTopUpCount)

	_, _, err = NormalizeMarketingAutomationTriggerConfig(MarketingSceneAffiliate, `{"active_within_days":0,"min_request_count":10,"min_topup_count":1,"max_sends_per_user":1,"repeat_interval_days":30}`)
	assert.ErrorIs(t, err, ErrMarketingInvalid)
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

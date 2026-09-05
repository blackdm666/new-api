package service

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutomationStagesRespectLifecycleRules(t *testing.T) {
	now := time.Now().Unix()
	user := &model.MarketingAudienceUser{CreatedAt: now - 90*86400}
	oneShot := model.MarketingAutomationTriggerConfig{MatchDays: 30, MaxSendsPerUser: 1, RepeatIntervalDays: 30}
	stage, ok := automationStage(user, 0, 0, oneShot, now)
	assert.True(t, ok)
	assert.Equal(t, "1", stage)
	_, ok = automationStage(user, 1, now-31*86400, oneShot, now)
	assert.False(t, ok)

	repeating := model.MarketingAutomationTriggerConfig{MatchDays: 30, MaxSendsPerUser: 2, RepeatIntervalDays: 30}
	_, ok = automationStage(user, 1, now-10*86400, repeating, now)
	assert.False(t, ok)
	stage, ok = automationStage(user, 1, now-31*86400, repeating, now)
	assert.True(t, ok)
	assert.Equal(t, "2", stage)
}

func TestAutomationAudienceRulesUseFirstCallAndAffiliateActivitySignals(t *testing.T) {
	now := int64(1_800_000_000)
	registration := automationAudienceRule(model.MarketingSceneRegistration, model.MarketingAutomationTriggerConfig{RegistrationWaitHours: 24}, now)
	assert.Equal(t, now-24*3600, registration.CreatedBefore)
	require.NotNil(t, registration.RequestCountMax)
	assert.Zero(t, *registration.RequestCountMax)

	halfHourRegistration := automationAudienceRule(model.MarketingSceneRegistration, model.MarketingAutomationTriggerConfig{RegistrationWaitHours: 0.5}, now)
	assert.Equal(t, now-30*60, halfHourRegistration.CreatedBefore)

	affiliate := automationAudienceRule(model.MarketingSceneAffiliate, model.MarketingAutomationTriggerConfig{ActiveWithinDays: 30, MinRequestCount: 10, MinTopUpCount: 1}, now)
	assert.True(t, affiliate.RequireAffiliateEnabled)
	assert.Equal(t, now-30*86400, affiliate.LastAPIUseAfter)
	require.NotNil(t, affiliate.RequestCountMin)
	require.NotNil(t, affiliate.TopUpCountMin)
	assert.Equal(t, 10, *affiliate.RequestCountMin)
	assert.Equal(t, 1, *affiliate.TopUpCountMin)
}

func TestAutomationBaselineExcludesOnlyUsersMatchingAtEnableTime(t *testing.T) {
	truncate(t)
	now := time.Now().Unix()
	require.NoError(t, model.EnsureMarketingAutomations())
	require.NoError(t, model.UpdateMarketingAutomation(model.MarketingSceneInactive, true, false, mustMarketingContent(t, model.MarketingSceneInactive), ""))
	automation := &model.MarketingAutomation{}
	require.NoError(t, model.DB.Where("scene = ?", model.MarketingSceneInactive).First(automation).Error)
	assert.False(t, automation.BaselineReady)

	existing := &model.User{Username: "baseline_existing", Password: "password", Email: "baseline-existing@example.com", AffCode: "baseline-aff-existing", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", CreatedAt: now - 90*86400, LastLoginAt: now - 40*86400}
	future := &model.User{Username: "baseline_future", Password: "password", Email: "baseline-future@example.com", AffCode: "baseline-aff-future", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", CreatedAt: now - 90*86400, LastLoginAt: now}
	require.NoError(t, model.DB.Create(existing).Error)
	require.NoError(t, model.DB.Create(future).Error)
	campaign := &model.MarketingCampaign{Name: "inactive", Scene: model.MarketingSceneInactive, Status: model.MarketingCampaignStatusRunning, AudienceRule: "{}", LocalizedContent: automation.LocalizedContent, ActionPath: "/dashboard/overview", Automatic: true, StartedTime: now}
	require.NoError(t, model.CreateMarketingCampaign(campaign))
	_, triggerConfig, err := model.NormalizeMarketingAutomationTriggerConfig(automation.Scene, automation.TriggerConfig)
	require.NoError(t, err)
	require.NoError(t, captureMarketingAutomationBaseline(automation, campaign, triggerConfig, now))

	require.NoError(t, model.DB.Where("scene = ?", model.MarketingSceneInactive).First(automation).Error)
	assert.True(t, automation.BaselineReady)
	var existingCount, futureCount int64
	require.NoError(t, model.DB.Model(&model.MarketingRecipient{}).Where("user_id = ? AND status = ?", existing.Id, model.MarketingRecipientStatusSkipped).Count(&existingCount).Error)
	require.NoError(t, model.DB.Model(&model.MarketingRecipient{}).Where("user_id = ?", future.Id).Count(&futureCount).Error)
	assert.Equal(t, int64(1), existingCount)
	assert.Zero(t, futureCount)

	require.NoError(t, model.DB.Model(future).Update("last_login_at", now-31*86400).Error)
	require.NoError(t, materializeMarketingAutomations(now))
	var futureRecipient model.MarketingRecipient
	require.NoError(t, model.DB.Where("user_id = ? AND status = ?", future.Id, model.MarketingRecipientStatusPending).First(&futureRecipient).Error)
}

func mustMarketingContent(t *testing.T, scene string) string {
	t.Helper()
	encoded, err := common.Marshal(model.DefaultMarketingContents()[scene])
	require.NoError(t, err)
	return string(encoded)
}

func TestMarketingSendWindowUsesShanghaiTime(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	assert.False(t, marketingSendWindowOpen(time.Date(2026, 8, 16, 8, 59, 0, 0, location)))
	assert.True(t, marketingSendWindowOpen(time.Date(2026, 8, 16, 9, 0, 0, 0, location)))
	assert.True(t, marketingSendWindowOpen(time.Date(2026, 8, 16, 19, 59, 0, 0, location)))
	assert.False(t, marketingSendWindowOpen(time.Date(2026, 8, 16, 20, 0, 0, 0, location)))
	assert.Equal(t, time.Date(2026, 8, 16, 9, 0, 0, 0, location).Unix(), nextMarketingSendWindow(time.Date(2026, 8, 16, 8, 0, 0, 0, location)).Unix())
	assert.Equal(t, time.Date(2026, 8, 17, 9, 0, 0, 0, location).Unix(), nextMarketingSendWindow(time.Date(2026, 8, 16, 20, 0, 0, 0, location)).Unix())
}

func TestGetLatestMarketingAnnouncementUsesNewestPublishedEntry(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	settings := console_setting.GetConsoleSetting()
	original := settings.Announcements
	t.Cleanup(func() { settings.Announcements = original })
	settings.Announcements = `[
		{"id":1,"content":"Older announcement","publishDate":"2026-08-20T00:00:00Z","type":"default"},
		{"id":3,"content":"Future announcement","publishDate":"2026-08-24T00:00:00Z","type":"warning"},
		{"id":2,"content":"Latest published announcement","extra":"Additional details","publishDate":"2026-08-23T08:00:00Z","type":"success"}
	]`

	announcement := GetLatestMarketingAnnouncement(now.Unix())
	require.NotNil(t, announcement)
	assert.Equal(t, 2, announcement.Id)
	assert.Equal(t, "Latest published announcement", announcement.Content)
	assert.Equal(t, "Additional details", announcement.Extra)
	assert.Equal(t, "2026-08-23T08:00:00Z", announcement.PublishDate)
}

func TestAnnouncementAutomationBackfillsOnlyLatestPublishedEntry(t *testing.T) {
	truncate(t)
	now := common.GetTimestamp()
	settings := console_setting.GetConsoleSetting()
	original := settings.Announcements
	t.Cleanup(func() { settings.Announcements = original })
	settings.Announcements = `[
		{"id":1,"content":"Older announcement","publishDate":"2026-08-20T00:00:00Z","type":"default"},
		{"id":2,"content":"Latest announcement","publishDate":"2026-08-23T00:00:00Z","type":"success"}
	]`
	require.NoError(t, model.EnsureMarketingAutomations())
	require.NoError(t, model.UpdateMarketingAutomation(model.MarketingSceneAnnouncement, true, true, mustMarketingContent(t, model.MarketingSceneAnnouncement), `{"expiry_hours":24}`))
	automation, err := model.GetMarketingAutomation(model.MarketingSceneAnnouncement)
	require.NoError(t, err)
	// Treat both fixture announcements as historical relative to enable time.
	require.NoError(t, model.DB.Model(automation).Update("enabled_time", now).Error)
	automation.EnabledTime = now

	require.NoError(t, materializeAnnouncementCampaigns(automation, now))
	require.NoError(t, materializeAnnouncementCampaigns(automation, now))
	var campaigns []model.MarketingCampaign
	require.NoError(t, model.DB.Where("scene = ?", model.MarketingSceneAnnouncement).Find(&campaigns).Error)
	require.Len(t, campaigns, 1)
	assert.Equal(t, 2, campaigns[0].AnnouncementId)
}

func TestMarketingDeliveryMinuteQuotaIsExactAcrossPolls(t *testing.T) {
	truncate(t)

	const minuteStart = int64(1_800_000_000)
	reserved, err := model.ReserveEmailDeliveryMinuteQuota("marketing-global", minuteStart, 2)
	require.NoError(t, err)
	assert.True(t, reserved)
	reserved, err = model.ReserveEmailDeliveryMinuteQuota("marketing-global", minuteStart, 2)
	require.NoError(t, err)
	assert.True(t, reserved)
	reserved, err = model.ReserveEmailDeliveryMinuteQuota("marketing-global", minuteStart, 2)
	require.NoError(t, err)
	assert.False(t, reserved)
	reserved, err = model.ReserveEmailDeliveryMinuteQuota("marketing-global", minuteStart+60, 2)
	require.NoError(t, err)
	assert.True(t, reserved)
}

func TestMarketingDeliveryRejectsUserDisabledAfterQueueing(t *testing.T) {
	truncate(t)
	user := &model.User{
		Username: "disabled-after-queue", Password: "password",
		Email: "disabled-after-queue@example.com", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default",
	}
	require.NoError(t, model.DB.Create(user).Error)
	campaign := &model.MarketingCampaign{
		Name: "status recheck", Scene: model.MarketingSceneCustom,
		Status: model.MarketingCampaignStatusRunning, AudienceRule: `{}`,
		LocalizedContent: `{}`, ActionPath: "/pricing",
	}
	require.NoError(t, model.CreateMarketingCampaign(campaign))
	delivery, _, err := model.EnqueueEmailDelivery(&model.EmailDelivery{
		DeliveryKey: "disabled-after-queue", Category: "marketing_custom",
		RelatedId: campaign.Id, UserId: user.Id, Recipient: user.Email,
		Subject: "subject", Body: "body", Priority: model.EmailPriorityMarketing,
	})
	require.NoError(t, err)
	recipient := &model.MarketingRecipient{
		CampaignId: campaign.Id, UserId: user.Id, DedupeKey: "disabled-after-queue",
		Language: "en", RecipientMasked: "d***@example.com",
		ClickTokenHash: "disabled-after-queue", EmailDeliveryId: delivery.Id,
		Status: model.MarketingRecipientStatusQueued,
	}
	require.NoError(t, model.DB.Create(recipient).Error)
	require.NoError(t, model.DB.Model(user).Update("status", common.UserStatusDisabled).Error)

	allowed, err := marketingEmailDeliveryAllowed(delivery)
	require.NoError(t, err)
	assert.False(t, allowed)
	require.NoError(t, model.DB.First(delivery, delivery.Id).Error)
	require.NoError(t, model.DB.First(recipient, recipient.Id).Error)
	assert.Equal(t, model.EmailDeliveryStatusExpired, delivery.State)
	assert.Equal(t, model.MarketingRecipientStatusSkipped, recipient.Status)
}

func TestFixedMarketingTemplateEscapesCustomContentAndUsesFixedLink(t *testing.T) {
	body := RenderFixedMarketingEmail(`<script>alert("x")</script>`, "<img src=x onerror=alert(1)>\nhello", "https://example.com/wallet", "Top up")
	assert.NotContains(t, body, "<script>")
	assert.NotContains(t, body, "<img")
	assert.Contains(t, body, "&lt;script&gt;")
	assert.Contains(t, body, "&lt;img src=x onerror=alert(1)&gt;<br>hello")
	assert.Equal(t, 1, strings.Count(body, `href="https://example.com/wallet"`))
}

func TestPreviewMarketingEmailUsesAnnouncementLayoutAndLink(t *testing.T) {
	content, err := common.Marshal(map[string]model.MarketingLocalizedContent{
		"zh-CN": {Subject: "新公告预览", Body: "公告正文"},
	})
	require.NoError(t, err)

	subject, body, err := PreviewMarketingEmail(string(content), "zh-CN", model.MarketingSceneAnnouncement)
	require.NoError(t, err)
	assert.Equal(t, "新公告预览", subject)
	assert.Contains(t, body, "公告正文")
	assert.Contains(t, body, "/dashboard/overview#announcements")
	assert.Contains(t, body, "新公告预览")
}

func TestSingleTopUpWinbackLinksToModelCatalog(t *testing.T) {
	assert.Equal(t, "/pricing", marketingActionPath(model.MarketingSceneSingleTopUp))
}

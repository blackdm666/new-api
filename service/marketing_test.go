package service

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutomationStagesRespectLifecycleRules(t *testing.T) {
	now := time.Now().Unix()
	user := &model.MarketingAudienceUser{LastTopUpId: 88, CreatedAt: now - 2*86400}
	stage, ok := automationStage(model.MarketingScenePaidLowBalance, user, 3, now-100, now)
	assert.True(t, ok)
	assert.Equal(t, "topup-88", stage)

	stage, ok = automationStage(model.MarketingSceneInactive, user, 0, 0, now)
	assert.True(t, ok)
	assert.Equal(t, "1", stage)
	_, ok = automationStage(model.MarketingSceneInactive, user, 1, now-10*86400, now)
	assert.False(t, ok)
	stage, ok = automationStage(model.MarketingSceneInactive, user, 1, now-31*86400, now)
	assert.True(t, ok)
	assert.Equal(t, "2", stage)
	_, ok = automationStage(model.MarketingSceneTrialLowBalance, &model.MarketingAudienceUser{CreatedAt: now - 3600}, 0, 0, now)
	assert.False(t, ok)
	stage, ok = automationStage(model.MarketingSceneTrialLowBalance, user, 1, now-8*86400, now)
	assert.True(t, ok)
	assert.Equal(t, "2", stage)
}

func TestAutomationBaselineExcludesOnlyUsersMatchingAtEnableTime(t *testing.T) {
	truncate(t)
	now := time.Now().Unix()
	require.NoError(t, model.EnsureMarketingAutomations())
	require.NoError(t, model.UpdateMarketingAutomation(model.MarketingSceneInactive, true, false, mustMarketingContent(t, model.MarketingSceneInactive)))
	automation := &model.MarketingAutomation{}
	require.NoError(t, model.DB.Where("scene = ?", model.MarketingSceneInactive).First(automation).Error)
	assert.False(t, automation.BaselineReady)

	existing := &model.User{Username: "baseline_existing", Password: "password", Email: "baseline-existing@example.com", AffCode: "baseline-aff-existing", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", CreatedAt: now - 90*86400, LastLoginAt: now - 40*86400}
	future := &model.User{Username: "baseline_future", Password: "password", Email: "baseline-future@example.com", AffCode: "baseline-aff-future", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", CreatedAt: now - 90*86400, LastLoginAt: now}
	require.NoError(t, model.DB.Create(existing).Error)
	require.NoError(t, model.DB.Create(future).Error)
	campaign := &model.MarketingCampaign{Name: "inactive", Scene: model.MarketingSceneInactive, Status: model.MarketingCampaignStatusRunning, AudienceRule: "{}", LocalizedContent: automation.LocalizedContent, ActionPath: "/dashboard/overview", Automatic: true, StartedTime: now}
	require.NoError(t, model.CreateMarketingCampaign(campaign))
	require.NoError(t, captureMarketingAutomationBaseline(automation, campaign, now))

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

func TestMarketingDeliveryMinuteQuotaIsExactAcrossPolls(t *testing.T) {
	marketingDeliveryMinuteQuota.Lock()
	marketingDeliveryMinuteQuota.minute = 0
	marketingDeliveryMinuteQuota.used = 0
	marketingDeliveryMinuteQuota.Unlock()

	const minuteStart = int64(1_800_000_000)
	assert.True(t, reserveMarketingDeliveryMinute(minuteStart, 2))
	assert.True(t, reserveMarketingDeliveryMinute(minuteStart+30, 2))
	assert.False(t, reserveMarketingDeliveryMinute(minuteStart+45, 2))
	assert.True(t, reserveMarketingDeliveryMinute(minuteStart+60, 2))
}

func TestFixedMarketingTemplateEscapesCustomContentAndUsesFixedLink(t *testing.T) {
	body := RenderFixedMarketingEmail(`<script>alert("x")</script>`, "<img src=x onerror=alert(1)>\nhello", "https://example.com/wallet", "Top up")
	assert.NotContains(t, body, "<script>")
	assert.NotContains(t, body, "<img")
	assert.Contains(t, body, "&lt;script&gt;")
	assert.Contains(t, body, "&lt;img src=x onerror=alert(1)&gt;<br>hello")
	assert.Equal(t, 1, strings.Count(body, `href="https://example.com/wallet"`))
}

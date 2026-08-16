package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"gorm.io/gorm"
)

const marketingDispatchExpiry = 7 * 24 * time.Hour

type marketingDispatchHandler struct{}

func (marketingDispatchHandler) Type() string            { return model.SystemTaskTypeMarketingDispatch }
func (marketingDispatchHandler) Enabled() bool           { return true }
func (marketingDispatchHandler) Interval() time.Duration { return time.Minute }
func (marketingDispatchHandler) NewPayload() any         { return nil }

func (marketingDispatchHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	err := RunMarketingDispatch(ctx)
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, nil, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func init() {
	RegisterSystemTaskHandler(marketingDispatchHandler{})
}

func RunMarketingDispatch(ctx context.Context) error {
	if err := model.EnsureMarketingAutomations(); err != nil {
		return err
	}
	if err := model.ReconcileMarketingRecipients(500); err != nil {
		return err
	}
	now := common.GetTimestamp()
	if err := startDueMarketingCampaigns(now); err != nil {
		return err
	}
	if err := materializeMarketingAutomations(now); err != nil {
		return err
	}
	if !marketingSendWindowOpen(time.Now()) {
		return nil
	}
	return dispatchPendingMarketingRecipients(ctx, now)
}

func startDueMarketingCampaigns(now int64) error {
	campaigns, err := model.ListDueMarketingCampaigns(now, 20)
	if err != nil {
		return err
	}
	for _, campaign := range campaigns {
		if err := MaterializeMarketingCampaign(campaign); err != nil {
			_ = model.SetMarketingCampaignStatus(campaign.Id, []string{model.MarketingCampaignStatusScheduled}, model.MarketingCampaignStatusPaused, err.Error())
			continue
		}
		_ = model.SetMarketingCampaignStatus(campaign.Id, []string{model.MarketingCampaignStatusScheduled}, model.MarketingCampaignStatusRunning, "")
	}
	return nil
}

func MaterializeMarketingCampaign(campaign *model.MarketingCampaign) error {
	if campaign == nil || campaign.Id <= 0 {
		return model.ErrMarketingInvalid
	}
	rule := model.MarketingAudienceRule{}
	if strings.TrimSpace(campaign.AudienceRule) != "" {
		if err := common.UnmarshalJsonStr(campaign.AudienceRule, &rule); err != nil {
			return err
		}
	}
	offset := 0
	for {
		users, _, err := model.ListMarketingAudience(rule, 500, offset)
		if err != nil {
			return err
		}
		for _, user := range users {
			if err := createMarketingRecipient(campaign, user, fmt.Sprintf("campaign:%d:user:%d", campaign.Id, user.Id)); err != nil {
				return err
			}
		}
		if len(users) < 500 {
			break
		}
		offset += len(users)
	}
	return nil
}

func materializeMarketingAutomations(now int64) error {
	automations, err := model.ListMarketingAutomations()
	if err != nil {
		return err
	}
	for _, automation := range automations {
		if !automation.Enabled {
			continue
		}
		if automation.Scene == model.MarketingSceneAnnouncement {
			if err := materializeAnnouncementCampaigns(automation, now); err != nil {
				return err
			}
			continue
		}
		campaign, err := model.GetActiveAutomationCampaign(automation.Scene)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			campaign = &model.MarketingCampaign{
				Name:             marketingSceneName(automation.Scene),
				Scene:            automation.Scene,
				Status:           model.MarketingCampaignStatusRunning,
				AudienceRule:     "{}",
				LocalizedContent: automation.LocalizedContent,
				ActionPath:       marketingActionPath(automation.Scene),
				Automatic:        true,
				StartedTime:      now,
			}
			if err := model.CreateMarketingCampaign(campaign); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if campaign.Status == model.MarketingCampaignStatusPaused {
			continue
		}
		if !automation.BaselineReady {
			if err := captureMarketingAutomationBaseline(automation, campaign, now); err != nil {
				return err
			}
			continue
		}
		rule := automationAudienceRule(automation.Scene, now)
		for offset := 0; ; offset += 500 {
			users, _, err := model.ListMarketingAudience(rule, 500, offset)
			if err != nil {
				return err
			}
			for _, user := range users {
				count, lastAttempt, err := model.CountUserMarketingSceneRecipients(user.Id, automation.Scene)
				if err != nil {
					return err
				}
				stage, ok := automationStage(automation.Scene, user, count, lastAttempt, now)
				if !ok {
					continue
				}
				dedupe := fmt.Sprintf("automation:%s:user:%d:stage:%s", automation.Scene, user.Id, stage)
				if err := createMarketingRecipient(campaign, user, dedupe); err != nil {
					return err
				}
			}
			if len(users) < 500 {
				break
			}
		}
	}
	return nil
}

func captureMarketingAutomationBaseline(automation *model.MarketingAutomation, campaign *model.MarketingCampaign, now int64) error {
	if automation == nil || campaign == nil || automation.ApplyExisting {
		return model.ErrMarketingInvalid
	}
	rule := automationAudienceRule(automation.Scene, now)
	for offset := 0; ; offset += 500 {
		users, _, err := model.ListMarketingAudience(rule, 500, offset)
		if err != nil {
			return err
		}
		for _, user := range users {
			count, lastAttempt, err := model.CountUserMarketingSceneRecipients(user.Id, automation.Scene)
			if err != nil {
				return err
			}
			stage, ok := automationStage(automation.Scene, user, count, lastAttempt, now)
			if !ok {
				continue
			}
			dedupe := fmt.Sprintf("automation:%s:user:%d:stage:%s", automation.Scene, user.Id, stage)
			_, err = model.CreateMarketingRecipient(&model.MarketingRecipient{
				CampaignId:      campaign.Id,
				UserId:          user.Id,
				DedupeKey:       dedupe,
				Language:        marketingUserLanguage(user.Setting),
				RecipientMasked: maskMarketingEmail(user.Email),
				ClickTokenHash:  model.HashMarketingToken("baseline:" + dedupe),
				Status:          model.MarketingRecipientStatusSkipped,
				LastError:       "excluded when automation was enabled",
				CreatedTime:     now,
			})
			if err != nil {
				return err
			}
		}
		if len(users) < 500 {
			break
		}
	}
	return model.MarkMarketingAutomationBaselineReady(automation.Scene, automation.EnabledTime)
}

func materializeAnnouncementCampaigns(automation *model.MarketingAutomation, now int64) error {
	for _, announcement := range console_setting.GetAnnouncements() {
		id, ok := marketingAnnouncementID(announcement["id"])
		if !ok || id <= 0 {
			continue
		}
		publishTime, ok := marketingAnnouncementTime(announcement["publishDate"])
		if !ok || publishTime > now {
			continue
		}
		if !automation.ApplyExisting && automation.EnabledTime > 0 && publishTime < automation.EnabledTime {
			continue
		}
		_, err := model.FindMarketingCampaignByAnnouncementId(id)
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		content := automation.LocalizedContent
		if text, ok := announcement["content"].(string); ok && strings.TrimSpace(text) != "" {
			localized := map[string]model.MarketingLocalizedContent{}
			if err := common.UnmarshalJsonStr(content, &localized); err == nil {
				entry := localized["zh-CN"]
				entry.Body = truncateMarketingText(strings.TrimSpace(text), 1000)
				localized["zh-CN"] = entry
				if encoded, err := common.Marshal(localized); err == nil {
					content = string(encoded)
				}
			}
		}
		campaign := &model.MarketingCampaign{Name: fmt.Sprintf("新公告 #%d", id), Scene: model.MarketingSceneAnnouncement, Status: model.MarketingCampaignStatusRunning, AudienceRule: "{}", LocalizedContent: content, ActionPath: marketingActionPath(model.MarketingSceneAnnouncement), Automatic: true, AnnouncementId: id, StartedTime: now}
		if err := model.CreateMarketingCampaign(campaign); err != nil {
			return err
		}
		if err := MaterializeMarketingCampaign(campaign); err != nil {
			return err
		}
	}
	return nil
}

func createMarketingRecipient(campaign *model.MarketingCampaign, audience *model.MarketingAudienceUser, dedupe string) error {
	if campaign == nil || audience == nil || model.IsMarketingSuppressed(audience.Id, audience.Email) {
		return nil
	}
	token, err := newMarketingToken()
	if err != nil {
		return err
	}
	recipient := &model.MarketingRecipient{
		CampaignId:      campaign.Id,
		UserId:          audience.Id,
		DedupeKey:       dedupe,
		Language:        marketingUserLanguage(audience.Setting),
		RecipientMasked: maskMarketingEmail(audience.Email),
		ClickTokenHash:  hashMarketingToken(token),
		Status:          model.MarketingRecipientStatusPending,
	}
	created, err := model.CreateMarketingRecipient(recipient)
	if err != nil || !created {
		return err
	}
	// Store the one-time clear token only until the message is queued. It is
	// encrypted with the existing application secret and removed afterward.
	encrypted, err := common.EncryptSensitiveValue("marketing-click-token", []byte(token))
	if err != nil {
		_ = model.SkipMarketingRecipient(recipient.Id, "click token encryption failed")
		return err
	}
	if err := model.SetMarketingRecipientDispatchToken(recipient.Id, encrypted); err != nil {
		_ = model.SkipMarketingRecipient(recipient.Id, "click token persistence failed")
		return err
	}
	return nil
}

func dispatchPendingMarketingRecipients(ctx context.Context, now int64) error {
	dayStart := marketingDayStart(time.Now())
	stats, err := model.GetEmailDeliveryStats(now, dayStart)
	if err != nil {
		return err
	}
	remaining := model.MarketingDailyLimit - int(stats.MarketingSentToday)
	if remaining <= 0 {
		return nil
	}
	limit := model.MarketingPerMinuteLimit
	if remaining < limit {
		limit = remaining
	}
	rows, err := model.ListPendingMarketingRecipients(limit * 3)
	if err != nil {
		return err
	}
	queued := 0
	for _, recipient := range rows {
		if queued >= limit {
			break
		}
		campaign, err := model.GetMarketingCampaign(recipient.CampaignId)
		if err != nil || campaign.Status != model.MarketingCampaignStatusRunning {
			continue
		}
		user, err := model.GetUserById(recipient.UserId, true)
		if err != nil || user.Status != common.UserStatusEnabled || user.Role != common.RoleCommonUser || strings.TrimSpace(user.Email) == "" {
			_ = model.SkipMarketingRecipient(recipient.Id, "recipient is no longer eligible")
			continue
		}
		if model.IsMarketingSuppressed(user.Id, user.Email) {
			_ = model.SkipMarketingRecipient(recipient.Id, "recipient is suppressed")
			continue
		}
		if campaign.Automatic && model.UserMarketingCooldownActive(user.Id, recipient.Id, now-model.MarketingUserCooldownDays*86400) {
			continue
		}
		if campaign.Automatic &&
			(campaign.Scene == model.MarketingScenePaidLowBalance || campaign.Scene == model.MarketingSceneTrialLowBalance) &&
			model.HasRecentEmailDelivery(user.Id, "quota_warning_user", now-model.MarketingUserCooldownDays*86400) {
			_ = model.SkipMarketingRecipient(recipient.Id, "recent system quota warning already sent")
			continue
		}
		token, err := model.GetMarketingRecipientDispatchToken(recipient.Id)
		if err != nil {
			_ = model.SkipMarketingRecipient(recipient.Id, "click token is unavailable")
			continue
		}
		content, err := marketingCampaignContent(campaign.LocalizedContent, recipient.Language)
		if err != nil {
			_ = model.SkipMarketingRecipient(recipient.Id, err.Error())
			continue
		}
		clickURL := strings.TrimRight(systemEmailSiteURL(), "/") + "/api/marketing/c/" + url.PathEscape(token)
		body := RenderFixedMarketingEmail(content.Subject, content.Body, clickURL, marketingActionLabel(recipient.Language))
		delivery, err := QueueMarketingEmail("marketing:"+recipient.DedupeKey, "marketing_"+campaign.Scene, campaign.Id, user.Id, user.Email, content.Subject, body, now+int64(marketingDispatchExpiry.Seconds()))
		if err != nil {
			continue
		}
		if err := model.UpdateMarketingRecipientQueued(recipient.Id, delivery.Id, now); err != nil {
			continue
		}
		_ = model.ClearMarketingRecipientDispatchToken(recipient.Id)
		queued++
	}
	return reconcileMarketingCampaignHealth(ctx)
}

func reconcileMarketingCampaignHealth(ctx context.Context) error {
	for offset := 0; ; offset += 100 {
		campaigns, err := model.ListRunningMarketingCampaigns(100, offset)
		if err != nil {
			return err
		}
		for _, campaign := range campaigns {
			outcomes, err := model.RecentCampaignDeliveryOutcomes(campaign.Id, 50)
			if err != nil {
				return err
			}
			failed := 0
			consecutiveFailures := 0
			for index, outcome := range outcomes {
				if outcome == model.MarketingRecipientStatusFailed {
					failed++
					if index == consecutiveFailures {
						consecutiveFailures++
					}
				}
			}
			if consecutiveFailures >= 20 || (len(outcomes) >= 50 && failed*100/len(outcomes) >= 20) {
				reason := "marketing delivery circuit breaker: SMTP failure threshold reached"
				_ = model.SetMarketingCampaignStatus(campaign.Id, []string{model.MarketingCampaignStatusRunning}, model.MarketingCampaignStatusPaused, reason)
				common.SysError(fmt.Sprintf("marketing campaign %d paused: %s", campaign.Id, reason))
				continue
			}
			if campaign.Automatic {
				continue
			}
			active, err := model.CountCampaignRecipientsByStatuses(campaign.Id, []string{model.MarketingRecipientStatusPending, model.MarketingRecipientStatusQueued})
			if err == nil && active == 0 {
				_ = model.SetMarketingCampaignStatus(campaign.Id, []string{model.MarketingCampaignStatusRunning}, model.MarketingCampaignStatusCompleted, "")
			}
		}
		if len(campaigns) < 100 {
			break
		}
	}
	return nil
}

// marketingEmailDeliveryAllowed rechecks mutable business state immediately
// before SMTP delivery. A paused campaign keeps its queued messages intact;
// cancelled or no-longer-eligible recipients are scrubbed and skipped.
func marketingEmailDeliveryAllowed(delivery *model.EmailDelivery) (bool, error) {
	if delivery == nil || delivery.Priority != model.EmailPriorityMarketing || !strings.HasPrefix(delivery.Category, "marketing_") {
		return true, nil
	}
	nowTime := time.Now()
	campaign, err := model.GetMarketingCampaign(delivery.RelatedId)
	if err != nil {
		return expireMarketingEmailDelivery(delivery, nil, "marketing campaign is unavailable")
	}
	if campaign.Status == model.MarketingCampaignStatusPaused {
		return false, model.DeferEmailDelivery(delivery.Id, common.GetTimestamp()+300)
	}
	recipient, err := model.GetMarketingRecipientByEmailDeliveryId(delivery.Id)
	if err != nil {
		return expireMarketingEmailDelivery(delivery, nil, "marketing recipient is unavailable")
	}
	if campaign.Status != model.MarketingCampaignStatusRunning {
		return expireMarketingEmailDelivery(delivery, recipient, "marketing campaign is not running")
	}
	rule := model.MarketingAudienceRule{}
	if campaign.Automatic && campaign.Scene != model.MarketingSceneAnnouncement {
		rule = automationAudienceRule(campaign.Scene, common.GetTimestamp())
	} else if !campaign.Automatic && strings.TrimSpace(campaign.AudienceRule) != "" {
		if err := common.UnmarshalJsonStr(campaign.AudienceRule, &rule); err != nil {
			return expireMarketingEmailDelivery(delivery, recipient, "marketing audience rule is invalid")
		}
	}
	eligible, user, err := model.MarketingUserMatchesAudience(recipient.UserId, rule)
	if err != nil {
		return false, err
	}
	if !eligible || user == nil || !strings.EqualFold(strings.TrimSpace(user.Email), strings.TrimSpace(delivery.Recipient)) {
		return expireMarketingEmailDelivery(delivery, recipient, "recipient is no longer eligible")
	}
	if model.IsMarketingSuppressed(user.Id, user.Email) {
		return expireMarketingEmailDelivery(delivery, recipient, "recipient is suppressed")
	}
	now := nowTime.Unix()
	if campaign.Automatic && model.UserMarketingCooldownActive(user.Id, recipient.Id, now-model.MarketingUserCooldownDays*86400) {
		return false, model.DeferEmailDelivery(delivery.Id, now+3600)
	}
	if campaign.Automatic &&
		(campaign.Scene == model.MarketingScenePaidLowBalance || campaign.Scene == model.MarketingSceneTrialLowBalance) &&
		model.HasRecentEmailDelivery(user.Id, "quota_warning_user", now-model.MarketingUserCooldownDays*86400) {
		return expireMarketingEmailDelivery(delivery, recipient, "recent system quota warning already sent")
	}
	if !marketingSendWindowOpen(nowTime) {
		return false, model.DeferEmailDelivery(delivery.Id, nextMarketingSendWindow(nowTime).Unix())
	}
	dayStart := marketingDayStart(nowTime)
	reserved, err := model.ReserveMarketingEmailQuota(delivery.Id, dayStart, now, model.MarketingDailyLimit)
	if err != nil {
		return false, err
	}
	if !reserved {
		return false, model.DeferEmailDelivery(delivery.Id, nextMarketingSendWindow(nowTime.Add(24*time.Hour)).Unix())
	}
	return true, nil
}

func expireMarketingEmailDelivery(delivery *model.EmailDelivery, recipient *model.MarketingRecipient, reason string) (bool, error) {
	if err := model.ExpireEmailDelivery(delivery.Id, reason); err != nil {
		return false, err
	}
	if recipient != nil {
		if err := model.SkipQueuedMarketingRecipient(recipient.Id, reason); err != nil {
			return false, err
		}
	}
	return false, nil
}

func RenderFixedMarketingEmail(subject string, content string, actionURL string, actionLabel string) string {
	systemName := html.EscapeString(common.SystemNameOrDefault())
	siteURL := html.EscapeString(systemEmailSiteURL())
	safeSubject := html.EscapeString(subject)
	safeContent := strings.ReplaceAll(html.EscapeString(content), "\n", "<br>")
	safeActionURL := html.EscapeString(actionURL)
	safeActionLabel := html.EscapeString(actionLabel)
	return `<div style="background:#f4f7ff;padding:40px 16px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','PingFang SC',sans-serif;color:#172033;line-height:1.6"><div style="max-width:560px;margin:0 auto;overflow:hidden;background:#fff;border:1px solid #dfe7ff;border-radius:18px;box-shadow:0 16px 40px rgba(37,57,128,.10)"><div style="padding:25px 32px;background:linear-gradient(135deg,#0891b2 0%,#4f46e5 55%,#7c3aed 100%)"><a href="` + siteURL + `" style="color:#fff;text-decoration:none;font-size:20px;font-weight:750">` + systemName + `</a></div><div style="padding:36px 40px 34px"><h1 style="margin:0 0 18px;font-size:25px;color:#172033">` + safeSubject + `</h1><div style="padding:20px;border:1px solid #e0e5ff;border-left:4px solid #5b5ce2;border-radius:12px;background:#f7f8ff;color:#44506a;font-size:15px">` + safeContent + `</div><div style="margin:30px 0 4px;text-align:center"><a href="` + safeActionURL + `" style="display:inline-block;padding:12px 26px;border-radius:11px;background:linear-gradient(135deg,#0891b2,#4f46e5 58%,#7c3aed);color:#fff;text-decoration:none;font-weight:650">` + safeActionLabel + `</a></div></div></div><p style="max-width:560px;margin:20px auto 0;text-align:center;font-size:12px"><a href="` + siteURL + `" style="color:#5b5ce2;text-decoration:none">` + systemName + `</a></p></div>`
}

func SendMarketingTestEmail(root *model.User, localizedContent string, language string) error {
	if root == nil || strings.TrimSpace(root.Email) == "" {
		return model.ErrMarketingInvalid
	}
	content, err := marketingCampaignContent(localizedContent, language)
	if err != nil {
		return err
	}
	body := RenderFixedMarketingEmail(content.Subject, content.Body, strings.TrimRight(systemEmailSiteURL(), "/")+"/wallet", marketingActionLabel(language))
	_, err = QueueSystemEmail("marketing-test:"+common.NewRequestId(), "email_preview", 0, root.Id, root.Email, "[TEST] "+content.Subject, body, common.GetTimestamp()+3600)
	return err
}

func MarketingClickTarget(token string) (string, error) {
	recipient, err := model.FindMarketingRecipientByToken(token)
	if err != nil {
		return "", err
	}
	campaign, err := model.GetMarketingCampaign(recipient.CampaignId)
	if err != nil {
		return "", err
	}
	path := campaign.ActionPath
	allowed := map[string]bool{
		"/wallet":                           true,
		"/dashboard/overview":               true,
		"/dashboard/overview#announcements": true,
	}
	if !allowed[path] {
		path = "/dashboard/overview"
	}
	if err := model.RecordMarketingClick(recipient); err != nil {
		return "", err
	}
	return strings.TrimRight(systemEmailSiteURL(), "/") + path, nil
}

func PreviewMarketingAudience(rule model.MarketingAudienceRule) (int64, error) {
	_, total, err := model.ListMarketingAudience(rule, 1, 0)
	return total, err
}

func PreviewMarketingAutomation(scene string) (int64, error) {
	if scene == model.MarketingSceneAnnouncement {
		return PreviewMarketingAudience(model.MarketingAudienceRule{})
	}
	return PreviewMarketingAudience(automationAudienceRule(scene, common.GetTimestamp()))
}

func marketingCampaignContent(raw string, language string) (model.MarketingLocalizedContent, error) {
	contents := map[string]model.MarketingLocalizedContent{}
	if err := common.UnmarshalJsonStr(raw, &contents); err != nil {
		return model.MarketingLocalizedContent{}, err
	}
	language = normalizeMarketingLanguage(language)
	content := contents[language]
	if strings.TrimSpace(content.Subject) == "" || strings.TrimSpace(content.Body) == "" {
		content = contents["zh-CN"]
	}
	if strings.TrimSpace(content.Subject) == "" || strings.TrimSpace(content.Body) == "" {
		return model.MarketingLocalizedContent{}, model.ErrMarketingInvalid
	}
	return content, nil
}

func automationAudienceRule(scene string, now int64) model.MarketingAudienceRule {
	zero := 0
	one := 1
	quotaOne := int(common.QuotaPerUnit)
	quotaTrial := int(common.QuotaPerUnit / 10)
	switch scene {
	case model.MarketingSceneSingleTopUp:
		return model.MarketingAudienceRule{TopUpCountMin: &one, TopUpCountMax: &one, LastTopUpBefore: now - 30*86400}
	case model.MarketingScenePaidLowBalance:
		return model.MarketingAudienceRule{TopUpCountMin: &one, QuotaMax: &quotaOne}
	case model.MarketingSceneTrialLowBalance:
		return model.MarketingAudienceRule{TopUpCountMax: &zero, QuotaMax: &quotaTrial, UsedQuotaPositive: true}
	case model.MarketingSceneInactive:
		return model.MarketingAudienceRule{InactiveDays: 30}
	default:
		return model.MarketingAudienceRule{}
	}
}

func automationStage(scene string, user *model.MarketingAudienceUser, count int64, lastAttempt int64, now int64) (string, bool) {
	if scene == model.MarketingScenePaidLowBalance {
		// A successful recharge opens a new low-balance cycle. The top-up id is
		// part of the dedupe key, so later recharges can legitimately trigger a
		// new reminder while duplicate scans remain harmless.
		return fmt.Sprintf("topup-%d", user.LastTopUpId), true
	}
	if count >= 2 {
		return "", false
	}
	waitDays := int64(30)
	if scene == model.MarketingSceneTrialLowBalance {
		waitDays = model.MarketingUserCooldownDays
	}
	if count == 1 && (lastAttempt == 0 || lastAttempt > now-waitDays*86400) {
		return "", false
	}
	if scene == model.MarketingSceneTrialLowBalance && user.CreatedAt > now-86400 {
		return "", false
	}
	if count == 0 {
		return "1", true
	}
	return "2", true
}

func marketingSendWindowOpen(now time.Time) bool {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	hour := now.In(location).Hour()
	return hour >= 9 && hour < 20
}

func marketingDayStart(now time.Time) int64 {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	local := now.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).Unix()
}

func nextMarketingSendWindow(now time.Time) time.Time {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	local := now.In(location)
	start := time.Date(local.Year(), local.Month(), local.Day(), 9, 0, 0, 0, location)
	if local.Before(start) {
		return start
	}
	return start.Add(24 * time.Hour)
}

func marketingSceneName(scene string) string {
	names := map[string]string{model.MarketingSceneSingleTopUp: "单次充值未复购", model.MarketingScenePaidLowBalance: "付费用户余额不足", model.MarketingSceneTrialLowBalance: "试用额度即将耗尽", model.MarketingSceneInactive: "长期未登录", model.MarketingSceneAnnouncement: "新公告通知"}
	return names[scene]
}

func marketingActionPath(scene string) string {
	if scene == model.MarketingSceneAnnouncement {
		return "/dashboard/overview#announcements"
	}
	if scene == model.MarketingSceneInactive {
		return "/dashboard/overview"
	}
	return "/wallet"
}

func marketingActionLabel(language string) string {
	if normalizeMarketingLanguage(language) == "zh-CN" || normalizeMarketingLanguage(language) == "zh-TW" {
		return "前往查看"
	}
	return "View now"
}

func marketingUserLanguage(rawSetting string) string {
	user := &model.User{Setting: rawSetting}
	return normalizeMarketingLanguage(user.GetSetting().Language)
}

func normalizeMarketingLanguage(language string) string {
	language = strings.TrimSpace(language)
	if strings.EqualFold(language, "zh") || strings.EqualFold(language, "zh-CN") {
		return "zh-CN"
	}
	if strings.EqualFold(language, "zh-TW") {
		return "zh-TW"
	}
	if language == "" {
		return "zh-CN"
	}
	return language
}

func newMarketingToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashMarketingToken(token string) string {
	return model.HashMarketingToken(token)
}

func maskMarketingEmail(address string) string {
	parts := strings.Split(strings.TrimSpace(address), "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "***"
	}
	local := []rune(parts[0])
	visible := string(local[:1])
	if len(local) > 2 {
		visible += string(local[len(local)-1:])
	}
	return visible + "***@" + parts[1]
}

func marketingAnnouncementID(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), typed > 0
	case int:
		return typed, typed > 0
	default:
		return 0, false
	}
}

func marketingAnnouncementTime(value any) (int64, bool) {
	text, ok := value.(string)
	if !ok {
		return 0, false
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return 0, false
	}
	return parsed.Unix(), true
}

func truncateMarketingText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

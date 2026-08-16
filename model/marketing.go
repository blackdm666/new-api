package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MarketingDailyLimit       = 500
	MarketingPerMinuteLimit   = 20
	MarketingUserCooldownDays = 7

	MarketingCampaignStatusDraft     = "draft"
	MarketingCampaignStatusScheduled = "scheduled"
	MarketingCampaignStatusRunning   = "running"
	MarketingCampaignStatusPaused    = "paused"
	MarketingCampaignStatusCompleted = "completed"
	MarketingCampaignStatusCancelled = "cancelled"

	MarketingRecipientStatusPending   = "pending"
	MarketingRecipientStatusQueued    = "queued"
	MarketingRecipientStatusDelivered = "delivered"
	MarketingRecipientStatusFailed    = "failed"
	MarketingRecipientStatusSkipped   = "skipped"

	MarketingSceneCustom          = "custom"
	MarketingSceneSingleTopUp     = "single_topup_winback"
	MarketingScenePaidLowBalance  = "paid_low_balance"
	MarketingSceneTrialLowBalance = "trial_low_balance"
	MarketingSceneInactive        = "inactive_user"
	MarketingSceneAnnouncement    = "announcement"

	MarketingEventClick      = "click"
	MarketingEventConversion = "conversion"

	marketingAutomationDisabledReason = "marketing automation disabled"
)

var ErrMarketingInvalid = errors.New("marketing request is invalid")

type MarketingLocalizedContent struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type MarketingAudienceRule struct {
	UserId            int      `json:"-"`
	Groups            []string `json:"groups,omitempty"`
	InactiveDays      int      `json:"inactive_days,omitempty"`
	TopUpCountMin     *int     `json:"topup_count_min,omitempty"`
	TopUpCountMax     *int     `json:"topup_count_max,omitempty"`
	LastTopUpBefore   int64    `json:"last_topup_before,omitempty"`
	QuotaMin          *int     `json:"quota_min,omitempty"`
	QuotaMax          *int     `json:"quota_max,omitempty"`
	UsedQuotaPositive bool     `json:"used_quota_positive,omitempty"`
}

type MarketingCampaign struct {
	Id               int    `json:"id"`
	Name             string `json:"name" gorm:"type:varchar(160);not null"`
	Scene            string `json:"scene" gorm:"type:varchar(48);not null;index"`
	Status           string `json:"status" gorm:"type:varchar(24);not null;index"`
	AudienceRule     string `json:"audience_rule" gorm:"type:text;not null"`
	LocalizedContent string `json:"localized_content" gorm:"type:text;not null"`
	ActionPath       string `json:"action_path" gorm:"type:varchar(160);not null"`
	Automatic        bool   `json:"automatic" gorm:"not null"`
	AnnouncementId   int    `json:"announcement_id" gorm:"index"`
	ScheduledTime    int64  `json:"scheduled_time" gorm:"bigint;not null;default:0;index"`
	StartedTime      int64  `json:"started_time" gorm:"bigint;not null;default:0"`
	CompletedTime    int64  `json:"completed_time" gorm:"bigint;not null;default:0"`
	PausedReason     string `json:"paused_reason" gorm:"type:varchar(512)"`
	CreatedBy        int    `json:"created_by" gorm:"index"`
	CreatedTime      int64  `json:"created_time" gorm:"bigint;autoCreateTime;index"`
	UpdatedTime      int64  `json:"updated_time" gorm:"bigint;autoUpdateTime"`

	RecipientCount int64 `json:"recipient_count" gorm:"-"`
	DeliveredCount int64 `json:"delivered_count" gorm:"-"`
	FailedCount    int64 `json:"failed_count" gorm:"-"`
	ClickedCount   int64 `json:"clicked_count" gorm:"-"`
	ConvertedCount int64 `json:"converted_count" gorm:"-"`
	ConvertedCents int64 `json:"converted_cents" gorm:"-"`
}

type MarketingRecipient struct {
	Id                     int    `json:"id"`
	CampaignId             int    `json:"campaign_id" gorm:"not null;index;index:idx_marketing_campaign_status,priority:1"`
	UserId                 int    `json:"user_id" gorm:"not null;index;index:idx_marketing_user_created,priority:1"`
	DedupeKey              string `json:"dedupe_key" gorm:"type:varchar(190);not null;uniqueIndex"`
	Language               string `json:"language" gorm:"type:varchar(16);not null"`
	RecipientMasked        string `json:"recipient_masked" gorm:"type:varchar(320);not null"`
	ClickTokenHash         string `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
	DispatchTokenEncrypted string `json:"-" gorm:"type:text"`
	EmailDeliveryId        int    `json:"email_delivery_id" gorm:"index"`
	Status                 string `json:"status" gorm:"type:varchar(24);not null;index;index:idx_marketing_campaign_status,priority:2"`
	LastError              string `json:"last_error" gorm:"type:varchar(1000)"`
	QueuedTime             int64  `json:"queued_time" gorm:"bigint;not null;default:0"`
	DeliveredTime          int64  `json:"delivered_time" gorm:"bigint;not null;default:0;index"`
	ClickedTime            int64  `json:"clicked_time" gorm:"bigint;not null;default:0;index"`
	ConvertedTime          int64  `json:"converted_time" gorm:"bigint;not null;default:0;index"`
	CreatedTime            int64  `json:"created_time" gorm:"bigint;autoCreateTime;index;index:idx_marketing_user_created,priority:2"`
	UpdatedTime            int64  `json:"updated_time" gorm:"bigint;autoUpdateTime"`

	Username string `json:"username" gorm:"-"`
}

type MarketingAutomation struct {
	Id               int    `json:"id"`
	Scene            string `json:"scene" gorm:"type:varchar(48);not null;uniqueIndex"`
	Enabled          bool   `json:"enabled" gorm:"not null"`
	ApplyExisting    bool   `json:"apply_existing" gorm:"not null"`
	BaselineReady    bool   `json:"baseline_ready" gorm:"not null"`
	EnabledTime      int64  `json:"enabled_time" gorm:"bigint;not null;default:0"`
	LocalizedContent string `json:"localized_content" gorm:"type:text;not null"`
	CreatedTime      int64  `json:"created_time" gorm:"bigint;autoCreateTime"`
	UpdatedTime      int64  `json:"updated_time" gorm:"bigint;autoUpdateTime"`
}

type MarketingSuppression struct {
	Id          int    `json:"id"`
	UserId      int    `json:"user_id" gorm:"index"`
	EmailHash   string `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
	EmailMasked string `json:"email_masked" gorm:"type:varchar(320);not null"`
	Reason      string `json:"reason" gorm:"type:varchar(255);not null"`
	CreatedBy   int    `json:"created_by"`
	CreatedTime int64  `json:"created_time" gorm:"bigint;autoCreateTime"`
}

type MarketingEvent struct {
	Id          int    `json:"id"`
	EventKey    string `json:"event_key" gorm:"type:varchar(190);not null;uniqueIndex"`
	CampaignId  int    `json:"campaign_id" gorm:"not null;index"`
	RecipientId int    `json:"recipient_id" gorm:"not null;index"`
	UserId      int    `json:"user_id" gorm:"not null;index"`
	EventType   string `json:"event_type" gorm:"type:varchar(24);not null;index"`
	TopUpId     int    `json:"top_up_id" gorm:"index"`
	AmountCents int64  `json:"amount_cents"`
	CreatedTime int64  `json:"created_time" gorm:"bigint;autoCreateTime;index"`
}

type MarketingAudienceUser struct {
	Id            int
	Username      string
	Email         string
	Setting       string
	Quota         int
	UsedQuota     int
	CreatedAt     int64
	LastLoginAt   int64
	TopUpCount    int64
	LastTopUpTime int64
	LastTopUpId   int
}

type MarketingOverview struct {
	Campaigns      int64 `json:"campaigns"`
	Queued         int64 `json:"queued"`
	Delivered      int64 `json:"delivered"`
	Failed         int64 `json:"failed"`
	Clicked        int64 `json:"clicked"`
	Converted      int64 `json:"converted"`
	ConvertedCents int64 `json:"converted_cents"`
}

type MarketingCircuitState struct {
	PausedCampaigns int64  `json:"paused_campaigns"`
	LastReason      string `json:"last_reason"`
}

func DefaultMarketingContents() map[string]map[string]MarketingLocalizedContent {
	return map[string]map[string]MarketingLocalizedContent{
		MarketingSceneSingleTopUp: {
			"zh-CN": {Subject: "继续探索更多模型能力", Body: "你曾在本站完成过一次充值。欢迎回来继续使用模型服务，账户充值入口已为你准备好。"},
			"en":    {Subject: "Continue exploring more AI models", Body: "You have topped up once before. Come back anytime to continue using our model services."},
		},
		MarketingScenePaidLowBalance: {
			"zh-CN": {Subject: "账户余额即将用完", Body: "你的账户余额已经接近用完。及时充值可以避免正在使用的服务中断。"},
			"en":    {Subject: "Your account balance is running low", Body: "Your balance is almost depleted. Top up now to keep your services running without interruption."},
		},
		MarketingSceneTrialLowBalance: {
			"zh-CN": {Subject: "试用额度即将用完", Body: "你已经体验过本站的模型服务。完成首次充值即可继续使用更多模型和能力。"},
			"en":    {Subject: "Your trial balance is almost used", Body: "You have tried our model services. Make your first top-up to continue exploring more models and capabilities."},
		},
		MarketingSceneInactive: {
			"zh-CN": {Subject: "好久不见，欢迎回来", Body: "你的账户仍然可以正常使用。登录控制台即可查看最新模型和服务更新。"},
			"en":    {Subject: "It has been a while", Body: "Your account is still ready to use. Sign in to see the latest models and service updates."},
		},
		MarketingSceneAnnouncement: {
			"zh-CN": {Subject: "站点发布了新公告", Body: "本站刚刚发布了一条新公告，登录控制台即可查看完整内容。"},
			"en":    {Subject: "A new site announcement is available", Body: "A new announcement has just been published. Sign in to view the complete update."},
		},
	}
}

func EnsureMarketingAutomations() error {
	defaults := DefaultMarketingContents()
	for _, scene := range []string{MarketingSceneSingleTopUp, MarketingScenePaidLowBalance, MarketingSceneTrialLowBalance, MarketingSceneInactive, MarketingSceneAnnouncement} {
		content, err := common.Marshal(defaults[scene])
		if err != nil {
			return err
		}
		record := MarketingAutomation{Scene: scene, Enabled: false, ApplyExisting: false, LocalizedContent: string(content)}
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error; err != nil {
			return err
		}
	}
	return nil
}

func ListMarketingAutomations() ([]*MarketingAutomation, error) {
	rows := []*MarketingAutomation{}
	err := DB.Order("id ASC").Find(&rows).Error
	return rows, err
}

func UpdateMarketingAutomation(scene string, enabled bool, applyExisting bool, localizedContent string) error {
	if _, ok := DefaultMarketingContents()[scene]; !ok || strings.TrimSpace(localizedContent) == "" {
		return ErrMarketingInvalid
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		current := &MarketingAutomation{}
		if err := lockForUpdate(tx).Where("scene = ?", scene).First(current).Error; err != nil {
			return err
		}
		updates := map[string]any{"enabled": enabled, "apply_existing": applyExisting, "localized_content": localizedContent}
		if enabled && !current.Enabled {
			updates["enabled_time"] = common.GetTimestamp()
			updates["baseline_ready"] = applyExisting
		} else if !enabled {
			updates["baseline_ready"] = false
		} else if applyExisting {
			updates["baseline_ready"] = true
		}
		result := tx.Model(current).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		// Keep the evergreen campaign in sync so copy edits take effect without
		// generating duplicate recipients or requiring a campaign restart.
		if err := tx.Model(&MarketingCampaign{}).
			Where("automatic = ? AND scene = ? AND announcement_id = 0", true, scene).
			Update("localized_content", localizedContent).Error; err != nil {
			return err
		}
		campaigns := []*MarketingCampaign{}
		if !enabled {
			if err := tx.Where("automatic = ? AND scene = ? AND status = ?", true, scene, MarketingCampaignStatusRunning).
				Order("id ASC").Find(&campaigns).Error; err != nil {
				return err
			}
			for _, campaign := range campaigns {
				if err := setMarketingCampaignStatusTx(tx, campaign.Id, []string{MarketingCampaignStatusRunning}, MarketingCampaignStatusPaused, marketingAutomationDisabledReason); err != nil {
					return err
				}
			}
			return nil
		}
		if err := tx.Where("automatic = ? AND scene = ? AND status = ? AND paused_reason = ?", true, scene, MarketingCampaignStatusPaused, marketingAutomationDisabledReason).
			Order("id ASC").Find(&campaigns).Error; err != nil {
			return err
		}
		for _, campaign := range campaigns {
			if err := setMarketingCampaignStatusTx(tx, campaign.Id, []string{MarketingCampaignStatusPaused}, MarketingCampaignStatusRunning, ""); err != nil {
				return err
			}
		}
		return nil
	})
}

func MarkMarketingAutomationBaselineReady(scene string, enabledTime int64) error {
	result := DB.Model(&MarketingAutomation{}).
		Where("scene = ? AND enabled = ? AND enabled_time = ?", scene, true, enabledTime).
		Update("baseline_ready", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrMarketingInvalid
	}
	return nil
}

func CreateMarketingCampaign(campaign *MarketingCampaign) error {
	if campaign == nil {
		return ErrMarketingInvalid
	}
	return DB.Create(campaign).Error
}

func UpdateMarketingCampaign(campaign *MarketingCampaign) error {
	if campaign == nil || campaign.Id <= 0 {
		return ErrMarketingInvalid
	}
	result := DB.Model(&MarketingCampaign{}).Where("id = ? AND status = ?", campaign.Id, MarketingCampaignStatusDraft).
		Updates(map[string]any{"name": campaign.Name, "audience_rule": campaign.AudienceRule, "localized_content": campaign.LocalizedContent, "action_path": campaign.ActionPath, "scheduled_time": campaign.ScheduledTime})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrMarketingInvalid
	}
	return nil
}

func GetMarketingCampaign(id int) (*MarketingCampaign, error) {
	campaign := &MarketingCampaign{}
	err := DB.First(campaign, id).Error
	return campaign, err
}

func ListMarketingCampaigns(pageInfo *common.PageInfo) ([]*MarketingCampaign, int64, error) {
	rows := []*MarketingCampaign{}
	var total int64
	if err := DB.Model(&MarketingCampaign{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := DB.Order("id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	for _, row := range rows {
		_ = DB.Model(&MarketingRecipient{}).Where("campaign_id = ?", row.Id).Count(&row.RecipientCount).Error
		_ = DB.Model(&MarketingRecipient{}).Where("campaign_id = ? AND status = ?", row.Id, MarketingRecipientStatusDelivered).Count(&row.DeliveredCount).Error
		_ = DB.Model(&MarketingRecipient{}).Where("campaign_id = ? AND status = ?", row.Id, MarketingRecipientStatusFailed).Count(&row.FailedCount).Error
		_ = DB.Model(&MarketingRecipient{}).Where("campaign_id = ? AND clicked_time > 0", row.Id).Count(&row.ClickedCount).Error
		_ = DB.Model(&MarketingRecipient{}).Where("campaign_id = ? AND converted_time > 0", row.Id).Count(&row.ConvertedCount).Error
		_ = DB.Model(&MarketingEvent{}).Where("campaign_id = ? AND event_type = ?", row.Id, MarketingEventConversion).Select("COALESCE(SUM(amount_cents), 0)").Scan(&row.ConvertedCents).Error
	}
	return rows, total, nil
}

func SetMarketingCampaignStatus(id int, from []string, status string, reason string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return setMarketingCampaignStatusTx(tx, id, from, status, reason)
	})
}

func setMarketingCampaignStatusTx(tx *gorm.DB, id int, from []string, status string, reason string) error {
	now := common.GetTimestamp()
	updates := map[string]any{"status": status, "paused_reason": reason, "updated_time": now}
	if status == MarketingCampaignStatusRunning {
		updates["started_time"] = now
	}
	if status == MarketingCampaignStatusCompleted || status == MarketingCampaignStatusCancelled {
		updates["completed_time"] = now
	}
	result := tx.Model(&MarketingCampaign{}).Where("id = ? AND status IN ?", id, from).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrMarketingInvalid
	}
	if status == MarketingCampaignStatusRunning {
		deliveryIds := []int{}
		if err := tx.Model(&MarketingRecipient{}).
			Where("campaign_id = ? AND status = ? AND email_delivery_id > 0", id, MarketingRecipientStatusQueued).
			Pluck("email_delivery_id", &deliveryIds).Error; err != nil {
			return err
		}
		if len(deliveryIds) > 0 {
			if err := tx.Model(&EmailDelivery{}).
				Where("id IN ? AND delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0", deliveryIds).
				Updates(map[string]any{"next_attempt_time": now, "locked_until": int64(0), "updated_time": now}).Error; err != nil {
				return err
			}
		}
	}
	if status != MarketingCampaignStatusCancelled {
		return nil
	}
	deliveryIds := []int{}
	if err := tx.Model(&MarketingRecipient{}).
		Where("campaign_id = ? AND status = ? AND email_delivery_id > 0", id, MarketingRecipientStatusQueued).
		Pluck("email_delivery_id", &deliveryIds).Error; err != nil {
		return err
	}
	if len(deliveryIds) > 0 {
		if err := tx.Model(&EmailDelivery{}).
			Where("id IN ? AND delivered_time = 0 AND expired_time = 0", deliveryIds).
			Updates(map[string]any{
				"recipient":         "",
				"subject":           "",
				"body":              "",
				"last_error":        "marketing campaign cancelled",
				"locked_until":      int64(0),
				"next_attempt_time": int64(0),
				"dead_letter_time":  int64(0),
				"expired_time":      now,
				"updated_time":      now,
			}).Error; err != nil {
			return err
		}
	}
	return tx.Model(&MarketingRecipient{}).
		Where("campaign_id = ? AND status IN ?", id, []string{MarketingRecipientStatusPending, MarketingRecipientStatusQueued}).
		Updates(map[string]any{
			"status":                   MarketingRecipientStatusSkipped,
			"last_error":               "marketing campaign cancelled",
			"dispatch_token_encrypted": "",
			"updated_time":             now,
		}).Error
}

func ScheduleMarketingCampaign(id int, scheduledTime int64) error {
	if scheduledTime <= 0 {
		scheduledTime = common.GetTimestamp()
	}
	result := DB.Model(&MarketingCampaign{}).Where("id = ? AND status = ?", id, MarketingCampaignStatusDraft).
		Updates(map[string]any{"status": MarketingCampaignStatusScheduled, "scheduled_time": scheduledTime, "paused_reason": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrMarketingInvalid
	}
	return nil
}

func ListDueMarketingCampaigns(now int64, limit int) ([]*MarketingCampaign, error) {
	rows := []*MarketingCampaign{}
	err := DB.Where("status = ? AND (scheduled_time = 0 OR scheduled_time <= ?)", MarketingCampaignStatusScheduled, now).
		Order("scheduled_time ASC, id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func ListRunningMarketingCampaigns(limit int, offset int) ([]*MarketingCampaign, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows := []*MarketingCampaign{}
	err := DB.Where("status = ?", MarketingCampaignStatusRunning).
		Order("id ASC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, err
}

func GetActiveAutomationCampaign(scene string) (*MarketingCampaign, error) {
	row := &MarketingCampaign{}
	err := DB.Where("scene = ? AND automatic = ? AND status IN ?", scene, true, []string{MarketingCampaignStatusRunning, MarketingCampaignStatusPaused}).Order("id DESC").First(row).Error
	return row, err
}

func FindMarketingCampaignByAnnouncementId(announcementId int) (*MarketingCampaign, error) {
	row := &MarketingCampaign{}
	err := DB.Where("scene = ? AND announcement_id = ?", MarketingSceneAnnouncement, announcementId).First(row).Error
	return row, err
}

func CountUserMarketingSceneRecipients(userId int, scene string) (int64, int64, error) {
	var count int64
	var lastAttempt int64
	query := DB.Model(&MarketingRecipient{}).
		Joins("JOIN marketing_campaigns ON marketing_campaigns.id = marketing_recipients.campaign_id").
		Where("marketing_recipients.user_id = ? AND marketing_campaigns.scene = ? AND marketing_campaigns.automatic = ? AND marketing_recipients.status <> ?", userId, scene, true, MarketingRecipientStatusSkipped)
	if err := query.Count(&count).Error; err != nil {
		return 0, 0, err
	}
	if err := query.Select("COALESCE(MAX(marketing_recipients.created_time), 0)").Scan(&lastAttempt).Error; err != nil {
		return 0, 0, err
	}
	return count, lastAttempt, nil
}

func CountCampaignRecipientsByStatuses(campaignId int, statuses []string) (int64, error) {
	var count int64
	err := DB.Model(&MarketingRecipient{}).Where("campaign_id = ? AND status IN ?", campaignId, statuses).Count(&count).Error
	return count, err
}

func RecentCampaignDeliveryOutcomes(campaignId int, limit int) ([]string, error) {
	type outcomeRow struct{ Status string }
	rows := []outcomeRow{}
	err := DB.Model(&MarketingRecipient{}).
		Select("status").Where("campaign_id = ? AND status IN ?", campaignId, []string{MarketingRecipientStatusDelivered, MarketingRecipientStatusFailed}).
		Order("updated_time DESC, id DESC").Limit(limit).Scan(&rows).Error
	outcomes := make([]string, 0, len(rows))
	for _, row := range rows {
		outcomes = append(outcomes, row.Status)
	}
	return outcomes, err
}

func GetMarketingRecipient(id int) (*MarketingRecipient, error) {
	row := &MarketingRecipient{}
	err := DB.First(row, id).Error
	return row, err
}

func GetMarketingRecipientByEmailDeliveryId(deliveryId int) (*MarketingRecipient, error) {
	if deliveryId <= 0 {
		return nil, ErrMarketingInvalid
	}
	row := &MarketingRecipient{}
	err := DB.Where("email_delivery_id = ?", deliveryId).First(row).Error
	return row, err
}

func CloneMarketingCampaign(id int, createdBy int) (*MarketingCampaign, error) {
	source, err := GetMarketingCampaign(id)
	if err != nil {
		return nil, err
	}
	clone := &MarketingCampaign{Name: source.Name + " (copy)", Scene: source.Scene, Status: MarketingCampaignStatusDraft, AudienceRule: source.AudienceRule, LocalizedContent: source.LocalizedContent, ActionPath: source.ActionPath, CreatedBy: createdBy}
	err = DB.Create(clone).Error
	return clone, err
}

func ListMarketingAudience(rule MarketingAudienceRule, limit int, offset int) ([]*MarketingAudienceUser, int64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	topUpStats := DB.Model(&TopUp{}).
		Select("user_id, COUNT(*) AS top_up_count, MAX(complete_time) AS last_top_up_time, MAX(id) AS last_top_up_id").
		Where("status = ? AND payment_provider = ?", common.TopUpStatusSuccess, PaymentProviderEpay).
		Group("user_id")
	query := DB.Table("users").
		Select("users.id, users.username, users.email, users.setting, users.quota, users.used_quota, users.created_at, users.last_login_at, COALESCE(marketing_topups.top_up_count, 0) AS top_up_count, COALESCE(marketing_topups.last_top_up_time, 0) AS last_top_up_time, COALESCE(marketing_topups.last_top_up_id, 0) AS last_top_up_id").
		Joins("LEFT JOIN (?) AS marketing_topups ON marketing_topups.user_id = users.id", topUpStats).
		Where("users.status = ? AND users.role = ? AND users.email <> '' AND users.deleted_at IS NULL", common.UserStatusEnabled, common.RoleCommonUser).
		Where("NOT EXISTS (?)", DB.Model(&MarketingSuppression{}).Select("1").Where("marketing_suppressions.user_id = users.id AND marketing_suppressions.user_id > 0"))
	if rule.UserId > 0 {
		query = query.Where("users.id = ?", rule.UserId)
	}
	if len(rule.Groups) > 0 {
		query = query.Where("users."+commonGroupCol+" IN ?", rule.Groups)
	}
	if rule.InactiveDays > 0 {
		cutoff := common.GetTimestamp() - int64(rule.InactiveDays)*86400
		query = query.Where("(users.last_login_at > 0 AND users.last_login_at <= ?) OR (users.last_login_at = 0 AND users.created_at <= ?)", cutoff, cutoff)
	}
	if rule.TopUpCountMin != nil {
		query = query.Where("COALESCE(marketing_topups.top_up_count, 0) >= ?", *rule.TopUpCountMin)
	}
	if rule.TopUpCountMax != nil {
		query = query.Where("COALESCE(marketing_topups.top_up_count, 0) <= ?", *rule.TopUpCountMax)
	}
	if rule.LastTopUpBefore > 0 {
		query = query.Where("COALESCE(marketing_topups.last_top_up_time, 0) > 0 AND marketing_topups.last_top_up_time <= ?", rule.LastTopUpBefore)
	}
	if rule.QuotaMin != nil {
		query = query.Where("users.quota >= ?", *rule.QuotaMin)
	}
	if rule.QuotaMax != nil {
		query = query.Where("users.quota <= ?", *rule.QuotaMax)
	}
	if rule.UsedQuotaPositive {
		query = query.Where("users.used_quota > 0")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []*MarketingAudienceUser{}
	err := query.Order("users.id ASC").Limit(limit).Offset(offset).Scan(&rows).Error
	return rows, total, err
}

func IsMarketingSuppressed(userId int, email string) bool {
	var count int64
	hash := hashMarketingValue(strings.ToLower(strings.TrimSpace(email)))
	_ = DB.Model(&MarketingSuppression{}).Where("user_id = ? OR email_hash = ?", userId, hash).Count(&count).Error
	return count > 0
}

func CreateMarketingSuppression(userId int, email string, reason string, createdBy int) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return ErrMarketingInvalid
	}
	// Manual suppressions are entered by email. Resolve the local user id when
	// possible so SQL audience previews exclude the address as accurately as
	// the final pre-send suppression check does.
	if userId <= 0 {
		_ = DB.Model(&User{}).
			Select("id").
			Where("LOWER(email) = ? AND deleted_at IS NULL", email).
			Limit(1).
			Scan(&userId).Error
	}
	record := &MarketingSuppression{UserId: userId, EmailHash: hashMarketingValue(email), EmailMasked: maskEmailAddress(email), Reason: strings.TrimSpace(reason), CreatedBy: createdBy}
	if record.Reason == "" {
		record.Reason = "manual"
	}
	return DB.Clauses(clause.OnConflict{DoNothing: true}).Create(record).Error
}

func DeleteMarketingSuppression(id int) error {
	return DB.Delete(&MarketingSuppression{}, id).Error
}

func ListMarketingSuppressions(pageInfo *common.PageInfo) ([]*MarketingSuppression, int64, error) {
	rows := []*MarketingSuppression{}
	var total int64
	if err := DB.Model(&MarketingSuppression{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := DB.Order("id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&rows).Error
	return rows, total, err
}

func CreateMarketingRecipient(recipient *MarketingRecipient) (bool, error) {
	result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(recipient)
	return result.RowsAffected == 1, result.Error
}

func ListPendingMarketingRecipients(limit int) ([]*MarketingRecipient, error) {
	rows := []*MarketingRecipient{}
	err := DB.Model(&MarketingRecipient{}).
		Select("marketing_recipients.*").
		Joins("JOIN marketing_campaigns ON marketing_campaigns.id = marketing_recipients.campaign_id").
		Where("marketing_recipients.status = ?", MarketingRecipientStatusPending).
		Order(`CASE marketing_campaigns.scene
			WHEN 'paid_low_balance' THEN 1
			WHEN 'trial_low_balance' THEN 2
			WHEN 'single_topup_winback' THEN 3
			WHEN 'inactive_user' THEN 4
			WHEN 'announcement' THEN 5
			ELSE 6 END ASC, marketing_recipients.id ASC`).
		Limit(limit).Find(&rows).Error
	return rows, err
}

func ListMarketingRecipients(campaignId int, pageInfo *common.PageInfo) ([]*MarketingRecipient, int64, error) {
	query := DB.Model(&MarketingRecipient{}).Where("campaign_id = ?", campaignId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []*MarketingRecipient{}
	err := query.Select("marketing_recipients.*, users.username").Joins("LEFT JOIN users ON users.id = marketing_recipients.user_id").Order("marketing_recipients.id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Scan(&rows).Error
	return rows, total, err
}

func MarketingUserMatchesAudience(userId int, rule MarketingAudienceRule) (bool, *MarketingAudienceUser, error) {
	if userId <= 0 {
		return false, nil, ErrMarketingInvalid
	}
	rule.UserId = userId
	rows, _, err := ListMarketingAudience(rule, 1, 0)
	if err != nil {
		return false, nil, err
	}
	if len(rows) == 0 {
		return false, nil, nil
	}
	return true, rows[0], nil
}

func UserMarketingCooldownActive(userId int, excludeRecipientId int, since int64) bool {
	var count int64
	_ = DB.Model(&MarketingRecipient{}).
		Where("user_id = ? AND id <> ? AND ((status = ? AND delivered_time >= ?) OR (status IN ? AND created_time >= ?))",
			userId,
			excludeRecipientId,
			MarketingRecipientStatusDelivered,
			since,
			[]string{MarketingRecipientStatusQueued},
			since,
		).
		Count(&count).Error
	return count > 0
}

func UpdateMarketingRecipientQueued(id int, deliveryId int, queuedTime int64) error {
	return DB.Model(&MarketingRecipient{}).Where("id = ? AND status = ?", id, MarketingRecipientStatusPending).
		Updates(map[string]any{"email_delivery_id": deliveryId, "status": MarketingRecipientStatusQueued, "queued_time": queuedTime}).Error
}

func SetMarketingRecipientDispatchToken(id int, encrypted string) error {
	return DB.Model(&MarketingRecipient{}).Where("id = ? AND status = ?", id, MarketingRecipientStatusPending).Update("dispatch_token_encrypted", encrypted).Error
}

func GetMarketingRecipientDispatchToken(id int) (string, error) {
	row := &MarketingRecipient{}
	if err := DB.Select("id", "dispatch_token_encrypted").First(row, id).Error; err != nil {
		return "", err
	}
	plain, err := common.DecryptSensitiveValue("marketing-click-token", row.DispatchTokenEncrypted)
	return string(plain), err
}

func ClearMarketingRecipientDispatchToken(id int) error {
	return DB.Model(&MarketingRecipient{}).Where("id = ?", id).Update("dispatch_token_encrypted", "").Error
}

func ReconcileMarketingRecipients(limit int) error {
	rows := []*MarketingRecipient{}
	err := DB.Where("status = ? AND email_delivery_id > 0", MarketingRecipientStatusQueued).Order("id ASC").Limit(limit).Find(&rows).Error
	if err != nil {
		return err
	}
	for _, row := range rows {
		delivery, err := GetEmailDeliveryById(row.EmailDeliveryId)
		if err != nil {
			continue
		}
		if delivery.DeliveredTime > 0 {
			_ = DB.Model(row).Updates(map[string]any{"status": MarketingRecipientStatusDelivered, "delivered_time": delivery.DeliveredTime, "last_error": ""}).Error
		} else if delivery.DeadLetterTime > 0 || delivery.ExpiredTime > 0 {
			_ = DB.Model(row).Updates(map[string]any{"status": MarketingRecipientStatusFailed, "last_error": delivery.LastError}).Error
			if delivery.DeadLetterTime > 0 && isPermanentMarketingEmailFailure(delivery.LastError) && delivery.Recipient != "" {
				_ = CreateMarketingSuppression(row.UserId, delivery.Recipient, "permanent SMTP rejection", 0)
			}
		}
	}
	return nil
}

func HasRecentMarketingBalanceRecipient(userId int, since int64) bool {
	var count int64
	_ = DB.Model(&MarketingRecipient{}).
		Joins("JOIN marketing_campaigns ON marketing_campaigns.id = marketing_recipients.campaign_id").
		Where("marketing_recipients.user_id = ? AND marketing_recipients.created_time >= ? AND marketing_campaigns.scene IN ? AND marketing_recipients.status IN ?",
			userId,
			since,
			[]string{MarketingScenePaidLowBalance, MarketingSceneTrialLowBalance},
			[]string{MarketingRecipientStatusPending, MarketingRecipientStatusQueued, MarketingRecipientStatusDelivered},
		).
		Count(&count).Error
	return count > 0
}

func HasRecentEmailDelivery(userId int, category string, since int64) bool {
	var count int64
	_ = DB.Model(&EmailDelivery{}).
		Where("user_id = ? AND category = ? AND created_time >= ?", userId, category, since).
		Count(&count).Error
	return count > 0
}

func isPermanentMarketingEmailFailure(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	for _, marker := range []string{" 550 ", " 551 ", " 553 ", "5.1.1", "user unknown", "recipient rejected", "address rejected", "no such user"} {
		if strings.Contains(" "+normalized+" ", marker) {
			return true
		}
	}
	return false
}

func SkipMarketingRecipient(id int, reason string) error {
	return DB.Model(&MarketingRecipient{}).Where("id = ? AND status = ?", id, MarketingRecipientStatusPending).
		Updates(map[string]any{"status": MarketingRecipientStatusSkipped, "last_error": reason}).Error
}

func SkipQueuedMarketingRecipient(id int, reason string) error {
	return DB.Model(&MarketingRecipient{}).Where("id = ? AND status IN ?", id, []string{MarketingRecipientStatusPending, MarketingRecipientStatusQueued}).
		Updates(map[string]any{
			"status":                   MarketingRecipientStatusSkipped,
			"last_error":               strings.TrimSpace(reason),
			"dispatch_token_encrypted": "",
			"updated_time":             common.GetTimestamp(),
		}).Error
}

func RetryMarketingRecipientByEmailDeliveryId(deliveryId int) error {
	return DB.Model(&MarketingRecipient{}).
		Where("email_delivery_id = ? AND status = ?", deliveryId, MarketingRecipientStatusFailed).
		Updates(map[string]any{
			"status":       MarketingRecipientStatusQueued,
			"last_error":   "",
			"updated_time": common.GetTimestamp(),
		}).Error
}

func FindMarketingRecipientByToken(token string) (*MarketingRecipient, error) {
	row := &MarketingRecipient{}
	err := DB.Where("click_token_hash = ?", hashMarketingValue(token)).First(row).Error
	return row, err
}

func RecordMarketingClick(recipient *MarketingRecipient) error {
	if recipient == nil || recipient.Id <= 0 {
		return ErrMarketingInvalid
	}
	now := common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&MarketingRecipient{}).Where("id = ? AND clicked_time = 0", recipient.Id).Update("clicked_time", now)
		if result.Error != nil {
			return result.Error
		}
		event := &MarketingEvent{EventKey: "click:" + recipient.DedupeKey, CampaignId: recipient.CampaignId, RecipientId: recipient.Id, UserId: recipient.UserId, EventType: MarketingEventClick, CreatedTime: now}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(event).Error
	})
}

func AttributeMarketingConversionTx(tx *gorm.DB, topUp *TopUp) error {
	if tx == nil || topUp == nil || topUp.Id <= 0 || topUp.UserId <= 0 || topUp.Status != common.TopUpStatusSuccess || topUp.PaymentProvider != PaymentProviderEpay {
		return nil
	}
	recipient := &MarketingRecipient{}
	clickAfter := topUp.CompleteTime - 7*86400
	err := tx.Where("user_id = ? AND clicked_time >= ? AND clicked_time <= ?", topUp.UserId, clickAfter, topUp.CompleteTime).Order("clicked_time DESC, id DESC").First(recipient).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// Never fall back to an older click after the last-click recipient has
	// already received its first-order attribution. A later top-up requires a
	// genuinely newer click to start another attribution window.
	if recipient.ConvertedTime > 0 {
		return nil
	}
	event := &MarketingEvent{EventKey: "conversion:topup:" + strconv.Itoa(topUp.Id), CampaignId: recipient.CampaignId, RecipientId: recipient.Id, UserId: topUp.UserId, EventType: MarketingEventConversion, TopUpId: topUp.Id, AmountCents: localMoneyCents(topUp.Money), CreatedTime: topUp.CompleteTime}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(event)
	if result.Error != nil || result.RowsAffected == 0 {
		return result.Error
	}
	return tx.Model(&MarketingRecipient{}).Where("id = ? AND converted_time = 0", recipient.Id).Update("converted_time", topUp.CompleteTime).Error
}

func AttributeMarketingConversion(topUp *TopUp) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return AttributeMarketingConversionTx(tx, topUp)
	})
}

func GetMarketingOverview() (MarketingOverview, error) {
	result := MarketingOverview{}
	if err := DB.Model(&MarketingCampaign{}).Count(&result.Campaigns).Error; err != nil {
		return result, err
	}
	counts := []struct {
		value *int64
		where string
		args  []any
	}{
		{&result.Queued, "status IN ?", []any{[]string{MarketingRecipientStatusPending, MarketingRecipientStatusQueued}}},
		{&result.Delivered, "status = ?", []any{MarketingRecipientStatusDelivered}},
		{&result.Failed, "status = ?", []any{MarketingRecipientStatusFailed}},
		{&result.Clicked, "clicked_time > 0", nil},
		{&result.Converted, "converted_time > 0", nil},
	}
	for _, item := range counts {
		if err := DB.Model(&MarketingRecipient{}).Where(item.where, item.args...).Count(item.value).Error; err != nil {
			return result, err
		}
	}
	if err := DB.Model(&MarketingEvent{}).Where("event_type = ?", MarketingEventConversion).Select("COALESCE(SUM(amount_cents), 0)").Scan(&result.ConvertedCents).Error; err != nil {
		return result, err
	}
	return result, nil
}

func GetMarketingCircuitState() (MarketingCircuitState, error) {
	result := MarketingCircuitState{}
	query := DB.Model(&MarketingCampaign{}).
		Where("status = ? AND paused_reason LIKE ?", MarketingCampaignStatusPaused, "marketing delivery circuit breaker:%")
	if err := query.Count(&result.PausedCampaigns).Error; err != nil {
		return result, err
	}
	if result.PausedCampaigns == 0 {
		return result, nil
	}
	row := &MarketingCampaign{}
	if err := query.Order("updated_time DESC, id DESC").First(row).Error; err != nil {
		return result, err
	}
	result.LastReason = row.PausedReason
	return result, nil
}

func hashMarketingValue(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func HashMarketingToken(token string) string {
	return hashMarketingValue(strings.TrimSpace(token))
}

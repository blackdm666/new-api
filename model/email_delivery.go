package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const EmailDeliveryMaxAttempts = 8

var (
	ErrEmailDeliveryIdInvalid          = errors.New("email delivery id is invalid")
	ErrMarketingEmailDailyLimitReached = errors.New("marketing email daily limit reached")
)

type EmailDeliveryQueryOptions struct {
	Keyword  string
	Status   string
	Category string
}

const (
	EmailDeliveryStatusQueued            = "queued"
	EmailDeliveryStatusSending           = "sending"
	EmailDeliveryStatusRetrying          = "retrying"
	EmailDeliveryStatusAwaitingReceipt   = "awaiting_receipt"
	EmailDeliveryStatusAcceptedUntracked = "accepted_untracked"
	EmailDeliveryStatusDelivered         = "delivered"
	EmailDeliveryStatusFailed            = "failed"
	EmailDeliveryStatusExpired           = "expired"

	EmailPriorityMarketing = 10
	EmailPriorityBusiness  = 100
	EmailPriorityCritical  = 200
)

// EmailDeliveryListItem deliberately excludes subject and body. The Root UI
// only needs delivery metadata to diagnose SMTP failures, and message content
// should not be returned to the browser merely for queue maintenance.
type EmailDeliveryListItem struct {
	Id                int    `json:"id"`
	Category          string `json:"category"`
	RelatedId         int    `json:"related_id"`
	UserId            int    `json:"user_id"`
	InvoiceDeliveryId int    `json:"invoice_delivery_id"`
	Recipient         string `json:"recipient"`
	Priority          int    `json:"priority"`
	Status            string `json:"status" gorm:"-"`
	State             string `json:"state"`
	SenderAccountId   int    `json:"sender_account_id"`
	SenderAccountName string `json:"sender_account_name" gorm:"-"`
	CurrentAttemptId  int    `json:"current_attempt_id"`
	Attempts          int    `json:"attempts"`
	LastError         string `json:"last_error"`
	FailureType       string `json:"failure_type"`
	NextAttemptTime   int64  `json:"next_attempt_time"`
	LockedUntil       int64  `json:"locked_until"`
	ExpiresTime       int64  `json:"expires_time"`
	AcceptedTime      int64  `json:"accepted_time"`
	FinalizedTime     int64  `json:"finalized_time"`
	ReceiptDeadline   int64  `json:"receipt_deadline"`
	DeliveredTime     int64  `json:"delivered_time"`
	DeadLetterTime    int64  `json:"dead_letter_time"`
	ExpiredTime       int64  `json:"expired_time"`
	CreatedTime       int64  `json:"created_time"`
	UpdatedTime       int64  `json:"updated_time"`
}

type EmailDeliveryStats struct {
	Queued                  int64   `json:"queued"`
	Sending                 int64   `json:"sending"`
	Retrying                int64   `json:"retrying"`
	AwaitingReceipt         int64   `json:"awaiting_receipt"`
	AcceptedUntracked24h    int64   `json:"accepted_untracked_24h"`
	FinalDelivered24h       int64   `json:"final_delivered_24h"`
	Failed                  int64   `json:"failed"`
	Delivered24h            int64   `json:"delivered_24h"`
	Failed24h               int64   `json:"failed_24h"`
	FailureRate24h          float64 `json:"failure_rate_24h"`
	OldestPendingTime       int64   `json:"oldest_pending_time"`
	LastDeliveredTime       int64   `json:"last_delivered_time"`
	MarketingSentToday      int64   `json:"marketing_sent_today"`
	MarketingQuotaUsedToday int64   `json:"marketing_quota_used_today"`
}

// EmailDelivery is the shared durable SMTP outbox for NewAPI system emails.
// Business modules provide a stable delivery key so retries and repeated API
// requests cannot send the same event twice.
type EmailDelivery struct {
	Id                 int    `json:"id"`
	DeliveryKey        string `json:"delivery_key" gorm:"type:varchar(160);uniqueIndex;not null"`
	Category           string `json:"category" gorm:"type:varchar(48);not null;index"`
	SMTPProfile        string `json:"smtp_profile" gorm:"type:varchar(24);not null;default:'notification';index"`
	SMTPChannel        string `json:"smtp_channel" gorm:"type:varchar(24);not null;default:'';index"`
	MessageID          string `json:"message_id" gorm:"column:message_id;type:varchar(191);not null;default:'';index"`
	RelatedId          int    `json:"related_id" gorm:"index"`
	UserId             int    `json:"user_id" gorm:"index"`
	InvoiceDeliveryId  int    `json:"invoice_delivery_id" gorm:"index"`
	Recipient          string `json:"recipient" gorm:"type:varchar(320);not null"`
	RecipientMasked    string `json:"recipient_masked" gorm:"type:varchar(320);not null;default:''"`
	Subject            string `json:"subject" gorm:"type:varchar(512);not null"`
	Body               string `json:"body" gorm:"type:text;not null"`
	Priority           int    `json:"priority" gorm:"not null;default:100;index"`
	State              string `json:"state" gorm:"type:varchar(32);not null;default:'queued';index"`
	SenderAccountId    int    `json:"sender_account_id" gorm:"index"`
	CurrentAttemptId   int    `json:"current_attempt_id" gorm:"index"`
	MarketingQuotaTime int64  `json:"-" gorm:"bigint;not null;default:0;index"`
	Attempts           int    `json:"attempts" gorm:"not null;default:0"`
	LastError          string `json:"last_error" gorm:"type:text"`
	FailureType        string `json:"failure_type" gorm:"type:varchar(64);not null;default:'';index"`
	NextAttemptTime    int64  `json:"next_attempt_time" gorm:"bigint;not null;index"`
	LockedUntil        int64  `json:"locked_until" gorm:"bigint;not null;default:0;index"`
	ExpiresTime        int64  `json:"expires_time" gorm:"bigint;not null;default:0;index"`
	AcceptedTime       int64  `json:"accepted_time" gorm:"bigint;not null;default:0;index"`
	FinalizedTime      int64  `json:"finalized_time" gorm:"bigint;not null;default:0;index"`
	ReceiptDeadline    int64  `json:"receipt_deadline" gorm:"bigint;not null;default:0;index"`
	DeliveredTime      int64  `json:"delivered_time" gorm:"bigint;not null;default:0;index"`
	DeadLetterTime     int64  `json:"dead_letter_time" gorm:"bigint;not null;default:0;index"`
	ExpiredTime        int64  `json:"expired_time" gorm:"bigint;not null;default:0;index"`
	CreatedTime        int64  `json:"created_time" gorm:"bigint;autoCreateTime;index"`
	UpdatedTime        int64  `json:"updated_time" gorm:"bigint;autoUpdateTime"`
}

// MarketingEmailDailyQuota serializes quota reservations for one Shanghai
// calendar day. The counter is separate from delivery state so multiple app
// instances cannot all pass a count-then-insert check at the daily boundary.
type MarketingEmailDailyQuota struct {
	DayStart    int64 `json:"day_start" gorm:"primaryKey;autoIncrement:false"`
	Reserved    int64 `json:"reserved" gorm:"not null;default:0"`
	UpdatedTime int64 `json:"updated_time" gorm:"bigint;not null"`
}

func EnqueueEmailDelivery(delivery *EmailDelivery) (*EmailDelivery, bool, error) {
	return enqueueEmailDelivery(DB, delivery)
}

func EnqueueEmailDeliveryTx(tx *gorm.DB, delivery *EmailDelivery) (*EmailDelivery, bool, error) {
	if tx == nil {
		return nil, false, gorm.ErrInvalidDB
	}
	return enqueueEmailDelivery(tx, delivery)
}

func enqueueEmailDelivery(tx *gorm.DB, delivery *EmailDelivery) (*EmailDelivery, bool, error) {
	if delivery == nil {
		return nil, false, gorm.ErrInvalidData
	}
	delivery.DeliveryKey = strings.TrimSpace(delivery.DeliveryKey)
	delivery.Category = strings.TrimSpace(delivery.Category)
	delivery.SMTPProfile = strings.TrimSpace(delivery.SMTPProfile)
	if delivery.SMTPProfile == "" {
		delivery.SMTPProfile = "notification"
	}
	delivery.Recipient = strings.TrimSpace(delivery.Recipient)
	if delivery.DeliveryKey == "" || delivery.Category == "" || delivery.Recipient == "" || strings.TrimSpace(delivery.Subject) == "" || strings.TrimSpace(delivery.Body) == "" {
		return nil, false, gorm.ErrInvalidData
	}
	now := common.GetTimestamp()
	if delivery.Priority <= 0 {
		delivery.Priority = EmailPriorityBusiness
	}
	delivery.CreatedTime = now
	delivery.UpdatedTime = now
	delivery.NextAttemptTime = now
	delivery.State = EmailDeliveryStatusQueued
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(delivery)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		return delivery, true, nil
	}
	existing := &EmailDelivery{}
	if err := tx.Where("delivery_key = ?", delivery.DeliveryKey).First(existing).Error; err != nil {
		return nil, false, err
	}
	return existing, false, nil
}

// EnqueueMarketingEmailDelivery persists a marketing message only after it
// acquires one slot from the day's allowance. Both operations share one
// transaction, so a rejected message never leaks into the deliverable queue.
func EnqueueMarketingEmailDelivery(delivery *EmailDelivery, dayStart int64, now int64, limit int64) (*EmailDelivery, bool, error) {
	if delivery == nil || dayStart <= 0 || now < dayStart || now >= dayStart+86400 || limit <= 0 {
		return nil, false, ErrEmailDeliveryIdInvalid
	}
	var queued *EmailDelivery
	created := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		queued, created, err = enqueueEmailDelivery(tx, delivery)
		if err != nil {
			return err
		}
		reserved, err := reserveMarketingEmailQuotaTx(tx, queued, dayStart, now, limit)
		if err != nil {
			return err
		}
		if !reserved {
			return ErrMarketingEmailDailyLimitReached
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return queued, created, nil
}

func ListDueEmailDeliveries(limit int, now int64) ([]*EmailDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows := []*EmailDelivery{}
	err := DB.Where("state IN ? AND delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0 AND next_attempt_time <= ? AND locked_until <= ? AND (expires_time = 0 OR expires_time > ?)", []string{EmailDeliveryStatusQueued, EmailDeliveryStatusRetrying, EmailDeliveryStatusSending}, now, now, now).
		Order("priority DESC, id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func ClaimEmailDelivery(id int, now int64, lockedUntil int64) (bool, error) {
	result := DB.Model(&EmailDelivery{}).
		Where("id = ? AND state IN ? AND delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0 AND next_attempt_time <= ? AND locked_until <= ? AND (expires_time = 0 OR expires_time > ?)", id, []string{EmailDeliveryStatusQueued, EmailDeliveryStatusRetrying, EmailDeliveryStatusSending}, now, now, now).
		Updates(map[string]any{"state": EmailDeliveryStatusSending, "locked_until": lockedUntil, "updated_time": now})
	return result.RowsAffected == 1, result.Error
}

// ReserveMarketingEmailQuota assigns an old queued or retried marketing
// message to today's allowance before SMTP delivery. Newly created messages
// are already reserved at enqueue time. The transaction keeps cross-day
// backlog and manual retries from bypassing the daily cap.
func ReserveMarketingEmailQuota(id int, dayStart int64, now int64, limit int64) (bool, error) {
	if id <= 0 || dayStart <= 0 || now < dayStart || now >= dayStart+86400 || limit <= 0 {
		return false, ErrEmailDeliveryIdInvalid
	}
	reserved := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		delivery := &EmailDelivery{}
		if err := lockForUpdate(tx).First(delivery, id).Error; err != nil {
			return err
		}
		var err error
		reserved, err = reserveMarketingEmailQuotaTx(tx, delivery, dayStart, now, limit)
		return err
	})
	return reserved, err
}

func reserveMarketingEmailQuotaTx(tx *gorm.DB, delivery *EmailDelivery, dayStart int64, now int64, limit int64) (bool, error) {
	if delivery.Priority != EmailPriorityMarketing && !strings.HasPrefix(delivery.Category, "marketing_") {
		return true, nil
	}
	dayEnd := dayStart + 86400
	if delivery.MarketingQuotaTime >= dayStart && delivery.MarketingQuotaTime < dayEnd {
		return true, nil
	}
	quota := &MarketingEmailDailyQuota{DayStart: dayStart, UpdatedTime: now}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(quota)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		var existing int64
		if err := tx.Model(&EmailDelivery{}).
			Where("category LIKE ? AND marketing_quota_time >= ? AND marketing_quota_time < ?", "marketing_%", dayStart, dayEnd).
			Count(&existing).Error; err != nil {
			return false, err
		}
		if err := tx.Model(quota).Updates(map[string]any{"reserved": existing, "updated_time": now}).Error; err != nil {
			return false, err
		}
	}
	if err := lockForUpdate(tx).Where("day_start = ?", dayStart).First(quota).Error; err != nil {
		return false, err
	}
	if quota.Reserved >= limit {
		return false, nil
	}
	result = tx.Model(&MarketingEmailDailyQuota{}).
		Where("day_start = ? AND reserved < ?", dayStart, limit).
		Updates(map[string]any{"reserved": gorm.Expr("reserved + 1"), "updated_time": now})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected != 1 {
		return false, nil
	}
	result = tx.Model(delivery).
		Updates(map[string]any{"marketing_quota_time": now, "updated_time": now})
	return result.RowsAffected == 1, result.Error
}

func CompleteEmailDelivery(id int, smtpMetadata ...string) error {
	smtpProfile := ""
	smtpChannel := ""
	messageID := ""
	if len(smtpMetadata) > 0 {
		smtpProfile = smtpMetadata[0]
	}
	if len(smtpMetadata) > 1 {
		smtpChannel = smtpMetadata[1]
	}
	if len(smtpMetadata) > 2 {
		messageID = smtpMetadata[2]
	}
	now := common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		delivery := &EmailDelivery{}
		if err := lockForUpdate(tx).First(delivery, id).Error; err != nil {
			return err
		}
		if delivery.DeliveredTime > 0 {
			return nil
		}
		if delivery.ExpiredTime > 0 {
			return errors.New("expired email delivery cannot be completed")
		}
		if err := tx.Model(delivery).Updates(map[string]any{
			"subject":           "",
			"body":              "",
			"last_error":        "",
			"locked_until":      int64(0),
			"next_attempt_time": now,
			"state":             EmailDeliveryStatusAcceptedUntracked,
			"accepted_time":     now,
			"finalized_time":    now,
			"delivered_time":    now,
			"smtp_profile":      strings.TrimSpace(smtpProfile),
			"smtp_channel":      strings.TrimSpace(smtpChannel),
			"message_id":        strings.TrimSpace(messageID),
			"dead_letter_time":  int64(0),
			"updated_time":      now,
		}).Error; err != nil {
			return err
		}
		if delivery.InvoiceDeliveryId <= 0 {
			return nil
		}
		return tx.Model(&InvoiceNotificationDelivery{}).Where("id = ?", delivery.InvoiceDeliveryId).Updates(map[string]any{
			"recipient":         "",
			"subject":           "",
			"body":              "",
			"last_error":        "",
			"locked_until":      int64(0),
			"next_attempt_time": now,
			"delivered_time":    now,
			"updated_time":      now,
		}).Error
	})
}

func RecordEmailDeliveryFailure(id int, message string, nextAttemptTime int64) error {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		delivery := &EmailDelivery{}
		if err := lockForUpdate(tx).Where("id = ? AND delivered_time = 0", id).First(delivery).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"attempts":          gorm.Expr("attempts + 1"),
			"last_error":        message,
			"next_attempt_time": nextAttemptTime,
			"locked_until":      int64(0),
			"updated_time":      common.GetTimestamp(),
		}
		if delivery.Attempts+1 >= setting.GetEmailDeliveryRules().EmailMaxAttempts {
			updates["dead_letter_time"] = common.GetTimestamp()
			updates["next_attempt_time"] = int64(0)
			updates["state"] = EmailDeliveryStatusFailed
		} else {
			updates["state"] = EmailDeliveryStatusRetrying
		}
		return tx.Model(delivery).Updates(updates).Error
	})
}

func ExpireEmailDeliveries(now int64) error {
	return DB.Model(&EmailDelivery{}).
		Where("delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0 AND expires_time > 0 AND expires_time <= ?", now).
		Updates(map[string]any{
			"subject":      "",
			"body":         "",
			"last_error":   "expired before delivery",
			"state":        EmailDeliveryStatusExpired,
			"locked_until": int64(0),
			"expired_time": now,
			"updated_time": now,
		}).Error
}

func ExpireAwaitingEmailReceipts(now int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		due := tx.Model(&EmailDelivery{}).Select("id").
			Where("state = ? AND receipt_deadline > 0 AND receipt_deadline <= ?", EmailDeliveryStatusAwaitingReceipt, now)
		attempts := tx.Model(&EmailDelivery{}).Select("current_attempt_id").
			Where("state = ? AND receipt_deadline > 0 AND receipt_deadline <= ?", EmailDeliveryStatusAwaitingReceipt, now)
		if err := tx.Model(&EmailDeliveryAttempt{}).Where("id IN (?) AND finalized_time = 0", attempts).
			Updates(map[string]any{
				"status": EmailAttemptStatusFailed, "failure_type": "receipt_timeout",
				"error_message": "EventBridge receipt timeout", "finalized_time": now, "updated_time": now,
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&MarketingRecipient{}).Where("email_delivery_id IN (?)", due).
			Updates(map[string]any{
				"status": MarketingRecipientStatusFailed, "last_error": "EventBridge receipt timeout", "updated_time": now,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&EmailDelivery{}).
			Where("state = ? AND receipt_deadline > 0 AND receipt_deadline <= ?", EmailDeliveryStatusAwaitingReceipt, now).
			Updates(map[string]any{
				"state": EmailDeliveryStatusFailed, "subject": "", "body": "",
				"last_error": "EventBridge receipt timeout", "failure_type": "receipt_timeout",
				"dead_letter_time": now, "finalized_time": now,
				"receipt_deadline": int64(0), "updated_time": now,
			}).Error
	})
}

func ExpireEmailDelivery(id int, reason string) error {
	if id <= 0 {
		return ErrEmailDeliveryIdInvalid
	}
	now := common.GetTimestamp()
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "delivery expired"
	}
	if len(reason) > 2000 {
		reason = reason[:2000]
	}
	result := DB.Model(&EmailDelivery{}).
		Where("id = ? AND delivered_time = 0 AND expired_time = 0", id).
		Updates(map[string]any{
			"subject":           "",
			"body":              "",
			"last_error":        reason,
			"state":             EmailDeliveryStatusExpired,
			"locked_until":      int64(0),
			"next_attempt_time": int64(0),
			"dead_letter_time":  int64(0),
			"expired_time":      now,
			"updated_time":      now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("email delivery cannot be expired")
	}
	return nil
}

func RetryEmailDelivery(id int) error {
	now := common.GetTimestamp()
	result := DB.Model(&EmailDelivery{}).
		Where("id = ? AND delivered_time = 0 AND expired_time = 0 AND locked_until <= ? AND (expires_time = 0 OR expires_time > ?)", id, now, now).
		Updates(map[string]any{
			"attempts":          0,
			"last_error":        "",
			"state":             EmailDeliveryStatusQueued,
			"next_attempt_time": now,
			"locked_until":      int64(0),
			"dead_letter_time":  int64(0),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("email delivery cannot be retried")
	}
	return nil
}

func RetryFailedEmailDelivery(id int) error {
	now := common.GetTimestamp()
	result := DB.Model(&EmailDelivery{}).
		Where("id = ? AND delivered_time = 0 AND dead_letter_time > 0 AND expired_time = 0 AND (expires_time = 0 OR expires_time > ?)", id, now).
		Updates(map[string]any{
			"attempts":          0,
			"last_error":        "",
			"state":             EmailDeliveryStatusQueued,
			"next_attempt_time": now,
			"locked_until":      int64(0),
			"dead_letter_time":  int64(0),
			"updated_time":      now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("failed email delivery cannot be retried")
	}
	return nil
}

func DeferEmailDelivery(id int, nextAttemptTime int64) error {
	if id <= 0 || nextAttemptTime <= common.GetTimestamp() {
		return ErrEmailDeliveryIdInvalid
	}
	return DB.Model(&EmailDelivery{}).
		Where("id = ? AND delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0", id).
		Updates(map[string]any{
			"state":             EmailDeliveryStatusQueued,
			"next_attempt_time": nextAttemptTime,
			"locked_until":      int64(0),
			"updated_time":      common.GetTimestamp(),
		}).Error
}

func GetEmailDeliveryById(id int) (*EmailDelivery, error) {
	if id <= 0 {
		return nil, ErrEmailDeliveryIdInvalid
	}
	delivery := &EmailDelivery{}
	if err := DB.Where("id = ?", id).First(delivery).Error; err != nil {
		return nil, err
	}
	return delivery, nil
}

// ListEmailDeliveries is intentionally Root-only at the controller layer.
// Recipient addresses remain visible and searchable for queue maintenance,
// while subject and body content are never selected into the response.
func ListEmailDeliveries(options EmailDeliveryQueryOptions, pageInfo *common.PageInfo) ([]*EmailDeliveryListItem, int64, error) {
	query := DB.Model(&EmailDelivery{})
	now := common.GetTimestamp()
	switch strings.TrimSpace(options.Status) {
	case EmailDeliveryStatusQueued:
		query = query.Where("state = ? AND locked_until <= ?", EmailDeliveryStatusQueued, now)
	case EmailDeliveryStatusSending:
		query = query.Where("state = ? AND locked_until > ?", EmailDeliveryStatusSending, now)
	case EmailDeliveryStatusRetrying:
		query = query.Where("state = ? AND locked_until <= ?", EmailDeliveryStatusRetrying, now)
	case EmailDeliveryStatusAwaitingReceipt, EmailDeliveryStatusAcceptedUntracked:
		query = query.Where("state = ?", strings.TrimSpace(options.Status))
	case EmailDeliveryStatusDelivered:
		query = query.Where("state = ? OR (state = '' AND delivered_time > 0)", EmailDeliveryStatusDelivered)
	case EmailDeliveryStatusFailed:
		query = query.Where("delivered_time = 0 AND dead_letter_time > 0")
	case EmailDeliveryStatusExpired:
		query = query.Where("expired_time > 0")
	}
	if category := strings.TrimSpace(options.Category); category != "" {
		query = query.Where("category = ?", category)
	}
	if keyword := strings.TrimSpace(options.Keyword); keyword != "" {
		pattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		if id, err := strconv.Atoi(keyword); err == nil && id > 0 {
			query = query.Where("(id = ? OR related_id = ? OR user_id = ? OR delivery_key LIKE ? ESCAPE '!' OR category LIKE ? ESCAPE '!' OR recipient LIKE ? ESCAPE '!' OR subject LIKE ? ESCAPE '!' OR user_id IN (?))", id, id, id, pattern, pattern, pattern, pattern,
				DB.Model(&User{}).Select("id").Where("username LIKE ? ESCAPE '!' OR display_name LIKE ? ESCAPE '!'", pattern, pattern))
		} else {
			query = query.Where("(delivery_key LIKE ? ESCAPE '!' OR category LIKE ? ESCAPE '!' OR recipient LIKE ? ESCAPE '!' OR subject LIKE ? ESCAPE '!' OR last_error LIKE ? ESCAPE '!' OR user_id IN (?))", pattern, pattern, pattern, pattern, pattern,
				DB.Model(&User{}).Select("id").Where("username LIKE ? ESCAPE '!' OR display_name LIKE ? ESCAPE '!'", pattern, pattern))
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []*EmailDeliveryListItem{}
	if err := query.Select("id, category, smtp_profile, smtp_channel, message_id, related_id, user_id, invoice_delivery_id, recipient, priority, state, sender_account_id, current_attempt_id, attempts, last_error, failure_type, next_attempt_time, locked_until, expires_time, accepted_time, finalized_time, receipt_deadline, delivered_time, dead_letter_time, expired_time, created_time, updated_time").Order("id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	for _, row := range rows {
		row.Status = emailDeliveryStatus(row, now)
	}
	if err := decorateEmailDeliverySenderNames(rows); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func decorateEmailDeliverySenderNames(rows []*EmailDeliveryListItem) error {
	ids := make([]int, 0, len(rows))
	seen := map[int]struct{}{}
	for _, row := range rows {
		if row.SenderAccountId <= 0 {
			continue
		}
		if _, exists := seen[row.SenderAccountId]; exists {
			continue
		}
		seen[row.SenderAccountId] = struct{}{}
		ids = append(ids, row.SenderAccountId)
	}
	if len(ids) == 0 {
		return nil
	}
	accounts := []EmailSenderAccount{}
	if err := DB.Select("id", "name").Where("id IN ?", ids).Find(&accounts).Error; err != nil {
		return err
	}
	names := make(map[int]string, len(accounts))
	for _, account := range accounts {
		names[account.Id] = account.Name
	}
	for _, row := range rows {
		row.SenderAccountName = names[row.SenderAccountId]
	}
	return nil
}

func RetryEmailDeliveries(ids []int) (int64, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return 0, gorm.ErrInvalidData
	}
	now := common.GetTimestamp()
	result := DB.Model(&EmailDelivery{}).
		Where("id IN ? AND delivered_time = 0 AND dead_letter_time > 0 AND expired_time = 0 AND (expires_time = 0 OR expires_time > ?)", ids, now).
		Updates(map[string]any{
			"attempts":          0,
			"last_error":        "",
			"state":             EmailDeliveryStatusQueued,
			"next_attempt_time": now,
			"locked_until":      int64(0),
			"dead_letter_time":  int64(0),
			"updated_time":      now,
		})
	return result.RowsAffected, result.Error
}

func GetEmailDeliveryStats(now int64, dayStart int64) (EmailDeliveryStats, error) {
	stats := EmailDeliveryStats{}
	dayEnd := dayStart + 86400
	type countRow struct {
		Count int64
	}
	queries := []struct {
		destination *int64
		where       string
		args        []any
	}{
		{&stats.Queued, "state = ? AND locked_until <= ?", []any{EmailDeliveryStatusQueued, now}},
		{&stats.Sending, "state = ? AND locked_until > ?", []any{EmailDeliveryStatusSending, now}},
		{&stats.Retrying, "state = ? AND locked_until <= ?", []any{EmailDeliveryStatusRetrying, now}},
		{&stats.AwaitingReceipt, "state = ?", []any{EmailDeliveryStatusAwaitingReceipt}},
		{&stats.AcceptedUntracked24h, "state = ? AND accepted_time >= ?", []any{EmailDeliveryStatusAcceptedUntracked, now - 86400}},
		{&stats.FinalDelivered24h, "state = ? AND finalized_time >= ?", []any{EmailDeliveryStatusDelivered, now - 86400}},
		{&stats.Failed, "delivered_time = 0 AND dead_letter_time > 0", nil},
		{&stats.Delivered24h, "delivered_time >= ?", []any{now - 86400}},
		{&stats.Failed24h, "dead_letter_time >= ?", []any{now - 86400}},
		{&stats.MarketingSentToday, "category LIKE ? AND delivered_time >= ? AND delivered_time < ?", []any{"marketing_%", dayStart, dayEnd}},
		{&stats.MarketingQuotaUsedToday, "category LIKE ? AND marketing_quota_time >= ? AND marketing_quota_time < ?", []any{"marketing_%", dayStart, dayEnd}},
	}
	for _, item := range queries {
		query := DB.Model(&EmailDelivery{}).Where(item.where, item.args...)
		if err := query.Count(item.destination).Error; err != nil {
			return stats, err
		}
	}
	denominator := stats.Delivered24h + stats.Failed24h
	if denominator > 0 {
		stats.FailureRate24h = float64(stats.Failed24h) / float64(denominator)
	}
	if err := DB.Model(&EmailDelivery{}).Where("delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0").Select("COALESCE(MIN(created_time), 0)").Scan(&stats.OldestPendingTime).Error; err != nil {
		return stats, err
	}
	if err := DB.Model(&EmailDelivery{}).Select("COALESCE(MAX(delivered_time), 0)").Scan(&stats.LastDeliveredTime).Error; err != nil {
		return stats, err
	}
	return stats, nil
}

func ListEmailDeliveryCategories() ([]string, error) {
	categories := []string{}
	err := DB.Model(&EmailDelivery{}).Distinct().Order("category ASC").Pluck("category", &categories).Error
	return categories, err
}

func CleanupEmailDeliveries(deliveredBefore int64, terminalBefore int64, limit int) (int64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	ids := make([]int, 0, limit)
	err := DB.Model(&EmailDelivery{}).Select("id").
		Where("(delivered_time > 0 AND delivered_time < ?) OR ((dead_letter_time > 0 OR expired_time > 0) AND updated_time < ?)", deliveredBefore, terminalBefore).
		Order("id ASC").Limit(limit).Scan(&ids).Error
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	deleted := int64(0)
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("delivery_id IN ?", ids).Delete(&EmailDeliveryAttempt{}).Error; err != nil {
			return err
		}
		result := tx.Where("id IN ?", ids).Delete(&EmailDelivery{})
		deleted = result.RowsAffected
		return result.Error
	})
	return deleted, err
}

func CleanupEmailDeliveryMetadata(before int64) error {
	if before <= 0 {
		return gorm.ErrInvalidData
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("received_time < ?", before).Delete(&EmailReceiptEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Where("created_time < ? AND (delivery_id = 0 OR NOT EXISTS (?))", before,
			tx.Model(&EmailDelivery{}).Select("1").Where("email_deliveries.id = email_delivery_attempts.delivery_id"),
		).Delete(&EmailDeliveryAttempt{}).Error; err != nil {
			return err
		}
		return tx.Where("updated_time < ?", before).Delete(&EmailDeliveryThrottle{}).Error
	})
}

func emailDeliveryStatus(row *EmailDeliveryListItem, now int64) string {
	if row.State == EmailDeliveryStatusAwaitingReceipt || row.State == EmailDeliveryStatusAcceptedUntracked || row.State == EmailDeliveryStatusDelivered || row.State == EmailDeliveryStatusFailed || row.State == EmailDeliveryStatusExpired {
		return row.State
	}
	if row.DeliveredTime > 0 {
		return EmailDeliveryStatusDelivered
	}
	if row.ExpiredTime > 0 {
		return EmailDeliveryStatusExpired
	}
	if row.DeadLetterTime > 0 {
		return EmailDeliveryStatusFailed
	}
	if row.LockedUntil > now {
		return EmailDeliveryStatusSending
	}
	if row.Attempts > 0 {
		return EmailDeliveryStatusRetrying
	}
	return EmailDeliveryStatusQueued
}

func backfillEmailDeliveryStates() error {
	if err := DB.Model(&EmailDelivery{}).
		Where("delivered_time > 0 AND accepted_time = 0").
		Updates(map[string]any{
			"state":          EmailDeliveryStatusAcceptedUntracked,
			"accepted_time":  gorm.Expr("delivered_time"),
			"finalized_time": gorm.Expr("delivered_time"),
		}).Error; err != nil {
		return err
	}
	if err := DB.Model(&EmailDelivery{}).
		Where("expired_time > 0 AND state <> ?", EmailDeliveryStatusExpired).
		Update("state", EmailDeliveryStatusExpired).Error; err != nil {
		return err
	}
	if err := DB.Model(&EmailDelivery{}).
		Where("dead_letter_time > 0 AND expired_time = 0 AND state <> ?", EmailDeliveryStatusFailed).
		Update("state", EmailDeliveryStatusFailed).Error; err != nil {
		return err
	}
	return DB.Model(&EmailDelivery{}).
		Where("delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0 AND attempts > 0 AND state = ?", EmailDeliveryStatusQueued).
		Update("state", EmailDeliveryStatusRetrying).Error
}

func maskEmailAddress(address string) string {
	address = strings.TrimSpace(address)
	parts := strings.Split(address, "@")
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

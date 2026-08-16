package model

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const EmailDeliveryMaxAttempts = 8

var ErrEmailDeliveryIdInvalid = errors.New("email delivery id is invalid")

type EmailDeliveryQueryOptions struct {
	Keyword  string
	Status   string
	Category string
}

const (
	EmailDeliveryStatusQueued    = "queued"
	EmailDeliveryStatusSending   = "sending"
	EmailDeliveryStatusRetrying  = "retrying"
	EmailDeliveryStatusDelivered = "delivered"
	EmailDeliveryStatusFailed    = "failed"
	EmailDeliveryStatusExpired   = "expired"

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
	RecipientMasked   string `json:"recipient_masked"`
	Priority          int    `json:"priority"`
	Status            string `json:"status" gorm:"-"`
	Attempts          int    `json:"attempts"`
	LastError         string `json:"last_error"`
	NextAttemptTime   int64  `json:"next_attempt_time"`
	LockedUntil       int64  `json:"locked_until"`
	ExpiresTime       int64  `json:"expires_time"`
	DeliveredTime     int64  `json:"delivered_time"`
	DeadLetterTime    int64  `json:"dead_letter_time"`
	ExpiredTime       int64  `json:"expired_time"`
	CreatedTime       int64  `json:"created_time"`
	UpdatedTime       int64  `json:"updated_time"`
}

type EmailDeliveryStats struct {
	Queued             int64   `json:"queued"`
	Sending            int64   `json:"sending"`
	Retrying           int64   `json:"retrying"`
	Failed             int64   `json:"failed"`
	Delivered24h       int64   `json:"delivered_24h"`
	Failed24h          int64   `json:"failed_24h"`
	FailureRate24h     float64 `json:"failure_rate_24h"`
	OldestPendingTime  int64   `json:"oldest_pending_time"`
	LastDeliveredTime  int64   `json:"last_delivered_time"`
	MarketingSentToday int64   `json:"marketing_sent_today"`
}

// EmailDelivery is the shared durable SMTP outbox for NewAPI system emails.
// Business modules provide a stable delivery key so retries and repeated API
// requests cannot send the same event twice.
type EmailDelivery struct {
	Id                 int    `json:"id"`
	DeliveryKey        string `json:"delivery_key" gorm:"type:varchar(160);uniqueIndex;not null"`
	Category           string `json:"category" gorm:"type:varchar(48);not null;index"`
	RelatedId          int    `json:"related_id" gorm:"index"`
	UserId             int    `json:"user_id" gorm:"index"`
	InvoiceDeliveryId  int    `json:"invoice_delivery_id" gorm:"index"`
	Recipient          string `json:"recipient" gorm:"type:varchar(320);not null"`
	RecipientMasked    string `json:"recipient_masked" gorm:"type:varchar(320);not null;default:''"`
	Subject            string `json:"subject" gorm:"type:varchar(512);not null"`
	Body               string `json:"body" gorm:"type:text;not null"`
	Priority           int    `json:"priority" gorm:"not null;default:100;index"`
	MarketingQuotaTime int64  `json:"-" gorm:"bigint;not null;default:0;index"`
	Attempts           int    `json:"attempts" gorm:"not null;default:0"`
	LastError          string `json:"last_error" gorm:"type:text"`
	NextAttemptTime    int64  `json:"next_attempt_time" gorm:"bigint;not null;index"`
	LockedUntil        int64  `json:"locked_until" gorm:"bigint;not null;default:0;index"`
	ExpiresTime        int64  `json:"expires_time" gorm:"bigint;not null;default:0;index"`
	DeliveredTime      int64  `json:"delivered_time" gorm:"bigint;not null;default:0;index"`
	DeadLetterTime     int64  `json:"dead_letter_time" gorm:"bigint;not null;default:0;index"`
	ExpiredTime        int64  `json:"expired_time" gorm:"bigint;not null;default:0;index"`
	CreatedTime        int64  `json:"created_time" gorm:"bigint;autoCreateTime;index"`
	UpdatedTime        int64  `json:"updated_time" gorm:"bigint;autoUpdateTime"`
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
	delivery.Recipient = strings.TrimSpace(delivery.Recipient)
	if delivery.DeliveryKey == "" || delivery.Category == "" || delivery.Recipient == "" || strings.TrimSpace(delivery.Subject) == "" || strings.TrimSpace(delivery.Body) == "" {
		return nil, false, gorm.ErrInvalidData
	}
	now := common.GetTimestamp()
	if delivery.Priority <= 0 {
		delivery.Priority = EmailPriorityBusiness
	}
	delivery.RecipientMasked = maskEmailAddress(delivery.Recipient)
	delivery.CreatedTime = now
	delivery.UpdatedTime = now
	delivery.NextAttemptTime = now
	if delivery.Priority == EmailPriorityMarketing || strings.HasPrefix(delivery.Category, "marketing_") {
		delivery.MarketingQuotaTime = now
	}
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

func ListDueEmailDeliveries(limit int, now int64) ([]*EmailDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows := []*EmailDelivery{}
	err := DB.Where("delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0 AND next_attempt_time <= ? AND locked_until <= ? AND (expires_time = 0 OR expires_time > ?)", now, now, now).
		Order("priority DESC, id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func ClaimEmailDelivery(id int, now int64, lockedUntil int64) (bool, error) {
	result := DB.Model(&EmailDelivery{}).
		Where("id = ? AND delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0 AND next_attempt_time <= ? AND locked_until <= ? AND (expires_time = 0 OR expires_time > ?)", id, now, now, now).
		Updates(map[string]any{"locked_until": lockedUntil, "updated_time": now})
	return result.RowsAffected == 1, result.Error
}

// ReserveMarketingEmailQuota assigns an old queued or retried marketing
// message to today's allowance before SMTP delivery. Newly created messages
// are already reserved at enqueue time. The transaction keeps cross-day
// backlog and manual retries from bypassing the daily cap.
func ReserveMarketingEmailQuota(id int, dayStart int64, now int64, limit int64) (bool, error) {
	if id <= 0 || dayStart <= 0 || limit <= 0 {
		return false, ErrEmailDeliveryIdInvalid
	}
	reserved := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		delivery := &EmailDelivery{}
		if err := lockForUpdate(tx).First(delivery, id).Error; err != nil {
			return err
		}
		if delivery.Priority != EmailPriorityMarketing && !strings.HasPrefix(delivery.Category, "marketing_") {
			reserved = true
			return nil
		}
		if delivery.MarketingQuotaTime >= dayStart {
			reserved = true
			return nil
		}
		var used int64
		if err := tx.Model(&EmailDelivery{}).
			Where("category LIKE ? AND marketing_quota_time >= ?", "marketing_%", dayStart).
			Count(&used).Error; err != nil {
			return err
		}
		if used >= limit {
			return nil
		}
		result := tx.Model(delivery).
			Where("marketing_quota_time < ?", dayStart).
			Updates(map[string]any{"marketing_quota_time": now, "updated_time": now})
		if result.Error != nil {
			return result.Error
		}
		reserved = result.RowsAffected == 1
		return nil
	})
	return reserved, err
}

func CompleteEmailDelivery(id int) error {
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
			"recipient":         "",
			"subject":           "",
			"body":              "",
			"last_error":        "",
			"locked_until":      int64(0),
			"next_attempt_time": now,
			"delivered_time":    now,
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
		if delivery.Attempts+1 >= EmailDeliveryMaxAttempts {
			updates["dead_letter_time"] = common.GetTimestamp()
			updates["next_attempt_time"] = int64(0)
		}
		return tx.Model(delivery).Updates(updates).Error
	})
}

func ExpireEmailDeliveries(now int64) error {
	return DB.Model(&EmailDelivery{}).
		Where("delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0 AND expires_time > 0 AND expires_time <= ?", now).
		Updates(map[string]any{
			"recipient":    "",
			"subject":      "",
			"body":         "",
			"last_error":   "expired before delivery",
			"locked_until": int64(0),
			"expired_time": now,
			"updated_time": now,
		}).Error
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
			"recipient":         "",
			"subject":           "",
			"body":              "",
			"last_error":        reason,
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

// ListEmailDeliveries is intentionally Root-only at the controller layer:
// pending and failed rows may still contain recipient addresses and message
// content required for retrying delivery.
func ListEmailDeliveries(options EmailDeliveryQueryOptions, pageInfo *common.PageInfo) ([]*EmailDeliveryListItem, int64, error) {
	query := DB.Model(&EmailDelivery{})
	now := common.GetTimestamp()
	switch strings.TrimSpace(options.Status) {
	case EmailDeliveryStatusQueued:
		query = query.Where("delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0 AND attempts = 0 AND locked_until <= ?", now)
	case EmailDeliveryStatusSending:
		query = query.Where("delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0 AND locked_until > ?", now)
	case EmailDeliveryStatusRetrying:
		query = query.Where("delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0 AND attempts > 0 AND locked_until <= ?", now)
	case EmailDeliveryStatusDelivered:
		query = query.Where("delivered_time > 0")
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
			query = query.Where("(id = ? OR related_id = ? OR user_id = ? OR delivery_key LIKE ? ESCAPE '!' OR category LIKE ? ESCAPE '!' OR recipient LIKE ? ESCAPE '!' OR recipient_masked LIKE ? ESCAPE '!' OR subject LIKE ? ESCAPE '!' OR user_id IN (?))", id, id, id, pattern, pattern, pattern, pattern, pattern,
				DB.Model(&User{}).Select("id").Where("username LIKE ? ESCAPE '!' OR display_name LIKE ? ESCAPE '!'", pattern, pattern))
		} else {
			query = query.Where("(delivery_key LIKE ? ESCAPE '!' OR category LIKE ? ESCAPE '!' OR recipient LIKE ? ESCAPE '!' OR recipient_masked LIKE ? ESCAPE '!' OR subject LIKE ? ESCAPE '!' OR last_error LIKE ? ESCAPE '!' OR user_id IN (?))", pattern, pattern, pattern, pattern, pattern, pattern,
				DB.Model(&User{}).Select("id").Where("username LIKE ? ESCAPE '!' OR display_name LIKE ? ESCAPE '!'", pattern, pattern))
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []*EmailDeliveryListItem{}
	if err := query.Select("id, category, related_id, user_id, invoice_delivery_id, recipient, recipient_masked, priority, attempts, last_error, next_attempt_time, locked_until, expires_time, delivered_time, dead_letter_time, expired_time, created_time, updated_time").Order("id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	for _, row := range rows {
		row.Status = emailDeliveryStatus(row, now)
		if row.RecipientMasked == "" {
			row.RecipientMasked = maskEmailAddress(row.Recipient)
		}
		row.LastError = maskEmailAddressesInText(row.LastError)
		row.Recipient = ""
	}
	return rows, total, nil
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
			"next_attempt_time": now,
			"locked_until":      int64(0),
			"dead_letter_time":  int64(0),
			"updated_time":      now,
		})
	return result.RowsAffected, result.Error
}

func GetEmailDeliveryStats(now int64, dayStart int64) (EmailDeliveryStats, error) {
	stats := EmailDeliveryStats{}
	type countRow struct {
		Count int64
	}
	queries := []struct {
		destination *int64
		where       string
		args        []any
	}{
		{&stats.Queued, "delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0 AND attempts = 0 AND locked_until <= ?", []any{now}},
		{&stats.Sending, "delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0 AND locked_until > ?", []any{now}},
		{&stats.Retrying, "delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0 AND attempts > 0 AND locked_until <= ?", []any{now}},
		{&stats.Failed, "delivered_time = 0 AND dead_letter_time > 0", nil},
		{&stats.Delivered24h, "delivered_time >= ?", []any{now - 86400}},
		{&stats.Failed24h, "dead_letter_time >= ?", []any{now - 86400}},
		// Reserve the daily allowance when a marketing email enters the queue,
		// not only after SMTP accepts it. Otherwise a slow or broken SMTP server
		// could allow every minute-long scheduler run to exceed the daily cap.
		{&stats.MarketingSentToday, "category LIKE ? AND marketing_quota_time >= ?", []any{"marketing_%", dayStart}},
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
	_ = DB.Model(&EmailDelivery{}).Where("delivered_time = 0 AND dead_letter_time = 0 AND expired_time = 0").Select("MIN(created_time)").Scan(&stats.OldestPendingTime).Error
	_ = DB.Model(&EmailDelivery{}).Select("MAX(delivered_time)").Scan(&stats.LastDeliveredTime).Error
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
	result := DB.Where("id IN ?", ids).Delete(&EmailDelivery{})
	return result.RowsAffected, result.Error
}

func emailDeliveryStatus(row *EmailDeliveryListItem, now int64) string {
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

var emailAddressInTextPattern = regexp.MustCompile(`(?i)[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+`)

func maskEmailAddressesInText(value string) string {
	return emailAddressInTextPattern.ReplaceAllStringFunc(value, maskEmailAddress)
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

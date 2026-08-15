package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const EmailDeliveryMaxAttempts = 8

var ErrEmailDeliveryIdInvalid = errors.New("email delivery id is invalid")

type EmailDeliveryQueryOptions struct {
	Keyword    string
	FailedOnly bool
}

// EmailDeliveryListItem deliberately excludes subject and body. The Root UI
// only needs delivery metadata to diagnose SMTP failures, and message content
// should not be returned to the browser merely for queue maintenance.
type EmailDeliveryListItem struct {
	Id             int    `json:"id"`
	DeliveryKey    string `json:"delivery_key"`
	Category       string `json:"category"`
	RelatedId      int    `json:"related_id"`
	UserId         int    `json:"user_id"`
	Recipient      string `json:"recipient"`
	Attempts       int    `json:"attempts"`
	LastError      string `json:"last_error"`
	DeadLetterTime int64  `json:"dead_letter_time"`
	CreatedTime    int64  `json:"created_time"`
}

// EmailDelivery is the shared durable SMTP outbox for NewAPI system emails.
// Business modules provide a stable delivery key so retries and repeated API
// requests cannot send the same event twice.
type EmailDelivery struct {
	Id              int    `json:"id"`
	DeliveryKey     string `json:"delivery_key" gorm:"type:varchar(160);uniqueIndex;not null"`
	Category        string `json:"category" gorm:"type:varchar(48);not null;index"`
	RelatedId       int    `json:"related_id" gorm:"index"`
	UserId          int    `json:"user_id" gorm:"index"`
	Recipient       string `json:"recipient" gorm:"type:varchar(320);not null"`
	Subject         string `json:"subject" gorm:"type:varchar(512);not null"`
	Body            string `json:"body" gorm:"type:text;not null"`
	Attempts        int    `json:"attempts" gorm:"not null;default:0"`
	LastError       string `json:"last_error" gorm:"type:text"`
	NextAttemptTime int64  `json:"next_attempt_time" gorm:"bigint;not null;index"`
	LockedUntil     int64  `json:"locked_until" gorm:"bigint;not null;default:0;index"`
	ExpiresTime     int64  `json:"expires_time" gorm:"bigint;not null;default:0;index"`
	DeliveredTime   int64  `json:"delivered_time" gorm:"bigint;not null;default:0;index"`
	DeadLetterTime  int64  `json:"dead_letter_time" gorm:"bigint;not null;default:0;index"`
	CreatedTime     int64  `json:"created_time" gorm:"bigint;autoCreateTime;index"`
	UpdatedTime     int64  `json:"updated_time" gorm:"bigint;autoUpdateTime"`
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
	delivery.CreatedTime = now
	delivery.UpdatedTime = now
	delivery.NextAttemptTime = now
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
	err := DB.Where("delivered_time = 0 AND dead_letter_time = 0 AND next_attempt_time <= ? AND locked_until <= ? AND (expires_time = 0 OR expires_time > ?)", now, now, now).
		Order("id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func ClaimEmailDelivery(id int, now int64, lockedUntil int64) (bool, error) {
	result := DB.Model(&EmailDelivery{}).
		Where("id = ? AND delivered_time = 0 AND dead_letter_time = 0 AND next_attempt_time <= ? AND locked_until <= ? AND (expires_time = 0 OR expires_time > ?)", id, now, now, now).
		Updates(map[string]any{"locked_until": lockedUntil, "updated_time": now})
	return result.RowsAffected == 1, result.Error
}

func CompleteEmailDelivery(id int) error {
	now := common.GetTimestamp()
	return DB.Model(&EmailDelivery{}).Where("id = ?", id).Updates(map[string]any{
		"recipient":         "",
		"subject":           "",
		"body":              "",
		"last_error":        "",
		"locked_until":      int64(0),
		"next_attempt_time": now,
		"delivered_time":    now,
		"dead_letter_time":  int64(0),
		"updated_time":      now,
	}).Error
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
		Where("delivered_time = 0 AND dead_letter_time = 0 AND expires_time > 0 AND expires_time <= ?", now).
		Updates(map[string]any{
			"recipient":        "",
			"subject":          "",
			"body":             "",
			"last_error":       "expired before delivery",
			"locked_until":     int64(0),
			"dead_letter_time": now,
			"updated_time":     now,
		}).Error
}

func RetryEmailDelivery(id int) error {
	result := DB.Model(&EmailDelivery{}).
		Where("id = ? AND delivered_time = 0 AND (expires_time = 0 OR expires_time > ?)", id, common.GetTimestamp()).
		Updates(map[string]any{
			"attempts":          0,
			"last_error":        "",
			"next_attempt_time": common.GetTimestamp(),
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
	if options.FailedOnly {
		now := common.GetTimestamp()
		query = query.Where("delivered_time = 0 AND dead_letter_time > 0 AND (expires_time = 0 OR expires_time > ?)", now)
	}
	if keyword := strings.TrimSpace(options.Keyword); keyword != "" {
		pattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		if id, err := strconv.Atoi(keyword); err == nil && id > 0 {
			query = query.Where("(id = ? OR related_id = ? OR user_id = ? OR delivery_key LIKE ? ESCAPE '!' OR category LIKE ? ESCAPE '!' OR recipient LIKE ? ESCAPE '!' OR subject LIKE ? ESCAPE '!')", id, id, id, pattern, pattern, pattern, pattern)
		} else {
			query = query.Where("(delivery_key LIKE ? ESCAPE '!' OR category LIKE ? ESCAPE '!' OR recipient LIKE ? ESCAPE '!' OR subject LIKE ? ESCAPE '!' OR last_error LIKE ? ESCAPE '!')", pattern, pattern, pattern, pattern, pattern)
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []*EmailDeliveryListItem{}
	if err := query.Select("id, delivery_key, category, related_id, user_id, recipient, attempts, last_error, dead_letter_time, created_time").Order("dead_letter_time DESC, id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

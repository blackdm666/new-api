package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	InvoiceNotificationKindAdminEmail = "admin_email"
	InvoiceNotificationKindUserEmail  = "user_email"
	InvoiceNotificationKindUserLegacy = "user"
	InvoiceNotificationKindUser       = InvoiceNotificationKindUserEmail
)

type InvoiceNotificationDelivery struct {
	Id               int    `json:"id"`
	DeliveryKey      string `json:"delivery_key" gorm:"type:varchar(128);uniqueIndex;not null"`
	InvoiceRequestId int    `json:"invoice_request_id" gorm:"index;not null"`
	Kind             string `json:"kind" gorm:"type:varchar(32);not null"`
	UserId           int    `json:"user_id" gorm:"index"`
	Recipient        string `json:"recipient" gorm:"type:varchar(320)"`
	Subject          string `json:"subject" gorm:"type:varchar(512);not null"`
	Body             string `json:"body" gorm:"type:text;not null"`
	Attempts         int    `json:"attempts" gorm:"default:0"`
	LastError        string `json:"last_error" gorm:"type:text"`
	NextAttemptTime  int64  `json:"next_attempt_time" gorm:"bigint;index"`
	LockedUntil      int64  `json:"locked_until" gorm:"bigint;index"`
	DeliveredTime    int64  `json:"delivered_time" gorm:"bigint;index"`
	CreatedTime      int64  `json:"created_time" gorm:"bigint;index"`
	UpdatedTime      int64  `json:"updated_time" gorm:"bigint"`
}

func EnqueueInvoiceNotification(delivery *InvoiceNotificationDelivery) (*InvoiceNotificationDelivery, bool, error) {
	return enqueueInvoiceNotification(DB, delivery)
}

func EnqueueInvoiceNotificationTx(tx *gorm.DB, delivery *InvoiceNotificationDelivery) (*InvoiceNotificationDelivery, bool, error) {
	if tx == nil {
		return nil, false, gorm.ErrInvalidDB
	}
	return enqueueInvoiceNotification(tx, delivery)
}

func enqueueInvoiceNotification(tx *gorm.DB, delivery *InvoiceNotificationDelivery) (*InvoiceNotificationDelivery, bool, error) {
	if delivery == nil {
		return nil, false, gorm.ErrInvalidData
	}
	delivery.DeliveryKey = strings.TrimSpace(delivery.DeliveryKey)
	delivery.Recipient = strings.TrimSpace(delivery.Recipient)
	if delivery.DeliveryKey == "" || delivery.InvoiceRequestId <= 0 || delivery.Subject == "" || delivery.Body == "" {
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
	var existing InvoiceNotificationDelivery
	if err := tx.Where("delivery_key = ?", delivery.DeliveryKey).First(&existing).Error; err != nil {
		return nil, false, err
	}
	return &existing, false, nil
}

func ListDueInvoiceNotifications(limit int, now int64) ([]*InvoiceNotificationDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var deliveries []*InvoiceNotificationDelivery
	err := DB.Where("delivered_time = 0 AND next_attempt_time <= ? AND locked_until <= ?", now, now).
		Order("id ASC").Limit(limit).Find(&deliveries).Error
	return deliveries, err
}

func ClaimInvoiceNotification(id int, now int64, lockedUntil int64) (bool, error) {
	result := DB.Model(&InvoiceNotificationDelivery{}).
		Where("id = ? AND delivered_time = 0 AND next_attempt_time <= ? AND locked_until <= ?", id, now, now).
		Updates(map[string]interface{}{
			"locked_until": lockedUntil,
			"updated_time": now,
		})
	return result.RowsAffected == 1, result.Error
}

func CompleteInvoiceNotification(id int) error {
	now := common.GetTimestamp()
	return DB.Model(&InvoiceNotificationDelivery{}).Where("id = ?", id).Updates(map[string]interface{}{
		"recipient":         "",
		"subject":           "",
		"body":              "",
		"last_error":        "",
		"locked_until":      int64(0),
		"delivered_time":    now,
		"updated_time":      now,
		"next_attempt_time": now,
	}).Error
}

func RecordInvoiceNotificationFailure(id int, message string, nextAttemptTime int64) error {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	return DB.Model(&InvoiceNotificationDelivery{}).Where("id = ?", id).Updates(map[string]interface{}{
		"attempts":          gorm.Expr("attempts + ?", 1),
		"last_error":        message,
		"next_attempt_time": nextAttemptTime,
		"locked_until":      int64(0),
		"updated_time":      common.GetTimestamp(),
	}).Error
}

func ListPendingInvoiceNotifications(limit int) ([]*InvoiceNotificationDelivery, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var deliveries []*InvoiceNotificationDelivery
	err := DB.Where("delivered_time = 0").Order("id DESC").Limit(limit).Find(&deliveries).Error
	return deliveries, err
}

func ListInvoiceNotificationsForRequest(requestId int) ([]*InvoiceNotificationDelivery, error) {
	if requestId <= 0 {
		return nil, gorm.ErrInvalidData
	}
	var deliveries []*InvoiceNotificationDelivery
	err := DB.Where("invoice_request_id = ?", requestId).
		Order("created_time DESC, id DESC").Find(&deliveries).Error
	return deliveries, err
}

func RetryInvoiceNotification(id int) (*InvoiceNotificationDelivery, error) {
	var delivery InvoiceNotificationDelivery
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&delivery, "id = ?", id).Error; err != nil {
			return err
		}
		if delivery.DeliveredTime != 0 {
			return errors.New("invoice notification was already delivered")
		}
		now := common.GetTimestamp()
		if err := tx.Model(&delivery).Updates(map[string]interface{}{
			"next_attempt_time": now,
			"locked_until":      int64(0),
			"updated_time":      now,
		}).Error; err != nil {
			return err
		}
		delivery.NextAttemptTime = now
		delivery.LockedUntil = 0
		delivery.UpdatedTime = now
		return nil
	})
	return &delivery, err
}

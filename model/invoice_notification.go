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
	EmailDeliveryId  int    `json:"email_delivery_id" gorm:"index"`
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
		if err := ensureInvoiceEmailDeliveryTx(tx, delivery); err != nil {
			return nil, false, err
		}
		return delivery, true, nil
	}
	var existing InvoiceNotificationDelivery
	if err := tx.Where("delivery_key = ?", delivery.DeliveryKey).First(&existing).Error; err != nil {
		return nil, false, err
	}
	if err := ensureInvoiceEmailDeliveryTx(tx, &existing); err != nil {
		return nil, false, err
	}
	return &existing, false, nil
}

func ensureInvoiceEmailDeliveryTx(tx *gorm.DB, delivery *InvoiceNotificationDelivery) error {
	if delivery == nil || delivery.Id <= 0 {
		return gorm.ErrInvalidData
	}
	if delivery.EmailDeliveryId > 0 {
		var count int64
		if err := tx.Model(&EmailDelivery{}).Where("id = ?", delivery.EmailDeliveryId).Count(&count).Error; err != nil {
			return err
		}
		if count == 1 {
			return nil
		}
		delivery.EmailDeliveryId = 0
	}
	category := "invoice_admin_email"
	if delivery.Kind == InvoiceNotificationKindUserEmail || delivery.Kind == InvoiceNotificationKindUserLegacy {
		category = "invoice_user_email"
	}
	recipient := strings.TrimSpace(delivery.Recipient)
	if recipient == "" && delivery.UserId > 0 {
		user := &User{}
		if err := tx.Select("email").First(user, delivery.UserId).Error; err != nil {
			return err
		}
		recipient = strings.TrimSpace(user.Email)
	}
	queued, _, err := enqueueEmailDelivery(tx, &EmailDelivery{
		DeliveryKey:       "invoice:" + delivery.DeliveryKey,
		Category:          category,
		RelatedId:         delivery.InvoiceRequestId,
		UserId:            delivery.UserId,
		InvoiceDeliveryId: delivery.Id,
		Recipient:         recipient,
		Subject:           delivery.Subject,
		Body:              delivery.Body,
		Priority:          EmailPriorityBusiness,
	})
	if err != nil {
		return err
	}
	if err := tx.Model(delivery).Update("email_delivery_id", queued.Id).Error; err != nil {
		return err
	}
	delivery.EmailDeliveryId = queued.Id
	return nil
}

func EnsureInvoiceEmailDelivery(delivery *InvoiceNotificationDelivery) (*EmailDelivery, error) {
	if delivery == nil || delivery.Id <= 0 {
		return nil, gorm.ErrInvalidData
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		current := &InvoiceNotificationDelivery{}
		if err := lockForUpdate(tx).First(current, delivery.Id).Error; err != nil {
			return err
		}
		if err := ensureInvoiceEmailDeliveryTx(tx, current); err != nil {
			return err
		}
		*delivery = *current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return GetEmailDeliveryById(delivery.EmailDeliveryId)
}

func GetInvoiceNotificationDelivery(id int) (*InvoiceNotificationDelivery, error) {
	delivery := &InvoiceNotificationDelivery{}
	err := DB.First(delivery, id).Error
	return delivery, err
}

func SyncInvoiceNotificationFromEmailDelivery(delivery *EmailDelivery) error {
	if delivery == nil || delivery.InvoiceDeliveryId <= 0 {
		return nil
	}
	return DB.Model(&InvoiceNotificationDelivery{}).Where("id = ?", delivery.InvoiceDeliveryId).Updates(map[string]any{
		"attempts":          delivery.Attempts,
		"last_error":        delivery.LastError,
		"next_attempt_time": delivery.NextAttemptTime,
		"locked_until":      delivery.LockedUntil,
		"delivered_time":    delivery.DeliveredTime,
		"updated_time":      delivery.UpdatedTime,
	}).Error
}

func deleteInvoiceEmailDeliveriesTx(tx *gorm.DB, requestId int) error {
	if tx == nil || requestId <= 0 {
		return gorm.ErrInvalidData
	}
	return tx.Where("invoice_delivery_id IN (?)",
		tx.Model(&InvoiceNotificationDelivery{}).Select("id").Where("invoice_request_id = ?", requestId),
	).Delete(&EmailDelivery{}).Error
}

func expireInvoiceEmailDeliveriesTx(tx *gorm.DB, requestId int, now int64) error {
	if tx == nil || requestId <= 0 {
		return gorm.ErrInvalidData
	}
	return tx.Model(&EmailDelivery{}).
		Where("invoice_delivery_id IN (?) AND delivered_time = 0 AND expired_time = 0",
			tx.Model(&InvoiceNotificationDelivery{}).Select("id").Where("invoice_request_id = ?", requestId),
		).
		Updates(map[string]any{
			"recipient":         "",
			"subject":           "",
			"body":              "",
			"last_error":        "invoice request data retention expired",
			"locked_until":      int64(0),
			"next_attempt_time": int64(0),
			"dead_letter_time":  int64(0),
			"expired_time":      now,
			"updated_time":      now,
		}).Error
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
	if err := DB.First(&delivery, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if delivery.DeliveredTime != 0 {
		return nil, errors.New("invoice notification was already delivered")
	}
	queued, err := EnsureInvoiceEmailDelivery(&delivery)
	if err != nil {
		return nil, err
	}
	if err := RetryEmailDelivery(queued.Id); err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	if err := DB.Model(&delivery).Updates(map[string]any{
		"attempts":          0,
		"last_error":        "",
		"next_attempt_time": now,
		"locked_until":      int64(0),
		"updated_time":      now,
	}).Error; err != nil {
		return nil, err
	}
	delivery.Attempts = 0
	delivery.LastError = ""
	delivery.NextAttemptTime = now
	delivery.LockedUntil = 0
	delivery.UpdatedTime = now
	return &delivery, nil
}

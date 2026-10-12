package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	InvoiceNotificationKindAdminEmail = "admin_email"
	// InvoiceNotificationKindUserEmail delivers the issued invoice with its
	// files attached.
	InvoiceNotificationKindUserEmail  = "user_email"
	InvoiceNotificationKindUserLegacy = "user"
	InvoiceNotificationKindUser       = InvoiceNotificationKindUserEmail
	// InvoiceNotificationKindUserNotice is a plain status email to the
	// applicant (rejected or expired) that carries no attachments.
	InvoiceNotificationKindUserNotice = "user_notice"
	// InvoiceNotificationKindWechatGroup is a text message to the finance
	// WeChat group. It never enters the email outbox; Recipient holds the
	// group ID and the invoice delivery worker sends it directly.
	InvoiceNotificationKindWechatGroup = "wechat_group"
)

const (
	InvoiceNotificationChannelEmail  = "email"
	InvoiceNotificationChannelWechat = "wechat"
)

// InvoiceWechatOutcomeUnknownError is recorded on a WeChat delivery whose send
// was interrupted after it was claimed.
const InvoiceWechatOutcomeUnknownError = "delivery outcome unknown, retry manually"

// ErrInvoiceNotificationSending is returned when an administrator retries a
// WeChat delivery that a sender is still posting.
var ErrInvoiceNotificationSending = errors.New("invoice notification is being sent")

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
	// FailedTime marks a WeChat delivery that will not be retried
	// automatically; email failures are tracked by the shared email outbox.
	FailedTime int64 `json:"failed_time" gorm:"bigint;default:0"`
	// SendStartedTime is written when a sender claims a WeChat delivery and
	// is cleared only by a manual retry. A claimed message is never claimed
	// again automatically, because the provider may already have posted it.
	SendStartedTime int64  `json:"send_started_time" gorm:"bigint;default:0"`
	CreatedTime     int64  `json:"created_time" gorm:"bigint;index"`
	UpdatedTime     int64  `json:"updated_time" gorm:"bigint"`
	Channel         string `json:"channel" gorm:"-"`
}

func invoiceNotificationChannel(kind string) string {
	if kind == InvoiceNotificationKindWechatGroup {
		return InvoiceNotificationChannelWechat
	}
	return InvoiceNotificationChannelEmail
}

func (delivery *InvoiceNotificationDelivery) AfterFind(tx *gorm.DB) error {
	delivery.Channel = invoiceNotificationChannel(delivery.Kind)
	return nil
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
	delivery.Channel = invoiceNotificationChannel(delivery.Kind)
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(delivery)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		if delivery.Kind == InvoiceNotificationKindWechatGroup {
			return delivery, true, nil
		}
		if err := ensureInvoiceEmailDeliveryTx(tx, delivery); err != nil {
			return nil, false, err
		}
		return delivery, true, nil
	}
	var existing InvoiceNotificationDelivery
	if err := tx.Where("delivery_key = ?", delivery.DeliveryKey).First(&existing).Error; err != nil {
		return nil, false, err
	}
	if existing.Kind == InvoiceNotificationKindWechatGroup {
		return &existing, false, nil
	}
	if err := ensureInvoiceEmailDeliveryTx(tx, &existing); err != nil {
		return nil, false, err
	}
	return &existing, false, nil
}

func ensureInvoiceEmailDeliveryTx(tx *gorm.DB, delivery *InvoiceNotificationDelivery) error {
	if delivery == nil || delivery.Id <= 0 || delivery.Kind == InvoiceNotificationKindWechatGroup {
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
	if delivery.Kind == InvoiceNotificationKindUserEmail || delivery.Kind == InvoiceNotificationKindUserLegacy || delivery.Kind == InvoiceNotificationKindUserNotice {
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
	if delivery.Kind == InvoiceNotificationKindWechatGroup {
		return retryInvoiceWechatNotification(&delivery)
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

// retryInvoiceWechatNotification re-arms a WeChat delivery an administrator
// chose to resend. Automatic retries never reach this path. A message whose
// send is still in progress cannot be re-armed, or it could be posted twice.
func retryInvoiceWechatNotification(delivery *InvoiceNotificationDelivery) (*InvoiceNotificationDelivery, error) {
	if strings.TrimSpace(delivery.Body) == "" {
		return nil, errors.New("invoice notification content is no longer available")
	}
	now := common.GetTimestamp()
	queued := delivery.SendStartedTime == 0 && delivery.FailedTime == 0
	result := DB.Model(&InvoiceNotificationDelivery{}).
		Where("id = ? AND delivered_time = 0 AND NOT (send_started_time > 0 AND failed_time = 0 AND locked_until > ?)", delivery.Id, now).
		Updates(map[string]any{
			"attempts":          0,
			"last_error":        "",
			"failed_time":       int64(0),
			"send_started_time": int64(0),
			"next_attempt_time": now,
			"locked_until":      int64(0),
			"updated_time":      now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	// MySQL reports an unchanged row as unaffected, so a message that was
	// still queued needs no re-arming; anything else was claimed meanwhile.
	if result.RowsAffected != 1 && !queued {
		return nil, ErrInvoiceNotificationSending
	}
	delivery.Attempts = 0
	delivery.LastError = ""
	delivery.FailedTime = 0
	delivery.SendStartedTime = 0
	delivery.NextAttemptTime = now
	delivery.LockedUntil = 0
	delivery.UpdatedTime = now
	return delivery, nil
}

// ListPendingInvoiceWechatNotifications returns queued WeChat deliveries that
// no sender has claimed yet.
func ListPendingInvoiceWechatNotifications(limit int, now int64) ([]*InvoiceNotificationDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var deliveries []*InvoiceNotificationDelivery
	err := DB.Where("kind = ? AND delivered_time = 0 AND failed_time = 0 AND send_started_time = 0 AND next_attempt_time <= ? AND locked_until <= ?",
		InvoiceNotificationKindWechatGroup, now, now).
		Order("id ASC").Limit(limit).Find(&deliveries).Error
	return deliveries, err
}

// ClaimInvoiceWechatNotification reserves a queued WeChat delivery for one
// sender until lockedUntil and marks it in flight with SendStartedTime = now,
// so neither a concurrent worker nor a later run posts it again. The caller
// passes the same now to RecordInvoiceWechatNotificationResult.
func ClaimInvoiceWechatNotification(id int, now int64, lockedUntil int64) (bool, error) {
	result := DB.Model(&InvoiceNotificationDelivery{}).
		Where("id = ? AND kind = ? AND delivered_time = 0 AND failed_time = 0 AND send_started_time = 0 AND locked_until <= ?",
			id, InvoiceNotificationKindWechatGroup, now).
		Updates(map[string]any{"locked_until": lockedUntil, "send_started_time": now, "updated_time": now})
	return result.RowsAffected == 1, result.Error
}

// FailInterruptedInvoiceWechatNotifications fails WeChat deliveries whose
// sender claimed them but never recorded a result before the claim expired,
// for example because the process stopped mid-send. The provider may already
// have posted them, so they wait for an administrator instead of being sent
// again.
func FailInterruptedInvoiceWechatNotifications(now int64) (int64, error) {
	result := DB.Model(&InvoiceNotificationDelivery{}).
		Where("kind = ? AND delivered_time = 0 AND failed_time = 0 AND send_started_time > 0 AND locked_until <= ?",
			InvoiceNotificationKindWechatGroup, now).
		Updates(map[string]any{
			"failed_time":  now,
			"last_error":   InvoiceWechatOutcomeUnknownError,
			"locked_until": int64(0),
			"updated_time": now,
		})
	return result.RowsAffected, result.Error
}

// RecordInvoiceWechatNotificationResult stores the final outcome of the send
// claimed at sendStartedTime. An empty failure marks the delivery as delivered
// and drops the message text, which contains the applicant's billing details;
// the group ID is kept. A result for a claim that was meanwhile failed as
// interrupted still replaces that unknown outcome, but one for a claim an
// administrator already re-armed is rejected.
func RecordInvoiceWechatNotificationResult(id int, sendStartedTime int64, attempts int, failure string) error {
	now := common.GetTimestamp()
	updates := map[string]any{
		"attempts":     attempts,
		"locked_until": int64(0),
		"updated_time": now,
	}
	if failure == "" {
		updates["delivered_time"] = now
		updates["failed_time"] = int64(0)
		updates["last_error"] = ""
		updates["subject"] = ""
		updates["body"] = ""
	} else {
		updates["failed_time"] = now
		updates["last_error"] = failure
	}
	result := DB.Model(&InvoiceNotificationDelivery{}).
		Where("id = ? AND send_started_time = ?", id, sendStartedTime).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("invoice wechat notification %d is no longer claimed by this send", id)
	}
	return nil
}

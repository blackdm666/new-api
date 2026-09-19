package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/bytedance/gopkg/util/gopool"
)

const (
	emailDeliveryInterval = 30 * time.Second
	emailDeliveryLease    = 10 * time.Minute
)

func smtpProfileForCategory(category string) string {
	switch strings.TrimSpace(category) {
	case "email_verification", "password_reset":
		return common.SMTPProfileSecurity
	case "marketing_custom", "email_preview":
		return common.SMTPProfileMarketing
	default:
		return common.SMTPProfileNotification
	}
}

func StartEmailDelivery() {
	if !common.IsMasterNode {
		return
	}
	gopool.Go(func() {
		deliverDueSystemEmails()
		ticker := time.NewTicker(emailDeliveryInterval)
		defer ticker.Stop()
		for range ticker.C {
			deliverDueSystemEmails()
		}
	})
}

func QueueSystemEmail(deliveryKey string, category string, relatedId int, userId int, recipient string, subject string, body string, expiresTime int64) (*model.EmailDelivery, error) {
	priority := model.EmailPriorityBusiness
	switch category {
	case "email_verification", "password_reset", "system_alert", "system_alert_user", "quota_warning_user", "channel_status_admin", "inspection_alert_admin":
		priority = model.EmailPriorityCritical
	}
	delivery, created, err := model.EnqueueEmailDelivery(&model.EmailDelivery{
		DeliveryKey: deliveryKey,
		Category:    category,
		SMTPProfile: smtpProfileForCategory(category),
		RelatedId:   relatedId,
		UserId:      userId,
		Recipient:   strings.TrimSpace(recipient),
		Subject:     subject,
		Body:        body,
		Priority:    priority,
		ExpiresTime: expiresTime,
	})
	if err != nil {
		return nil, err
	}
	if created || (delivery.DeliveredTime == 0 && delivery.DeadLetterTime == 0) {
		ScheduleSystemEmail(delivery)
	}
	return delivery, nil
}

// QueueMarketingEmail persists a low-priority marketing message without
// immediately bypassing campaign rate limits. The regular outbox poller will
// deliver it after all due system and business messages.
func QueueMarketingEmail(deliveryKey string, category string, relatedId int, userId int, recipient string, subject string, body string, expiresTime int64) (*model.EmailDelivery, error) {
	now := time.Now()
	rules := setting.GetEmailDeliveryRules()
	delivery, _, err := model.EnqueueMarketingEmailDelivery(&model.EmailDelivery{
		DeliveryKey: deliveryKey,
		Category:    category,
		SMTPProfile: common.SMTPProfileMarketing,
		RelatedId:   relatedId,
		UserId:      userId,
		Recipient:   recipient,
		Subject:     subject,
		Body:        body,
		Priority:    model.EmailPriorityMarketing,
		ExpiresTime: expiresTime,
	}, marketingDayStart(now), now.Unix(), int64(rules.MarketingDailyLimit))
	return delivery, err
}

func ScheduleSystemEmail(delivery *model.EmailDelivery) {
	if delivery == nil || delivery.Priority == model.EmailPriorityMarketing || delivery.DeliveredTime != 0 || delivery.DeadLetterTime != 0 {
		return
	}
	gopool.Go(func() { deliverSystemEmail(delivery) })
}

func deliverDueSystemEmails() {
	now := common.GetTimestamp()
	if err := model.ExpireEmailDeliveries(now); err != nil {
		common.SysError("failed to expire email deliveries: " + err.Error())
	}
	if err := model.ExpireAwaitingEmailReceipts(now); err != nil {
		common.SysError("failed to expire EventBridge email receipts: " + err.Error())
	}
	deliveries, err := model.ListDueEmailDeliveries(100, now)
	if err != nil {
		common.SysError("failed to list pending email deliveries: " + err.Error())
		return
	}
	for _, delivery := range deliveries {
		deliverSystemEmail(delivery)
	}
}

func deliverSystemEmail(delivery *model.EmailDelivery) {
	if strings.TrimSpace(delivery.SMTPProfile) == "" {
		delivery.SMTPProfile = smtpProfileForCategory(delivery.Category)
	}
	now := common.GetTimestamp()
	claimed, err := model.ClaimEmailDelivery(delivery.Id, now, time.Now().Add(emailDeliveryLease).Unix())
	if err != nil || !claimed {
		if err != nil {
			common.SysError(fmt.Sprintf("failed to claim email delivery %d: %s", delivery.Id, err.Error()))
		}
		return
	}
	allowed, err := marketingEmailDeliveryAllowed(delivery)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to validate email delivery %d: %s", delivery.Id, err.Error()))
		_ = model.DeferEmailDelivery(delivery.Id, time.Now().Add(time.Minute).Unix())
		return
	}
	if !allowed {
		return
	}
	if delivery.Priority == model.EmailPriorityMarketing || strings.HasPrefix(delivery.Category, "marketing_") {
		err = SendMarketingEmailDelivery(delivery)
		if err == nil {
			return
		}
		if errors.Is(err, ErrNoUsableMarketingEmailAccount) || errors.Is(err, ErrEmailReceiptEndpointDisabled) {
			_ = model.DeferEmailDelivery(delivery.Id, time.Now().Add(5*time.Minute).Unix())
			return
		}
	}
	result := common.SMTPDeliveryResult{Profile: delivery.SMTPProfile}
	if delivery.InvoiceDeliveryId > 0 {
		err = sendInvoiceEmailDelivery(delivery)
	} else if delivery.Priority != model.EmailPriorityMarketing && !strings.HasPrefix(delivery.Category, "marketing_") {
		result, err = common.SendEmailWithProfileResult(delivery.SMTPProfile, delivery.Subject, delivery.Recipient, delivery.Body)
	}
	if err == nil {
		if completeErr := model.CompleteEmailDelivery(delivery.Id, result.Profile, result.Channel, result.MessageID); completeErr != nil {
			common.SysError(fmt.Sprintf("failed to complete email delivery %d: %s", delivery.Id, completeErr.Error()))
			return
		}
		return
	}
	rules := setting.GetEmailDeliveryRules()
	delay := time.Duration(rules.EmailRetryInitialSeconds) * time.Second
	maxDelay := time.Duration(rules.EmailRetryMaxSeconds) * time.Second
	for attempt := 0; attempt < delivery.Attempts && delay < maxDelay; attempt++ {
		delay *= 2
	}
	if delay > maxDelay {
		delay = maxDelay
	}
	if recordErr := model.RecordEmailDeliveryFailure(delivery.Id, err.Error(), time.Now().Add(delay).Unix()); recordErr != nil {
		common.SysError(fmt.Sprintf("failed to record email delivery %d failure: %s", delivery.Id, recordErr.Error()))
		return
	}
	if delivery.InvoiceDeliveryId > 0 {
		updated, getErr := model.GetEmailDeliveryById(delivery.Id)
		if getErr == nil {
			_ = model.SyncInvoiceNotificationFromEmailDelivery(updated)
		}
	}
}

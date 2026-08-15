package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/bytedance/gopkg/util/gopool"
)

const (
	emailDeliveryInterval = 30 * time.Second
	emailDeliveryLease    = 10 * time.Minute
)

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
	delivery, created, err := model.EnqueueEmailDelivery(&model.EmailDelivery{
		DeliveryKey: deliveryKey,
		Category:    category,
		RelatedId:   relatedId,
		UserId:      userId,
		Recipient:   strings.TrimSpace(recipient),
		Subject:     subject,
		Body:        body,
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

func ScheduleSystemEmail(delivery *model.EmailDelivery) {
	if delivery == nil || delivery.DeliveredTime != 0 || delivery.DeadLetterTime != 0 {
		return
	}
	gopool.Go(func() { deliverSystemEmail(delivery) })
}

func deliverDueSystemEmails() {
	now := common.GetTimestamp()
	if err := model.ExpireEmailDeliveries(now); err != nil {
		common.SysError("failed to expire email deliveries: " + err.Error())
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
	now := common.GetTimestamp()
	claimed, err := model.ClaimEmailDelivery(delivery.Id, now, time.Now().Add(emailDeliveryLease).Unix())
	if err != nil || !claimed {
		if err != nil {
			common.SysError(fmt.Sprintf("failed to claim email delivery %d: %s", delivery.Id, err.Error()))
		}
		return
	}
	if err = common.SendEmail(delivery.Subject, delivery.Recipient, delivery.Body); err == nil {
		if completeErr := model.CompleteEmailDelivery(delivery.Id); completeErr != nil {
			common.SysError(fmt.Sprintf("failed to complete email delivery %d: %s", delivery.Id, completeErr.Error()))
		}
		return
	}
	delay := emailDeliveryInterval
	for attempt := 0; attempt < delivery.Attempts && delay < 24*time.Hour; attempt++ {
		delay *= 2
	}
	if delay > 24*time.Hour {
		delay = 24 * time.Hour
	}
	if recordErr := model.RecordEmailDeliveryFailure(delivery.Id, err.Error(), time.Now().Add(delay).Unix()); recordErr != nil {
		common.SysError(fmt.Sprintf("failed to record email delivery %d failure: %s", delivery.Id, recordErr.Error()))
	}
}

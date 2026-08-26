package service

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicefile"
	"github.com/bytedance/gopkg/util/gopool"
)

const (
	invoiceNotificationInterval = time.Minute
	invoiceEmailAttachmentLimit = 25 * 1024 * 1024
)

func StartInvoiceNotificationDelivery() {
	if !common.IsMasterNode {
		return
	}
	gopool.Go(func() {
		bridgePendingInvoiceNotifications()
		ticker := time.NewTicker(invoiceNotificationInterval)
		defer ticker.Stop()
		for range ticker.C {
			bridgePendingInvoiceNotifications()
		}
	})
}

func ScheduleInvoiceNotification(delivery *model.InvoiceNotificationDelivery) {
	if delivery == nil || delivery.DeliveredTime != 0 {
		return
	}
	queued, err := model.EnsureInvoiceEmailDelivery(delivery)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to queue invoice notification %d: %s", delivery.Id, err.Error()))
		return
	}
	ScheduleSystemEmail(queued)
}

// bridgePendingInvoiceNotifications migrates legacy invoice notifications
// into the shared durable email outbox. New records are linked atomically when
// they are created, so this path only remains for upgrades from older builds.
func bridgePendingInvoiceNotifications() {
	deliveries, err := model.ListPendingInvoiceNotifications(100)
	if err != nil {
		common.SysError("failed to list pending invoice notifications: " + err.Error())
		return
	}
	for _, delivery := range deliveries {
		if delivery.EmailDeliveryId != 0 {
			queued, getErr := model.GetEmailDeliveryById(delivery.EmailDeliveryId)
			if getErr == nil {
				if queued.DeliveredTime > 0 {
					_ = model.CompleteInvoiceNotification(delivery.Id)
				} else {
					_ = model.SyncInvoiceNotificationFromEmailDelivery(queued)
				}
				continue
			}
			delivery.EmailDeliveryId = 0
			if _, ensureErr := model.EnsureInvoiceEmailDelivery(delivery); ensureErr != nil {
				common.SysError(fmt.Sprintf("failed to restore invoice notification %d: %s", delivery.Id, ensureErr.Error()))
			}
			continue
		}
		ScheduleInvoiceNotification(delivery)
	}
}

func sendInvoiceEmailDelivery(emailDelivery *model.EmailDelivery) error {
	if emailDelivery == nil || emailDelivery.InvoiceDeliveryId <= 0 {
		return fmt.Errorf("invoice email delivery is invalid")
	}
	delivery, err := model.GetInvoiceNotificationDelivery(emailDelivery.InvoiceDeliveryId)
	if err != nil {
		return err
	}
	delivery.Recipient = emailDelivery.Recipient
	delivery.Subject = emailDelivery.Subject
	delivery.Body = emailDelivery.Body
	switch delivery.Kind {
	case model.InvoiceNotificationKindAdminEmail:
		return common.SendEmail(delivery.Subject, delivery.Recipient, delivery.Body)
	case model.InvoiceNotificationKindUserEmail, model.InvoiceNotificationKindUserLegacy:
		return sendIssuedInvoiceEmail(delivery)
	default:
		return fmt.Errorf("unsupported invoice notification kind: %s", delivery.Kind)
	}
}

func sendIssuedInvoiceEmail(delivery *model.InvoiceNotificationDelivery) error {
	recipient := strings.TrimSpace(delivery.Recipient)
	if recipient == "" {
		user, err := model.GetUserById(delivery.UserId, false)
		if err != nil {
			return err
		}
		recipient = strings.TrimSpace(user.Email)
	}
	if recipient == "" {
		return fmt.Errorf("invoice user %d has no registered email", delivery.UserId)
	}
	files, err := model.ListInvoiceFiles(delivery.InvoiceRequestId)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return model.ErrInvoiceFileRequired
	}
	var totalSize int64
	for _, file := range files {
		totalSize += file.Size
	}
	if totalSize > invoiceEmailAttachmentLimit {
		common.SysLog(fmt.Sprintf("invoice notification %d attachments exceed SMTP limit: %d bytes", delivery.Id, totalSize))
		return common.SendEmail(delivery.Subject, recipient, delivery.Body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	attachments := make([]common.EmailAttachment, 0, len(files))
	readers := make([]io.Closer, 0, len(files))
	defer func() {
		for _, reader := range readers {
			_ = reader.Close()
		}
	}()
	for _, file := range files {
		storage, storageErr := invoicefile.ForProfile(file.StorageProfileId, file.StorageType)
		if storageErr != nil {
			return storageErr
		}
		reader, getErr := storage.Get(ctx, file.StorageKey)
		if getErr != nil {
			return getErr
		}
		readers = append(readers, reader)
		attachments = append(attachments, common.EmailAttachment{
			Filename:    file.FileName,
			ContentType: file.MimeType,
			Reader:      reader,
		})
	}
	return common.SendEmailWithAttachments(delivery.Subject, recipient, delivery.Body, attachments)
}

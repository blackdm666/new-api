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
	invoiceNotificationLease    = 10 * time.Minute
	invoiceEmailAttachmentLimit = 25 * 1024 * 1024
)

func StartInvoiceNotificationDelivery() {
	if !common.IsMasterNode {
		return
	}
	gopool.Go(func() {
		deliverPendingInvoiceNotifications()
		ticker := time.NewTicker(invoiceNotificationInterval)
		defer ticker.Stop()
		for range ticker.C {
			deliverPendingInvoiceNotifications()
		}
	})
}

func ScheduleInvoiceNotification(delivery *model.InvoiceNotificationDelivery) {
	if delivery == nil || delivery.DeliveredTime != 0 {
		return
	}
	gopool.Go(func() { deliverInvoiceNotification(delivery) })
}

func deliverPendingInvoiceNotifications() {
	deliveries, err := model.ListDueInvoiceNotifications(100, common.GetTimestamp())
	if err != nil {
		common.SysError("failed to list pending invoice notifications: " + err.Error())
		return
	}
	for _, delivery := range deliveries {
		deliverInvoiceNotification(delivery)
	}
}

func deliverInvoiceNotification(delivery *model.InvoiceNotificationDelivery) {
	now := common.GetTimestamp()
	claimed, err := model.ClaimInvoiceNotification(delivery.Id, now, time.Now().Add(invoiceNotificationLease).Unix())
	if err != nil || !claimed {
		if err != nil {
			common.SysError(fmt.Sprintf("failed to claim invoice notification %d: %s", delivery.Id, err.Error()))
		}
		return
	}

	switch delivery.Kind {
	case model.InvoiceNotificationKindAdminEmail:
		err = common.SendEmail(delivery.Subject, delivery.Recipient, delivery.Body)
	case model.InvoiceNotificationKindUserEmail, model.InvoiceNotificationKindUserLegacy:
		err = sendIssuedInvoiceEmail(delivery)
	default:
		err = fmt.Errorf("unsupported invoice notification kind: %s", delivery.Kind)
	}
	if err == nil {
		if completeErr := model.CompleteInvoiceNotification(delivery.Id); completeErr != nil {
			common.SysError(fmt.Sprintf("failed to complete invoice notification %d: %s", delivery.Id, completeErr.Error()))
		}
		return
	}

	delay := invoiceNotificationInterval
	for attempt := 0; attempt < delivery.Attempts && delay < 24*time.Hour; attempt++ {
		delay *= 2
	}
	if delay > 24*time.Hour {
		delay = 24 * time.Hour
	}
	if recordErr := model.RecordInvoiceNotificationFailure(delivery.Id, err.Error(), time.Now().Add(delay).Unix()); recordErr != nil {
		common.SysError(fmt.Sprintf("failed to record invoice notification failure %d: %s", delivery.Id, recordErr.Error()))
	}
}

func sendIssuedInvoiceEmail(delivery *model.InvoiceNotificationDelivery) error {
	user, err := model.GetUserById(delivery.UserId, false)
	if err != nil {
		return err
	}
	recipient := strings.TrimSpace(user.Email)
	if recipient == "" {
		return fmt.Errorf("invoice user %d has no registered email", user.Id)
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

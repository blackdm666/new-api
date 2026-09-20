package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceNotificationDeliveryIsIdempotentAndUsesSharedOutbox(t *testing.T) {
	truncateTables(t)
	delivery, created, err := EnqueueInvoiceNotification(&InvoiceNotificationDelivery{
		DeliveryKey:      "invoice-notification-key",
		InvoiceRequestId: 42,
		Kind:             InvoiceNotificationKindAdminEmail,
		Recipient:        "billing@example.com",
		Subject:          "Invoice request",
		Body:             "body",
	})
	require.NoError(t, err)
	assert.True(t, created)

	duplicate, created, err := EnqueueInvoiceNotification(&InvoiceNotificationDelivery{
		DeliveryKey:      "invoice-notification-key",
		InvoiceRequestId: 42,
		Kind:             InvoiceNotificationKindAdminEmail,
		Recipient:        "other@example.com",
		Subject:          "duplicate",
		Body:             "duplicate",
	})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, delivery.Id, duplicate.Id)
	assert.NotZero(t, delivery.EmailDeliveryId)
	assert.Equal(t, delivery.EmailDeliveryId, duplicate.EmailDeliveryId)

	queued, err := GetEmailDeliveryById(delivery.EmailDeliveryId)
	require.NoError(t, err)
	assert.Equal(t, "invoice_admin_email", queued.Category)
	assert.Equal(t, delivery.Id, queued.InvoiceDeliveryId)
	assert.Equal(t, EmailPriorityBusiness, queued.Priority)
}

func TestInvoiceNotificationDeliveryMirrorsRetryAndRetainsOutboxRecipientAfterSuccess(t *testing.T) {
	truncateTables(t)
	delivery, _, err := EnqueueInvoiceNotification(&InvoiceNotificationDelivery{
		DeliveryKey:      "invoice-notification-retry",
		InvoiceRequestId: 43,
		Kind:             InvoiceNotificationKindUser,
		UserId:           7,
		Recipient:        "user@example.com",
		Subject:          "Invoice status",
		Body:             "sensitive body",
	})
	require.NoError(t, err)

	now := common.GetTimestamp()
	require.NoError(t, RecordEmailDeliveryFailure(delivery.EmailDeliveryId, "smtp unavailable", now+60))
	queued, err := GetEmailDeliveryById(delivery.EmailDeliveryId)
	require.NoError(t, err)
	require.NoError(t, SyncInvoiceNotificationFromEmailDelivery(queued))
	var mirrored InvoiceNotificationDelivery
	require.NoError(t, DB.First(&mirrored, delivery.Id).Error)
	assert.Equal(t, 1, mirrored.Attempts)
	assert.Equal(t, "smtp unavailable", mirrored.LastError)

	require.NoError(t, CompleteEmailDelivery(delivery.EmailDeliveryId))
	require.NoError(t, CompleteInvoiceNotification(delivery.Id))
	var completed InvoiceNotificationDelivery
	require.NoError(t, DB.First(&completed, delivery.Id).Error)
	assert.NotZero(t, completed.DeliveredTime)
	assert.Empty(t, completed.Recipient)
	assert.Empty(t, completed.Subject)
	assert.Empty(t, completed.Body)
	queued, err = GetEmailDeliveryById(delivery.EmailDeliveryId)
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", queued.Recipient)
	assert.Empty(t, queued.Subject)
	assert.Empty(t, queued.Body)
}

func TestInvoiceNotificationCanBeRetriedManuallyThroughSharedOutbox(t *testing.T) {
	truncateTables(t)
	delivery, _, err := EnqueueInvoiceNotification(&InvoiceNotificationDelivery{
		DeliveryKey: "invoice-notification-manual-retry", InvoiceRequestId: 44,
		Kind: InvoiceNotificationKindAdminEmail, Recipient: "billing@example.com",
		Subject: "Invoice", Body: "body",
	})
	require.NoError(t, err)
	now := common.GetTimestamp()
	for attempt := 0; attempt < EmailDeliveryMaxAttempts; attempt++ {
		require.NoError(t, RecordEmailDeliveryFailure(delivery.EmailDeliveryId, "smtp unavailable", now+3600))
	}

	retried, err := RetryInvoiceNotification(delivery.Id)
	require.NoError(t, err)
	assert.Zero(t, retried.LockedUntil)
	assert.LessOrEqual(t, retried.NextAttemptTime, common.GetTimestamp())
	queued, err := GetEmailDeliveryById(delivery.EmailDeliveryId)
	require.NoError(t, err)
	assert.Zero(t, queued.Attempts)
	assert.Zero(t, queued.DeadLetterTime)
	pending, err := ListPendingInvoiceNotifications(10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
}

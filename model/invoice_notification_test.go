package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceNotificationDeliveryIsIdempotentAndLeaseProtected(t *testing.T) {
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

	now := common.GetTimestamp()
	claimed, err := ClaimInvoiceNotification(delivery.Id, now, now+120)
	require.NoError(t, err)
	assert.True(t, claimed)
	claimed, err = ClaimInvoiceNotification(delivery.Id, now, now+120)
	require.NoError(t, err)
	assert.False(t, claimed)
}

func TestInvoiceNotificationDeliveryRetriesAndClearsPayloadAfterSuccess(t *testing.T) {
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
	claimed, err := ClaimInvoiceNotification(delivery.Id, now, now+120)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, RecordInvoiceNotificationFailure(delivery.Id, "smtp unavailable", now+60))

	due, err := ListDueInvoiceNotifications(10, now)
	require.NoError(t, err)
	assert.Empty(t, due)
	due, err = ListDueInvoiceNotifications(10, now+61)
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, 1, due[0].Attempts)

	require.NoError(t, CompleteInvoiceNotification(delivery.Id))
	var completed InvoiceNotificationDelivery
	require.NoError(t, DB.First(&completed, delivery.Id).Error)
	assert.NotZero(t, completed.DeliveredTime)
	assert.Empty(t, completed.Recipient)
	assert.Empty(t, completed.Subject)
	assert.Empty(t, completed.Body)
}

func TestInvoiceNotificationCanBeRetriedManually(t *testing.T) {
	truncateTables(t)
	delivery, _, err := EnqueueInvoiceNotification(&InvoiceNotificationDelivery{
		DeliveryKey: "invoice-notification-manual-retry", InvoiceRequestId: 44,
		Kind: InvoiceNotificationKindAdminEmail, Recipient: "billing@example.com",
		Subject: "Invoice", Body: "body",
	})
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, RecordInvoiceNotificationFailure(delivery.Id, "smtp unavailable", now+3600))

	retried, err := RetryInvoiceNotification(delivery.Id)
	require.NoError(t, err)
	assert.Zero(t, retried.LockedUntil)
	assert.LessOrEqual(t, retried.NextAttemptTime, common.GetTimestamp())
	pending, err := ListPendingInvoiceNotifications(10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
}

package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailDeliveryOutboxIsIdempotentAndRetriesToDeadLetter(t *testing.T) {
	truncateTables(t)
	delivery, created, err := EnqueueEmailDelivery(&EmailDelivery{
		DeliveryKey: "test-email-event:1",
		Category:    "test",
		RelatedId:   1,
		Recipient:   "user@example.com",
		Subject:     "Subject",
		Body:        "Body",
	})
	require.NoError(t, err)
	assert.True(t, created)
	duplicate, created, err := EnqueueEmailDelivery(&EmailDelivery{
		DeliveryKey: "test-email-event:1",
		Category:    "test",
		RelatedId:   1,
		Recipient:   "changed@example.com",
		Subject:     "Changed",
		Body:        "Changed",
	})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, delivery.Id, duplicate.Id)
	assert.Equal(t, "user@example.com", duplicate.Recipient)

	for attempt := 0; attempt < EmailDeliveryMaxAttempts; attempt++ {
		require.NoError(t, RecordEmailDeliveryFailure(delivery.Id, fmt.Sprintf("smtp failure %d", attempt+1), common.GetTimestamp()))
	}
	require.NoError(t, DB.First(delivery, delivery.Id).Error)
	assert.Equal(t, EmailDeliveryMaxAttempts, delivery.Attempts)
	assert.Positive(t, delivery.DeadLetterTime)
	assert.Contains(t, delivery.LastError, "smtp failure")

	rows, total, err := ListEmailDeliveries(EmailDeliveryQueryOptions{
		Keyword:    "user@example.com",
		FailedOnly: true,
	}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, delivery.Id, rows[0].Id)

	require.NoError(t, RetryEmailDelivery(delivery.Id))
	require.NoError(t, DB.First(delivery, delivery.Id).Error)
	assert.Zero(t, delivery.Attempts)
	assert.Zero(t, delivery.DeadLetterTime)
	assert.Empty(t, delivery.LastError)
}

func TestCompleteEmailDeliveryScrubsSensitiveContent(t *testing.T) {
	truncateTables(t)
	delivery, _, err := EnqueueEmailDelivery(&EmailDelivery{
		DeliveryKey: "test-email-event:2",
		Category:    "test",
		Recipient:   "user@example.com",
		Subject:     "Sensitive subject",
		Body:        "Sensitive body",
	})
	require.NoError(t, err)
	require.NoError(t, CompleteEmailDelivery(delivery.Id))
	require.NoError(t, DB.First(delivery, delivery.Id).Error)
	assert.Positive(t, delivery.DeliveredTime)
	assert.Empty(t, delivery.Recipient)
	assert.Empty(t, delivery.Subject)
	assert.Empty(t, delivery.Body)
}

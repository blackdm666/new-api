package model

import (
	"fmt"
	"sync"
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
		require.NoError(t, RecordEmailDeliveryFailure(delivery.Id, fmt.Sprintf("smtp failure %d for user@example.com", attempt+1), common.GetTimestamp()))
	}
	require.NoError(t, DB.First(delivery, delivery.Id).Error)
	assert.Equal(t, EmailDeliveryMaxAttempts, delivery.Attempts)
	assert.Positive(t, delivery.DeadLetterTime)
	assert.Contains(t, delivery.LastError, "smtp failure")

	rows, total, err := ListEmailDeliveries(EmailDeliveryQueryOptions{
		Keyword: "user@example.com",
		Status:  EmailDeliveryStatusFailed,
	}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, delivery.Id, rows[0].Id)
	assert.NotContains(t, rows[0].LastError, "user@example.com")
	assert.Contains(t, rows[0].LastError, "ur***@example.com")

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
	assert.Equal(t, "ur***@example.com", delivery.RecipientMasked)
}

func TestEmailDeliveryPriorityStatusBatchRetryAndCleanup(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	marketing, _, err := EnqueueEmailDelivery(&EmailDelivery{
		DeliveryKey: "priority:marketing", Category: "marketing_custom", Recipient: "marketing@example.com", Subject: "m", Body: "m", Priority: EmailPriorityMarketing,
	})
	require.NoError(t, err)
	critical, _, err := EnqueueEmailDelivery(&EmailDelivery{
		DeliveryKey: "priority:critical", Category: "email_verification", Recipient: "critical@example.com", Subject: "c", Body: "c", Priority: EmailPriorityCritical,
	})
	require.NoError(t, err)
	due, err := ListDueEmailDeliveries(10, now+1)
	require.NoError(t, err)
	require.Len(t, due, 2)
	assert.Equal(t, critical.Id, due[0].Id)
	assert.Equal(t, marketing.Id, due[1].Id)

	require.NoError(t, DB.Model(marketing).Updates(map[string]any{"attempts": EmailDeliveryMaxAttempts, "dead_letter_time": now, "last_error": "550 user unknown"}).Error)
	require.NoError(t, DB.Model(critical).Updates(map[string]any{"attempts": EmailDeliveryMaxAttempts, "dead_letter_time": now, "last_error": "temporary"}).Error)
	retried, err := RetryEmailDeliveries([]int{marketing.Id, critical.Id})
	require.NoError(t, err)
	assert.Equal(t, int64(2), retried)

	require.NoError(t, DB.Model(marketing).Updates(map[string]any{"delivered_time": now - 31*86400, "created_time": now - 40*86400}).Error)
	require.NoError(t, DB.Model(critical).Updates(map[string]any{"dead_letter_time": now - 91*86400, "updated_time": now - 91*86400}).Error)
	removed, err := CleanupEmailDeliveries(now-30*86400, now-90*86400, 500)
	require.NoError(t, err)
	assert.Equal(t, int64(2), removed)
}

func TestMarketingQuotaReservationIncludesCrossDayBacklog(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	dayStart := now - 3600
	current, _, err := EnqueueEmailDelivery(&EmailDelivery{
		DeliveryKey: "marketing-quota-current", Category: "marketing_custom", Recipient: "current@example.com", Subject: "current", Body: "current", Priority: EmailPriorityMarketing,
	})
	require.NoError(t, err)
	backlog, _, err := EnqueueEmailDelivery(&EmailDelivery{
		DeliveryKey: "marketing-quota-backlog", Category: "marketing_custom", Recipient: "backlog@example.com", Subject: "backlog", Body: "backlog", Priority: EmailPriorityMarketing,
	})
	require.NoError(t, err)
	require.NoError(t, DB.Model(backlog).Update("marketing_quota_time", dayStart-1).Error)
	reserved, err := ReserveMarketingEmailQuota(backlog.Id, dayStart, now, 1)
	require.NoError(t, err)
	assert.False(t, reserved)
	require.NoError(t, DB.Model(current).Update("marketing_quota_time", dayStart-1).Error)
	reserved, err = ReserveMarketingEmailQuota(backlog.Id, dayStart, now, 1)
	require.NoError(t, err)
	assert.True(t, reserved)
}

func TestMarketingDailyQuotaStopsCrossDayBacklogAtExactLimit(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	dayStart := now - 1

	for i := 0; i < MarketingDailyLimit; i++ {
		_, _, err := EnqueueEmailDelivery(&EmailDelivery{
			DeliveryKey: fmt.Sprintf("marketing-daily-limit-%d", i),
			Category:    "marketing_custom",
			Recipient:   fmt.Sprintf("recipient-%d@example.com", i),
			Subject:     "quota test",
			Body:        "quota test",
			Priority:    EmailPriorityMarketing,
		})
		require.NoError(t, err)
	}

	stats, err := GetEmailDeliveryStats(now, dayStart)
	require.NoError(t, err)
	assert.EqualValues(t, MarketingDailyLimit, stats.MarketingSentToday)

	backlog, _, err := EnqueueEmailDelivery(&EmailDelivery{
		DeliveryKey: "marketing-daily-limit-backlog",
		Category:    "marketing_custom",
		Recipient:   "backlog@example.com",
		Subject:     "backlog",
		Body:        "backlog",
		Priority:    EmailPriorityMarketing,
	})
	require.NoError(t, err)
	require.NoError(t, DB.Model(backlog).Update("marketing_quota_time", dayStart-1).Error)

	reserved, err := ReserveMarketingEmailQuota(backlog.Id, dayStart, now, MarketingDailyLimit)
	require.NoError(t, err)
	assert.False(t, reserved)
}

func TestEmailDeliveryClaimHasSingleWinnerAcrossRunners(t *testing.T) {
	truncateTables(t)
	delivery, _, err := EnqueueEmailDelivery(&EmailDelivery{
		DeliveryKey: "email-claim-single-winner",
		Category:    "system_alert",
		Recipient:   "root@example.com",
		Subject:     "claim test",
		Body:        "claim test",
		Priority:    EmailPriorityCritical,
	})
	require.NoError(t, err)

	const runners = 12
	results := make(chan bool, runners)
	var wg sync.WaitGroup
	wg.Add(runners)
	for i := 0; i < runners; i++ {
		go func() {
			defer wg.Done()
			claimed, claimErr := ClaimEmailDelivery(delivery.Id, common.GetTimestamp(), common.GetTimestamp()+60)
			results <- claimErr == nil && claimed
		}()
	}
	wg.Wait()
	close(results)

	winners := 0
	for won := range results {
		if won {
			winners++
		}
	}
	assert.Equal(t, 1, winners)
}

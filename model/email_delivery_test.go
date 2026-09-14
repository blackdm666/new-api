package model

import (
	"errors"
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
	assert.Equal(t, "user@example.com", rows[0].Recipient)
	assert.Contains(t, rows[0].LastError, "user@example.com")

	require.NoError(t, RetryEmailDelivery(delivery.Id))
	require.NoError(t, DB.First(delivery, delivery.Id).Error)
	assert.Zero(t, delivery.Attempts)
	assert.Zero(t, delivery.DeadLetterTime)
	assert.Empty(t, delivery.LastError)
}

func TestCompleteEmailDeliveryRetainsRecipientForRootMaintenance(t *testing.T) {
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
	assert.Equal(t, "user@example.com", delivery.Recipient)
	assert.Empty(t, delivery.Subject)
	assert.Empty(t, delivery.Body)

	rows, total, err := ListEmailDeliveries(EmailDeliveryQueryOptions{
		Keyword: "user@example.com",
		Status:  EmailDeliveryStatusAcceptedUntracked,
	}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, "user@example.com", rows[0].Recipient)
	_, finalTotal, err := ListEmailDeliveries(EmailDeliveryQueryOptions{
		Status: EmailDeliveryStatusDelivered,
	}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Zero(t, finalTotal)
}

func TestExpiredEmailDeliveryRetainsRecipientForRootMaintenance(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	bulkExpired, _, err := EnqueueEmailDelivery(&EmailDelivery{
		DeliveryKey: "test-email-expired:bulk",
		Category:    "email_verification",
		Recipient:   "bulk-expired@example.com",
		Subject:     "Expiring subject",
		Body:        "Expiring body",
		ExpiresTime: now - 1,
	})
	require.NoError(t, err)
	singleExpired, _, err := EnqueueEmailDelivery(&EmailDelivery{
		DeliveryKey: "test-email-expired:single",
		Category:    "marketing_custom",
		Recipient:   "single-expired@example.com",
		Subject:     "Expiring subject",
		Body:        "Expiring body",
	})
	require.NoError(t, err)

	require.NoError(t, ExpireEmailDeliveries(now))
	require.NoError(t, ExpireEmailDelivery(singleExpired.Id, "campaign stopped"))
	require.NoError(t, DB.First(bulkExpired, bulkExpired.Id).Error)
	require.NoError(t, DB.First(singleExpired, singleExpired.Id).Error)
	assert.Equal(t, "bulk-expired@example.com", bulkExpired.Recipient)
	assert.Equal(t, "single-expired@example.com", singleExpired.Recipient)
	assert.Empty(t, bulkExpired.Subject)
	assert.Empty(t, bulkExpired.Body)
	assert.Empty(t, singleExpired.Subject)
	assert.Empty(t, singleExpired.Body)
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
	account := newTestEmailSenderAccount(t, "cleanup-secret")
	attempt, err := CreateEmailDeliveryAttempt(marketing.Id, account, EmailAttemptPurposeDelivery, marketing.Recipient, "<cleanup@example.com>", "notify-cleanup@example.com")
	require.NoError(t, err)
	removed, err := CleanupEmailDeliveries(now-30*86400, now-90*86400, 500)
	require.NoError(t, err)
	assert.Equal(t, int64(2), removed)
	var attemptCount int64
	require.NoError(t, DB.Model(&EmailDeliveryAttempt{}).Where("id = ?", attempt.Id).Count(&attemptCount).Error)
	assert.Zero(t, attemptCount)
}

func TestMarketingQuotaReservationIncludesCrossDayBacklog(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	dayStart := now - 3600
	backlog, _, err := EnqueueEmailDelivery(&EmailDelivery{
		DeliveryKey: "marketing-quota-backlog", Category: "marketing_custom", Recipient: "backlog@example.com", Subject: "backlog", Body: "backlog", Priority: EmailPriorityMarketing,
	})
	require.NoError(t, err)
	require.NoError(t, DB.Model(backlog).Update("marketing_quota_time", dayStart-1).Error)
	reserved, err := ReserveMarketingEmailQuota(backlog.Id, dayStart, now, 1)
	require.NoError(t, err)
	assert.True(t, reserved)

	second, _, err := EnqueueEmailDelivery(&EmailDelivery{
		DeliveryKey: "marketing-quota-second", Category: "marketing_custom", Recipient: "second@example.com", Subject: "second", Body: "second", Priority: EmailPriorityMarketing,
	})
	require.NoError(t, err)
	reserved, err = ReserveMarketingEmailQuota(second.Id, dayStart, now, 1)
	require.NoError(t, err)
	assert.False(t, reserved)
}

func TestMarketingEmailEnqueueReservesQuotaAtomically(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	dayStart := now - 1

	first, created, err := EnqueueMarketingEmailDelivery(&EmailDelivery{
		DeliveryKey: "marketing-atomic-first", Category: "marketing_custom", Recipient: "first@example.com", Subject: "first", Body: "first", Priority: EmailPriorityMarketing,
	}, dayStart, now, 1)
	require.NoError(t, err)
	assert.True(t, created)
	assert.GreaterOrEqual(t, first.MarketingQuotaTime, dayStart)

	_, _, err = EnqueueMarketingEmailDelivery(&EmailDelivery{
		DeliveryKey: "marketing-atomic-second", Category: "marketing_custom", Recipient: "second@example.com", Subject: "second", Body: "second", Priority: EmailPriorityMarketing,
	}, dayStart, now, 1)
	assert.ErrorIs(t, err, ErrMarketingEmailDailyLimitReached)

	var deliveries int64
	require.NoError(t, DB.Model(&EmailDelivery{}).Count(&deliveries).Error)
	assert.EqualValues(t, 1, deliveries)
}

func TestMarketingEmailStatsSeparateQuotaUsageFromDelivery(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	dayStart := now - 1
	delivery, _, err := EnqueueMarketingEmailDelivery(&EmailDelivery{
		DeliveryKey: "marketing-stats", Category: "marketing_custom", Recipient: "stats@example.com", Subject: "stats", Body: "stats", Priority: EmailPriorityMarketing,
	}, dayStart, now, 10)
	require.NoError(t, err)

	stats, err := GetEmailDeliveryStats(now, dayStart)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.MarketingQuotaUsedToday)
	assert.Zero(t, stats.MarketingSentToday)

	require.NoError(t, CompleteEmailDelivery(delivery.Id))
	stats, err = GetEmailDeliveryStats(common.GetTimestamp(), dayStart)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.MarketingQuotaUsedToday)
	assert.EqualValues(t, 1, stats.MarketingSentToday)
}

func TestMarketingEmailQuotaHasExactWinnerCountAcrossConcurrentEnqueues(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	dayStart := now - 1
	const dailyLimit = 5
	const candidates = 20

	results := make(chan error, candidates)
	var wg sync.WaitGroup
	wg.Add(candidates)
	for index := 0; index < candidates; index++ {
		go func() {
			defer wg.Done()
			_, _, err := EnqueueMarketingEmailDelivery(&EmailDelivery{
				DeliveryKey: fmt.Sprintf("marketing-concurrent-%d", index),
				Category:    "marketing_custom",
				Recipient:   fmt.Sprintf("concurrent-%d@example.com", index),
				Subject:     "concurrent",
				Body:        "concurrent",
				Priority:    EmailPriorityMarketing,
			}, dayStart, now, dailyLimit)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	winners := 0
	limited := 0
	for err := range results {
		if err == nil {
			winners++
			continue
		}
		if errors.Is(err, ErrMarketingEmailDailyLimitReached) {
			limited++
			continue
		}
		require.NoError(t, err)
	}
	assert.Equal(t, dailyLimit, winners)
	assert.Equal(t, candidates-dailyLimit, limited)
}

func TestEmailDeliveryStatsUseZeroTimestampsForAnEmptyQueue(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	stats, err := GetEmailDeliveryStats(now, now-3600)
	require.NoError(t, err)
	assert.Zero(t, stats.OldestPendingTime)
	assert.Zero(t, stats.LastDeliveredTime)
}

func TestMarketingDailyQuotaStopsCrossDayBacklogAtExactLimit(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	dayStart := now - 1

	for i := 0; i < MarketingDailyLimit; i++ {
		_, _, err := EnqueueMarketingEmailDelivery(&EmailDelivery{
			DeliveryKey: fmt.Sprintf("marketing-daily-limit-%d", i),
			Category:    "marketing_custom",
			Recipient:   fmt.Sprintf("recipient-%d@example.com", i),
			Subject:     "quota test",
			Body:        "quota test",
			Priority:    EmailPriorityMarketing,
		}, dayStart, now, MarketingDailyLimit)
		require.NoError(t, err)
	}

	stats, err := GetEmailDeliveryStats(now, dayStart)
	require.NoError(t, err)
	assert.EqualValues(t, MarketingDailyLimit, stats.MarketingQuotaUsedToday)
	assert.Zero(t, stats.MarketingSentToday)

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

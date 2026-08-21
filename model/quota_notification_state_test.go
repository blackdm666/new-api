package model

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotaWarningIsClaimedOnlyWhenBalanceCrossesBelowThreshold(t *testing.T) {
	truncateTables(t)
	claimed, version, err := ClaimQuotaWarning(101, QuotaNotificationSourceWallet, 0, 1000, 900)
	require.NoError(t, err)
	assert.True(t, claimed)
	assert.Equal(t, int64(1), version)

	claimed, _, err = ClaimQuotaWarning(101, QuotaNotificationSourceWallet, 0, 1000, 800)
	require.NoError(t, err)
	assert.False(t, claimed)

	claimed, _, err = ClaimQuotaWarning(101, QuotaNotificationSourceWallet, 0, 1000, 1200)
	require.NoError(t, err)
	assert.False(t, claimed)
	claimed, version, err = ClaimQuotaWarning(101, QuotaNotificationSourceWallet, 0, 1000, 700)
	require.NoError(t, err)
	assert.True(t, claimed)
	assert.Equal(t, int64(2), version)
}

func TestConcurrentQuotaWarningClaimsProduceOneEvent(t *testing.T) {
	truncateTables(t)
	const workers = 12
	var wait sync.WaitGroup
	wait.Add(workers)
	results := make(chan bool, workers)
	errors := make(chan error, workers)
	for index := 0; index < workers; index++ {
		go func() {
			defer wait.Done()
			claimed, _, err := ClaimQuotaWarning(202, QuotaNotificationSourceSubscription, 88, 1000, 500)
			results <- claimed
			errors <- err
		}()
	}
	wait.Wait()
	close(results)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	claimedCount := 0
	for claimed := range results {
		if claimed {
			claimedCount++
		}
	}
	assert.Equal(t, 1, claimedCount)
}

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertMidjourneyTask(t *testing.T) Midjourney {
	t.Helper()
	task := Midjourney{
		MjId:             "mj-billing-race",
		Status:           "IN_PROGRESS",
		Progress:         "50%",
		Quota:            1000,
		TokenId:          23,
		BillingChannelId: 17,
	}
	require.NoError(t, DB.Create(&task).Error)
	return task
}

func TestMidjourneyUpdateDoesNotRestoreClearedBillingState(t *testing.T) {
	truncateTables(t)
	stale := insertMidjourneyTask(t)

	require.NoError(t, DB.Model(&Midjourney{}).Where("id = ?", stale.Id).Updates(map[string]any{
		"quota":              0,
		"token_id":           0,
		"billing_channel_id": 0,
	}).Error)
	stale.Progress = "75%"
	require.NoError(t, stale.Update())

	var got Midjourney
	require.NoError(t, DB.First(&got, stale.Id).Error)
	assert.Equal(t, "75%", got.Progress)
	assert.Zero(t, got.Quota)
	assert.Zero(t, got.TokenId)
	assert.Zero(t, got.BillingChannelId)
}

func TestMidjourneyStatusCASDoesNotRestoreClearedBillingState(t *testing.T) {
	truncateTables(t)
	stale := insertMidjourneyTask(t)

	require.NoError(t, DB.Model(&Midjourney{}).Where("id = ?", stale.Id).Updates(map[string]any{
		"quota":              0,
		"token_id":           0,
		"billing_channel_id": 0,
	}).Error)
	stale.Status = "FAILURE"
	stale.Progress = "100%"
	won, err := stale.UpdateWithStatus("IN_PROGRESS")
	require.NoError(t, err)
	assert.True(t, won)

	var got Midjourney
	require.NoError(t, DB.First(&got, stale.Id).Error)
	assert.Equal(t, "FAILURE", got.Status)
	assert.Zero(t, got.Quota)
	assert.Zero(t, got.TokenId)
	assert.Zero(t, got.BillingChannelId)
}

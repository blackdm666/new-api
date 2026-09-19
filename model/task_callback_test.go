package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInsertWithCallbackCreatesDurableOutboxEntry(t *testing.T) {
	truncateTables(t)
	task := &Task{
		TaskID:   "task_callback_atomic",
		UserId:   7,
		Status:   TaskStatusNotStart,
		Progress: "0%",
		PrivateData: TaskPrivateData{
			TokenId: 9,
		},
	}

	require.NoError(t, task.InsertWithCallback("https://example.com/task-callback"))
	delivery, err := GetTaskCallbackByPublicTaskId(task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, task.ID, delivery.TaskId)
	assert.Equal(t, task.UserId, delivery.UserId)
	assert.Equal(t, task.PrivateData.TokenId, delivery.TokenId)
	assert.True(t, task.PrivateData.CallbackEnabled)
	assert.Equal(t, "https://example.com/task-callback", delivery.CallbackURL)
	assert.Zero(t, delivery.Attempts)
	assert.Zero(t, delivery.DeliveredTime)
}

func TestInsertWithCallbackRollsBackTaskWhenOutboxIsInvalid(t *testing.T) {
	truncateTables(t)
	task := &Task{TaskID: "task_callback_rollback", UserId: 7, Status: TaskStatusNotStart}

	err := task.InsertWithCallback("https://example.com/task-callback")
	require.ErrorIs(t, err, ErrTaskCallbackInvalid)
	var count int64
	require.NoError(t, DB.Model(&Task{}).Where("task_id = ?", task.TaskID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestTaskCallbackFailureMovesToDeadLetterAtLimit(t *testing.T) {
	truncateTables(t)
	task := &Task{
		TaskID:   "task_callback_dead_letter",
		UserId:   8,
		Status:   TaskStatusFailure,
		Progress: "100%",
		PrivateData: TaskPrivateData{
			TokenId: 10,
		},
	}
	require.NoError(t, task.InsertWithCallback("https://example.com/task-callback"))
	delivery, err := GetTaskCallbackByPublicTaskId(task.TaskID)
	require.NoError(t, err)

	for attempt := 0; attempt < TaskCallbackMaxAttempts; attempt++ {
		require.NoError(t, RecordTaskCallbackFailure(delivery.Id, "upstream unavailable", 1234))
	}
	require.NoError(t, DB.First(delivery, delivery.Id).Error)
	assert.Equal(t, TaskCallbackMaxAttempts, delivery.Attempts)
	assert.NotZero(t, delivery.DeadLetterTime)
	assert.Zero(t, delivery.NextAttemptTime)
}

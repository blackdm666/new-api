package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliverTaskCallbackSignsAndCompletesDelivery(t *testing.T) {
	truncate(t)
	const userID = 81
	const tokenID = 82
	const tokenKey = "callback-signing-key"
	seedUser(t, userID, 1000)
	seedToken(t, tokenID, userID, tokenKey, 1000)

	var received taskCallbackEvent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = common.Unmarshal(body, &received)
		timestamp := r.Header.Get("X-NewAPI-Timestamp")
		expected := common.GenerateHMACWithKey([]byte("sk-"+tokenKey), timestamp+"."+string(body))
		assert.Equal(t, "sha256="+expected, r.Header.Get("X-NewAPI-Signature"))
		assert.Equal(t, "video.completed", r.Header.Get("X-NewAPI-Event"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	previousValidator := taskCallbackURLValidator
	previousClient := taskCallbackHTTPClient
	taskCallbackURLValidator = func(string) error { return nil }
	taskCallbackHTTPClient = server.Client
	t.Cleanup(func() {
		taskCallbackURLValidator = previousValidator
		taskCallbackHTTPClient = previousClient
	})

	task := &model.Task{
		TaskID:     "task_callback_success",
		UserId:     userID,
		Status:     model.TaskStatusSuccess,
		Progress:   "100%",
		CreatedAt:  time.Now().Add(-time.Minute).Unix(),
		FinishTime: time.Now().Unix(),
		Properties: model.Properties{OriginModelName: "gemini-omni-flash-preview"},
		PrivateData: model.TaskPrivateData{
			TokenId:   tokenID,
			ResultURL: "https://api.example.com/v1/videos/task_callback_success/content",
		},
	}
	require.NoError(t, task.InsertWithCallback(server.URL))
	delivery, err := model.GetTaskCallbackByPublicTaskId(task.TaskID)
	require.NoError(t, err)

	deliverTaskCallback(delivery)

	require.NoError(t, model.DB.First(delivery, delivery.Id).Error)
	assert.NotZero(t, delivery.DeliveredTime)
	assert.Zero(t, delivery.Attempts)
	assert.Equal(t, "evt_task_callback_success_completed", received.ID)
	assert.Equal(t, task.TaskID, received.Data.TaskID)
	assert.Equal(t, "completed", received.Data.Status)
	assert.Equal(t, task.GetResultURL(), received.Data.OutputURL)
}

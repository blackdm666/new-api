package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/bytedance/gopkg/util/gopool"
)

const (
	taskCallbackDeliveryInterval = 15 * time.Second
	taskCallbackLease            = 2 * time.Minute
	taskCallbackRequestTimeout   = 15 * time.Second
	taskCallbackRetryInitial     = 10 * time.Second
	taskCallbackRetryMax         = 15 * time.Minute
)

type taskCallbackEvent struct {
	ID        string           `json:"id"`
	Type      string           `json:"type"`
	CreatedAt int64            `json:"created_at"`
	Data      taskCallbackData `json:"data"`
}

type taskCallbackData struct {
	TaskID      string `json:"task_id"`
	Model       string `json:"model"`
	Status      string `json:"status"`
	Progress    string `json:"progress"`
	OutputURL   string `json:"output_url,omitempty"`
	Error       string `json:"error,omitempty"`
	CreatedAt   int64  `json:"created_at"`
	CompletedAt int64  `json:"completed_at"`
}

var taskCallbackURLValidator = ValidateSSRFProtectedFetchURL
var taskCallbackHTTPClient = GetSSRFProtectedHTTPClient

func StartTaskCallbackDelivery() {
	if !common.IsMasterNode {
		return
	}
	gopool.Go(func() {
		deliverDueTaskCallbacks()
		ticker := time.NewTicker(taskCallbackDeliveryInterval)
		defer ticker.Stop()
		for range ticker.C {
			deliverDueTaskCallbacks()
		}
	})
}

func ScheduleTaskCallback(taskID string) {
	delivery, err := model.GetTaskCallbackByPublicTaskId(taskID)
	if err != nil || delivery == nil || delivery.DeliveredTime != 0 || delivery.DeadLetterTime != 0 {
		return
	}
	gopool.Go(func() { deliverTaskCallback(delivery) })
}

func deliverDueTaskCallbacks() {
	now := common.GetTimestamp()
	deliveries, err := model.ListDueTaskCallbacks(100, now)
	if err != nil {
		common.SysError("failed to list pending task callbacks: " + err.Error())
		return
	}
	for _, delivery := range deliveries {
		deliverTaskCallback(delivery)
	}
}

func deliverTaskCallback(delivery *model.TaskCallbackDelivery) {
	if delivery == nil {
		return
	}
	task, exists, err := model.GetByTaskId(delivery.UserId, delivery.PublicTaskId)
	if err != nil || !exists || task == nil {
		recordTaskCallbackFailure(delivery, fmt.Errorf("task is unavailable"))
		return
	}
	if task.Status != model.TaskStatusSuccess && task.Status != model.TaskStatusFailure {
		_ = model.DeferTaskCallback(delivery.Id, time.Now().Add(taskCallbackDeliveryInterval).Unix())
		return
	}

	now := common.GetTimestamp()
	claimed, err := model.ClaimTaskCallback(delivery.Id, now, time.Now().Add(taskCallbackLease).Unix())
	if err != nil || !claimed {
		if err != nil {
			common.SysError(fmt.Sprintf("failed to claim task callback %d: %s", delivery.Id, err.Error()))
		}
		return
	}

	token, err := model.GetTokenById(delivery.TokenId)
	if err != nil || token.UserId != delivery.UserId || strings.TrimSpace(token.Key) == "" {
		recordTaskCallbackFailure(delivery, fmt.Errorf("callback signing token is unavailable"))
		return
	}
	if err := taskCallbackURLValidator(delivery.CallbackURL); err != nil {
		recordTaskCallbackFailure(delivery, fmt.Errorf("callback URL rejected: %w", err))
		return
	}

	eventType := "video.completed"
	if task.Status == model.TaskStatusFailure {
		eventType = "video.failed"
	}
	event := taskCallbackEvent{
		ID:        "evt_" + task.TaskID + "_" + strings.TrimPrefix(eventType, "video."),
		Type:      eventType,
		CreatedAt: now,
		Data: taskCallbackData{
			TaskID:      task.TaskID,
			Model:       task.Properties.OriginModelName,
			Status:      task.Status.ToVideoStatus(),
			Progress:    task.Progress,
			OutputURL:   task.GetResultURL(),
			Error:       task.FailReason,
			CreatedAt:   task.CreatedAt,
			CompletedAt: task.FinishTime,
		},
	}
	payload, err := common.Marshal(event)
	if err != nil {
		recordTaskCallbackFailure(delivery, fmt.Errorf("marshal callback payload: %w", err))
		return
	}
	timestamp := strconv.FormatInt(now, 10)
	signature := common.GenerateHMACWithKey([]byte("sk-"+token.Key), timestamp+"."+string(payload))
	ctx, cancel := context.WithTimeout(context.Background(), taskCallbackRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, delivery.CallbackURL, bytes.NewReader(payload))
	if err != nil {
		recordTaskCallbackFailure(delivery, fmt.Errorf("create callback request: %w", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NewAPI-Task-Callback/1.0")
	req.Header.Set("X-NewAPI-Event", event.Type)
	req.Header.Set("X-NewAPI-Delivery", event.ID)
	req.Header.Set("X-NewAPI-Timestamp", timestamp)
	req.Header.Set("X-NewAPI-Signature", "sha256="+signature)
	resp, err := taskCallbackHTTPClient().Do(req)
	if err != nil {
		recordTaskCallbackFailure(delivery, fmt.Errorf("callback request failed: %s", common.MaskSensitiveInfo(err.Error())))
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		recordTaskCallbackFailure(delivery, fmt.Errorf("callback returned HTTP %d", resp.StatusCode))
		return
	}
	if err := model.CompleteTaskCallback(delivery.Id); err != nil {
		common.SysError(fmt.Sprintf("failed to complete task callback %d: %s", delivery.Id, err.Error()))
	}
}

func recordTaskCallbackFailure(delivery *model.TaskCallbackDelivery, err error) {
	if delivery == nil || err == nil {
		return
	}
	delay := taskCallbackRetryInitial
	for attempt := 0; attempt < delivery.Attempts && delay < taskCallbackRetryMax; attempt++ {
		delay *= 2
	}
	if delay > taskCallbackRetryMax {
		delay = taskCallbackRetryMax
	}
	if recordErr := model.RecordTaskCallbackFailure(delivery.Id, err.Error(), time.Now().Add(delay).Unix()); recordErr != nil {
		common.SysError(fmt.Sprintf("failed to record task callback %d failure: %s", delivery.Id, recordErr.Error()))
	}
}

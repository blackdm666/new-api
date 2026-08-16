package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const emailDeliveryCleanupBatchSize = 500

type emailDeliveryCleanupHandler struct{}

func (emailDeliveryCleanupHandler) Type() string            { return model.SystemTaskTypeEmailDeliveryCleanup }
func (emailDeliveryCleanupHandler) Enabled() bool           { return true }
func (emailDeliveryCleanupHandler) Interval() time.Duration { return 24 * time.Hour }
func (emailDeliveryCleanupHandler) NewPayload() any         { return nil }

func (emailDeliveryCleanupHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	now := common.GetTimestamp()
	total := int64(0)
	for {
		select {
		case <-ctx.Done():
			failSystemTask(task, runnerID, ctx.Err())
			return
		default:
		}
		deleted, err := model.CleanupEmailDeliveries(now-30*86400, now-90*86400, emailDeliveryCleanupBatchSize)
		if err != nil {
			failSystemTask(task, runnerID, err)
			return
		}
		total += deleted
		if deleted < emailDeliveryCleanupBatchSize {
			break
		}
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, map[string]any{"deleted": total}, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func init() {
	RegisterSystemTaskHandler(emailDeliveryCleanupHandler{})
}

package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const TaskCallbackMaxAttempts = 8

var ErrTaskCallbackInvalid = errors.New("task callback is invalid")

// TaskCallbackDelivery is a durable outbox entry for one task terminal event.
// The API token is resolved by TokenId at delivery time and is never copied here.
type TaskCallbackDelivery struct {
	Id              int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	TaskId          int64  `json:"task_id" gorm:"uniqueIndex;not null"`
	PublicTaskId    string `json:"public_task_id" gorm:"type:varchar(191);uniqueIndex;not null"`
	UserId          int    `json:"user_id" gorm:"index;not null"`
	TokenId         int    `json:"token_id" gorm:"index;not null"`
	CallbackURL     string `json:"-" gorm:"type:text;not null"`
	Attempts        int    `json:"attempts" gorm:"not null;default:0"`
	LastError       string `json:"last_error" gorm:"type:text"`
	NextAttemptTime int64  `json:"next_attempt_time" gorm:"bigint;not null;index"`
	LockedUntil     int64  `json:"locked_until" gorm:"bigint;not null;default:0;index"`
	DeliveredTime   int64  `json:"delivered_time" gorm:"bigint;not null;default:0;index"`
	DeadLetterTime  int64  `json:"dead_letter_time" gorm:"bigint;not null;default:0;index"`
	CreatedTime     int64  `json:"created_time" gorm:"bigint;autoCreateTime;index"`
	UpdatedTime     int64  `json:"updated_time" gorm:"bigint;autoUpdateTime"`
}

func EnqueueTaskCallbackTx(tx *gorm.DB, task *Task, callbackURL string) (*TaskCallbackDelivery, error) {
	if tx == nil || task == nil || task.ID <= 0 || strings.TrimSpace(task.TaskID) == "" || task.PrivateData.TokenId <= 0 || strings.TrimSpace(callbackURL) == "" {
		return nil, ErrTaskCallbackInvalid
	}
	now := common.GetTimestamp()
	delivery := &TaskCallbackDelivery{
		TaskId:          task.ID,
		PublicTaskId:    task.TaskID,
		UserId:          task.UserId,
		TokenId:         task.PrivateData.TokenId,
		CallbackURL:     strings.TrimSpace(callbackURL),
		NextAttemptTime: now,
		CreatedTime:     now,
		UpdatedTime:     now,
	}
	if err := tx.Create(delivery).Error; err != nil {
		return nil, err
	}
	return delivery, nil
}

func GetTaskCallbackByPublicTaskId(taskID string) (*TaskCallbackDelivery, error) {
	delivery := &TaskCallbackDelivery{}
	err := DB.Where("public_task_id = ?", strings.TrimSpace(taskID)).First(delivery).Error
	return delivery, err
}

func ListDueTaskCallbacks(limit int, now int64) ([]*TaskCallbackDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows := []*TaskCallbackDelivery{}
	err := DB.Where("delivered_time = 0 AND dead_letter_time = 0 AND next_attempt_time <= ? AND locked_until <= ?", now, now).
		Order("id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func ClaimTaskCallback(id int64, now int64, lockedUntil int64) (bool, error) {
	result := DB.Model(&TaskCallbackDelivery{}).
		Where("id = ? AND delivered_time = 0 AND dead_letter_time = 0 AND next_attempt_time <= ? AND locked_until <= ?", id, now, now).
		Updates(map[string]any{"locked_until": lockedUntil, "updated_time": now})
	return result.RowsAffected == 1, result.Error
}

func DeferTaskCallback(id int64, nextAttemptTime int64) error {
	return DB.Model(&TaskCallbackDelivery{}).
		Where("id = ? AND delivered_time = 0 AND dead_letter_time = 0", id).
		Updates(map[string]any{"next_attempt_time": nextAttemptTime, "locked_until": int64(0), "updated_time": common.GetTimestamp()}).Error
}

func CompleteTaskCallback(id int64) error {
	now := common.GetTimestamp()
	return DB.Model(&TaskCallbackDelivery{}).
		Where("id = ? AND delivered_time = 0 AND dead_letter_time = 0", id).
		Updates(map[string]any{
			"last_error":        "",
			"next_attempt_time": now,
			"locked_until":      int64(0),
			"delivered_time":    now,
			"updated_time":      now,
		}).Error
}

func RecordTaskCallbackFailure(id int64, message string, nextAttemptTime int64) error {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		delivery := &TaskCallbackDelivery{}
		if err := lockForUpdate(tx).
			Where("id = ? AND delivered_time = 0 AND dead_letter_time = 0", id).
			First(delivery).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		updates := map[string]any{
			"attempts":          delivery.Attempts + 1,
			"last_error":        message,
			"next_attempt_time": nextAttemptTime,
			"locked_until":      int64(0),
			"updated_time":      now,
		}
		if delivery.Attempts+1 >= TaskCallbackMaxAttempts {
			updates["dead_letter_time"] = now
			updates["next_attempt_time"] = int64(0)
		}
		return tx.Model(delivery).Updates(updates).Error
	})
}

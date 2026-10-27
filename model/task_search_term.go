package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm/clause"
)

// Task search terms index the IDs a task log can be searched by besides our
// own task_id, the same way invoice_search_terms indexes invoice searches. The
// model platform's official task ID is computed by the task plugin from the
// upstream snapshot, so it cannot be an ordinary column lookup.
const (
	// TaskSearchTermKindOfficial is the official task ID the plugin publishes;
	// every viewer of the task sees it.
	TaskSearchTermKindOfficial = "official"
	// TaskSearchTermKindRootOfficial is the best-effort ID root sees when the
	// plugin publishes none.
	TaskSearchTermKindRootOfficial = "root_official"
	// TaskSearchTermKindUpstream is the upstream task ID, shown to root only.
	TaskSearchTermKindUpstream = "upstream"

	TaskSearchTermsBackfillCursorOptionKey  = "TaskSearchTermsBackfillCursor"
	TaskSearchTermsBackfillDoneOptionKey    = "TaskSearchTermsBackfillDone"
	TaskSearchTermsBackfillVersionOptionKey = "TaskSearchTermsBackfillVersion"

	// taskSearchTermsBackfillVersion is raised when the index starts holding
	// IDs it skipped before; a lower stored version re-runs the backfill from
	// the first task. Version 2 indexes IDs longer than the column as hashes.
	taskSearchTermsBackfillVersion = "2"
	taskSearchTermHashPrefix       = "sha256:"

	taskSearchTermMaxLength      = 191
	taskSearchTermBackfillBatch  = 200
	taskSearchTermBackfillPause  = 200 * time.Millisecond
	taskSearchTermLookupMaxTasks = 200
)

type TaskSearchTerm struct {
	Id        int64  `json:"id"`
	TaskRowId int64  `json:"task_row_id" gorm:"not null;index;uniqueIndex:idx_task_search_term"`
	Kind      string `json:"kind" gorm:"type:varchar(16);not null;uniqueIndex:idx_task_search_term"`
	Value     string `json:"value" gorm:"type:varchar(191);not null;index;uniqueIndex:idx_task_search_term"`
}

// TaskSearchTermResolver returns the searchable IDs of a task. The relay
// package sets it because official IDs come from task plugins.
var TaskSearchTermResolver func(task *Task) []TaskSearchTerm

// taskSearchTermsSynced remembers the terms last written for an unfinished
// task so repeated polls do not rewrite them.
var taskSearchTermsSynced sync.Map

func taskSearchTermsKey(terms []TaskSearchTerm) string {
	var builder strings.Builder
	for _, term := range terms {
		builder.WriteString(term.Kind)
		builder.WriteByte('=')
		builder.WriteString(term.Value)
		builder.WriteByte(';')
	}
	return builder.String()
}

// taskSearchTermValue is how an ID is stored and looked up. IDs longer than
// the column, such as Vertex operation names, are kept as their SHA-256 so
// they can still be matched exactly.
func taskSearchTermValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= taskSearchTermMaxLength {
		return value
	}
	sum := sha256.Sum256([]byte(value))
	return taskSearchTermHashPrefix + hex.EncodeToString(sum[:])
}

func resolveTaskSearchTerms(task *Task) []TaskSearchTerm {
	if TaskSearchTermResolver == nil || task == nil || task.ID <= 0 {
		return nil
	}
	terms := []TaskSearchTerm{}
	for _, term := range TaskSearchTermResolver(task) {
		term.Value = taskSearchTermValue(term.Value)
		if term.Value == "" {
			continue
		}
		term.TaskRowId = task.ID
		terms = append(terms, term)
	}
	return terms
}

func insertTaskSearchTerms(terms []TaskSearchTerm) error {
	if len(terms) == 0 {
		return nil
	}
	return DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&terms).Error
}

// SyncTaskSearchTerms records the task's searchable IDs after it is written.
// Failures are logged only: search terms must never fail a task write.
func SyncTaskSearchTerms(task *Task) {
	terms := resolveTaskSearchTerms(task)
	finished := task != nil && (task.Status == TaskStatusSuccess || task.Status == TaskStatusFailure)
	if len(terms) == 0 {
		if finished {
			taskSearchTermsSynced.Delete(task.ID)
		}
		return
	}
	key := taskSearchTermsKey(terms)
	if previous, ok := taskSearchTermsSynced.Load(task.ID); ok && previous == key {
		if finished {
			taskSearchTermsSynced.Delete(task.ID)
		}
		return
	}
	if err := insertTaskSearchTerms(terms); err != nil {
		common.SysError(common.LogText("task search terms write failed task_id=%s: %s", task.TaskID, err.Error()))
		return
	}
	if finished {
		taskSearchTermsSynced.Delete(task.ID)
		return
	}
	taskSearchTermsSynced.Store(task.ID, key)
}

// FindTaskIDsBySearchTerm returns the task rows whose indexed ID of one of the
// given kinds equals value.
func FindTaskIDsBySearchTerm(value string, kinds []string) ([]int64, error) {
	value = taskSearchTermValue(value)
	ids := []int64{}
	if value == "" || len(kinds) == 0 {
		return ids, nil
	}
	err := DB.Model(&TaskSearchTerm{}).
		Where("value = ? AND kind IN ?", value, kinds).
		Distinct("task_row_id").
		Limit(taskSearchTermLookupMaxTasks).
		Pluck("task_row_id", &ids).Error
	return ids, err
}

// TaskSearchTermsBackfilled reports whether every task that existed before
// search terms were introduced has been indexed.
func TaskSearchTermsBackfilled() bool {
	common.OptionMapRWMutex.RLock()
	done := common.OptionMap[TaskSearchTermsBackfillDoneOptionKey]
	common.OptionMapRWMutex.RUnlock()
	return done == "true"
}

func taskSearchTermsBackfillCursor() int64 {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[TaskSearchTermsBackfillCursorOptionKey]
	common.OptionMapRWMutex.RUnlock()
	cursor, _ := strconv.ParseInt(raw, 10, 64)
	return max(cursor, 0)
}

// BackfillTaskSearchTerms indexes tasks after the saved cursor in small
// batches. It runs on the master after task plugins are loaded, resumes where
// a previous run stopped, and marks the backfill done once it reaches the
// newest task. Later runs pick up tasks written by nodes that were still on
// an older version during a rolling deploy.
func BackfillTaskSearchTerms(ctx context.Context) error {
	if TaskSearchTermResolver == nil {
		return nil
	}
	common.OptionMapRWMutex.RLock()
	storedVersion := common.OptionMap[TaskSearchTermsBackfillVersionOptionKey]
	common.OptionMapRWMutex.RUnlock()
	if storedVersion != taskSearchTermsBackfillVersion {
		// Start over; searches fall back to payload matching until it finishes.
		for key, value := range map[string]string{
			TaskSearchTermsBackfillDoneOptionKey:   "false",
			TaskSearchTermsBackfillCursorOptionKey: "0",
		} {
			if err := UpdateOption(key, value); err != nil {
				return err
			}
		}
	}
	cursor := taskSearchTermsBackfillCursor()
	indexed := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var tasks []*Task
		if err := DB.Where("id > ?", cursor).Order("id").Limit(taskSearchTermBackfillBatch).Find(&tasks).Error; err != nil {
			return err
		}
		if len(tasks) == 0 {
			break
		}
		terms := []TaskSearchTerm{}
		for _, task := range tasks {
			terms = append(terms, resolveTaskSearchTerms(task)...)
		}
		if err := insertTaskSearchTerms(terms); err != nil {
			return err
		}
		indexed += len(tasks)
		cursor = tasks[len(tasks)-1].ID
		if err := UpdateOption(TaskSearchTermsBackfillCursorOptionKey, strconv.FormatInt(cursor, 10)); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(taskSearchTermBackfillPause):
		}
	}
	if !TaskSearchTermsBackfilled() {
		if err := UpdateOption(TaskSearchTermsBackfillDoneOptionKey, "true"); err != nil {
			return err
		}
	}
	if storedVersion != taskSearchTermsBackfillVersion {
		if err := UpdateOption(TaskSearchTermsBackfillVersionOptionKey, taskSearchTermsBackfillVersion); err != nil {
			return err
		}
	}
	if indexed > 0 {
		common.SysLog(common.LogText("task search terms backfilled %d tasks up to id %d", indexed, cursor))
	}
	return nil
}

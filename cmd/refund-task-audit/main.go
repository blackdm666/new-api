package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

type auditResult struct {
	ID            int64
	TaskID        string
	Status        model.TaskStatus
	Quota         int
	RefundLogSeen bool
	Action        string
}

func main() {
	var idsCSV string
	var apply bool
	flag.StringVar(&idsCSV, "ids", "", "comma-separated task database IDs")
	flag.BoolVar(&apply, "apply", false, "refund eligible failed tasks; default is dry-run")
	flag.Parse()

	ids, err := parseIDs(idsCSV)
	if err != nil {
		fatal(err)
	}
	if len(ids) == 0 {
		fatal(fmt.Errorf("at least one task ID is required"))
	}

	// A maintenance command must never run schema migrations.
	_ = os.Setenv("NODE_TYPE", "slave")
	common.InitEnv()
	logger.SetupLogger()
	if err := model.InitDB(); err != nil {
		fatal(fmt.Errorf("init database: %w", err))
	}
	defer func() { _ = model.CloseDB() }()
	if err := model.InitLogDB(); err != nil {
		fatal(fmt.Errorf("init log database: %w", err))
	}
	if err := common.InitRedisClient(); err != nil {
		fatal(fmt.Errorf("init redis: %w", err))
	}

	var tasks []*model.Task
	if err := model.DB.Where("id IN ?", ids).Order("id ASC").Find(&tasks).Error; err != nil {
		fatal(fmt.Errorf("load tasks: %w", err))
	}
	if len(tasks) != len(ids) {
		fatal(fmt.Errorf("requested %d tasks but loaded %d", len(ids), len(tasks)))
	}

	ctx := context.Background()
	results := make([]auditResult, 0, len(tasks))
	for _, task := range tasks {
		seen, err := hasRefundLog(task.TaskID)
		if err != nil {
			fatal(fmt.Errorf("check refund log for task %d: %w", task.ID, err))
		}
		result := auditResult{
			ID:            task.ID,
			TaskID:        task.TaskID,
			Status:        task.Status,
			Quota:         task.Quota,
			RefundLogSeen: seen,
			Action:        "skip",
		}

		eligible := task.Status == model.TaskStatusFailure && task.Quota > 0 && !seen
		if eligible && !apply {
			result.Action = "would_refund"
		}
		if eligible && apply {
			if !service.RefundTaskQuota(ctx, task, "historical Vertex failure refund audit") {
				fatal(fmt.Errorf("refund failed for task %d (%s)", task.ID, task.TaskID))
			}
			var persistedQuota int
			if err := model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Pluck("quota", &persistedQuota).Error; err != nil {
				fatal(fmt.Errorf("read back task %d quota: %w", task.ID, err))
			}
			seen, err = hasRefundLog(task.TaskID)
			if err != nil {
				fatal(fmt.Errorf("read back task %d refund log: %w", task.ID, err))
			}
			if persistedQuota != 0 || !seen {
				fatal(fmt.Errorf("refund verification failed for task %d: quota=%d refund_log=%t", task.ID, persistedQuota, seen))
			}
			result.Quota = persistedQuota
			result.RefundLogSeen = seen
			result.Action = "refunded"
		}
		results = append(results, result)
	}

	mode := "dry-run"
	if apply {
		mode = "apply"
	}
	fmt.Printf("mode=%s tasks=%d\n", mode, len(results))
	for _, result := range results {
		fmt.Printf("id=%d task_id=%s status=%s quota=%d refund_log=%t action=%s\n",
			result.ID, result.TaskID, result.Status, result.Quota, result.RefundLogSeen, result.Action)
	}
}

func parseIDs(raw string) ([]int64, error) {
	seen := make(map[int64]struct{})
	ids := make([]int64, 0)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var id int64
		if _, err := fmt.Sscanf(part, "%d", &id); err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid task ID %q", part)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func hasRefundLog(taskID string) (bool, error) {
	var count int64
	needle := fmt.Sprintf("%%\"task_id\":\"%s\"%%", taskID)
	err := model.LOG_DB.Model(&model.Log{}).
		Where("type = ? AND other LIKE ?", model.LogTypeRefund, needle).
		Count(&count).Error
	return count > 0, err
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

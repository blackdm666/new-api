package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	defaultTaskVideoTransferConcurrency = 32
	maxTaskVideoTransferConcurrency     = 256
	defaultTaskVideoTransferPerHost     = 8
)

// TaskVideoTransferAsyncEnabled reports whether finished videos are copied to
// object storage by the background transfer pool instead of inline in the
// poller. Turning it off restores the synchronous path unchanged.
func TaskVideoTransferAsyncEnabled() bool {
	return common.GetEnvOrDefaultBool("TASK_VIDEO_TRANSFER_ASYNC", false)
}

// TaskVideoTransferConcurrency bounds how many result transfers run at once on
// this node.
func TaskVideoTransferConcurrency() int {
	limit := common.GetEnvOrDefault("TASK_VIDEO_TRANSFER_CONCURRENCY", defaultTaskVideoTransferConcurrency)
	if limit < 1 || limit > maxTaskVideoTransferConcurrency {
		return defaultTaskVideoTransferConcurrency
	}
	return limit
}

// TaskVideoTransferPerHost bounds the transfers one source host may hold, so a
// slow or broken upstream cannot occupy the whole pool.
func TaskVideoTransferPerHost() int {
	limit := common.GetEnvOrDefault("TASK_VIDEO_TRANSFER_PER_HOST", defaultTaskVideoTransferPerHost)
	if limit < 1 || limit > maxTaskVideoTransferConcurrency {
		limit = defaultTaskVideoTransferPerHost
	}
	return min(limit, TaskVideoTransferConcurrency())
}

// TaskVideoTransferStats is a point-in-time view of the transfer pool on this
// node. Counters are cumulative since process start.
type TaskVideoTransferStats struct {
	InFlight      int   `json:"in_flight"`
	OldestSeconds int64 `json:"oldest_seconds"`
	DeferredFull  int64 `json:"deferred_full"`
	Succeeded     int64 `json:"succeeded"`
	Failed        int64 `json:"failed"`
}

type taskVideoTransfer struct {
	host      string
	startedAt time.Time
}

// taskVideoTransfers registers every transfer in flight on this node. While a
// task is registered the pool owns it: nothing else may write the task row,
// because UpdateWithStatus rewrites the whole row and would race the pool's
// terminal write. The registry is deliberately in memory: the task row is never
// written during a transfer, so after a restart the next poll rediscovers the
// finished upstream task and submits it again, and a copy that already reached
// storage is reused (see reuseCachedTaskVideo).
var taskVideoTransfers = struct {
	sync.Mutex
	inFlight     map[string]taskVideoTransfer
	perHost      map[string]int
	deferredFull int64
	succeeded    int64
	failed       int64
}{
	inFlight: make(map[string]taskVideoTransfer),
	perHost:  make(map[string]int),
}

func taskVideoTransferInFlight(taskID string) bool {
	taskVideoTransfers.Lock()
	defer taskVideoTransfers.Unlock()
	_, ok := taskVideoTransfers.inFlight[taskID]
	return ok
}

// taskAwaitingVideoTransfer reports whether the poller should leave a task
// alone this pass: its result is being transferred, or the last transfer
// failed and the task is backing off.
func taskAwaitingVideoTransfer(taskID string, now time.Time) bool {
	if taskVideoTransferInFlight(taskID) {
		return true
	}
	_, deferred := taskVideoPrepareRetryAt(taskID, now)
	return deferred
}

func TaskVideoTransferStatsSnapshot() TaskVideoTransferStats {
	taskVideoTransfers.Lock()
	defer taskVideoTransfers.Unlock()
	stats := TaskVideoTransferStats{
		InFlight:     len(taskVideoTransfers.inFlight),
		DeferredFull: taskVideoTransfers.deferredFull,
		Succeeded:    taskVideoTransfers.succeeded,
		Failed:       taskVideoTransfers.failed,
	}
	now := time.Now()
	for _, transfer := range taskVideoTransfers.inFlight {
		stats.OldestSeconds = max(stats.OldestSeconds, int64(now.Sub(transfer.startedAt).Seconds()))
	}
	return stats
}

// submitTaskVideoTransfer hands a task whose upstream generation succeeded to
// the transfer pool. The pool takes ownership of task and taskResult; the
// caller must not use them afterwards. snap is the task state last persisted,
// which the terminal write compares against. A task is not admitted when it is
// already in flight or the pool or its source host is full; it then stays
// unfinished and the next poll submits it again with a fresh result URL.
func submitTaskVideoTransfer(adaptor TaskPollingAdaptor, task *model.Task, taskResult *relaycommon.TaskInfo, snap model.TaskSnapshot) bool {
	host := "data"
	resultURL := ResolveTaskVideoResultURL(task, taskResult.Url)
	if !strings.HasPrefix(resultURL, "data:") {
		host = taskVideoResultHost(resultURL)
	}
	concurrency := TaskVideoTransferConcurrency()
	perHost := TaskVideoTransferPerHost()

	taskVideoTransfers.Lock()
	if _, ok := taskVideoTransfers.inFlight[task.TaskID]; ok {
		taskVideoTransfers.Unlock()
		return false
	}
	if len(taskVideoTransfers.inFlight) >= concurrency || taskVideoTransfers.perHost[host] >= perHost {
		taskVideoTransfers.deferredFull++
		taskVideoTransfers.Unlock()
		return false
	}
	taskVideoTransfers.inFlight[task.TaskID] = taskVideoTransfer{host: host, startedAt: time.Now()}
	taskVideoTransfers.perHost[host]++
	inFlight := len(taskVideoTransfers.inFlight)
	taskVideoTransfers.Unlock()

	common.SysLog(fmt.Sprintf("video transfer queued task=%s host=%s in_flight=%d/%d", task.TaskID, host, inFlight, concurrency))
	gopool.Go(func() {
		runTaskVideoTransfer(adaptor, task, taskResult, snap, host)
	})
	return true
}

// runTaskVideoTransfer makes one finished video deliverable and, only if that
// succeeds, writes the terminal state through the same CAS and settlement path
// as the synchronous poller. It runs detached from the polling pass, whose
// context ends with the pass.
func runTaskVideoTransfer(adaptor TaskPollingAdaptor, task *model.Task, taskResult *relaycommon.TaskInfo, snap model.TaskSnapshot, host string) {
	succeeded := false
	defer func() {
		taskVideoTransfers.Lock()
		delete(taskVideoTransfers.inFlight, task.TaskID)
		if taskVideoTransfers.perHost[host] <= 1 {
			delete(taskVideoTransfers.perHost, host)
		} else {
			taskVideoTransfers.perHost[host]--
		}
		if succeeded {
			taskVideoTransfers.succeeded++
		} else {
			taskVideoTransfers.failed++
		}
		taskVideoTransfers.Unlock()
	}()

	ctx := context.Background()
	started := time.Now()
	prepareCtx, cancel := context.WithTimeout(ctx, taskVideoPrepareTimeout())
	preparation, err := PrepareTaskVideoResult(prepareCtx, task, taskResult.Url)
	cancel()
	if err != nil {
		retry := recordTaskVideoPrepareFailure(task.TaskID, time.Now())
		logger.LogWarn(ctx, fmt.Sprintf("video transfer failed task=%s host=%s took=%s attempt=%d next_retry=%s: %v",
			task.TaskID, host, time.Since(started).Round(time.Millisecond), retry.failures, retry.retryAt.Format(time.DateTime), err))
		return
	}
	clearTaskVideoPrepareFailure(task.TaskID)
	succeeded = true
	result := "cached"
	if preparation.DirectURL != "" {
		result = "direct"
	}
	common.SysLog(fmt.Sprintf("video transfer finished task=%s host=%s took=%s result=%s",
		task.TaskID, host, time.Since(started).Round(time.Millisecond), result))
	completeVideoTaskResult(ctx, adaptor, task, taskResult, snap, preparation)
}

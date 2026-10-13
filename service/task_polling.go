package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"
)

// TaskPollingAdaptor 定义轮询所需的最小适配器接口，避免 service -> relay 的循环依赖
type TaskPollingAdaptor interface {
	Init(info *relaycommon.RelayInfo)
	FetchTask(baseURL string, key string, task *model.Task, proxy string) (*http.Response, error)
	ParseTaskResult(task *model.Task, resp *http.Response, body []byte) (*relaycommon.TaskInfo, error)
	// AdjustBillingOnComplete 在任务到达终态（成功/失败）时由轮询循环调用。
	// 返回正数触发差额结算（补扣/退还），返回 0 保持预扣费金额不变。
	AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int
}

type BatchTaskPollingAdaptor interface {
	TaskPollingAdaptor
	FetchMode() string
	FetchBatchTasks(baseURL, key string, tasks []*model.Task, proxy string) (*http.Response, error)
	ParseBatchResult(tasks []*model.Task, resp *http.Response, body []byte) (map[string]*BatchTaskResult, error)
}

const (
	pollClassOK           = "ok"
	pollClassOtherClient  = "other_client"
	pollClassNotFound     = "not_found"
	pollClassAuth         = "auth"
	pollClassTransient    = "transient"
	pollClassUnrecognized = "unrecognized"
	pollClassHookError    = "hook_error"
	pollClassTransport    = "transport_error"
)

type BatchTaskResult struct {
	TaskInfo   relaycommon.TaskInfo
	Action     string
	SubmitTime int64
	StartTime  int64
	FinishTime int64
	Data       any
}

// GetTaskAdaptorFunc 由 main 包注入，用于获取指定平台的任务适配器。
// 打破 service -> relay -> relay/channel -> service 的循环依赖。
var GetTaskAdaptorFunc func(platform constant.TaskPlatform) TaskPollingAdaptor

func recordTaskTerminalSampleAsync(task *model.Task) {
	if task == nil {
		return
	}
	snapshot := *task
	gopool.Go(func() {
		perfmetrics.RecordTaskResult(&snapshot, nil)
	})
}

// sweepTimedOutTasks 在主轮询之前独立清理超时任务。
// 每次最多处理 100 条，剩余的下个周期继续处理。
// 使用 per-task CAS (UpdateWithStatus) 防止覆盖被正常轮询已推进的任务。
func sweepTimedOutTasks(ctx context.Context) {
	if constant.TaskTimeoutMinutes <= 0 {
		return
	}
	cutoff := time.Now().Unix() - int64(constant.TaskTimeoutMinutes)*60
	tasks := model.GetTimedOutUnfinishedTasks(cutoff, 100)
	if len(tasks) == 0 {
		return
	}

	reason := fmt.Sprintf("任务超时（%d分钟）", constant.TaskTimeoutMinutes)
	legacyReason := "任务超时（旧系统遗留任务，不进行退款，请联系管理员）"
	now := time.Now().Unix()
	timedOutCount := 0

	for _, task := range tasks {
		isLegacy := task.SubmitTime > 0 && task.SubmitTime < model.TaskRefundLegacyCutoff

		oldStatus := task.Status
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		task.FinishTime = now
		if isLegacy {
			task.FailReason = legacyReason
			// 旧系统任务明确不退款，随终态 CAS 一并清掉 quota，
			// 避免留下可再次退款的计费状态。
			task.Quota = 0
		} else {
			task.FailReason = reason
		}

		won, err := task.UpdateWithStatus(oldStatus)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("sweepTimedOutTasks CAS update error for task %s: %v", task.TaskID, err))
			continue
		}
		if !won {
			logger.LogInfo(ctx, fmt.Sprintf("sweepTimedOutTasks: task %s already transitioned, skip", task.TaskID))
			continue
		}
		timedOutCount++
		recordTaskTerminalSampleAsync(task)
		if !isLegacy && task.Quota != 0 {
			RefundTaskQuota(ctx, task, reason)
		}
		if task.PrivateData.CallbackEnabled {
			ScheduleTaskCallback(task.TaskID)
		}
	}

	if timedOutCount > 0 {
		logger.LogInfo(ctx, fmt.Sprintf("sweepTimedOutTasks: timed out %d tasks", timedOutCount))
	}
}

const maxTaskPollChannelConcurrency = 32

// TaskPollSummary is the result recorded on an async_task_poll system task row,
// summarizing one polling pass.
type TaskPollSummary struct {
	UnfinishedTasks  int `json:"unfinished_tasks"`
	PlatformsScanned int `json:"platforms_scanned"`
	NullTasksFailed  int `json:"null_tasks_failed"`
	// Video transfer pool state at the end of the pass. TransferDeferredFull
	// counts finished tasks this pass could not hand over because the pool or
	// their source host was full.
	TransferInFlight      int   `json:"transfer_in_flight"`
	TransferOldestSeconds int64 `json:"transfer_oldest_seconds"`
	TransferDeferredFull  int64 `json:"transfer_deferred_full"`
}

// RunTaskPollingOnce performs one async-task (Suno/video) polling pass
// synchronously. It honors ctx cancellation (the system-task runner cancels it
// when the lease is lost) and, when report is non-nil, reports progress as
// (processedPlatforms, totalPlatforms). It returns immediately if the task
// adaptor factory has not been wired yet, to avoid a nil call during startup.
func RunTaskPollingOnce(ctx context.Context, report func(processed, total int)) TaskPollSummary {
	summary := TaskPollSummary{}
	if GetTaskAdaptorFunc == nil {
		return summary
	}
	if ctx == nil {
		ctx = context.Background()
	}

	common.SysLog("任务进度轮询开始")
	deferredBefore := TaskVideoTransferStatsSnapshot().DeferredFull
	sweepTimedOutTasks(ctx)
	allTasks := model.GetAllUnFinishSyncTasks(constant.TaskQueryLimit)
	summary.UnfinishedTasks = len(allTasks)
	platformTask := make(map[constant.TaskPlatform][]*model.Task)
	for _, t := range allTasks {
		platformTask[t.Platform] = append(platformTask[t.Platform], t)
	}

	totalPlatforms := len(platformTask)
	processedPlatforms := 0
	var reportMu sync.Mutex
	markPlatformProcessed := func() {
		reportMu.Lock()
		defer reportMu.Unlock()
		processedPlatforms++
		if report != nil && ctx.Err() == nil {
			report(processedPlatforms, totalPlatforms)
		}
	}
	if report != nil {
		report(0, totalPlatforms)
	}
	// Platforms are polled concurrently so a slow upstream or result transfer
	// on one platform cannot hold back status updates on every other platform.
	var wg sync.WaitGroup
	for platform, tasks := range platformTask {
		if ctx.Err() != nil {
			break
		}
		if len(tasks) == 0 {
			markPlatformProcessed()
			continue
		}
		summary.PlatformsScanned++
		taskChannelM := make(map[int][]string)
		taskM := make(map[string]*model.Task)
		nullTaskIds := make([]int64, 0)
		for _, task := range tasks {
			upstreamID := task.GetUpstreamTaskID()
			if upstreamID == "" {
				// 统计失败的未完成任务
				nullTaskIds = append(nullTaskIds, task.ID)
				continue
			}
			taskM[upstreamID] = task
			taskChannelM[task.ChannelId] = append(taskChannelM[task.ChannelId], upstreamID)
		}
		if len(nullTaskIds) > 0 {
			summary.NullTasksFailed += len(nullTaskIds)
			err := model.TaskBulkUpdateByID(nullTaskIds, map[string]any{
				"status":   "FAILURE",
				"progress": "100%",
			})
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Fix null task_id task error: %v", err))
			} else {
				logger.LogInfo(ctx, fmt.Sprintf("Fix null task_id task success: %v", nullTaskIds))
			}
		}
		if len(taskChannelM) == 0 {
			markPlatformProcessed()
			continue
		}

		wg.Add(1)
		gopool.Go(func() {
			defer wg.Done()
			defer markPlatformProcessed()
			DispatchPlatformUpdate(ctx, platform, taskChannelM, taskM)
		})
	}
	wg.Wait()
	if report != nil && ctx.Err() == nil {
		report(totalPlatforms, totalPlatforms)
	}
	transfers := TaskVideoTransferStatsSnapshot()
	summary.TransferInFlight = transfers.InFlight
	summary.TransferOldestSeconds = transfers.OldestSeconds
	summary.TransferDeferredFull = transfers.DeferredFull - deferredBefore
	if summary.TransferDeferredFull > 0 {
		common.SysLog(fmt.Sprintf("video transfer pool full: %d finished tasks wait for the next pass (in_flight=%d)", summary.TransferDeferredFull, transfers.InFlight))
	}
	common.SysLog("任务进度轮询完成")
	return summary
}

// DispatchPlatformUpdate 按平台分发轮询更新
func DispatchPlatformUpdate(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) {
	if ctx == nil {
		ctx = context.Background()
	}
	if platform == constant.TaskPlatformMidjourney {
		// MJ 轮询由其自身处理，这里预留入口
		return
	}
	adaptor := GetTaskAdaptorFunc(platform)
	if batchAdaptor, ok := adaptor.(BatchTaskPollingAdaptor); ok && batchAdaptor.FetchMode() == "batch" {
		if err := UpdateBatchTasks(ctx, batchAdaptor, taskChannelM, taskM); err != nil {
			common.SysLog(fmt.Sprintf("UpdateBatchTasks fail: %s", err))
		}
		return
	}
	if err := UpdateVideoTasks(ctx, platform, taskChannelM, taskM); err != nil {
		common.SysLog(fmt.Sprintf("UpdateVideoTasks fail: %s", err))
	}
}

func UpdateBatchTasks(ctx context.Context, adaptor BatchTaskPollingAdaptor, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	for channelId, taskIds := range taskChannelM {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := updateBatchTasks(ctx, adaptor, channelId, taskIds, taskM)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("渠道 #%d 更新异步任务失败: %s", channelId, err.Error()))
		}
	}
	return nil
}

func updateBatchTasks(ctx context.Context, adaptor BatchTaskPollingAdaptor, channelId int, taskIds []string, taskM map[string]*model.Task) error {
	logger.LogInfo(ctx, fmt.Sprintf("渠道 #%d 未完成的任务有: %d", channelId, len(taskIds)))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(taskIds) == 0 {
		return nil
	}
	ch, err := model.CacheGetChannel(channelId)
	if err != nil {
		common.SysLog(fmt.Sprintf("CacheGetChannel: %v", err))
		// Collect DB primary key IDs for bulk update (taskIds are upstream IDs, not task_id column values)
		var failedIDs []int64
		for _, upstreamID := range taskIds {
			if t, ok := taskM[upstreamID]; ok {
				failedIDs = append(failedIDs, t.ID)
			}
		}
		err = model.TaskBulkUpdateByID(failedIDs, map[string]any{
			"fail_reason": fmt.Sprintf("获取渠道信息失败，请联系管理员，渠道ID：%d", channelId),
			"status":      "FAILURE",
			"progress":    "100%",
		})
		if err != nil {
			common.SysLog(fmt.Sprintf("UpdateSunoTask error: %v", err))
		}
		return err
	}
	proxy := ch.GetSetting().Proxy
	baseURL := ch.GetBaseURL()
	if baseURL == "" {
		baseURL = constant.GetChannelBaseURL(ch.Type)
	}
	tasks := make([]*model.Task, 0, len(taskIds))
	for _, upstreamID := range taskIds {
		if task := taskM[upstreamID]; task != nil {
			tasks = append(tasks, task)
		}
	}
	// The channel type tells plugin adaptors whether the upstream is another
	// New API gateway, the same signal submission derives from the request.
	info := &relaycommon.RelayInfo{}
	info.ChannelMeta = &relaycommon.ChannelMeta{ChannelType: ch.Type, ChannelId: ch.Id, ChannelBaseUrl: baseURL}
	info.ApiKey = ch.Key
	adaptor.Init(info)
	resp, err := adaptor.FetchBatchTasks(baseURL, ch.Key, tasks, proxy)
	if err != nil {
		common.SysLog(fmt.Sprintf("Get Task Do req error: %v", err))
		return recordPollFailureForTasks(ctx, adaptor, tasks, pollClassTransport, 0, err.Error())
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		common.SysLog(fmt.Sprintf("Get Suno Task parse body error: %v", err))
		return recordPollFailureForTasks(ctx, adaptor, tasks, pollClassTransport, resp.StatusCode, err.Error())
	}
	switch classifyPollHTTP(resp.StatusCode) {
	case pollClassNotFound:
		return failTasksFromPoll(ctx, adaptor, tasks, fmt.Sprintf("upstream task not found (HTTP %d)", resp.StatusCode))
	case pollClassAuth:
		logger.LogWarn(ctx, fmt.Sprintf("task poll auth failure channel_id=%d http=%d", channelId, resp.StatusCode))
		return recordPollFailureForTasks(ctx, adaptor, tasks, pollClassAuth, resp.StatusCode, "")
	case pollClassTransient:
		return recordPollFailureForTasks(ctx, adaptor, tasks, pollClassTransient, resp.StatusCode, "")
	}
	responseItems, err := adaptor.ParseBatchResult(tasks, resp, responseBody)
	if err != nil {
		return recordPollFailureForTasks(ctx, adaptor, tasks, pollClassHookError, resp.StatusCode, err.Error())
	}
	for upstreamID, responseItem := range responseItems {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		task := taskM[upstreamID]
		if task == nil {
			logger.LogWarn(ctx, fmt.Sprintf("Batch task response ignored: unknown task_id=%s", upstreamID))
			continue
		}
		snap := task.Snapshot()
		httpClass := classifyPollHTTP(resp.StatusCode)
		parsedStatus := model.TaskStatus(responseItem.TaskInfo.Status)
		if parsedStatus == model.TaskStatusUnknown || parsedStatus == "" || !knownPollStatus(parsedStatus) {
			if err := recordPollFailure(ctx, adaptor, task, snap.Status, pollClassUnrecognized, resp.StatusCode, responseItem.TaskInfo.Reason); err != nil {
				common.SysLog("UpdateSunoTask task error: " + err.Error())
			}
			continue
		}
		if httpClass == pollClassOtherClient && isNonTerminalPollStatus(parsedStatus) {
			if err := recordPollFailure(ctx, adaptor, task, snap.Status, pollClassUnrecognized, resp.StatusCode, responseItem.TaskInfo.Reason); err != nil {
				common.SysLog("UpdateSunoTask task error: " + err.Error())
			}
			continue
		}
		if isNonTerminalPollStatus(parsedStatus) {
			task.PrivateData.PollFailures = 0
		}
		if len(responseItem.TaskInfo.PluginState) > 0 {
			task.PrivateData.PluginState = responseItem.TaskInfo.PluginState
		}
		task.Status = lo.If(parsedStatus != "", parsedStatus).Else(task.Status)
		task.FailReason = lo.If(responseItem.TaskInfo.Reason != "", responseItem.TaskInfo.Reason).Else(task.FailReason)
		task.SubmitTime = lo.If(responseItem.SubmitTime != 0, responseItem.SubmitTime).Else(task.SubmitTime)
		task.StartTime = lo.If(responseItem.StartTime != 0, responseItem.StartTime).Else(task.StartTime)
		task.FinishTime = lo.If(responseItem.FinishTime != 0, responseItem.FinishTime).Else(task.FinishTime)
		if responseItem.TaskInfo.Progress != "" {
			task.Progress = responseItem.TaskInfo.Progress
		}
		if responseItem.TaskInfo.Reason != "" || task.Status == model.TaskStatusFailure {
			logger.LogInfo(ctx, task.TaskID+" 构建失败，"+task.FailReason)
			task.Status = model.TaskStatusFailure
			task.Progress = "100%"
		}
		if responseItem.TaskInfo.Status == model.TaskStatusSuccess {
			task.Progress = "100%"
		}
		if responseItem.Data != nil {
			task.SetData(responseItem.Data)
		} else if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
			logger.LogWarn(ctx, fmt.Sprintf(
				"Batch task %s reached terminal status without data; preserving existing task data",
				task.TaskID,
			))
		}
		if responseItem.TaskInfo.Url != "" {
			task.PrivateData.ResultURL = responseItem.TaskInfo.Url
		}

		isDone := task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure
		terminalTransition := isDone && snap.Status != task.Status
		won, updateErr := task.UpdateWithStatus(snap.Status)
		if updateErr != nil {
			common.SysLog("UpdateSunoTask task error: " + updateErr.Error())
			continue
		}
		if !won {
			logger.LogWarn(ctx, fmt.Sprintf("Batch task %s already transitioned by another process, skip billing", task.TaskID))
			continue
		}
		if terminalTransition {
			finalizeTerminalTask(ctx, adaptor, task, &responseItem.TaskInfo)
		}
	}
	return nil
}

// UpdateVideoTasks 按渠道更新所有视频任务
func UpdateVideoTasks(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	channelIDs := make([]int, 0, len(taskChannelM))
	for channelID := range taskChannelM {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Ints(channelIDs)

	var wg sync.WaitGroup
	for _, channelId := range channelIDs {
		taskIds := taskChannelM[channelId]
		if len(taskIds) == 0 {
			continue
		}
		taskIds = append([]string(nil), taskIds...)

		wg.Add(1)
		gopool.Go(func() {
			defer wg.Done()
			if err := updateVideoTasks(ctx, platform, channelId, taskIds, taskM); err != nil {
				logger.LogError(ctx, fmt.Sprintf("Channel #%d failed to update video async tasks: %s", channelId, err.Error()))
			}
		})
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func updateVideoTasks(ctx context.Context, platform constant.TaskPlatform, channelId int, taskIds []string, taskM map[string]*model.Task) error {
	logger.LogInfo(ctx, fmt.Sprintf("Channel #%d pending video tasks: %d", channelId, len(taskIds)))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(taskIds) == 0 {
		return nil
	}
	cacheGetChannel, err := model.CacheGetChannel(channelId)
	if err != nil {
		// Collect DB primary key IDs for bulk update (taskIds are upstream IDs, not task_id column values)
		var failedIDs []int64
		for _, upstreamID := range taskIds {
			if t, ok := taskM[upstreamID]; ok {
				failedIDs = append(failedIDs, t.ID)
			}
		}
		errUpdate := model.TaskBulkUpdateByID(failedIDs, map[string]any{
			"fail_reason": fmt.Sprintf("Failed to get channel info, channel ID: %d", channelId),
			"status":      "FAILURE",
			"progress":    "100%",
		})
		if errUpdate != nil {
			common.SysLog(fmt.Sprintf("UpdateVideoTask error: %v", errUpdate))
		}
		return fmt.Errorf("CacheGetChannel failed: %w", err)
	}
	if GetTaskAdaptorFunc(platform) == nil {
		return fmt.Errorf("video adaptor not found")
	}
	info := &relaycommon.RelayInfo{}
	info.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelType:    cacheGetChannel.Type,
		ChannelId:      cacheGetChannel.Id,
		ChannelBaseUrl: cacheGetChannel.GetBaseURL(),
	}
	info.ApiKey = cacheGetChannel.Key
	disablePollingSleep := cacheGetChannel.GetOtherSettings().DisableTaskPollingSleep

	// Tasks whose result is being transferred or backing off need no upstream
	// query this pass, and must not cost the channel a pacing second.
	now := time.Now()
	pending := make([]string, 0, len(taskIds))
	for _, taskId := range taskIds {
		if task := taskM[taskId]; task != nil && taskAwaitingVideoTransfer(task.TaskID, now) {
			continue
		}
		pending = append(pending, taskId)
	}

	// Each worker queries one task at a time and, unless the channel disables
	// it, waits one second before its next task, so a channel sees at most
	// TaskPollChannelConcurrency() queries per second.
	var next atomic.Int64
	var wg sync.WaitGroup
	for range min(TaskPollChannelConcurrency(), len(pending)) {
		adaptor := GetTaskAdaptorFunc(platform)
		adaptor.Init(info)
		wg.Add(1)
		gopool.Go(func() {
			defer wg.Done()
			for {
				index := int(next.Add(1)) - 1
				if index >= len(pending) || ctx.Err() != nil {
					return
				}
				taskId := pending[index]
				if err := updateVideoSingleTask(ctx, adaptor, cacheGetChannel, taskId, taskM); err != nil {
					logger.LogError(ctx, fmt.Sprintf("Failed to update video task %s: %s", taskId, err.Error()))
				}
				if disablePollingSleep || int(next.Load()) >= len(pending) {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(1 * time.Second):
				}
			}
		})
	}
	wg.Wait()
	return ctx.Err()
}

// TaskPollChannelConcurrency is how many tasks of one channel are queried at
// the same time. The default of 1 keeps the historical one-at-a-time pacing.
func TaskPollChannelConcurrency() int {
	limit := common.GetEnvOrDefault("TASK_POLL_CHANNEL_CONCURRENCY", 1)
	if limit < 1 || limit > maxTaskPollChannelConcurrency {
		return 1
	}
	return limit
}

func updateVideoSingleTask(ctx context.Context, adaptor TaskPollingAdaptor, ch *model.Channel, taskId string, taskM map[string]*model.Task) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	baseURL := constant.GetChannelBaseURL(ch.Type)
	if ch.GetBaseURL() != "" {
		baseURL = ch.GetBaseURL()
	}
	proxy := ch.GetSetting().Proxy

	task := taskM[taskId]
	if task == nil {
		logger.LogError(ctx, fmt.Sprintf("Task %s not found in taskM", taskId))
		return fmt.Errorf("task %s not found", taskId)
	}
	key := ch.Key

	privateData := task.PrivateData
	if privateData.Key != "" {
		key = privateData.Key
	}
	snap := task.Snapshot()
	resp, err := adaptor.FetchTask(baseURL, key, task, proxy)
	if err != nil {
		return recordPollFailure(ctx, adaptor, task, snap.Status, pollClassTransport, 0, err.Error())
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return recordPollFailure(ctx, adaptor, task, snap.Status, pollClassTransport, resp.StatusCode, err.Error())
	}

	logger.LogDebug(ctx, "updateVideoSingleTask response status=%d bytes=%d", resp.StatusCode, len(responseBody))

	switch classifyPollHTTP(resp.StatusCode) {
	case pollClassNotFound:
		return failTaskFromPoll(ctx, adaptor, task, snap.Status, fmt.Sprintf("upstream task not found (HTTP %d)", resp.StatusCode))
	case pollClassAuth:
		logger.LogWarn(ctx, fmt.Sprintf("task poll auth failure channel_id=%d task=%s http=%d", ch.Id, task.TaskID, resp.StatusCode))
		return recordPollFailure(ctx, adaptor, task, snap.Status, pollClassAuth, resp.StatusCode, "")
	case pollClassTransient:
		return recordPollFailure(ctx, adaptor, task, snap.Status, pollClassTransient, resp.StatusCode, "")
	}
	taskResult := &relaycommon.TaskInfo{}
	// try parse as New API response format
	var responseItems taskdto.TaskResponse[model.Task]
	if err = common.Unmarshal(responseBody, &responseItems); err == nil && responseItems.IsSuccess() {
		logger.LogDebug(ctx, "updateVideoSingleTask parsed as new api response format: %+v", responseItems)
		t := responseItems.Data
		taskResult.TaskID = t.TaskID
		taskResult.Status = string(t.Status)
		taskResult.Url = t.GetResultURL()
		taskResult.Progress = t.Progress
		taskResult.Reason = t.FailReason
		task.Data = t.Data
	} else if taskResult, err = adaptor.ParseTaskResult(task, resp, responseBody); err != nil {
		return recordPollFailure(ctx, adaptor, task, snap.Status, pollClassHookError, resp.StatusCode, err.Error())
	}
	parsedStatus := model.TaskStatus(taskResult.Status)
	if parsedStatus == model.TaskStatusUnknown || parsedStatus == "" || !knownPollStatus(parsedStatus) {
		return recordPollFailure(ctx, adaptor, task, snap.Status, pollClassUnrecognized, resp.StatusCode, unrecognizedPollDetail(taskResult.Reason, responseBody))
	}
	if classifyPollHTTP(resp.StatusCode) == pollClassOtherClient && isNonTerminalPollStatus(parsedStatus) {
		return recordPollFailure(ctx, adaptor, task, snap.Status, pollClassUnrecognized, resp.StatusCode, unrecognizedPollDetail(taskResult.Reason, responseBody))
	}

	logger.LogDebug(ctx, "updateVideoSingleTask parsed status=%s progress=%s", taskResult.Status, taskResult.Progress)
	return FinalizeVideoTaskResult(ctx, adaptor, task, taskResult, responseBody)
}

// FinalizeVideoTaskResult is the single terminal-state path for both the
// background poller and request-time Vertex/Gemini refreshes. The CAS winner
// persists the terminal state and is the only caller allowed to settle or
// refund quota.
func FinalizeVideoTaskResult(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, taskResult *relaycommon.TaskInfo, responseBody []byte) error {
	if task == nil || taskResult == nil {
		return errors.New("task and task result are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if taskVideoTransferInFlight(task.TaskID) {
		// The transfer pool owns this task until its terminal write.
		return nil
	}
	snap := task.Snapshot()
	parsedStatus := model.TaskStatus(taskResult.Status)

	task.Data = redactVideoResponseBody(responseBody)
	if len(taskResult.PluginState) > 0 {
		task.PrivateData.PluginState = taskResult.PluginState
	}
	if isNonTerminalPollStatus(parsedStatus) {
		task.PrivateData.PollFailures = 0
	}

	videoPreparation := TaskVideoPreparation{}
	if taskResult.Status == model.TaskStatusSuccess {
		if retryAt, deferred := taskVideoPrepareRetryAt(task.TaskID, time.Now()); deferred {
			logger.LogDebug(ctx, "video result for task %s is backing off until %s", task.TaskID, retryAt.Format(time.RFC3339))
			return nil
		}
		if TaskVideoTransferAsyncEnabled() {
			// The pool gets its own copy; the caller's task keeps the persisted
			// view so a response rendered from it cannot show the unpublished
			// upstream result.
			owned := *task
			task.Data = snap.Data
			task.PrivateData.PluginState = snap.PluginState
			task.PrivateData.PollFailures = snap.PollFailures
			submitTaskVideoTransfer(adaptor, &owned, taskResult, snap)
			return nil
		}
		prepareCtx, cancel := context.WithTimeout(ctx, taskVideoPrepareTimeout())
		var prepareErr error
		videoPreparation, prepareErr = PrepareTaskVideoResult(prepareCtx, task, taskResult.Url)
		cancel()
		if prepareErr != nil {
			retry := recordTaskVideoPrepareFailure(task.TaskID, time.Now())
			return fmt.Errorf("prepare video result for task %s (attempt %d, next retry %s): %w",
				task.TaskID, retry.failures, retry.retryAt.Format(time.DateTime), prepareErr)
		}
		clearTaskVideoPrepareFailure(task.TaskID)
	}
	completeVideoTaskResult(ctx, adaptor, task, taskResult, snap, videoPreparation)
	return nil
}

// completeVideoTaskResult applies an upstream result to the task and persists
// it. A success result must already be deliverable (videoPreparation). Terminal
// transitions go through the status CAS so that exactly one caller — poller,
// transfer pool, timeout sweep or cancellation — settles or refunds.
func completeVideoTaskResult(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, taskResult *relaycommon.TaskInfo, snap model.TaskSnapshot, videoPreparation TaskVideoPreparation) {
	parsedStatus := model.TaskStatus(taskResult.Status)
	now := time.Now().Unix()
	shouldFinalizeBilling := false
	task.Status = parsedStatus
	switch parsedStatus {
	case model.TaskStatusSubmitted:
		task.Progress = taskcommon.ProgressSubmitted
	case model.TaskStatusQueued:
		task.Progress = taskcommon.ProgressQueued
	case model.TaskStatusInProgress:
		task.Progress = taskcommon.ProgressInProgress
		if task.StartTime == 0 {
			task.StartTime = now
		}
	case model.TaskStatusSuccess:
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		if videoPreparation.Cached || videoPreparation.DirectURL != "" {
			// PrepareTaskVideoResult has installed either a private-cache proxy or
			// an anonymously readable direct result URL.
		} else if strings.HasPrefix(taskResult.Url, "data:") {
			// data: URI (e.g. Vertex base64 encoded video) — keep in Data, not in ResultURL
			task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
		} else if taskResult.Url != "" {
			// Direct upstream URL (e.g. Kling, Ali, Doubao, etc.)
			task.PrivateData.ResultURL = taskResult.Url
		} else {
			// No URL from adaptor — construct proxy URL using public task ID
			task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
		}
		shouldFinalizeBilling = true
	case model.TaskStatusFailure:
		logger.LogJson(ctx, fmt.Sprintf("Task %s failed", task.TaskID), task)
		task.Status = model.TaskStatusFailure
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = taskResult.Reason
		logger.LogInfo(ctx, fmt.Sprintf("Task %s failed: %s", task.TaskID, task.FailReason))
		taskResult.Progress = taskcommon.ProgressComplete
		shouldFinalizeBilling = true
	}
	if taskResult.Progress != "" {
		task.Progress = taskResult.Progress
	}

	isDone := task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure
	terminalTransitionWon := false
	if isDone && snap.Status != task.Status {
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("UpdateWithStatus failed for task %s: %s", task.TaskID, err.Error()))
			shouldFinalizeBilling = false
		} else if !won {
			logger.LogWarn(ctx, fmt.Sprintf("Task %s CAS lost or no-op update, skip billing", task.TaskID))
		} else {
			terminalTransitionWon = true
		}
	} else if !snap.Equal(task.Snapshot()) {
		if _, err := task.UpdateWithStatus(snap.Status); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to update task %s: %s", task.TaskID, err.Error()))
		}
	} else {
		// No changes, skip update
		logger.LogDebug(ctx, "No update needed for task %s", task.TaskID)
	}
	if shouldFinalizeBilling && terminalTransitionWon {
		finalizeTerminalTask(ctx, adaptor, task, taskResult)
	}
}

// finalizeTerminalTask 终态统一收尾（状态 CAS 赢家调用，恰好一次）：采样 + 结算 + 失败兜底退款。
func finalizeTerminalTask(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, taskResult *relaycommon.TaskInfo) {
	perfmetrics.RecordTaskResult(task, taskResult)
	billingSettled := settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)
	if task.Status == model.TaskStatusFailure && !billingSettled && task.Quota != 0 {
		RefundTaskQuota(ctx, task, task.FailReason)
	}
	if task.PrivateData.CallbackEnabled {
		ScheduleTaskCallback(task.TaskID)
	}
}

func redactVideoResponseBody(body []byte) []byte {
	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil {
		return body
	}
	resp, _ := m["response"].(map[string]any)
	if resp != nil {
		delete(resp, "bytesBase64Encoded")
		if v, ok := resp["video"].(string); ok {
			resp["video"] = truncateBase64(v)
		}
		if vs, ok := resp["videos"].([]any); ok {
			for i := range vs {
				if vm, ok := vs[i].(map[string]any); ok {
					delete(vm, "bytesBase64Encoded")
				}
			}
		}
	}
	for _, field := range []string{"steps", "outputs"} {
		items, _ := m[field].([]any)
		for _, item := range items {
			step, _ := item.(map[string]any)
			stepType, _ := step["type"].(string)
			stepMimeType, _ := step["mime_type"].(string)
			if stepType == "image" || stepType == "audio" || stepType == "video" ||
				strings.HasPrefix(stepMimeType, "image/") ||
				strings.HasPrefix(stepMimeType, "audio/") ||
				strings.HasPrefix(stepMimeType, "video/") {
				delete(step, "data")
			}
			contents, _ := step["content"].([]any)
			for _, content := range contents {
				part, _ := content.(map[string]any)
				partType, _ := part["type"].(string)
				mimeType, _ := part["mime_type"].(string)
				if partType == "image" || partType == "audio" || partType == "video" ||
					strings.HasPrefix(mimeType, "image/") ||
					strings.HasPrefix(mimeType, "audio/") ||
					strings.HasPrefix(mimeType, "video/") {
					delete(part, "data")
				}
			}
		}
	}
	b, err := common.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

func truncateBase64(s string) string {
	const maxKeep = 256
	if len(s) <= maxKeep {
		return s
	}
	return s[:maxKeep] + "..."
}

// settleTaskBillingOnComplete 任务完成时的统一计费调整。
// 返回 true 表示用量结算路径已接管最终计费；失败任务仅在返回 false 时补做全额退款。
// 优先级：1. tiered snapshot → 2. adaptor 调整 → 3. token 重算。
//
// 表达式求值失败会保留预扣额度，因此也视为已接管，避免错误全退。
func settleTaskBillingOnComplete(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, taskResult *relaycommon.TaskInfo) bool {
	if bc := task.PrivateData.BillingContext; bc != nil && bc.TieredSnapshot != nil {
		// 用量表达式结算只适用于成功任务；失败任务由调用方全额退款。
		if task.Status == model.TaskStatusFailure {
			return false
		}
		result, usageFacts, err := EvaluateTaskCompletionUsage(bc.TieredSnapshot, taskResult.UsageFacts)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("任务 %s 表达式结算失败，保留预扣额度: %v", task.TaskID, err))
			return true
		}
		if result.Clamp != nil {
			logger.LogWarn(ctx, fmt.Sprintf("任务 %s 表达式结算额度发生饱和: %+v", task.TaskID, result.Clamp))
		}
		bc.TieredSnapshot.UsageFacts = usageFacts
		bc.TieredSnapshot.EstimatedTier = result.MatchedTier
		bc.TieredSnapshot.ToolSurcharges = result.ToolSurcharges
		bc.TieredSnapshot.EstimatedFacts = RemainingTaskEstimates(bc.TieredSnapshot.EstimatedFacts, taskResult.UsageFacts)
		reason := "任务用量表达式结算"
		for _, item := range result.ToolSurcharges {
			reason += fmt.Sprintf("，%s 调用 %d 次", item.Name, item.Count)
		}
		RecalculateTaskQuota(ctx, task, result.ActualQuotaAfterGroup, reason, result.Clamp)
		// RecalculateTaskQuota persists only the quota; keep the measured
		// usage on the stored snapshot as well.
		if err := task.UpdateBillingSnapshot(); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("任务 %s 结算快照保存失败: %v", task.TaskID, err))
		}
		return true
	}
	// 按次计费的成功任务保持预扣；失败任务由调用方全额退款。
	if bc := task.PrivateData.BillingContext; bc != nil && bc.PerCallBilling {
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 按次计费，跳过差额结算", task.TaskID))
		return false
	}
	// 优先让 adaptor 决定最终额度。
	if actualQuota := adaptor.AdjustBillingOnComplete(task, taskResult); actualQuota > 0 {
		RecalculateTaskQuota(ctx, task, actualQuota, "adaptor计费调整")
		return true
	}
	// 按秒任务以适配器的时长倍率为权威；部分视频上游会返回
	// total_tokens 作为生成统计，不能将其误用为文本 Token 计费。
	if bc := task.PrivateData.BillingContext; bc != nil && bc.BillingUnit == billing_setting.BillingUnitSecond {
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 按秒计费，跳过 total_tokens 回退结算", task.TaskID))
		return task.Status == model.TaskStatusSuccess
	}
	// 回退到 token 重算。
	tokens := taskResult.TotalTokens
	if tokens == 0 && taskResult.CompletionTokens > 0 {
		tokens = taskResult.CompletionTokens
	}
	if tokens > 0 {
		return RecalculateTaskQuotaByTokens(ctx, task, tokens)
	}
	return false
}

func classifyPollHTTP(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return pollClassOK
	case statusCode == http.StatusNotFound || statusCode == http.StatusGone:
		return pollClassNotFound
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return pollClassAuth
	case statusCode == http.StatusTooManyRequests || statusCode >= 500:
		return pollClassTransient
	case statusCode >= 400 && statusCode < 500:
		return pollClassOtherClient
	default:
		return pollClassTransient
	}
}

func knownPollStatus(status model.TaskStatus) bool {
	switch status {
	case model.TaskStatusNotStart, model.TaskStatusSubmitted, model.TaskStatusQueued, model.TaskStatusInProgress, model.TaskStatusSuccess, model.TaskStatusFailure:
		return true
	default:
		return false
	}
}

func isNonTerminalPollStatus(status model.TaskStatus) bool {
	switch status {
	case model.TaskStatusNotStart, model.TaskStatusSubmitted, model.TaskStatusQueued, model.TaskStatusInProgress:
		return true
	default:
		return false
	}
}

func pollFailureReason(class string, statusCode int, detail string) string {
	reason := fmt.Sprintf("poll failed: %s", class)
	if statusCode > 0 {
		reason = fmt.Sprintf("poll failed: %s (HTTP %d)", class, statusCode)
	}
	if detail != "" {
		reason = reason + ": " + detail
	}
	return reason
}

// unrecognizedPollDetail pairs the plugin's reason with a bounded copy of the
// upstream body so the WARN line is enough to diagnose a parser gap.
func unrecognizedPollDetail(reason string, body []byte) string {
	const maxBodyChars = 512
	redacted := string(redactVideoResponseBody(body))
	if len(redacted) > maxBodyChars {
		redacted = redacted[:maxBodyChars] + "…"
	}
	if strings.TrimSpace(reason) == "" {
		return "body=" + redacted
	}
	return reason + "; body=" + redacted
}

func recordPollFailure(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, fromStatus model.TaskStatus, class string, statusCode int, detail string) error {
	task.PrivateData.PollFailures++
	if class == pollClassUnrecognized || class == pollClassHookError {
		// The redacted body is intentionally not persisted to Task.Data on these
		// paths, so the WARN line is the only operator-visible copy of what the
		// plugin could not interpret.
		logger.LogWarn(ctx, fmt.Sprintf("task %s poll %s (failures=%d, http=%d): %s", task.TaskID, class, task.PrivateData.PollFailures, statusCode, detail))
	}
	// TASK_POLL_MAX_FAILURES <= 0 disables the consecutive-failure cutoff, matching
	// TASK_TIMEOUT_MINUTES semantics; the 24h sweep remains the only backstop.
	if constant.TaskPollMaxFailures > 0 && task.PrivateData.PollFailures >= constant.TaskPollMaxFailures {
		return failTaskFromPoll(ctx, adaptor, task, fromStatus, pollFailureReason(class, statusCode, detail))
	}
	if _, err := task.UpdateWithStatus(fromStatus); err != nil {
		return err
	}
	return nil
}

func recordPollFailureForTasks(ctx context.Context, adaptor TaskPollingAdaptor, tasks []*model.Task, class string, statusCode int, detail string) error {
	var firstErr error
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if err := recordPollFailure(ctx, adaptor, task, task.Status, class, statusCode, detail); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func failTaskFromPoll(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, fromStatus model.TaskStatus, reason string) error {
	_, err := FailTask(ctx, adaptor, task, fromStatus, reason)
	return err
}

// FailTask moves a task from fromStatus to FAILURE and, only for the caller
// that wins that transition, settles it like any failed task: full refund,
// metrics and callback. It reports whether this caller won; a loser's task
// fields are stale and must be reloaded.
func FailTask(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, fromStatus model.TaskStatus, reason string) (bool, error) {
	now := time.Now().Unix()
	task.Status = model.TaskStatusFailure
	task.Progress = taskcommon.ProgressComplete
	if task.FinishTime == 0 {
		task.FinishTime = now
	}
	task.FailReason = reason
	won, err := task.UpdateWithStatus(fromStatus)
	if err != nil || !won {
		return false, err
	}
	finalizeTerminalTask(ctx, adaptor, task, relaycommon.FailTaskInfo(reason))
	return true, nil
}

func failTasksFromPoll(ctx context.Context, adaptor TaskPollingAdaptor, tasks []*model.Task, reason string) error {
	var firstErr error
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if err := failTaskFromPoll(ctx, adaptor, task, task.Status, reason); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

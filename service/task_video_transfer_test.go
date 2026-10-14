package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// transferPollingAdaptor reports every task in results as finished upstream.
type transferPollingAdaptor struct {
	mu      sync.Mutex
	results map[string]relaycommon.TaskInfo
	fetched []string
}

func (a *transferPollingAdaptor) Init(*relaycommon.RelayInfo) {}

func (a *transferPollingAdaptor) FetchTask(_ string, _ string, task *model.Task, _ string) (*http.Response, error) {
	a.mu.Lock()
	a.fetched = append(a.fetched, task.GetUpstreamTaskID())
	a.mu.Unlock()
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader([]byte(`{"provider":"payload"}`)))}, nil
}

func (a *transferPollingAdaptor) ParseTaskResult(task *model.Task, _ *http.Response, _ []byte) (*relaycommon.TaskInfo, error) {
	result, ok := a.results[task.GetUpstreamTaskID()]
	if !ok {
		return &relaycommon.TaskInfo{Status: model.TaskStatusInProgress}, nil
	}
	return &result, nil
}

func (a *transferPollingAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func (a *transferPollingAdaptor) fetchCount(upstreamID string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	count := 0
	for _, id := range a.fetched {
		if id == upstreamID {
			count++
		}
	}
	return count
}

// gatedVideoSources serves video bodies that block until released, counting
// how often each task's source is opened.
type gatedVideoSources struct {
	mu         sync.Mutex
	opens      map[string]int
	release    chan struct{}
	releaseAll func()
	fail       bool
}

func useGatedVideoSources(t *testing.T) *gatedVideoSources {
	t.Helper()
	sources := &gatedVideoSources{opens: make(map[string]int), release: make(chan struct{})}
	var releaseOnce sync.Once
	previousOpener := OpenTaskVideoSourceFunc
	OpenTaskVideoSourceFunc = func(_ context.Context, task *model.Task, _ string) (*TaskVideoSource, error) {
		sources.mu.Lock()
		sources.opens[task.TaskID]++
		fail := sources.fail
		sources.mu.Unlock()
		if fail {
			return nil, errors.New("source unavailable")
		}
		return &TaskVideoSource{
			Body:        io.NopCloser(&gatedReader{release: sources.release, body: strings.NewReader("video-bytes")}),
			ContentType: "video/mp4",
			Header:      make(http.Header),
			StatusCode:  http.StatusOK,
		}, nil
	}
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(sources.release) })
		require.Eventually(t, func() bool { return TaskVideoTransferStatsSnapshot().InFlight == 0 }, 5*time.Second, 10*time.Millisecond)
		OpenTaskVideoSourceFunc = previousOpener
	})
	sources.releaseAll = func() { releaseOnce.Do(func() { close(sources.release) }) }
	return sources
}

func (s *gatedVideoSources) openCount(taskID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opens[taskID]
}

type gatedReader struct {
	release chan struct{}
	body    io.Reader
}

func (r *gatedReader) Read(p []byte) (int, error) {
	<-r.release
	return r.body.Read(p)
}

func useAsyncVideoTransfer(t *testing.T, concurrency, perHost string) {
	t.Helper()
	t.Setenv("TASK_VIDEO_TRANSFER_ASYNC", "true")
	t.Setenv("TASK_VIDEO_TRANSFER_CONCURRENCY", concurrency)
	t.Setenv("TASK_VIDEO_TRANSFER_PER_HOST", perHost)
	t.Setenv("TASK_VIDEO_CACHE_ENABLED", "true")
	t.Setenv("TASK_VIDEO_WORKER_ENABLED", "false")
	t.Setenv("TASK_VIDEO_DIRECT_HOSTS", "")
	useLocalTaskVideoCache(t)
}

// seedTransferTask stores an in-progress task whose completion settles a
// tiered usage expression: 3 units cost 3,000 of the 5,000 pre-consumed.
func seedTransferTask(t *testing.T, userID, tokenID, channelID int, publicID string) *model.Task {
	t.Helper()
	expression := `tier("actual", u("units"))`
	task := makeTask(userID, channelID, 5_000, tokenID, BillingSourceWallet, 0)
	task.TaskID = publicID
	task.Platform = constant.TaskPlatform("kling")
	task.Progress = "30%"
	task.PrivateData.UpstreamTaskID = "upstream_" + publicID
	task.PrivateData.BillingContext.TieredSnapshot = &billingexpr.BillingSnapshot{
		ExprString:       expression,
		ExprHash:         billingexpr.ExprHashString(expression),
		GroupRatio:       1,
		QuotaPerUnit:     1_000,
		ExprVersion:      1,
		TaskUsageBilling: true,
	}
	require.NoError(t, model.DB.Create(task).Error)
	t.Cleanup(func() { clearTaskVideoPrepareFailure(publicID) })
	return task
}

func finishedUpstream(host string) relaycommon.TaskInfo {
	return relaycommon.TaskInfo{
		Status:     model.TaskStatusSuccess,
		Url:        "https://" + host + "/result.mp4",
		UsageFacts: map[string]any{"units": float64(3)},
	}
}

func pollVideoChannel(t *testing.T, adaptor TaskPollingAdaptor, channelID int, tasks ...*model.Task) {
	t.Helper()
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	defer func() { GetTaskAdaptorFunc = previousFactory }()
	ids := make([]string, 0, len(tasks))
	taskM := make(map[string]*model.Task, len(tasks))
	for _, task := range tasks {
		var fresh model.Task
		require.NoError(t, model.DB.First(&fresh, task.ID).Error)
		ids = append(ids, fresh.GetUpstreamTaskID())
		taskM[fresh.GetUpstreamTaskID()] = &fresh
	}
	done := make(chan error, 1)
	gopool.Go(func() {
		done <- UpdateVideoTasks(context.Background(), "kling", map[int][]string{channelID: ids}, taskM)
	})
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("polling pass waited for a video transfer")
	}
}

func storedTask(t *testing.T, id int64) model.Task {
	t.Helper()
	var task model.Task
	require.NoError(t, model.DB.First(&task, id).Error)
	return task
}

func TestAsyncVideoTransferDoesNotHoldPollingAndSettlesOnce(t *testing.T) {
	truncate(t)
	useAsyncVideoTransfer(t, "32", "8")
	sources := useGatedVideoSources(t)
	const userID, tokenID, channelID = 81, 81, 181
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-async-transfer", 8_000)
	seedTaskPollingChannel(t, channelID, true)
	finished := seedTransferTask(t, userID, tokenID, channelID, "task_async_finished")
	running := seedTransferTask(t, userID, tokenID, channelID, "task_async_running")
	adaptor := &transferPollingAdaptor{results: map[string]relaycommon.TaskInfo{
		finished.GetUpstreamTaskID(): finishedUpstream("slow.example"),
	}}

	// The pass returns while the slow source is still blocked, and the other
	// task on the same channel is still polled.
	pollVideoChannel(t, adaptor, channelID, finished, running)
	assert.Equal(t, 1, adaptor.fetchCount(running.GetUpstreamTaskID()))
	require.Eventually(t, func() bool { return sources.openCount(finished.TaskID) == 1 }, time.Second, 10*time.Millisecond)

	// Until the video is stored, nothing is published or settled.
	pending := storedTask(t, finished.ID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), pending.Status)
	assert.Empty(t, pending.GetResultURL())
	assert.Equal(t, 5_000, pending.Quota)
	assert.Equal(t, 10_000, getUserQuota(t, userID))
	assert.Equal(t, int64(0), countLogs(t))

	// Later passes and a request-time refresh leave the in-flight task alone.
	pollVideoChannel(t, adaptor, channelID, finished, running)
	assert.Equal(t, 1, adaptor.fetchCount(finished.GetUpstreamTaskID()))
	refreshed := storedTask(t, finished.ID)
	result := finishedUpstream("slow.example")
	require.NoError(t, FinalizeVideoTaskResult(context.Background(), adaptor, &refreshed, &result, []byte(`{"provider":"payload"}`)))
	assert.Equal(t, 1, sources.openCount(finished.TaskID))

	// Settlement runs after the terminal write, so wait for the whole transfer.
	sources.releaseAll()
	require.Eventually(t, func() bool { return TaskVideoTransferStatsSnapshot().InFlight == 0 }, 5*time.Second, 10*time.Millisecond)
	completed := storedTask(t, finished.ID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), completed.Status)
	assert.Equal(t, "100%", completed.Progress)
	assert.NotZero(t, completed.FinishTime)
	assert.Contains(t, completed.GetResultURL(), "/v1/videos/task_async_finished/content")
	assert.NotEmpty(t, completed.PrivateData.ResultStorageKey)
	assert.Equal(t, 3_000, completed.Quota)
	assert.Equal(t, 12_000, getUserQuota(t, userID))
	assert.Equal(t, 10_000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(1), countLogs(t))

	// A stale success seen again (for example from a node that lost its lease)
	// reuses the stored copy and loses the terminal CAS without settling again.
	stale := *finished
	staleResult := finishedUpstream("slow.example")
	require.NoError(t, FinalizeVideoTaskResult(context.Background(), adaptor, &stale, &staleResult, []byte(`{"provider":"payload"}`)))
	require.Eventually(t, func() bool { return TaskVideoTransferStatsSnapshot().InFlight == 0 }, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, 1, sources.openCount(finished.TaskID))
	assert.Equal(t, 12_000, getUserQuota(t, userID))
	assert.Equal(t, int64(1), countLogs(t))
}

func TestAsyncVideoTransferRespectsPoolAndHostLimits(t *testing.T) {
	truncate(t)
	useAsyncVideoTransfer(t, "2", "1")
	sources := useGatedVideoSources(t)
	const userID, tokenID, channelID = 82, 82, 182
	seedUser(t, userID, 100_000)
	seedToken(t, tokenID, userID, "sk-async-limits", 100_000)
	seedTaskPollingChannel(t, channelID, true)
	firstA := seedTransferTask(t, userID, tokenID, channelID, "task_limit_a1")
	secondA := seedTransferTask(t, userID, tokenID, channelID, "task_limit_a2")
	onlyB := seedTransferTask(t, userID, tokenID, channelID, "task_limit_b1")
	adaptor := &transferPollingAdaptor{results: map[string]relaycommon.TaskInfo{
		firstA.GetUpstreamTaskID():  finishedUpstream("a.example"),
		secondA.GetUpstreamTaskID(): finishedUpstream("a.example"),
		onlyB.GetUpstreamTaskID():   finishedUpstream("b.example"),
	}}

	// Host a may hold one transfer, so its second task waits even though the
	// pool has room for host b.
	pollVideoChannel(t, adaptor, channelID, firstA, secondA, onlyB)
	require.Eventually(t, func() bool {
		return sources.openCount(firstA.TaskID) == 1 && sources.openCount(onlyB.TaskID) == 1
	}, time.Second, 10*time.Millisecond)
	assert.Zero(t, sources.openCount(secondA.TaskID))
	assert.Equal(t, 2, TaskVideoTransferStatsSnapshot().InFlight)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), storedTask(t, secondA.ID).Status)

	sources.releaseAll()
	require.Eventually(t, func() bool { return TaskVideoTransferStatsSnapshot().InFlight == 0 }, 5*time.Second, 10*time.Millisecond)
	pollVideoChannel(t, adaptor, channelID, secondA)
	require.Eventually(t, func() bool { return TaskVideoTransferStatsSnapshot().InFlight == 0 }, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), storedTask(t, secondA.ID).Status)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), storedTask(t, firstA.ID).Status)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), storedTask(t, onlyB.ID).Status)
}

func TestAsyncVideoTransferFailureBacksOffWithoutPublishing(t *testing.T) {
	truncate(t)
	useAsyncVideoTransfer(t, "32", "8")
	sources := useGatedVideoSources(t)
	sources.fail = true
	const userID, tokenID, channelID = 83, 83, 183
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-async-failure", 8_000)
	seedTaskPollingChannel(t, channelID, true)
	task := seedTransferTask(t, userID, tokenID, channelID, "task_async_failure")
	adaptor := &transferPollingAdaptor{results: map[string]relaycommon.TaskInfo{
		task.GetUpstreamTaskID(): finishedUpstream("broken.example"),
	}}

	pollVideoChannel(t, adaptor, channelID, task)
	require.Eventually(t, func() bool { return TaskVideoTransferStatsSnapshot().InFlight == 0 }, 5*time.Second, 10*time.Millisecond)
	_, backingOff := taskVideoPrepareRetryAt(task.TaskID, time.Now())
	assert.True(t, backingOff)
	stored := storedTask(t, task.ID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), stored.Status)
	assert.Empty(t, stored.GetResultURL())
	assert.Equal(t, 10_000, getUserQuota(t, userID))
	assert.Equal(t, int64(0), countLogs(t))

	// While backing off, the task is not queried upstream again.
	pollVideoChannel(t, adaptor, channelID, task)
	assert.Equal(t, 1, adaptor.fetchCount(task.GetUpstreamTaskID()))
	assert.Equal(t, 1, sources.openCount(task.TaskID))
}

func TestAsyncVideoTransferLosesToConcurrentCancellation(t *testing.T) {
	truncate(t)
	useAsyncVideoTransfer(t, "32", "8")
	sources := useGatedVideoSources(t)
	const userID, tokenID, channelID = 84, 84, 184
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-async-cancel", 8_000)
	seedTaskPollingChannel(t, channelID, true)
	task := seedTransferTask(t, userID, tokenID, channelID, "task_async_cancel")
	adaptor := &transferPollingAdaptor{results: map[string]relaycommon.TaskInfo{
		task.GetUpstreamTaskID(): finishedUpstream("slow.example"),
	}}

	pollVideoChannel(t, adaptor, channelID, task)
	require.Eventually(t, func() bool { return sources.openCount(task.TaskID) == 1 }, time.Second, 10*time.Millisecond)

	cancelled := storedTask(t, task.ID)
	won, err := FailTask(context.Background(), adaptor, &cancelled, cancelled.Status, "任务已被用户取消")
	require.NoError(t, err)
	require.True(t, won)

	sources.releaseAll()
	require.Eventually(t, func() bool { return TaskVideoTransferStatsSnapshot().InFlight == 0 }, 5*time.Second, 10*time.Millisecond)
	final := storedTask(t, task.ID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), final.Status)
	assert.Equal(t, 15_000, getUserQuota(t, userID))
	assert.Equal(t, int64(1), countLogs(t))
}

func TestAsyncVideoTransferReusesStoredCopyAfterRestart(t *testing.T) {
	truncate(t)
	useAsyncVideoTransfer(t, "32", "8")
	sources := useGatedVideoSources(t)
	const userID, tokenID, channelID = 85, 85, 185
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-async-restart", 8_000)
	seedTaskPollingChannel(t, channelID, true)
	task := seedTransferTask(t, userID, tokenID, channelID, "task_async_restart")
	// A previous process stored the video but died before the terminal write.
	storage, err := taskVideoCacheStorageFactory()
	require.NoError(t, err)
	content := "stored-before-restart"
	require.NoError(t, storage.Put(context.Background(), taskVideoCacheKeyPrefix(task)+".mp4", strings.NewReader(content), int64(len(content)), "video/mp4"))
	adaptor := &transferPollingAdaptor{results: map[string]relaycommon.TaskInfo{
		task.GetUpstreamTaskID(): finishedUpstream("slow.example"),
	}}

	pollVideoChannel(t, adaptor, channelID, task)
	require.Eventually(t, func() bool { return TaskVideoTransferStatsSnapshot().InFlight == 0 }, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), storedTask(t, task.ID).Status)
	assert.Zero(t, sources.openCount(task.TaskID))
	assert.Equal(t, 12_000, getUserQuota(t, userID))
	assert.Equal(t, int64(1), countLogs(t))
}

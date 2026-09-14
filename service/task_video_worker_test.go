package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicefile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskVideoWorkerStorage struct {
	invoicefile.Storage
	existingKeys map[string]bool
}

func (s *taskVideoWorkerStorage) Kind() string { return taskVideoCacheKind }

func (s *taskVideoWorkerStorage) Exists(_ context.Context, key string) (bool, error) {
	return s.existingKeys[key], nil
}

func useTaskVideoWorkerConfig(t *testing.T) {
	t.Helper()
	t.Setenv("TASK_VIDEO_WORKER_ENABLED", "true")
	t.Setenv("TASK_VIDEO_WORKER_URL", "https://new-api-video-transfer.example.workers.dev/transfer")
	t.Setenv("TASK_VIDEO_WORKER_SECRET", "worker-shared-test-secret")
	t.Setenv("TASK_VIDEO_WORKER_TIMEOUT_SECONDS", "900")
}

func TestCacheTaskVideoRemoteSourceUsesWorkerBeforeVPS(t *testing.T) {
	useTaskVideoWorkerConfig(t)
	task := &model.Task{
		TaskID:    "task_worker_primary_123",
		Platform:  constant.TaskPlatform("62"),
		CreatedAt: 1787860000,
	}
	prefix := taskVideoCacheKeyPrefix(task)
	key := prefix + ".mp4"
	storage := &taskVideoWorkerStorage{existingKeys: map[string]bool{key: true}}
	previousFactory := taskVideoCacheStorageFactory
	taskVideoCacheStorageFactory = func() (invoicefile.Storage, error) { return storage, nil }
	t.Cleanup(func() { taskVideoCacheStorageFactory = previousFactory })

	previousDo := taskVideoWorkerDo
	taskVideoWorkerDo = func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		timestamp := req.Header.Get("X-NewAPI-Timestamp")
		mac := hmac.New(sha256.New, []byte("worker-shared-test-secret"))
		_, _ = mac.Write([]byte(timestamp + "."))
		_, _ = mac.Write(body)
		assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), req.Header.Get("X-NewAPI-Signature"))
		var requestData taskVideoWorkerRequest
		require.NoError(t, common.Unmarshal(body, &requestData))
		assert.Equal(t, task.TaskID, requestData.TaskID)
		assert.Equal(t, prefix, requestData.KeyPrefix)
		responseBody, err := common.Marshal(taskVideoWorkerResponse{
			Success:  true,
			Key:      key,
			MimeType: "video/mp4",
			Size:     11,
			ETag:     "etag-11",
		})
		require.NoError(t, err)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(string(responseBody))),
		}, nil
	}
	t.Cleanup(func() { taskVideoWorkerDo = previousDo })

	previousOpener := OpenTaskVideoSourceFunc
	localOpened := false
	OpenTaskVideoSourceFunc = func(context.Context, *model.Task, string) (*TaskVideoSource, error) {
		localOpened = true
		return nil, assert.AnError
	}
	t.Cleanup(func() { OpenTaskVideoSourceFunc = previousOpener })

	cached, err := cacheTaskVideoRemoteSource(context.Background(), task, "https://media.provider.example/result.mp4")

	require.NoError(t, err)
	require.True(t, cached)
	assert.False(t, localOpened)
	assert.Equal(t, key, task.PrivateData.ResultStorageKey)
	assert.Equal(t, taskVideoCacheKind, task.PrivateData.ResultStorageKind)
	assert.Equal(t, "video/mp4", task.PrivateData.ResultMimeType)
}

func TestCacheTaskVideoRemoteSourceFallsBackToVPS(t *testing.T) {
	useTaskVideoWorkerConfig(t)
	useLocalTaskVideoCache(t)
	task := &model.Task{TaskID: "task_worker_fallback_123", Platform: constant.TaskPlatform("62")}

	previousDo := taskVideoWorkerDo
	taskVideoWorkerDo = func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"success":false,"code":"upstream_unavailable"}`)),
		}, nil
	}
	t.Cleanup(func() { taskVideoWorkerDo = previousDo })

	previousOpener := OpenTaskVideoSourceFunc
	localOpened := false
	OpenTaskVideoSourceFunc = func(context.Context, *model.Task, string) (*TaskVideoSource, error) {
		localOpened = true
		return &TaskVideoSource{
			Body:          io.NopCloser(strings.NewReader("fallback-video")),
			ContentLength: int64(len("fallback-video")),
			ContentType:   "video/mp4",
			Header:        make(http.Header),
			StatusCode:    http.StatusOK,
		}, nil
	}
	t.Cleanup(func() { OpenTaskVideoSourceFunc = previousOpener })

	cached, err := cacheTaskVideoRemoteSource(context.Background(), task, "https://media.provider.example/result.mp4")

	require.NoError(t, err)
	require.True(t, cached)
	assert.True(t, localOpened)
	assert.Equal(t, "local", task.PrivateData.ResultStorageKind)
}

func TestTaskVideoWorkerEligibilityKeepsPrivateAndSpecialAuthOnVPS(t *testing.T) {
	useTaskVideoWorkerConfig(t)

	assert.True(t, taskVideoWorkerEligible(
		&model.Task{TaskID: "task_public_123", Platform: constant.TaskPlatform("62")},
		"https://media.provider.example/result.mp4",
	))
	assert.False(t, taskVideoWorkerEligible(
		&model.Task{TaskID: "task_sub2api_123", Platform: constant.TaskPlatform("59")},
		"https://sub.example.com/v1/videos/id/content",
	))
	assert.False(t, taskVideoWorkerEligible(
		&model.Task{TaskID: "task_internal_123", Platform: constant.TaskPlatform("62")},
		"http://sub2api:8080/v1/videos/id/content",
	))
	assert.False(t, taskVideoWorkerEligible(
		&model.Task{TaskID: "task_credential_123", Platform: constant.TaskPlatform("62")},
		"https://media.provider.example/result.mp4?access_token=secret",
	))
}

func TestTaskVideoWorkerRequiresHTTPSAndCompleteConfiguration(t *testing.T) {
	t.Setenv("TASK_VIDEO_WORKER_ENABLED", "true")
	t.Setenv("TASK_VIDEO_WORKER_URL", "http://worker.example/transfer")
	t.Setenv("TASK_VIDEO_WORKER_SECRET", "secret")
	assert.False(t, TaskVideoWorkerEnabled())

	t.Setenv("TASK_VIDEO_WORKER_URL", "https://worker.example/transfer")
	assert.True(t, TaskVideoWorkerEnabled())

	t.Setenv("TASK_VIDEO_WORKER_SECRET", "")
	assert.False(t, TaskVideoWorkerEnabled())
}

package service

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service/invoicefile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskVideoSignedURLStorage struct {
	invoicefile.Storage
	url      string
	ttl      time.Duration
	filename string
	inline   bool
}

func (s *taskVideoSignedURLStorage) SignedURL(_ context.Context, _ string, ttl time.Duration, filename string, inline bool) (string, error) {
	s.ttl = ttl
	s.filename = filename
	s.inline = inline
	return s.url, nil
}

func useLocalTaskVideoCache(t *testing.T) invoicefile.Storage {
	t.Helper()
	storage, err := invoicefile.NewLocalStorage(t.TempDir())
	require.NoError(t, err)
	previousFactory := taskVideoCacheStorageFactory
	taskVideoCacheStorageFactory = func() (invoicefile.Storage, error) { return storage, nil }
	t.Cleanup(func() { taskVideoCacheStorageFactory = previousFactory })
	return storage
}

func TestPrepareTaskVideoResultPersistsAndReopensDataURL(t *testing.T) {
	t.Setenv("TASK_VIDEO_CACHE_ENABLED", "true")
	useLocalTaskVideoCache(t)
	videoBytes := []byte("vertex-video-bytes")
	resultURL := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(videoBytes)
	task := &model.Task{TaskID: "task_public_cache"}

	prepared, err := PrepareTaskVideoResult(context.Background(), task, resultURL)
	require.NoError(t, err)
	require.True(t, prepared.Cached)
	assert.Equal(t, "local", task.PrivateData.ResultStorageKind)
	assert.NotEmpty(t, task.PrivateData.ResultStorageKey)
	assert.Equal(t, "video/mp4", task.PrivateData.ResultMimeType)
	assert.Contains(t, task.PrivateData.ResultURL, "/v1/videos/task_public_cache/content")

	reader, mimeType, found, err := OpenTaskVideoCache(context.Background(), task)
	require.NoError(t, err)
	require.True(t, found)
	defer reader.Close()
	actual, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, videoBytes, actual)
	assert.Equal(t, "video/mp4", mimeType)
}

func TestPrepareTaskVideoResultKeepsUpstreamR2URLDirect(t *testing.T) {
	t.Setenv("TASK_VIDEO_CACHE_ENABLED", "true")
	task := &model.Task{
		TaskID: "task_upstream_r2",
		Data:   []byte(`{"video_url":"https://account.r2.cloudflarestorage.com/cdn/video.mp4?X-Amz-Signature=signed"}`),
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://provider.example/v1/videos/upstream/content",
		},
	}

	prepared, err := PrepareTaskVideoResult(context.Background(), task, task.GetResultURL())

	require.NoError(t, err)
	assert.False(t, prepared.Cached)
	assert.Equal(t, "https://account.r2.cloudflarestorage.com/cdn/video.mp4?X-Amz-Signature=signed", prepared.DirectURL)
	assert.Empty(t, task.PrivateData.ResultStorageKey)
	assert.Equal(t, prepared.DirectURL, task.PrivateData.ResultURL)
}

func TestPrepareTaskVideoResultKeepsAnonymousOfficialURLDirect(t *testing.T) {
	t.Setenv("TASK_VIDEO_CACHE_ENABLED", "true")
	previousProbe := taskVideoDirectProbe
	taskVideoDirectProbe = func(context.Context, string) (bool, error) { return true, nil }
	t.Cleanup(func() { taskVideoDirectProbe = previousProbe })
	task := &model.Task{TaskID: "task_official_direct"}
	resultURL := "https://official.example/video/result.mp4"

	prepared, err := PrepareTaskVideoResult(context.Background(), task, resultURL)

	require.NoError(t, err)
	assert.False(t, prepared.Cached)
	assert.Equal(t, resultURL, prepared.DirectURL)
	assert.Empty(t, task.PrivateData.ResultStorageKey)
}

func TestPrepareTaskVideoResultCachesProtectedInternalURL(t *testing.T) {
	t.Setenv("TASK_VIDEO_CACHE_ENABLED", "true")
	useLocalTaskVideoCache(t)
	previousOpener := OpenTaskVideoSourceFunc
	OpenTaskVideoSourceFunc = func(context.Context, *model.Task, string) (*TaskVideoSource, error) {
		return &TaskVideoSource{
			Body:          io.NopCloser(strings.NewReader("protected-video")),
			ContentLength: int64(len("protected-video")),
			ContentType:   "video/mp4",
			Header:        make(http.Header),
			StatusCode:    http.StatusOK,
		}, nil
	}
	t.Cleanup(func() { OpenTaskVideoSourceFunc = previousOpener })
	task := &model.Task{TaskID: "task_internal_video"}

	prepared, err := PrepareTaskVideoResult(context.Background(), task, "http://sub2api:8080/v1/videos/upstream/content")

	require.NoError(t, err)
	require.True(t, prepared.Cached)
	assert.NotEmpty(t, task.PrivateData.ResultStorageKey)
	assert.Contains(t, task.PrivateData.ResultURL, "/v1/videos/task_internal_video/content")
}

func TestPrepareTaskVideoResultNeverReturnsInternalURLDirectly(t *testing.T) {
	t.Setenv("TASK_VIDEO_CACHE_ENABLED", "false")
	task := &model.Task{TaskID: "task_internal_not_direct"}

	prepared, err := PrepareTaskVideoResult(context.Background(), task, "http://sub2api:8080/v1/videos/upstream/content")

	require.NoError(t, err)
	assert.False(t, prepared.Cached)
	assert.Empty(t, prepared.DirectURL)
	assert.Empty(t, task.PrivateData.ResultURL)
}

func TestFinalizeVideoTaskResultDoesNotPublishSuccessWhenCacheFails(t *testing.T) {
	truncate(t)
	t.Setenv("TASK_VIDEO_CACHE_ENABLED", "true")
	previousFactory := taskVideoCacheStorageFactory
	taskVideoCacheStorageFactory = func() (invoicefile.Storage, error) { return nil, errors.New("r2 unavailable") }
	t.Cleanup(func() { taskVideoCacheStorageFactory = previousFactory })
	task := &model.Task{TaskID: "task_cache_failure", Status: model.TaskStatusInProgress}
	require.NoError(t, model.DB.Create(task).Error)
	resultURL := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString([]byte("video"))

	err := FinalizeVideoTaskResult(context.Background(), &taskPollingFetchAdaptor{}, task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, Url: resultURL}, []byte(`{"status":"SUCCESS"}`))
	require.ErrorContains(t, err, "r2 unavailable")

	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), stored.Status)
}

func TestGetTaskVideoPreviewURLCreatesSevenDayDirectLink(t *testing.T) {
	t.Setenv("TASK_VIDEO_CACHE_SIGNED_URL_TTL_SECONDS", "604800")
	localStorage := useLocalTaskVideoCache(t)
	signedStorage := &taskVideoSignedURLStorage{
		Storage: localStorage,
		url:     "https://example.r2.cloudflarestorage.com/bucket/video.mp4?signed=true",
	}
	previousFactory := taskVideoCacheStorageFactory
	taskVideoCacheStorageFactory = func() (invoicefile.Storage, error) { return signedStorage, nil }
	t.Cleanup(func() { taskVideoCacheStorageFactory = previousFactory })
	task := &model.Task{
		TaskID: "task_direct_preview",
		PrivateData: model.TaskPrivateData{
			ResultStorageKind: localStorage.Kind(),
			ResultStorageKey:  "task-videos/2026/08/video.mp4",
			ResultMimeType:    "video/mp4",
		},
	}

	previewURL, cached, err := GetTaskVideoPreviewURL(context.Background(), task)

	require.NoError(t, err)
	require.True(t, cached)
	assert.Equal(t, signedStorage.url, previewURL)
	assert.Equal(t, 7*24*time.Hour, signedStorage.ttl)
	assert.Empty(t, signedStorage.filename)
	assert.True(t, signedStorage.inline)
}

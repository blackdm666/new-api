package service

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service/invoicefile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useLocalTaskVideoCache(t *testing.T) invoicefile.Storage {
	t.Helper()
	storage, err := invoicefile.NewLocalStorage(t.TempDir())
	require.NoError(t, err)
	previousFactory := taskVideoCacheStorageFactory
	taskVideoCacheStorageFactory = func() (invoicefile.Storage, error) { return storage, nil }
	t.Cleanup(func() { taskVideoCacheStorageFactory = previousFactory })
	return storage
}

func TestCacheTaskVideoResultPersistsAndReopensDataURL(t *testing.T) {
	t.Setenv("TASK_VIDEO_CACHE_ENABLED", "true")
	useLocalTaskVideoCache(t)
	videoBytes := []byte("vertex-video-bytes")
	resultURL := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(videoBytes)
	task := &model.Task{TaskID: "task_public_cache"}

	cached, err := CacheTaskVideoResult(context.Background(), task, resultURL)
	require.NoError(t, err)
	require.True(t, cached)
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

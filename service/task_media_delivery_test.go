package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func deliveryTask(t *testing.T) *model.Task {
	t.Helper()
	t.Setenv("TASK_MEDIA_PUBLIC_ENABLED", "true")
	t.Setenv("TASK_MEDIA_PUBLIC_BASE_URL", "https://assets.88api.ai/media")
	t.Setenv("TASK_VIDEO_DIRECT_HOSTS", "official.example")
	return &model.Task{TaskID: "task_delivery", Status: model.TaskStatusSuccess, Data: []byte(`{"content":{"url":"https://official.example/result.mp4"}}`), PrivateData: model.TaskPrivateData{
		ResultStorageKind: "s3", ResultStorageKey: "task-videos/2026/09/" + strings.Repeat("a", 64) + ".mp4", ResultMimeType: "video/mp4", ResultURL: "https://gateway.example/v1/videos/task_delivery/content",
	}}
}

func TestTaskArtifactDeliveryMapsOnlyExactSingleVideo(t *testing.T) {
	for _, tc := range []struct {
		name, kind, source string
		single, mapped     bool
	}{
		{"single", "video", "https://official.example/result.mp4", true, true},
		{"different-video", "video", "https://untrusted.example/other.mp4", true, false},
		{"multiple", "video", "https://official.example/result.mp4", false, false},
		{"poster", "image", "https://official.example/result.mp4", true, false},
		{"audio", "audio", "https://official.example/result.mp4", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := deliveryTask(t)
			got := TaskArtifactDeliveryURL(context.Background(), task, types.TaskArtifact{Key: "result", Type: tc.kind}, tc.single, &TaskArtifactDeliverySource{URL: tc.source, Method: "GET"}, "capability")
			if tc.mapped {
				want, err := PublicTaskVideoURL(task)
				require.NoError(t, err)
				require.Equal(t, want, got)
			} else {
				require.Equal(t, "capability", got)
			}
		})
	}
}

func TestTaskArtifactDeliveryArchivedStringResults(t *testing.T) {
	for _, data := range []string{
		`{"id":"upstream","status":"completed","result":"https://untrusted.example/result.mp4"}`,
		`{"data":{"result":"https://untrusted.example/result.mp4"}}`,
		`{"results":["https://untrusted.example/result.mp4"]}`,
	} {
		t.Run(data, func(t *testing.T) {
			task := deliveryTask(t)
			task.Data = []byte(data)
			before := task.PrivateData
			source := &TaskArtifactDeliverySource{URL: "https://untrusted.example/result.mp4", Method: "GET", Anonymous: true}
			want, err := PublicTaskVideoURL(task)
			require.NoError(t, err)
			require.Equal(t, want, TaskArtifactDeliveryURL(context.Background(), task, types.TaskArtifact{Key: "video", Type: "video"}, true, source, "capability"))
			// Keep exact-source and single-video guards; never map another artifact.
			require.Equal(t, "capability", TaskArtifactDeliveryURL(context.Background(), task, types.TaskArtifact{Key: "video", Type: "video"}, false, source, "capability"))
			source.URL = "https://untrusted.example/other.mp4"
			require.Equal(t, "capability", TaskArtifactDeliveryURL(context.Background(), task, types.TaskArtifact{Key: "video", Type: "video"}, true, source, "capability"))
			require.Equal(t, data, string(task.Data))
			require.Equal(t, before, task.PrivateData)
		})
	}
}

func TestTaskArtifactDeliveryKeepsAnonymousTrustedSourceOnly(t *testing.T) {
	for _, tc := range []struct {
		name, source, method     string
		anonymous, probe, direct bool
	}{
		{"official", "https://official.example/result.mp4?signature=original", "GET", true, true, true},
		{"head", "https://official.example/result.mp4", "HEAD", true, true, true},
		{"auth", "https://official.example/result.mp4", "GET", false, true, false},
		{"forbidden", "https://official.example/result.mp4", "GET", true, false, false},
		{"untrusted", "https://unknown.example/result.mp4", "GET", true, true, false},
		{"plain-http", "http://official.example/result.mp4", "GET", true, true, false},
		{"credential", "https://official.example/result.mp4?api_key=secret", "GET", true, true, false},
		{"post", "https://official.example/result.mp4", "POST", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := deliveryTask(t)
			task.PrivateData.ResultStorageKey = ""
			previous := taskVideoDirectProbe
			taskVideoDirectProbe = func(context.Context, string) (bool, error) { return tc.probe, nil }
			t.Cleanup(func() { taskVideoDirectProbe = previous })
			got := TaskArtifactDeliveryURL(context.Background(), task, types.TaskArtifact{Key: "video", Type: "video"}, true, &TaskArtifactDeliverySource{URL: tc.source, Method: tc.method, Anonymous: tc.anonymous}, "capability")
			if tc.direct {
				require.Equal(t, tc.source, got)
			} else {
				require.Equal(t, "capability", got)
			}
		})
	}
}

func TestPublicVideoPresentationKeepsSnapshotAndOriginalExpiry(t *testing.T) {
	task := deliveryTask(t)
	task.FinishTime = time.Now().Add(-35 * 24 * time.Hour).Unix()
	before := string(task.Data)
	key := task.PrivateData.ResultStorageKey
	finish := task.FinishTime
	want, err := PublicTaskVideoURL(task)
	require.NoError(t, err)
	for range 2 {
		require.Equal(t, want, TaskVideoDeliveryURL(context.Background(), task))
		payload, err := PresentPublicTaskVideo([]byte(`{"url":"old","content":{"video_url":"old"},"metadata":{"custom":"preserved"}}`), task)
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, common.Unmarshal(payload, &result))
		require.Equal(t, want, result["url"])
		require.Equal(t, "preserved", result["metadata"].(map[string]any)["custom"])
	}
	require.Equal(t, before, string(task.Data))
	require.Equal(t, key, task.PrivateData.ResultStorageKey)
	require.Equal(t, finish, task.FinishTime)
}

func TestExpiredUnarchivedPreviewCannotBeRecreated(t *testing.T) {
	task := deliveryTask(t)
	task.PrivateData.ResultStorageKey = ""
	task.FinishTime = time.Now().Add(-31 * 24 * time.Hour).Unix()
	previous := taskVideoDirectProbe
	taskVideoDirectProbe = func(context.Context, string) (bool, error) {
		t.Fatal("expired preview must not fetch or rearchive")
		return false, errors.New("unexpected")
	}
	t.Cleanup(func() { taskVideoDirectProbe = previous })
	_, _, err := PrepareTaskVideoPreviewURL(context.Background(), task)
	require.ErrorContains(t, err, "retention")
	require.Empty(t, task.PrivateData.ResultStorageKey)
}

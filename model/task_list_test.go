package model

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskListsOmitVideoDataAndPreserveSunoPreviewData(t *testing.T) {
	truncateTables(t)

	videoData := json.RawMessage(`{"steps":[{"type":"user_input","content":[{"type":"image","data":"large-base64-image"}]}]}`)
	sunoData := json.RawMessage(`[{"audio_url":"https://cdn.example/audio.mp3"}]`)
	videoTask := &Task{
		TaskID:    "task_video_list_payload",
		Platform:  constant.TaskPlatform("41"),
		UserId:    7,
		ChannelId: 41,
		Data:      videoData,
		PrivateData: TaskPrivateData{
			Key:       "secret-video-key",
			ResultURL: "https://api.example.com/v1/videos/task_video_list_payload/content",
		},
	}
	sunoTask := &Task{
		TaskID:    "task_suno_list_payload",
		Platform:  constant.TaskPlatformSuno,
		UserId:    7,
		ChannelId: 9,
		Data:      sunoData,
		PrivateData: TaskPrivateData{
			Key: "secret-suno-key",
		},
	}
	require.NoError(t, DB.Create(videoTask).Error)
	require.NoError(t, DB.Create(sunoTask).Error)

	for _, tc := range []struct {
		name  string
		load  func() []*Task
		admin bool
	}{
		{
			name:  "admin",
			load:  func() []*Task { return TaskGetAllTasks(0, 100, SyncTaskQueryParams{}) },
			admin: true,
		},
		{
			name: "user",
			load: func() []*Task {
				return TaskGetAllUserTask(7, 0, 100, SyncTaskQueryParams{})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tasks := tc.load()
			require.Len(t, tasks, 2)

			byID := make(map[int64]*Task, len(tasks))
			for _, task := range tasks {
				byID[task.ID] = task
			}

			listedVideo := byID[videoTask.ID]
			require.NotNil(t, listedVideo)
			assert.Empty(t, listedVideo.Data)
			assert.Equal(t, videoTask.GetResultURL(), listedVideo.GetResultURL())
			if tc.admin {
				assert.Equal(t, 41, listedVideo.ChannelId)
			} else {
				assert.Zero(t, listedVideo.ChannelId)
			}

			listedSuno := byID[sunoTask.ID]
			require.NotNil(t, listedSuno)
			assert.JSONEq(t, string(sunoData), string(listedSuno.Data))
		})
	}
}

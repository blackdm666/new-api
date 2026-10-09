package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useTaskVideoDirectHostsOption(t *testing.T, value *string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = map[string]string{}
	if value != nil {
		common.OptionMap[system_setting.TaskVideoDirectHostsOptionKey] = *value
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})
}

func setTaskVideoDirectHostsOption(value string) {
	common.OptionMapRWMutex.Lock()
	common.OptionMap[system_setting.TaskVideoDirectHostsOptionKey] = value
	common.OptionMapRWMutex.Unlock()
}

func TestTaskVideoDirectPolicyIsSharedByArchiveQueryAndContent(t *testing.T) {
	t.Setenv("TASK_MEDIA_PUBLIC_ENABLED", "true")
	t.Setenv("TASK_MEDIA_PUBLIC_BASE_URL", "https://assets.88api.ai/media")
	t.Setenv("TASK_VIDEO_CACHE_ENABLED", "true")
	t.Setenv("TASK_VIDEO_DIRECT_HOSTS", "*.env.example")
	useLocalTaskVideoCache(t)
	useStaticTaskVideoSource(t, "archived-video")
	previousProbe := taskVideoDirectProbe
	taskVideoDirectProbe = func(context.Context, string) (bool, error) { return true, nil }
	t.Cleanup(func() { taskVideoDirectProbe = previousProbe })
	useTaskVideoDirectHostsOption(t, nil)

	const resultURL = "https://store.vod.example/result.mp4?sign=abc"
	unarchived := func() *model.Task {
		return &model.Task{TaskID: "task_shared_policy", Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{ResultURL: resultURL}}
	}
	decide := func() (archived bool, query string, content string, contentOK bool) {
		prepared, err := PrepareTaskVideoResult(context.Background(), &model.Task{TaskID: "task_shared_policy"}, resultURL)
		require.NoError(t, err)
		task := unarchived()
		content, contentOK = TaskVideoDirectContentURL(task)
		return prepared.Cached, TaskVideoDeliveryURL(context.Background(), task), content, contentOK
	}

	archived, query, _, contentOK := decide()
	assert.True(t, archived, "unlisted host is archived")
	assert.NotEqual(t, resultURL, query)
	assert.False(t, contentOK)

	// A saved option replaces the environment list immediately; no restart.
	setTaskVideoDirectHostsOption("store.vod.example")
	archived, query, content, contentOK := decide()
	assert.False(t, archived, "listed host stays direct")
	assert.Equal(t, resultURL, query)
	require.True(t, contentOK)
	assert.Equal(t, query, content, "content redirects to the URL the query presents")

	setTaskVideoDirectHostsOption("")
	archived, query, _, contentOK = decide()
	assert.True(t, archived)
	assert.NotEqual(t, resultURL, query)
	assert.False(t, contentOK)
}

func TestTaskVideoDirectContentURLRejectsUnsafeSources(t *testing.T) {
	t.Setenv("TASK_VIDEO_DIRECT_HOSTS", "*.official.example,official.example")
	useTaskVideoDirectHostsOption(t, nil)

	for _, tc := range []struct {
		name, url string
		direct    bool
	}{
		{"exact", "https://official.example/v.mp4", true},
		{"wildcard", "https://cdn.official.example/v.mp4", true},
		{"default-port", "https://cdn.official.example:443/v.mp4", true},
		{"uppercase", "https://CDN.Official.Example/v.mp4", true},
		{"other-port", "https://cdn.official.example:8443/v.mp4", false},
		{"plain-http", "http://cdn.official.example/v.mp4", false},
		{"hyphen-bypass", "https://evil-official.example/v.mp4", false},
		{"suffix-bypass", "https://official.example.evil.test/v.mp4", false},
		{"userinfo", "https://official.example@evil.test/v.mp4", false},
		{"credential", "https://cdn.official.example/v.mp4?api_key=secret", false},
		{"fragment", "https://cdn.official.example/v.mp4#x", false},
		{"proxy", "https://gateway.example/v1/videos/task_direct_content/content", false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := &model.Task{TaskID: "task_direct_content", Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{ResultURL: tc.url}}
			got, ok := TaskVideoDirectContentURL(task)
			assert.Equal(t, tc.direct, ok)
			if tc.direct {
				assert.Equal(t, tc.url, got)
			}
		})
	}

	listed := "https://cdn.official.example/v.mp4"
	for name, task := range map[string]*model.Task{
		"archived":    {TaskID: "task_direct_content", Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{ResultURL: listed, ResultStorageKey: "task-videos/x.mp4"}},
		"in-progress": {TaskID: "task_direct_content", Status: model.TaskStatusInProgress, PrivateData: model.TaskPrivateData{ResultURL: listed}},
		"failed":      {TaskID: "task_direct_content", Status: model.TaskStatusFailure, FailReason: listed},
	} {
		_, ok := TaskVideoDirectContentURL(task)
		assert.False(t, ok, name)
	}
	_, ok := TaskVideoDirectContentURL(nil)
	assert.False(t, ok)
}

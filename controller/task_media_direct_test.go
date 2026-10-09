package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useTaskVideoDirectHostsOptionMap(t *testing.T, value *string) {
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

func requestPublicVideoContent(t *testing.T, method, taskID string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	engine := gin.New()
	engine.GET("/v1/videos/:task_id/content", PublicVideoContent)
	engine.HEAD("/v1/videos/:task_id/content", PublicVideoContent)
	request := httptest.NewRequest(method, "/v1/videos/"+taskID+"/content", nil)
	for key, values := range header {
		request.Header[key] = values
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestPublicVideoContentDeliveryOrder(t *testing.T) {
	setupGenericTaskTest(t)
	t.Setenv("TASK_MEDIA_PUBLIC_ENABLED", "true")
	t.Setenv("TASK_MEDIA_PUBLIC_BASE_URL", "https://assets.88api.ai/media")
	t.Setenv("TASK_VIDEO_DIRECT_HOSTS", "*.official.example")
	useTaskVideoDirectHostsOptionMap(t, nil)

	const officialURL = "https://cdn.official.example/result.mp4?sign=upstream"
	const hiddenURL = "https://private-upstream.invalid/result.mp4"
	storageKey := "task-videos/2026/10/" + strings.Repeat("b", 64) + ".mp4"
	archivedURL, err := service.TaskMediaPublicURL(storageKey)
	require.NoError(t, err)
	for _, task := range []*model.Task{
		{TaskID: "task_archived_direct", PrivateData: model.TaskPrivateData{ResultURL: officialURL, ResultStorageKind: "s3", ResultStorageKey: storageKey, ResultMimeType: "video/mp4"}},
		{TaskID: "task_unarchived_listed", PrivateData: model.TaskPrivateData{ResultURL: officialURL}},
		{TaskID: "task_unarchived_hidden", PrivateData: model.TaskPrivateData{ResultURL: hiddenURL}},
		{TaskID: "task_unarchived_discarded", PrivateData: model.TaskPrivateData{ResultURL: officialURL, ResultDiscarded: true}},
		{TaskID: "task_unarchived_running", Status: model.TaskStatusInProgress, PrivateData: model.TaskPrivateData{ResultURL: officialURL}},
	} {
		if task.Status == "" {
			task.Status = model.TaskStatusSuccess
		}
		task.UserId, task.ChannelId, task.Platform = 7, 1, "openai_video"
		require.NoError(t, model.DB.Create(task).Error)
	}

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, header := range []http.Header{nil, {"Range": []string{"bytes=0-1023"}}} {
			name := method
			if header != nil {
				name += "-range"
			}
			t.Run(name, func(t *testing.T) {
				archived := requestPublicVideoContent(t, method, "task_archived_direct", header)
				assert.Equal(t, http.StatusTemporaryRedirect, archived.Code)
				assert.Equal(t, archivedURL, archived.Header().Get("Location"), "archived object takes priority over a listed upstream")

				listed := requestPublicVideoContent(t, method, "task_unarchived_listed", header)
				assert.Equal(t, http.StatusTemporaryRedirect, listed.Code)
				assert.Equal(t, officialURL, listed.Header().Get("Location"))
				assert.Equal(t, "no-store", listed.Header().Get("Cache-Control"))
				assert.Equal(t, "no-referrer", listed.Header().Get("Referrer-Policy"))

				for _, taskID := range []string{"task_unarchived_hidden", "task_unarchived_discarded", "task_unarchived_running"} {
					hidden := requestPublicVideoContent(t, method, taskID, header)
					assert.Equal(t, http.StatusNotFound, hidden.Code, taskID)
					assert.Empty(t, hidden.Header().Get("Location"), taskID)
					assert.NotContains(t, hidden.Body.String(), "upstream", taskID)
					assert.NotContains(t, hidden.Body.String(), "official.example", taskID)
				}
			})
		}
	}

	// The same option drives the redirect; saving it applies without restart.
	common.OptionMapRWMutex.Lock()
	common.OptionMap[system_setting.TaskVideoDirectHostsOptionKey] = "other.example"
	common.OptionMapRWMutex.Unlock()
	assert.Equal(t, http.StatusNotFound, requestPublicVideoContent(t, http.MethodGet, "task_unarchived_listed", nil).Code)
	common.OptionMapRWMutex.Lock()
	common.OptionMap[system_setting.TaskVideoDirectHostsOptionKey] = "cdn.official.example"
	common.OptionMapRWMutex.Unlock()
	assert.Equal(t, officialURL, requestPublicVideoContent(t, http.MethodGet, "task_unarchived_listed", nil).Header().Get("Location"))
}

func TestGetOptionsShowsEffectiveTaskVideoDirectHosts(t *testing.T) {
	t.Setenv(system_setting.TaskVideoDirectHostsEnv, "*.env.example")
	readOption := func() (string, int) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/option/", nil)
		GetOptions(c)
		var response struct {
			Data []model.Option `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		value, count := "", 0
		for _, option := range response.Data {
			if option.Key == system_setting.TaskVideoDirectHostsOptionKey {
				value, count = option.Value, count+1
			}
		}
		return value, count
	}

	useTaskVideoDirectHostsOptionMap(t, nil)
	value, count := readOption()
	assert.Equal(t, 1, count)
	assert.Equal(t, "*.env.example", value)

	saved := ""
	useTaskVideoDirectHostsOptionMap(t, &saved)
	value, count = readOption()
	assert.Equal(t, 1, count)
	assert.Empty(t, value)
}

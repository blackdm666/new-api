package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type taskPreviewProbeTransport struct{ t *testing.T }

func (transport taskPreviewProbeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	assert.Equal(transport.t, "account.r2.cloudflarestorage.com", request.URL.Host)
	assert.Empty(transport.t, request.Header.Get("Authorization"))
	assert.Equal(transport.t, "bytes=0-0", request.Header.Get("Range"))
	return &http.Response{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Type": {"video/mp4"}}, Body: io.NopCloser(strings.NewReader("v")), Request: request}, nil
}

func mockTaskPreviewAnonymousProbe(t *testing.T) {
	t.Helper()
	setting := system_setting.GetFetchSetting()
	previousSetting := *setting
	setting.EnableSSRFProtection = false
	client := service.GetSSRFProtectedHTTPClient()
	if client == nil {
		service.InitHttpClient()
		client = service.GetSSRFProtectedHTTPClient()
	}
	require.NotNil(t, client)
	previousTransport := client.Transport
	client.Transport = taskPreviewProbeTransport{t}
	t.Cleanup(func() {
		client.Transport = previousTransport
		*setting = previousSetting
	})
}

func setupTaskControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousRedisEnabled := common.RedisEnabled
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedisEnabled
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})

	dsn := fmt.Sprintf("file:task-dto-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Task{}))
	model.DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	return db
}

func TestTasksToDtoAddsAdminChannelName(t *testing.T) {
	db := setupTaskControllerTestDB(t)

	require.NoError(t, db.Create(&model.User{Id: 7, Username: "task-user"}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 71, Name: "Vertex video channel"}).Error)

	adminResult := tasksToDto([]*model.Task{{UserId: 7, ChannelId: 71}}, true, common.RoleAdminUser)
	require.Len(t, adminResult, 1)
	assert.Equal(t, "task-user", adminResult[0].Username)
	assert.Equal(t, "Vertex video channel", adminResult[0].ChannelName)

	userResult := tasksToDto([]*model.Task{{UserId: 7, ChannelId: 71}}, false, common.RoleCommonUser)
	require.Len(t, userResult, 1)
	assert.Empty(t, userResult[0].Username)
	assert.Empty(t, userResult[0].ChannelName)
}

func TestGetTaskPreviewURLAllowsAdminToOpenAnotherUsersDirectResult(t *testing.T) {
	mockTaskPreviewAnonymousProbe(t)
	gin.SetMode(gin.TestMode)
	db := setupTaskControllerTestDB(t)
	task := &model.Task{
		TaskID: "task_admin_preview",
		UserId: 8,
		Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://account.r2.cloudflarestorage.com/video.mp4?X-Amz-Signature=signed",
		},
	}
	require.NoError(t, db.Create(task).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/task/task_admin_preview/preview-url", nil)
	context.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	context.Set("id", 1)
	context.Set("role", common.RoleAdminUser)

	GetTaskPreviewURL(context)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			URL       string `json:"url"`
			ExpiresIn int64  `json:"expires_in"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, task.PrivateData.ResultURL, response.Data.URL)
	assert.Zero(t, response.Data.ExpiresIn)

	userRecorder := httptest.NewRecorder()
	userContext, _ := gin.CreateTestContext(userRecorder)
	userContext.Request = httptest.NewRequest(http.MethodGet, "/api/task/task_admin_preview/preview-url", nil)
	userContext.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	userContext.Set("id", 1)
	userContext.Set("role", common.RoleCommonUser)

	GetTaskPreviewURL(userContext)

	var userResponse struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(userRecorder.Body.Bytes(), &userResponse))
	assert.False(t, userResponse.Success)
}

func TestGetTaskPreviewURLExtractsAndPersistsUpstreamR2Result(t *testing.T) {
	mockTaskPreviewAnonymousProbe(t)
	gin.SetMode(gin.TestMode)
	db := setupTaskControllerTestDB(t)
	const directURL = "https://account.r2.cloudflarestorage.com/cdn/video.mp4?X-Amz-Signature=signed"
	task := &model.Task{
		TaskID: "task_r2_in_data",
		UserId: 8,
		Status: model.TaskStatusSuccess,
		Data:   []byte(`{"video_url":"` + directURL + `"}`),
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://gateway.example/v1/videos/task_r2_in_data/content",
		},
	}
	require.NoError(t, db.Create(task).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/task/task_r2_in_data/preview-url", nil)
	context.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	context.Set("id", 1)
	context.Set("role", common.RoleAdminUser)

	GetTaskPreviewURL(context)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, directURL, response.Data.URL)

	var stored model.Task
	require.NoError(t, db.First(&stored, task.ID).Error)
	assert.Equal(t, directURL, stored.PrivateData.ResultURL)
	assert.Empty(t, stored.PrivateData.ResultStorageKey)
}

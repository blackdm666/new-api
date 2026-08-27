package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

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

	adminResult := tasksToDto([]*model.Task{{UserId: 7, ChannelId: 71}}, true)
	require.Len(t, adminResult, 1)
	assert.Equal(t, "task-user", adminResult[0].Username)
	assert.Equal(t, "Vertex video channel", adminResult[0].ChannelName)

	userResult := tasksToDto([]*model.Task{{UserId: 7, ChannelId: 71}}, false)
	require.Len(t, userResult, 1)
	assert.Empty(t, userResult[0].Username)
	assert.Empty(t, userResult[0].ChannelName)
}

func TestGetTaskPreviewURLAllowsAdminToOpenAnotherUsersDirectResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTaskControllerTestDB(t)
	task := &model.Task{
		TaskID: "task_admin_preview",
		UserId: 8,
		Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://cdn.example.com/video.mp4",
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

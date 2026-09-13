package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOpenAIVideoRouteRendersJimengTask(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousDB := model.DB
	previousDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousSQLitePath := common.SQLitePath
	previousMasterNode := common.IsMasterNode
	previousRedisEnabled := common.RedisEnabled
	common.SQLitePath = t.TempDir() + "/router-video.db"
	common.IsMasterNode = false
	common.RedisEnabled = false
	t.Setenv("SQL_DSN", "")
	require.NoError(t, model.InitDB())
	database := model.DB
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Task{}))
	t.Cleanup(func() {
		sqlDB, closeErr := database.DB()
		require.NoError(t, closeErr)
		require.NoError(t, sqlDB.Close())
		model.DB = previousDB
		common.SetDatabaseTypes(previousDatabaseType, previousLogDatabaseType)
		common.SQLitePath = previousSQLitePath
		common.IsMasterNode = previousMasterNode
		common.RedisEnabled = previousRedisEnabled
	})

	require.NoError(t, database.Create(&model.User{
		Id:          91,
		Username:    "jimeng-fetch-user",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Quota:       100,
		Group:       "default",
		AuthVersion: 1,
	}).Error)
	require.NoError(t, database.Create(&model.Token{
		Id:             1,
		UserId:         91,
		Key:            "jimengfetch",
		Status:         common.TokenStatusEnabled,
		Name:           "jimeng fetch",
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}).Error)
	require.NoError(t, database.Create(&model.Channel{
		Id:     17,
		Type:   constant.ChannelTypeJimeng,
		Key:    "unused",
		Status: common.ChannelStatusEnabled,
		Name:   "jimeng fetch",
		Models: "jimeng_vgfm_t2v_l20",
		Group:  "default",
	}).Error)

	task := &model.Task{
		CreatedAt: 1710000000,
		UpdatedAt: 1710000060,
		TaskID:    "task_jimeng_public",
		Platform:  constant.TaskPlatform("jimeng"),
		UserId:    91,
		Group:     "default",
		ChannelId: 17,
		Status:    model.TaskStatusSuccess,
		Progress:  "100%",
		PrivateData: model.TaskPrivateData{
			ResultURL: "data:video/mp4;base64,ZGF0YQ==",
		},
	}
	task.SetData(map[string]any{
		"code": 10000,
		"data": map[string]any{
			"status":    "done",
			"task_id":   "jimeng-private-1",
			"video_url": "https://cdn.example/video.mp4",
		},
		"message": "success",
	})
	require.NoError(t, database.Create(task).Error)

	engine := gin.New()
	SetVideoRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	request := httptest.NewRequest(http.MethodGet, "/v1/videos/task_jimeng_public", nil)
	request.Header.Set("Authorization", "Bearer sk-jimengfetch")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response struct {
		ID          string `json:"id"`
		Object      string `json:"object"`
		Status      string `json:"status"`
		Progress    int    `json:"progress"`
		CreatedAt   int64  `json:"created_at"`
		CompletedAt int64  `json:"completed_at"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "task_jimeng_public", response.ID)
	assert.Equal(t, "video", response.Object)
	assert.Equal(t, "completed", response.Status)
	assert.Equal(t, 100, response.Progress)
	assert.Equal(t, int64(1710000000), response.CreatedAt)
	assert.Equal(t, int64(1710000060), response.CompletedAt)
	assert.NotContains(t, recorder.Body.String(), "cdn.example")
	assert.NotContains(t, recorder.Body.String(), "jimeng-private-1")

	for _, testCase := range []struct {
		name          string
		authorization string
		query         string
		wantStatus    int
	}{
		{name: "missing credential rejected", wantStatus: http.StatusUnauthorized},
		{name: "access rejected", query: "?access=not-a-video-credential", wantStatus: http.StatusUnauthorized},
		{name: "bearer accepted", authorization: "Bearer sk-jimengfetch", wantStatus: http.StatusOK},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodGet,
				"/v1/videos/task_jimeng_public/content"+testCase.query,
				nil,
			)
			if testCase.authorization != "" {
				request.Header.Set("Authorization", testCase.authorization)
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			assert.Equal(t, testCase.wantStatus, recorder.Code, recorder.Body.String())
			if testCase.wantStatus == http.StatusOK {
				assert.Equal(t, "data", recorder.Body.String())
			}
		})
	}

	t.Run("public media is separate from authenticated task operations", func(t *testing.T) {
		t.Setenv("TASK_MEDIA_PUBLIC_ENABLED", "true")
		t.Setenv("TASK_MEDIA_PUBLIC_BASE_URL", "https://media.example/media")
		t.Setenv("TASK_VIDEO_WORKER_SECRET", strings.Repeat("s", 32))
		task.PrivateData.ResultStorageKind = "s3"
		task.PrivateData.ResultStorageKey = "task-videos/2026/09/" + strings.Repeat("a", 64) + ".mp4"
		task.PrivateData.ResultMimeType = "video/mp4"
		require.NoError(t, database.Save(task).Error)
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			r := httptest.NewRecorder()
			engine.ServeHTTP(r, httptest.NewRequest(method, "/v1/videos/"+task.TaskID+"/content", nil))
			assert.Equal(t, http.StatusTemporaryRedirect, r.Code)
			assert.Equal(t, "https://media.example/media/"+task.PrivateData.ResultStorageKey, r.Header().Get("Location"))
			assert.Equal(t, "no-store", r.Header().Get("Cache-Control"))
			assert.NotContains(t, r.Body.String(), "jimeng-private")
		}
		query := httptest.NewRequest(http.MethodGet, "/v1/videos/"+task.TaskID, nil)
		query.Header.Set("Authorization", "Bearer sk-jimengfetch")
		queryResult := httptest.NewRecorder()
		engine.ServeHTTP(queryResult, query)
		require.Equal(t, http.StatusOK, queryResult.Code, queryResult.Body.String())
		var publicResult map[string]any
		require.NoError(t, common.Unmarshal(queryResult.Body.Bytes(), &publicResult))
		assert.Equal(t, "completed", publicResult["status"])
		assert.Equal(t, "https://media.example/media/"+task.PrivateData.ResultStorageKey, publicResult["video_url"])
		assert.NotContains(t, queryResult.Body.String(), "cdn.example")
		for _, path := range []string{"/v1/videos/" + task.TaskID, "/v1/media/uploads"} {
			method := http.MethodGet
			if path == "/v1/media/uploads" {
				method = http.MethodPost
			}
			r := httptest.NewRecorder()
			engine.ServeHTTP(r, httptest.NewRequest(method, path, nil))
			assert.Equal(t, http.StatusUnauthorized, r.Code)
		}
		for _, status := range []model.TaskStatus{model.TaskStatusInProgress, model.TaskStatusFailure} {
			task.Status = status
			require.NoError(t, database.Save(task).Error)
			r := httptest.NewRecorder()
			engine.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/videos/"+task.TaskID+"/content", nil))
			assert.Equal(t, http.StatusNotFound, r.Code)
			assert.NotContains(t, r.Body.String(), status)
		}
		request := httptest.NewRequest(http.MethodPost, "/v1/media/uploads", strings.NewReader(`{"mime_type":"video/mp4","size":24,"sha256":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`))
		request.Header.Set("Authorization", "Bearer sk-jimengfetch")
		r := httptest.NewRecorder()
		engine.ServeHTTP(r, request)
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		assert.Contains(t, r.Body.String(), "https://media.example/media/reference-media/")
		assert.Contains(t, r.Body.String(), "X-Media-Upload-Token")
		assert.NotContains(t, r.Body.String(), "sk-jimengfetch")
		assert.NotContains(t, r.Body.String(), strings.Repeat("s", 32))
	})
}

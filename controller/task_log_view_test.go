package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskLogDTOSeparatesUserAdminAndRootDetails(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_public",
		Platform: "document-parser",
		Data:     []byte(`{"id":"official-public"}`),
		Properties: model.Properties{
			OriginModelName:   "private-sales-alias",
			UpstreamModelName: "provider-model",
		},
		PrivateData: model.TaskPrivateData{
			Key:            "channel-secret-canary",
			UpstreamTaskID: "upstream-private",
			NodeName:       "node-a",
			Execution: &model.TaskExecutionSnapshot{
				RequestID:   "request-public",
				RequestPath: "/v1/documents",
				TaskPlugin: &model.TaskPluginSnapshot{
					Key:     "document-parser",
					Name:    "Document Parser",
					Version: "1.2.3",
					Author: &model.TaskPluginAuthorSnapshot{
						Name: "Community Author",
						URL:  "https://plugins.example/author",
					},
					APIVersion: 1,
					Generation: 42,
				},
			},
		},
	}

	userView := tasksToDto([]*model.Task{task}, false, common.RoleCommonUser)[0]
	assert.Nil(t, userView.AdminInfo)
	assert.Nil(t, userView.RootInfo)
	require.IsType(t, model.Properties{}, userView.Properties)
	userProperties := userView.Properties.(model.Properties)
	// The owner sees the model they requested, as in their consume logs; the
	// channel-mapped upstream model can name the provider's internal model.
	assert.Empty(t, userProperties.OriginModelName)
	assert.Equal(t, "private-sales-alias", userProperties.UpstreamModelName)
	userJSON, err := common.Marshal(userView)
	require.NoError(t, err)
	assert.NotContains(t, string(userJSON), "provider-model")
	assert.NotContains(t, string(userJSON), "document-parser")

	adminView := tasksToDto([]*model.Task{task}, false, common.RoleAdminUser)[0]
	require.IsType(t, model.Properties{}, adminView.Properties)
	adminProperties := adminView.Properties.(model.Properties)
	assert.Equal(t, "private-sales-alias", adminProperties.OriginModelName)
	assert.Equal(t, "provider-model", adminProperties.UpstreamModelName)
	require.NotNil(t, adminView.AdminInfo)
	require.NotNil(t, adminView.AdminInfo.TaskPlugin)
	assert.Equal(t, "document-parser", adminView.AdminInfo.TaskPlugin.Key)
	assert.Equal(t, "Document Parser", adminView.AdminInfo.TaskPlugin.Name)
	assert.Equal(t, "1.2.3", adminView.AdminInfo.TaskPlugin.Version)
	require.NotNil(t, adminView.AdminInfo.TaskPlugin.Author)
	assert.Equal(t, "Community Author", adminView.AdminInfo.TaskPlugin.Author.Name)
	assert.Equal(t, "https://plugins.example/author", adminView.AdminInfo.TaskPlugin.Author.URL)
	assert.Equal(t, "request-public", adminView.AdminInfo.RequestID)
	assert.Equal(t, "/v1/documents", adminView.AdminInfo.RequestPath)
	assert.Nil(t, adminView.RootInfo)

	rootView := tasksToDto([]*model.Task{task}, false, common.RoleRootUser)[0]
	require.NotNil(t, rootView.AdminInfo)
	require.NotNil(t, rootView.RootInfo)
	require.NotNil(t, rootView.RootInfo.TaskPlugin)
	assert.Equal(t, 1, rootView.RootInfo.TaskPlugin.APIVersion)
	assert.Equal(t, uint64(42), rootView.RootInfo.TaskPlugin.Generation)
	assert.Equal(t, "upstream-private", rootView.RootInfo.UpstreamTaskID)
	assert.Equal(t, "node-a", rootView.RootInfo.NodeName)
	assert.Equal(t, "official-public", rootView.OfficialTaskID)
	assert.Empty(t, adminView.OfficialTaskID)

	adminJSON, err := common.Marshal(adminView)
	require.NoError(t, err)
	assert.NotContains(t, string(adminJSON), "channel-secret-canary")
	assert.NotContains(t, string(adminJSON), "upstream-private")

	rootJSON, err := common.Marshal(rootView)
	require.NoError(t, err)
	assert.NotContains(t, string(rootJSON), "channel-secret-canary")
	assert.Contains(t, string(rootJSON), "upstream-private")
}

func TestTaskLogDTODoesNotInventHistoricalPluginProvenance(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_without_snapshot",
		Platform: "document-parser",
	}

	adminView := tasksToDto([]*model.Task{task}, false, common.RoleAdminUser)[0]

	assert.Nil(t, adminView.AdminInfo)
	assert.Nil(t, adminView.RootInfo)
}

func TestTaskLogDTOReplacesLegacyVideoURLWithAvailabilityFlag(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_legacy_video",
		Platform:   "jimeng",
		Action:     constant.TaskActionTextToVideo,
		Status:     model.TaskStatusSuccess,
		FailReason: "https://private-upstream.invalid/video.mp4?signature=secret",
	}

	view := tasksToDto([]*model.Task{task}, false, common.RoleCommonUser)[0]
	assert.True(t, view.LegacyVideoAvailable)
	assert.Empty(t, view.ResultURL)
	assert.Empty(t, view.FailReason)
	encoded, err := common.Marshal(view)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-upstream.invalid")
	assert.NotContains(t, string(encoded), "result_url")
	assert.Contains(t, string(encoded), "legacy_video_available")
}

func TestTaskLogDTOKeepsFailureReasonAndDoesNotMarkPluginTaskLegacy(t *testing.T) {
	failed := &model.Task{
		TaskID:     "task_failed",
		Platform:   "jimeng",
		Action:     constant.TaskActionTextToVideo,
		Status:     model.TaskStatusFailure,
		FailReason: "provider rejected the request",
	}
	failedView := tasksToDto([]*model.Task{failed}, false, common.RoleCommonUser)[0]
	assert.Equal(t, "provider rejected the request", failedView.FailReason)
	assert.False(t, failedView.LegacyVideoAvailable)

	pluginTask := &model.Task{
		TaskID:     "task_plugin_video",
		Platform:   "community-video",
		Action:     constant.TaskActionTextToVideo,
		Status:     model.TaskStatusSuccess,
		FailReason: "https://stale-upstream.invalid/plugin-video.mp4",
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://private-upstream.invalid/plugin-video.mp4",
			Execution: &model.TaskExecutionSnapshot{
				TaskPlugin: &model.TaskPluginSnapshot{Key: "community-video"},
			},
		},
	}
	pluginView := tasksToDto([]*model.Task{pluginTask}, false, common.RoleCommonUser)[0]
	assert.False(t, pluginView.LegacyVideoAvailable)
	assert.Empty(t, pluginView.ResultURL)
	assert.Empty(t, pluginView.FailReason)
}

func TestTaskLogDTOPublishesPluginOfficialTaskIDAndHidesRouting(t *testing.T) {
	source := `
export const meta = {apiVersion: 1, key: "official-relay", name: "Official Relay", version: "1.0.0", author: {name: "Test"}, models: ["m"], fetchMode: "per_task"};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {}; }
export function officialTaskId(task) { return /^cgt-/.test(task.data && task.data.id) ? task.data.id : ""; }
`
	_, err := jsplugin.DefaultRegistry.Register(source, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = jsplugin.DefaultRegistry.Unregister("official-relay") })

	tests := []struct {
		name       string
		data       string
		officialID string
	}{
		{name: "provider task id is published", data: `{"id":"cgt-20261012-abc"}`, officialID: "cgt-20261012-abc"},
		{name: "gateway private id stays hidden while queued", data: `{"id":"task_gateway_private"}`},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			task := &model.Task{
				TaskID: "task_public", Platform: "official-relay", ChannelId: 162, Data: []byte(testCase.data),
				PrivateData: model.TaskPrivateData{UpstreamTaskID: "task_gateway_private"},
			}
			userView := tasksToDto([]*model.Task{task}, false, common.RoleCommonUser)[0]
			assert.Equal(t, testCase.officialID, userView.OfficialTaskID)
			assert.Empty(t, userView.Platform)
			assert.Zero(t, userView.ChannelId)
			encoded, err := common.Marshal(userView)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "official-relay")
			assert.NotContains(t, string(encoded), "task_gateway_private")

			adminView := tasksToDto([]*model.Task{task}, false, common.RoleAdminUser)[0]
			assert.Equal(t, "official-relay", adminView.Platform)
			assert.Equal(t, testCase.officialID, adminView.OfficialTaskID)
		})
	}
}

func TestUserTaskListLoadsSnapshotsForOfficialTaskIDs(t *testing.T) {
	source := `
export const meta = {apiVersion: 1, key: "official-list", name: "Official List", version: "1.0.0", author: {name: "Test"}, models: ["m"], fetchMode: "per_task"};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {}; }
export function officialTaskId(task) { return /^cgt-/.test(task.data && task.data.id) ? task.data.id : ""; }
`
	_, err := jsplugin.DefaultRegistry.Register(source, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = jsplugin.DefaultRegistry.Unregister("official-list") })

	task := setupGenericTaskTest(t)
	task.Platform = "official-list"
	task.Data = []byte(`{"id":"cgt-list-1","status":"succeeded"}`)
	require.NoError(t, model.DB.Save(task).Error)

	// The list query omits snapshots; the official ID must still be resolved.
	listed := model.TaskGetAllUserTask(task.UserId, 0, 10, model.SyncTaskQueryParams{})
	require.Len(t, listed, 1)
	view := tasksToDto(listed, false, common.RoleCommonUser)[0]
	assert.Equal(t, "cgt-list-1", view.OfficialTaskID)
	assert.Nil(t, view.Data)
	assert.Empty(t, view.Platform)
}

func TestTaskLogSearchMatchesOfficialTaskIDModelAndUsername(t *testing.T) {
	source := `
export const meta = {apiVersion: 1, key: "official-search", name: "Official Search", version: "1.0.0", author: {name: "Test"}, models: ["m"], fetchMode: "per_task"};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {}; }
export function officialTaskId(task) { const id = String((task.data && task.data.id) || ""); return /^[0-9]+$/.test(id) ? id : ""; }
`
	_, err := jsplugin.DefaultRegistry.Register(source, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = jsplugin.DefaultRegistry.Unregister("official-search") })

	ownTask := setupGenericTaskTest(t)
	require.NoError(t, model.DB.Model(ownTask).Update("model_name", "doc-parser").Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 8, Username: "other-owner", AffCode: "search-other", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default"}).Error)
	// The official ID is a JSON number, so it never appears quoted in the payload.
	official := &model.Task{TaskID: "task_official", Platform: "official-search", UserId: 7, ModelName: "MiniMax-Hailuo-H3", Status: model.TaskStatusSuccess, SubmitTime: 11, Data: []byte(`{"id":385412976501234}`)}
	foreign := &model.Task{TaskID: "task_foreign", Platform: "official-search", UserId: 8, ModelName: "MiniMax-Hailuo-H3", Status: model.TaskStatusSuccess, SubmitTime: 12, Data: []byte(`{"id":999000111222333}`)}
	// A payload that merely contains the digits must not match.
	lookalike := &model.Task{TaskID: "task_lookalike", Platform: "document", UserId: 7, ModelName: "doc-parser", Status: model.TaskStatusSuccess, SubmitTime: 13, Data: []byte(`{"note":"385412976501234"}`)}
	for _, task := range []*model.Task{official, foreign, lookalike} {
		require.NoError(t, model.DB.Create(task).Error)
	}

	list := func(handler gin.HandlerFunc, role int, query string) []string {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/task?"+query, nil)
		ctx.Set("id", 7)
		ctx.Set("role", role)
		handler(ctx)
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				Items []struct {
					TaskID string `json:"task_id"`
				} `json:"items"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.True(t, response.Success, recorder.Body.String())
		ids := []string{}
		for _, item := range response.Data.Items {
			ids = append(ids, item.TaskID)
		}
		return ids
	}

	assert.Equal(t, []string{"task_official"}, list(GetUserTask, common.RoleCommonUser, "task_id=385412976501234"))
	assert.Equal(t, []string{"task_generic"}, list(GetUserTask, common.RoleCommonUser, "task_id=task_generic"))
	assert.Empty(t, list(GetUserTask, common.RoleCommonUser, "task_id=999000111222333"), "another user's official ID")
	assert.Empty(t, list(GetUserTask, common.RoleCommonUser, "task_id=38541297650"), "partial official ID")
	assert.Equal(t, []string{"task_lookalike", "task_generic"}, list(GetUserTask, common.RoleCommonUser, "model_name=doc-parser"))
	assert.Equal(t, []string{"task_official"}, list(GetUserTask, common.RoleCommonUser, "model_name=MiniMax%25"))

	assert.Equal(t, []string{"task_foreign"}, list(GetAllTask, common.RoleAdminUser, "task_id=999000111222333"))
	assert.Equal(t, []string{"task_foreign"}, list(GetAllTask, common.RoleAdminUser, "username=other-owner"))
	assert.Equal(t, []string{"task_foreign", "task_official"}, list(GetAllTask, common.RoleAdminUser, "model_name=MiniMax-Hailuo-H3"))
	assert.Empty(t, list(GetAllTask, common.RoleAdminUser, "username=nobody"))
}

package model

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
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

func TestBackfillTaskModelNamesUsesRequestedModel(t *testing.T) {
	truncateTables(t)
	tasks := []*Task{
		{TaskID: "task_backfill_origin", UserId: 7, Properties: Properties{OriginModelName: "veo-3.1-fast", UpstreamModelName: "provider-veo"}},
		{TaskID: "task_backfill_upstream", UserId: 7, Properties: Properties{UpstreamModelName: "kling-v3"}},
		{TaskID: "task_backfill_none", UserId: 7},
		{TaskID: "task_backfill_kept", UserId: 7, ModelName: "already-set", Properties: Properties{OriginModelName: "other"}},
	}
	for _, task := range tasks {
		require.NoError(t, DB.Create(task).Error)
	}

	// Run twice: startup migration must be idempotent.
	require.NoError(t, backfillTaskModelNames())
	require.NoError(t, backfillTaskModelNames())

	names := map[string]string{}
	var rows []*Task
	require.NoError(t, DB.Select("task_id", "model_name").Find(&rows).Error)
	for _, row := range rows {
		names[row.TaskID] = row.ModelName
	}
	assert.Equal(t, map[string]string{
		"task_backfill_origin":   "veo-3.1-fast",
		"task_backfill_upstream": "kling-v3",
		"task_backfill_none":     "",
		"task_backfill_kept":     "already-set",
	}, names)
}

func TestTaskSearchDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var dialector gorm.Dialector
			databaseType := common.DatabaseTypeSQLite
			switch dialect {
			case "sqlite":
				dialector = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN is not configured")
				}
				dialector = mysql.Open(dsn)
				databaseType = common.DatabaseTypeMySQL
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN is not configured")
				}
				dialector = postgres.Open(dsn)
				databaseType = common.DatabaseTypePostgreSQL
			}
			db, err := gorm.Open(dialector, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "task_search_test_"}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousMain, previousLog := DB, common.MainDatabaseType(), common.LogDatabaseType()
			DB = db
			common.SetDatabaseTypes(databaseType, databaseType)
			t.Cleanup(func() {
				DB = previousDB
				common.SetDatabaseTypes(previousMain, previousLog)
				require.NoError(t, db.Migrator().DropTable(&Task{}, &TaskSearchTerm{}))
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.Migrator().DropTable(&Task{}, &TaskSearchTerm{}))
			require.NoError(t, db.AutoMigrate(&Task{}, &TaskSearchTerm{}))

			tasks := []*Task{
				{TaskID: "task_numeric_official", UserId: 7, Properties: Properties{OriginModelName: "MiniMax-Hailuo-H3"}, Data: json.RawMessage(`{"id":385412976501234}`)},
				{TaskID: "task_string_official", UserId: 7, Properties: Properties{OriginModelName: "seedance_2.0"}, Data: json.RawMessage(`{"id":"cgt-20261026-abc"}`)},
				{TaskID: "task_private_upstream", UserId: 8, Properties: Properties{UpstreamModelName: "veo-3.1"}, PrivateData: TaskPrivateData{UpstreamTaskID: "op_upstream_42"}},
			}
			for _, task := range tasks {
				require.NoError(t, db.Create(task).Error)
			}
			// Simulate rows written before the column existed, then migrate twice.
			require.NoError(t, db.Model(&Task{}).Where("1 = 1").Update("model_name", "").Error)
			require.NoError(t, db.AutoMigrate(&Task{}))
			require.NoError(t, backfillTaskModelNames())
			require.NoError(t, backfillTaskModelNames())

			candidateIDs := func(keyword string, userID int) []string {
				found, err := FindTaskSearchCandidates(keyword, userID, 10)
				require.NoError(t, err)
				ids := []string{}
				for _, task := range found {
					ids = append(ids, task.TaskID)
				}
				return ids
			}
			assert.Equal(t, []string{"task_numeric_official"}, candidateIDs("385412976501234", 7))
			assert.Equal(t, []string{"task_string_official"}, candidateIDs("cgt-20261026-abc", 0))
			assert.Equal(t, []string{"task_private_upstream"}, candidateIDs("op_upstream_42", 0))
			assert.Equal(t, []string{"task_string_official"}, candidateIDs("task_string_official", 7))
			assert.Empty(t, candidateIDs("op_upstream_42", 7), "another user's task")
			assert.Empty(t, candidateIDs("op%upstream", 0), "LIKE wildcards are literal")

			listed := func(params SyncTaskQueryParams) []string {
				ids := []string{}
				for _, task := range TaskGetAllTasks(0, 10, params) {
					ids = append(ids, task.TaskID)
				}
				assert.Equal(t, int64(len(ids)), TaskCountAllTasks(params))
				return ids
			}
			assert.Equal(t, []string{"task_private_upstream"}, listed(SyncTaskQueryParams{ModelName: "veo-3.1"}))
			assert.Equal(t, []string{"task_numeric_official"}, listed(SyncTaskQueryParams{ModelName: "MiniMax%"}))
			assert.Equal(t, []string{"task_string_official"}, listed(SyncTaskQueryParams{ModelName: "seedance_2%"}))
			assert.Empty(t, listed(SyncTaskQueryParams{ModelName: "seedance-2%"}), "underscore is not a wildcard")
			assert.Equal(t, []string{"task_string_official"}, listed(SyncTaskQueryParams{IDs: []int64{tasks[1].ID}}))
			assert.Empty(t, listed(SyncTaskQueryParams{IDs: []int64{}}))

			terms := []TaskSearchTerm{
				{TaskRowId: tasks[0].ID, Kind: TaskSearchTermKindOfficial, Value: "385412976501234"},
				{TaskRowId: tasks[2].ID, Kind: TaskSearchTermKindUpstream, Value: "op_upstream_42"},
			}
			// Writing the same terms again (every poll) must not fail or duplicate.
			require.NoError(t, insertTaskSearchTerms(terms))
			require.NoError(t, insertTaskSearchTerms([]TaskSearchTerm{terms[0], terms[1]}))
			var termCount int64
			require.NoError(t, db.Model(&TaskSearchTerm{}).Count(&termCount).Error)
			assert.Equal(t, int64(2), termCount)
			found, err := FindTaskIDsBySearchTerm("op_upstream_42", []string{TaskSearchTermKindOfficial, TaskSearchTermKindUpstream})
			require.NoError(t, err)
			assert.Equal(t, []int64{tasks[2].ID}, found)
			found, err = FindTaskIDsBySearchTerm("op_upstream_42", []string{TaskSearchTermKindOfficial})
			require.NoError(t, err)
			assert.Empty(t, found)
		})
	}
}

func TestTaskSearchTermsSyncOnWriteAndBackfillResumes(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&TaskSearchTerm{}, &Option{}))
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB.Exec("DELETE FROM task_search_terms")
		DB.Where("key IN ?", []string{TaskSearchTermsBackfillCursorOptionKey, TaskSearchTermsBackfillDoneOptionKey}).Delete(&Option{})
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, TaskSearchTermsBackfillCursorOptionKey)
		delete(common.OptionMap, TaskSearchTermsBackfillDoneOptionKey)
		common.OptionMapRWMutex.Unlock()
	})
	previousResolver := TaskSearchTermResolver
	t.Cleanup(func() { TaskSearchTermResolver = previousResolver })
	TaskSearchTermResolver = func(task *Task) []TaskSearchTerm {
		var snapshot struct {
			ID string `json:"id"`
		}
		_ = common.Unmarshal(task.Data, &snapshot)
		return []TaskSearchTerm{
			{Kind: TaskSearchTermKindOfficial, Value: snapshot.ID},
			{Kind: TaskSearchTermKindUpstream, Value: task.PrivateData.UpstreamTaskID},
		}
	}
	countTerms := func() int64 {
		var count int64
		require.NoError(t, DB.Model(&TaskSearchTerm{}).Count(&count).Error)
		return count
	}

	// Tasks written before the index existed.
	legacy := []*Task{
		{TaskID: "task_legacy_1", UserId: 7, Data: json.RawMessage(`{"id":"cgt-legacy-1"}`), PrivateData: TaskPrivateData{UpstreamTaskID: "up-legacy-1"}},
		{TaskID: "task_legacy_2", UserId: 7, Data: json.RawMessage(`{"status":"queued"}`)},
	}
	for _, task := range legacy {
		require.NoError(t, DB.Create(task).Error)
	}
	assert.False(t, TaskSearchTermsBackfilled())
	require.NoError(t, BackfillTaskSearchTerms(context.Background()))
	assert.True(t, TaskSearchTermsBackfilled())
	assert.Equal(t, int64(2), countTerms(), "empty IDs are not indexed")

	ids, err := FindTaskIDsBySearchTerm("cgt-legacy-1", []string{TaskSearchTermKindOfficial})
	require.NoError(t, err)
	assert.Equal(t, []int64{legacy[0].ID}, ids)
	ids, err = FindTaskIDsBySearchTerm("up-legacy-1", []string{TaskSearchTermKindOfficial})
	require.NoError(t, err)
	assert.Empty(t, ids, "the upstream ID is not visible as an official ID")

	// New tasks are indexed when written, including IDs that appear on a poll.
	fresh := &Task{TaskID: "task_fresh", UserId: 7, Status: TaskStatusSubmitted, Data: json.RawMessage(`{"status":"queued"}`), PrivateData: TaskPrivateData{UpstreamTaskID: "up-fresh"}}
	require.NoError(t, fresh.InsertWithContext(context.Background(), "data"))
	SyncTaskSearchTerms(fresh)
	assert.Equal(t, int64(3), countTerms())
	fresh.Data = json.RawMessage(`{"id":"cgt-fresh"}`)
	fresh.Status = TaskStatusInProgress
	won, err := fresh.UpdateWithStatus(TaskStatusSubmitted)
	require.NoError(t, err)
	require.True(t, won)
	ids, err = FindTaskIDsBySearchTerm("cgt-fresh", []string{TaskSearchTermKindOfficial})
	require.NoError(t, err)
	assert.Equal(t, []int64{fresh.ID}, ids)

	// A later run resumes after the saved cursor and never duplicates terms.
	require.NoError(t, BackfillTaskSearchTerms(context.Background()))
	assert.Equal(t, int64(4), countTerms())
}

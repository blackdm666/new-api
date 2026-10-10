package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	plugin "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	failoverQuotaBody = `{"error":{"message":"预扣费额度失败, 用户剩余额度: ＄0.01, 需要预扣费额度: ＄1.00","code":"insufficient_user_quota"}}`
	failoverBusyBody  = `{"error":{"message":"fixture unavailable"}}`
)

type failoverChannel struct {
	letter   string
	priority int64
	weight   uint
	reply    string // quota, busy or ok
}

type failoverAttempt struct {
	path, idempotencyKey string
}

type failoverResult struct {
	attempts []failoverAttempt
	outcome  *taskSubmissionOutcome
	taskErr  *dto.TaskError
	status   int
	body     string
	quota    int
	initial  int
	tasks    int64
	logs     []model.Log
}

// Submissions go through the real middleware, channel cache, plugin adaptor
// and wallet billing; each channel's provider answers with its fixed reply.
func runTaskFailover(t *testing.T, channels []failoverChannel) failoverResult {
	t.Helper()
	events := []string{}
	db := setupTaskSubmissionDatabase(t, true, &events)
	require.NoError(t, db.Callback().Create().Remove("test:task-submit-order"))
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Log{}, &model.QuotaNotificationState{}))
	oldLog, oldMemory, oldRedis, oldBatch, oldConsume, oldRetry := model.LOG_DB, common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes
	oldRanges, oldErrorLog := operation_setting.AutomaticRetryStatusCodeRanges, constant.ErrorLogEnabled
	model.LOG_DB = db
	common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes = true, false, false, true, 2
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		model.LOG_DB = oldLog
		common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes = oldMemory, oldRedis, oldBatch, oldConsume, oldRetry
		operation_setting.AutomaticRetryStatusCodeRanges, constant.ErrorLogEnabled = oldRanges, oldErrorLog
	})
	require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("100-199,300-399,401-407,409-450,452-503,505-523,525-599"))
	withTieredBillingConfig(t, map[string]string{"failover-sale": "tiered_expr"}, map[string]string{"failover-sale": `tier("base", u("seconds") * 0.1)`})

	replies := map[string]string{}
	for _, ch := range channels {
		replies["/"+ch.letter] = ch.reply
	}
	var mu sync.Mutex
	result := failoverResult{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		result.attempts = append(result.attempts, failoverAttempt{r.URL.Path, r.Header.Get("Idempotency-Key")})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch replies[r.URL.Path] {
		case "quota":
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, failoverQuotaBody)
		case "busy":
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, failoverBusyBody)
		default:
			fmt.Fprintf(w, `{"task_id":"accepted%s"}`, strings.ReplaceAll(r.URL.Path, "/", "-"))
		}
	}))
	t.Cleanup(srv.Close)

	for i, spec := range channels {
		key := "failover-provider-" + spec.letter
		source := fmt.Sprintf(`
export const meta={apiVersion:1,key:%q,name:"Failover",version:"1.0.0",author:{name:"Test"},models:[],dynamicModels:true,fetchMode:"per_task",protocols:["openai_video"],usageSchema:{seconds:{type:"number",unit:"second"}}};
export const protocols={openai_video:{decodeRequest:function(ctx){return {kind:"submit",model:ctx.model,action:"text_to_video",requestBody:ctx.body.value};},render:function(ctx,task){return task;}}};
export function buildSubmitRequest(ctx){return {url:ctx.baseUrl+%q,method:"POST",headers:{"Idempotency-Key":ctx.publicTaskId},body:{model:ctx.upstreamModel,duration:ctx.requestBody.duration}};}
export function parseSubmitResponse(ctx,resp){return {taskId:resp.body.task_id,taskData:resp.body};}
export function extractUsage(ctx){return {seconds:ctx.requestBody.duration};}
export function buildQueryRequest(){return {};}
export function parseTaskResult(){return {status:"IN_PROGRESS"};}
export function listArtifacts(){return [];}
export function buildContentRequest(){return {};}
`, key, "/"+spec.letter)
		_, err := plugin.DefaultRegistry.Register(source, plugin.Options{})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, plugin.DefaultRegistry.Unregister(key)) })
		setting := fmt.Sprintf(`{"task_plugin_key":%q}`, key)
		priority, weight := spec.priority, spec.weight
		ch := model.Channel{Id: 942001 + i, Name: key, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Models: "failover-sale", Group: "default", Key: "fixture", BaseURL: &srv.URL, Setting: &setting, Priority: &priority, Weight: &weight}
		require.NoError(t, ch.Insert())
	}
	model.InitChannelCache()

	result.initial = int(10 * common.QuotaPerUnit)
	user := model.User{Username: "failover-user", AffCode: "failover", Quota: result.initial}
	require.NoError(t, db.Create(&user).Error)
	router := gin.New()
	router.POST("/v1/videos", middleware.PinTaskPluginEndpoint(), middleware.PrepareTaskPluginEndpoint(), func(c *gin.Context) {
		c.Set("id", user.Id)
		c.Set("group", "default")
		c.Set("username", user.Username)
		info := taskSubmissionRelayInfo(nil)
		info.UserId, info.UserGroup, info.TokenGroup = user.Id, "default", "default"
		info.OriginModelName, info.IsPlayground, info.LockedChannel = "failover-sale", true, nil
		info.UserSetting.BillingPreference = "wallet_only"
		info.PublicTaskID = model.GenerateTaskID()
		result.outcome, result.taskErr = executeTaskSubmission(c, info)
		if result.taskErr != nil {
			respondTaskSubmissionError(c, result.taskErr)
			return
		}
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"failover-sale","prompt":"test","duration":5}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	result.status, result.body = w.Code, w.Body.String()

	var updated model.User
	if result.taskErr != nil {
		require.Eventually(t, func() bool { db.First(&updated, user.Id); return updated.Quota == result.initial }, time.Second, 10*time.Millisecond, "a failed submission refunds the customer")
	}
	require.NoError(t, db.First(&updated, user.Id).Error)
	result.quota = updated.Quota
	require.NoError(t, db.Model(&model.Task{}).Count(&result.tasks).Error)
	require.NoError(t, db.Where("type = ?", model.LogTypeError).Find(&result.logs).Error)
	return result
}

func (r failoverResult) paths() []string {
	paths := make([]string, 0, len(r.attempts))
	for _, attempt := range r.attempts {
		paths = append(paths, attempt.path)
	}
	return paths
}

// A channel never sees the same submission twice, so a provider holding the
// Idempotency-Key cannot answer 409.
func assertNoChannelReplay(t *testing.T, r failoverResult) {
	t.Helper()
	seen := map[failoverAttempt]bool{}
	for _, attempt := range r.attempts {
		require.NotEmpty(t, attempt.idempotencyKey)
		assert.False(t, seen[attempt], "channel %s received the same Idempotency-Key twice", attempt.path)
		seen[attempt] = true
	}
}

func assertPublicUnavailable(t *testing.T, r failoverResult, wantMessage string) {
	t.Helper()
	assert.Nil(t, r.outcome)
	require.NotNil(t, r.taskErr)
	assert.Equal(t, http.StatusServiceUnavailable, r.status, r.body)
	var response map[string]any
	require.NoError(t, common.Unmarshal([]byte(r.body), &response))
	assert.Equal(t, wantMessage, response["message"])
	for _, private := range []string{"预扣费", "剩余额度", "insufficient", "fixture unavailable", "409"} {
		assert.NotContains(t, r.body, private)
	}
	assert.Zero(t, r.tasks)
	require.NotEmpty(t, r.logs, "the customer's error log records the failure")
	for _, log := range r.logs {
		assert.Contains(t, log.Content, wantMessage)
		assert.NotContains(t, log.Content, "预扣费")
		assert.NotContains(t, log.Content, "insufficient")
	}
}

func TestTaskSubmissionQuotaFailover(t *testing.T) {
	t.Run("single channel answers 503 without retrying it", func(t *testing.T) {
		r := runTaskFailover(t, []failoverChannel{{letter: "a", priority: 10, reply: "quota"}})
		assert.Equal(t, []string{"/a"}, r.paths())
		assert.True(t, r.taskErr.UpstreamQuotaExhausted)
		assertPublicUnavailable(t, r, "服务暂时不可用，请稍后再试。")
	})

	t.Run("multiple channels fail over to one with balance", func(t *testing.T) {
		r := runTaskFailover(t, []failoverChannel{
			{letter: "a", priority: 10, weight: 1000, reply: "quota"},
			{letter: "b", priority: 10, weight: 1, reply: "ok"},
		})
		require.Nil(t, r.taskErr, r.body)
		require.NotNil(t, r.outcome)
		assert.Equal(t, http.StatusNoContent, r.status)
		assert.Equal(t, 942002, r.outcome.Task.ChannelId)
		assert.Equal(t, "accepted-b", r.outcome.Task.PrivateData.UpstreamTaskID)
		paths := r.paths()
		require.NotEmpty(t, paths)
		assert.Equal(t, "/b", paths[len(paths)-1])
		assert.LessOrEqual(t, len(paths), 2)
		assertNoChannelReplay(t, r)
		assert.Equal(t, r.initial-common.QuotaRound(0.5*common.QuotaPerUnit), r.quota)
		assert.EqualValues(t, 1, r.tasks)
	})

	t.Run("every channel out of balance answers 503", func(t *testing.T) {
		r := runTaskFailover(t, []failoverChannel{
			{letter: "a", priority: 20, reply: "quota"},
			{letter: "b", priority: 10, reply: "quota"},
		})
		assert.Equal(t, []string{"/a", "/b"}, r.paths(), "the third attempt has no channel left and is not sent")
		assertNoChannelReplay(t, r)
		assertPublicUnavailable(t, r, "服务暂时不可用，请稍后再试。")
	})

	t.Run("a busy single channel is not replayed", func(t *testing.T) {
		r := runTaskFailover(t, []failoverChannel{{letter: "a", priority: 10, reply: "busy"}})
		assert.Equal(t, []string{"/a"}, r.paths())
		assert.False(t, r.taskErr.UpstreamQuotaExhausted)
		assertPublicUnavailable(t, r, "上游服务繁忙，请稍后重试。")
	})
}

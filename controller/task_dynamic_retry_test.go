package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	plugin "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reproduces the canvas path: the decoder accepts the sales alias, then the
// mapped provider rejects auto in buildSubmitRequest, before billing or HTTP.
func TestDynamicPluginValidationMessageHTTP(t *testing.T) {
	for _, message := range []string{
		"当前模型不支持自动比例，请选择具体画幅比例。",
		"请选择具体画幅比例后再提交，例如 16:9、9:16 或 1:1。",
	} {
		t.Run(message, func(t *testing.T) {
			events := []string{}
			db := setupTaskSubmissionDatabase(t, true, &events)
			require.NoError(t, db.Callback().Create().Remove("test:task-submit-order"))
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Log{}))
			oldLog, oldMemory, oldRedis, oldBatch, oldRetry := model.LOG_DB, common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.RetryTimes
			model.LOG_DB = db
			common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.RetryTimes = true, false, false, 2
			t.Cleanup(func() {
				model.LOG_DB = oldLog
				common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.RetryTimes = oldMemory, oldRedis, oldBatch, oldRetry
			})
			key := "private-validation-provider"
			source := fmt.Sprintf(`
export const meta={apiVersion:1,key:%q,name:"Validation",version:"1.0.0",author:{name:"Test"},models:[],dynamicModels:true,fetchMode:"per_task",protocols:["openai_video"]};
export const protocols={openai_video:{decodeRequest:function(ctx){return {kind:"submit",model:ctx.model,action:"text_to_video",requestBody:ctx.body.value};},render:function(ctx,task){return task;}}};
export function buildSubmitRequest(ctx){if(ctx.upstreamModel!=="mapped-provider")throw new Error("mapping missing");throw new Error(%q);}
export function parseSubmitResponse(){return {taskId:"must-not-submit"};}
export function buildQueryRequest(){return {};}
export function parseTaskResult(){return {status:"IN_PROGRESS"};}
export function listArtifacts(){return [];}
export function buildContentRequest(){return {};}
`, key, message)
			_, err := plugin.DefaultRegistry.Register(source, plugin.Options{})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, plugin.DefaultRegistry.Unregister(key)) })
			upstreamCalls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls++
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer srv.Close()
			setting := fmt.Sprintf(`{"task_plugin_key":%q}`, key)
			mapping := `{"validation-sale":"mapped-provider"}`
			ch := model.Channel{Id: 941001, Name: key, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Models: "validation-sale", Group: "default", Key: "fixture", BaseURL: &srv.URL, Setting: &setting, ModelMapping: &mapping}
			require.NoError(t, ch.Insert())
			model.InitChannelCache()
			user := model.User{Username: "validation-user", AffCode: "validation", Quota: 123456}
			require.NoError(t, db.Create(&user).Error)
			router := gin.New()
			router.POST("/v1/videos", middleware.PinTaskPluginEndpoint(), middleware.PrepareTaskPluginEndpoint(), func(c *gin.Context) {
				c.Set("group", "default")
				c.Set("username", user.Username)
				info := taskSubmissionRelayInfo(nil)
				info.UserId, info.UserGroup, info.TokenGroup = user.Id, "default", "default"
				info.OriginModelName, info.IsPlayground, info.LockedChannel = "validation-sale", true, nil
				info.UserSetting.BillingPreference = "wallet_only"
				info.PublicTaskID = model.GenerateTaskID()
				outcome, taskErr := executeTaskSubmission(c, info)
				assert.Nil(t, outcome)
				require.NotNil(t, taskErr)
				assert.True(t, taskErr.LocalError)
				assert.Equal(t, "stop", decideTaskRetry(c, taskErr, 2).Action)
				require.Error(t, taskErr.Error)
				assert.Contains(t, taskErr.Error.Error(), "plugin "+key+"@1.0.0 hook buildSubmitRequest failed:")
				respondTaskSubmissionError(c, taskErr)
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"validation-sale","prompt":"Fixture","ratio":"auto"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			var response map[string]any
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, "plugin_request_invalid", response["code"])
			assert.Equal(t, message, response["message"])
			assert.NotContains(t, w.Body.String(), key)
			assert.NotContains(t, w.Body.String(), "buildSubmitRequest")
			assert.Zero(t, upstreamCalls)
			var updated model.User
			require.NoError(t, db.First(&updated, user.Id).Error)
			assert.Equal(t, user.Quota, updated.Quota)
			var tasks int64
			require.NoError(t, db.Model(&model.Task{}).Count(&tasks).Error)
			assert.Zero(t, tasks)
		})
	}
}

func TestTaskHookErrorKeepsDiagnosticsOutsidePublicResponse(t *testing.T) {
	engine, err := plugin.Compile(`
export function buildSubmitRequest(){throw new Error("请选择具体画幅比例。");}
export function extractUsage(){throw new Error("视频时长请输入整数秒数。");}
export function parseSubmitResponse(){throw new Error("视频服务未返回任务编号。");}
`, plugin.Options{Key: "private-provider", Version: "1.0.0"})
	require.NoError(t, err)
	for _, tc := range []struct {
		hook, message, code string
		status              int
		local               bool
	}{
		{"buildSubmitRequest", "请选择具体画幅比例。", "plugin_request_invalid", 400, true},
		{"extractUsage", "视频时长请输入整数秒数。", "plugin_usage_invalid", 400, true},
		{"parseSubmitResponse", "视频服务未返回任务编号。", "plugin_submit_response_failed", 502, false},
	} {
		t.Run(tc.hook, func(t *testing.T) {
			_, err := engine.Call(context.Background(), tc.hook)
			require.Error(t, err)
			original := fmt.Errorf("internal adapter context: %w", err)
			taskErr := service.TaskErrorWrapper(original, tc.code, tc.status)
			if tc.local {
				taskErr = service.TaskErrorWrapperLocal(original, tc.code, tc.status)
			}
			assert.Same(t, original, taskErr.Error)
			assert.Equal(t, tc.local, taskErr.LocalError)
			var hookErr *plugin.HookError
			require.True(t, errors.As(taskErr.Error, &hookErr))
			assert.Equal(t, tc.hook, hookErr.Hook)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			respondTaskError(c, taskErr)
			assert.Equal(t, tc.status, w.Code)
			var response map[string]any
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, tc.code, response["code"])
			assert.Equal(t, tc.message, response["message"])
			assert.NotContains(t, w.Body.String(), "private-provider")
			assert.NotContains(t, w.Body.String(), "internal adapter context")
		})
	}
	// Do not parse plain error strings as JS stacks or rewrite provider details.
	plain := errors.New("ordinary provider rejection")
	assert.Equal(t, plain.Error(), service.TaskErrorWrapper(plain, "provider_error", 422).Message)
}

// Exercises real middleware, distribution, HTTP adapters, wallet settlement,
// and durable task identity across two differently mapped dynamic plugins.
func TestDynamicPluginHTTPRetryAndBilling(t *testing.T) {
	for _, tc := range []struct {
		status int
		actual bool
	}{{503, false}, {422, false}, {503, true}, {422, true}} {
		t.Run(fmt.Sprintf("%d/actual=%v", tc.status, tc.actual), func(t *testing.T) {
			status := tc.status
			events := []string{}
			db := setupTaskSubmissionDatabase(t, true, &events)
			require.NoError(t, db.Callback().Create().Remove("test:task-submit-order"))
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Log{}, &model.QuotaNotificationState{}))
			oldLog, oldMemory, oldRedis, oldBatch, oldConsume, oldRetry := model.LOG_DB, common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes
			oldRanges := operation_setting.AutomaticRetryStatusCodeRanges
			model.LOG_DB = db
			common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes = true, false, false, true, 1
			t.Cleanup(func() {
				model.LOG_DB = oldLog
				common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes = oldMemory, oldRedis, oldBatch, oldConsume, oldRetry
				operation_setting.AutomaticRetryStatusCodeRanges = oldRanges
			})
			require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("503"))
			withTieredBillingConfig(t, map[string]string{"dynamic-sale": "tiered_expr"}, map[string]string{"dynamic-sale": `tier("base", u("seconds") * 0.1)`})
			attempts := []string{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				require.NoError(t, common.DecodeJson(r.Body, &body))
				attempts = append(attempts, r.URL.Path)
				expected := "upstream-" + strings.TrimPrefix(r.URL.Path, "/")
				if tc.actual {
					expected = "minimax-h3-768p"
					if r.URL.Path == "/v2/video_generation" {
						expected = "MiniMax-H3"
					}
				}
				require.Equal(t, expected, body["model"])
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/a" || r.URL.Path == "/v1/videos/generations" {
					w.WriteHeader(status)
					fmt.Fprint(w, `{"error":{"message":"fixture unavailable"}}`)
					return
				}
				require.EqualValues(t, 5, body["duration"])
				fmt.Fprint(w, `{"task_id":"accepted-b"}`)
			}))
			defer srv.Close()
			for i, key := range []string{"dynamic-http-a", "dynamic-http-b"} {
				letter := string(rune('a' + i))
				source := fmt.Sprintf(`
export const meta={apiVersion:1,key:%q,name:"Dynamic",version:"1.0.0",author:{name:"Test"},models:[],dynamicModels:true,fetchMode:"per_task",protocols:["openai_video"],usageSchema:{seconds:{type:"number",unit:"second"}}};
export const protocols={openai_video:{decodeRequest:function(ctx){return {kind:"submit",model:ctx.model,action:"text_to_video",requestBody:ctx.body.value};},render:function(ctx,task){return task;}}};
export function buildSubmitRequest(ctx){return {url:ctx.baseUrl+%q,method:"POST",body:{model:ctx.upstreamModel,duration:ctx.requestBody.duration}};}
export function parseSubmitResponse(ctx,resp){return {taskId:resp.body.task_id,taskData:resp.body};}
export function extractUsage(ctx){return {seconds:ctx.requestBody.duration};}
export function buildQueryRequest(){return {};}
export function parseTaskResult(){return {status:"IN_PROGRESS"};}
export function listArtifacts(){return [];}
export function buildContentRequest(){return {};}
`, key, "/"+letter)
				if tc.actual {
					folder, oldKey := "xm-video", "xm-video"
					if i == 1 {
						folder, oldKey = "minimax-h3", "minimax-h3"
					}
					raw, err := os.ReadFile("../plugins/tasks/" + folder + "/plugin.js")
					require.NoError(t, err)
					source = strings.Replace(string(raw), `key: "`+oldKey+`"`, `key: "`+key+`"`, 1)
				}
				_, err := plugin.DefaultRegistry.Register(source, plugin.Options{})
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, plugin.DefaultRegistry.Unregister(key)) })
				setting := fmt.Sprintf(`{"task_plugin_key":%q}`, key)
				mapping := fmt.Sprintf(`{"dynamic-sale":"upstream-%s"}`, letter)
				if tc.actual {
					mapping = `{"dynamic-sale":"minimax-h3-768p"}`
					if i == 1 {
						mapping = `{"dynamic-sale":"dmc-minimax-h3"}`
					}
				}
				priority := int64(20 - i*10)
				ch := model.Channel{Id: 940001 + i, Name: key, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Models: "dynamic-sale", Group: "default", Key: "fixture", BaseURL: &srv.URL, Setting: &setting, ModelMapping: &mapping, Priority: &priority}
				require.NoError(t, ch.Insert())
			}
			model.InitChannelCache()
			initial := int(10 * common.QuotaPerUnit)
			user := model.User{Username: "dynamic-retry", AffCode: "retry", Quota: initial}
			require.NoError(t, db.Create(&user).Error)
			router := gin.New()
			router.POST("/v1/videos", middleware.PinTaskPluginEndpoint(), middleware.PrepareTaskPluginEndpoint(), func(c *gin.Context) {
				c.Set("group", "default")
				c.Set("username", user.Username)
				info := taskSubmissionRelayInfo(nil)
				info.UserId = user.Id
				info.UserGroup = "default"
				info.TokenGroup = "default"
				info.OriginModelName = "dynamic-sale"
				info.IsPlayground = true
				info.UserSetting.BillingPreference = "wallet_only"
				info.LockedChannel = nil
				info.PublicTaskID = model.GenerateTaskID()
				outcome, taskErr := executeTaskSubmission(c, info)
				if status == 503 {
					require.Nil(t, taskErr)
					require.NotNil(t, outcome)
					require.Equal(t, 940002, outcome.Task.ChannelId)
					require.Equal(t, "accepted-b", outcome.Task.PrivateData.UpstreamTaskID)
					expectedModel := "upstream-b"
					if tc.actual {
						expectedModel = "dmc-minimax-h3"
					}
					require.Equal(t, expectedModel, outcome.Task.Properties.UpstreamModelName)
					require.Equal(t, "dynamic-sale", outcome.Task.Properties.OriginModelName)
					require.Equal(t, common.QuotaRound(0.5*common.QuotaPerUnit), outcome.Task.Quota)
				} else {
					require.NotNil(t, taskErr)
					require.Nil(t, outcome)
				}
				c.Status(204)
			})
			req := httptest.NewRequest("POST", "/v1/videos", strings.NewReader(`{"model":"dynamic-sale","prompt":"test","duration":5}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, 204, w.Code, w.Body.String())
			var updated model.User
			require.NoError(t, db.First(&updated, user.Id).Error)
			var count int64
			require.NoError(t, db.Model(&model.Task{}).Count(&count).Error)
			if status == 503 {
				expectedAttempts := []string{"/a", "/b"}
				if tc.actual {
					expectedAttempts = []string{"/v1/videos/generations", "/v2/video_generation"}
				}
				require.Equal(t, expectedAttempts, attempts)
				require.Equal(t, initial-common.QuotaRound(0.5*common.QuotaPerUnit), updated.Quota)
				require.EqualValues(t, 1, count)
			} else {
				expectedAttempts := []string{"/a"}
				if tc.actual {
					expectedAttempts = []string{"/v1/videos/generations"}
				}
				require.Equal(t, expectedAttempts, attempts)
				require.Eventually(t, func() bool { db.First(&updated, user.Id); return initial == updated.Quota }, time.Second, 10*time.Millisecond)
				require.Zero(t, count)
			}
		})
	}
}

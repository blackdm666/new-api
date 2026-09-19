package controller

import (
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
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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

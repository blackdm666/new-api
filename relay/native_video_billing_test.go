package relay

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel"
	geminitask "github.com/QuantumNous/new-api/relay/channel/task/gemini"
	groktask "github.com/QuantumNous/new-api/relay/channel/task/sub2api"
	vertextask "github.com/QuantumNous/new-api/relay/channel/task/vertex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeVideoExpressionPreservesRequestAndQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, model, upstream, body, expression string
		adaptor                                 channel.TaskAdaptor
		unitPrice                               float64
	}{
		{"vertex veo 720", "veo-3.1", "veo-3.1-generate-001", `{"duration":4,"metadata":{"resolution":"720p"}}`, `u("resolution") == "4k" ? tier("4k", u("seconds") * 0.375) : tier("base", u("seconds") * 0.25)`, &vertextask.TaskAdaptor{}, 0.25},
		{"vertex veo 4k", "veo-3.1", "veo-3.1-generate-001", `{"seconds":"8","metadata":{"resolution":"4K"}}`, `u("resolution") == "4k" ? tier("4k", u("seconds") * 0.375) : tier("base", u("seconds") * 0.25)`, &vertextask.TaskAdaptor{}, 0.25},
		{"vertex fast 4k", "veo-3.1-fast", "veo-3.1-fast-generate-001", `{"duration":4,"metadata":{"resolution":"4k"}}`, `u("resolution") == "4k" ? tier("4k", u("seconds") * 0.2333333) : tier("base", u("seconds") * 0.1)`, &vertextask.TaskAdaptor{}, 0.1},
		{"gemini veo default", "veo-3.1", "veo-3.1-generate-001", `{}`, `u("resolution") == "4k" ? tier("4k", u("seconds") * 0.375) : tier("base", u("seconds") * 0.25)`, &geminitask.TaskAdaptor{}, 0.25},
		{"gemini metadata duration", "veo-3.1", "veo-3.1-generate-001", `{"duration":4,"metadata":{"durationSeconds":6,"resolution":"1080p"}}`, `tier("base", u("seconds") * 0.25)`, &geminitask.TaskAdaptor{}, 0.25},
		{"vertex omni default", "gemini-omni-flash", "gemini-omni-flash-preview", `{}`, `tier("base", u("seconds") * 0.2)`, &vertextask.TaskAdaptor{}, 0.2},
		{"gemini omni", "gemini-omni-flash", "gemini-omni-flash-preview", `{"duration":10}`, `tier("base", u("seconds") * 0.2)`, &geminitask.TaskAdaptor{}, 0.2},
		{"grok base", "grok-imagine-video", "grok-imagine-video", `{"duration":4}`, `tier("base", u("seconds") * 0.1)`, &groktask.TaskAdaptor{}, 0.1},
		{"grok 1.5 default", "grok-imagine-video-1.5", "grok-imagine-video-1.5", `{}`, `tier("base", u("seconds") * 0.2)`, &groktask.TaskAdaptor{}, 0.2},
		{"grok mapped 1080", "grok-imagine-video-1.5-1080p", "grok-imagine-video-1.5", `{"duration":15}`, `tier("base", u("seconds") * 0.3)`, &groktask.TaskAdaptor{}, 0.3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saveBillingConfig(t)
			var payload map[string]any
			require.NoError(t, common.Unmarshal([]byte(tc.body), &payload))
			payload["model"], payload["prompt"] = tc.model, "A simple video."
			body, err := common.Marshal(payload)
			require.NoError(t, err)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(string(body)))
			ctx.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
			info := &relaycommon.RelayInfo{OriginModelName: tc.model, TaskRelayInfo: &relaycommon.TaskRelayInfo{},
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.upstream}}
			require.Nil(t, tc.adaptor.ValidateRequestAndSetAction(ctx, info))
			legacyRatios := tc.adaptor.EstimateBilling(ctx, info)
			before, err := tc.adaptor.BuildRequestBody(ctx, info)
			require.NoError(t, err)
			beforeBody, err := io.ReadAll(before)
			require.NoError(t, err)
			provider, ok := tc.adaptor.(channel.TaskUsageFactsProvider)
			require.True(t, ok, "native adapter must expose expression usage")
			facts := provider.ExtractUsageFacts(ctx, info)
			require.Equal(t, legacyRatios["seconds"], facts["seconds"])
			require.NoError(t, billing_setting.SmokeTestTaskExpr(tc.expression, billing_setting.NativeVideoUsageSchema(tc.model)))
			require.NoError(t, model.ValidateModelPricing(tc.model, model.PricingValues{
				"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": tc.expression,
			}))
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_mode": fmt.Sprintf(`{%q:"tiered_expr"}`, tc.model),
			}))
			for _, ratio := range []float64{1, 0.5, 0.001} {
				oldCost := tc.unitPrice * common.QuotaPerUnit * ratio
				for _, multiplier := range legacyRatios {
					oldCost *= multiplier
				}
				snapshot := &billingexpr.BillingSnapshot{ExprString: tc.expression, ExprHash: billingexpr.ExprHashString(tc.expression),
					ExprVersion: 1, TaskUsageBilling: true, GroupRatio: ratio, QuotaPerUnit: common.QuotaPerUnit, UsageFacts: facts}
				result, keptFacts, err := service.EvaluateTaskCompletionUsage(snapshot, nil)
				require.NoError(t, err)
				// Expression settlement uses nearest-quota rounding, while the old
				// fixed-price path truncated. Preserve the price within one quota.
				assert.InDelta(t, oldCost, float64(result.ActualQuotaAfterGroup), 1)
				assert.Equal(t, facts, keptFacts)
			}
			after, err := tc.adaptor.BuildRequestBody(ctx, info)
			require.NoError(t, err)
			afterBody, err := io.ReadAll(after)
			require.NoError(t, err)
			assert.JSONEq(t, string(beforeBody), string(afterBody), "billing must not change upstream payload")
		})
	}
}

func TestNativeVideoExpressionRejectsUnknownUsage(t *testing.T) {
	require.Error(t, model.ValidateModelPricing("veo-3.1", model.PricingValues{
		"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": `tier("base", u("duration_typo") * 0.2)`,
	}))
	require.Nil(t, billing_setting.NativeVideoUsageSchema("gpt-image-2"))
}

func TestNativeVideoExpressionRunsThroughSubmit(t *testing.T) {
	service.InitHttpClient()
	for _, tc := range []struct {
		model, mapping, response string
		channelType              int
	}{
		{"grok-imagine-video", "", `{"request_id":"native-upstream-task"}`, constant.ChannelTypeSub2API},
		{"veo-3.1", `{"veo-3.1":"veo-3.1-generate-001"}`, `{"name":"models/veo-3.1/operations/native-upstream-task"}`, constant.ChannelTypeGemini},
	} {
		t.Run(tc.model, func(t *testing.T) {
			saveBillingConfig(t)
			expr := `tier("base", u("seconds") * 0.1)`
			exprs, err := common.Marshal(map[string]string{tc.model: expr})
			require.NoError(t, err)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_mode": fmt.Sprintf(`{%q:"tiered_expr"}`, tc.model),
				"billing_setting.billing_expr": string(exprs),
			}))
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer upstream.Close()
			ctx, info := newTaskSubmitContext(t, tc.model, tc.mapping)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(fmt.Sprintf(`{"model":%q,"prompt":"test","duration":4}`, tc.model)))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Set("task_request", relaycommon.TaskSubmitReq{Model: tc.model, Prompt: "test", Duration: 4})
			common.SetContextKey(ctx, constant.ContextKeyChannelType, tc.channelType)
			common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, upstream.URL)
			common.SetContextKey(ctx, constant.ContextKeyChannelKey, "fixture-only")
			info.OriginModelName, info.UsingGroup, info.UserGroup = tc.model, "default", "default"
			info.Billing = &imageReservation{held: 1000000, limit: 1000000}
			t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
			result, taskErr := RelayTaskSubmit(ctx, info)
			require.Nil(t, taskErr)
			require.NotNil(t, result)
			require.NotNil(t, info.TieredBillingSnapshot)
			assert.Equal(t, 1, calls)
			assert.Equal(t, int(0.4*common.QuotaPerUnit), result.Quota)
			assert.Equal(t, float64(4), info.TieredBillingSnapshot.UsageFacts["seconds"])
		})
	}
}

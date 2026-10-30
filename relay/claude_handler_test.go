package relay

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

type claudeUpstreamReply struct {
	status      int
	contentType string
	body        string
}

const (
	claudeAdaptiveRequiredBody   = `{"type":"error","error":{"type":"invalid_request_error","message":"claude-opus-5-5 requires adaptive thinking; omit thinking or use thinking.type=adaptive and output_config.effort"}}`
	claudeShortAdaptiveBody      = `{"type":"error","error":{"type":"invalid_request_error","message":"thinking.type: omit thinking or use type adaptive"}}`
	claudeAnthropicAdaptiveBody  = `{"type":"error","error":{"type":"invalid_request_error","message":"\"thinking.type.enabled\" is not supported for this model. Use \"thinking.type.adaptive\" and \"output_config.effort\" to control thinking behavior."}}`
	claudeMaskedAdaptiveBody     = `{"type":"error","error":{"type":"invalid_request_error","message":"\"***.***.enabled\" is not supported for this model. Use \"***.***.adaptive\" and \"output_config.effort\" to control thinking behavior. (request id: 20261030)"}}`
	claudeAdaptiveRejectedBody   = `{"type":"error","error":{"type":"invalid_request_error","message":"thinking: Input tag 'adaptive' found using 'type' does not match any of the expected tags: 'disabled', 'enabled'"}}`
	claudeUnrelatedBadRequest    = `{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens: 999999 > 128000, which is the maximum allowed number of output tokens"}}`
	claudeMessageResponseBody    = `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2}}`
	claudeMessageStreamResponse  = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-opus-5-5\",\"content\":[],\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	claudeManualThinkingRequest  = `{"model":"%s","max_tokens":4000,"stream":%t,"thinking":{"type":"enabled","budget_tokens":2048},"messages":[{"role":"user","content":"hello"}]}`
	claudeThinkingCompatChannel  = 108
	claudeThinkingCompatOpusName = "claude-opus-5-5"
)

var (
	claudeRequiredReply   = claudeUpstreamReply{status: http.StatusBadRequest, body: claudeAdaptiveRequiredBody}
	claudeRejectedReply   = claudeUpstreamReply{status: http.StatusBadRequest, body: claudeAdaptiveRejectedBody}
	claudeMessageReply    = claudeUpstreamReply{status: http.StatusOK, body: claudeMessageResponseBody}
	claudeStreamReply     = claudeUpstreamReply{status: http.StatusOK, contentType: "text/event-stream", body: claudeMessageStreamResponse}
	claudeUnrelatedReply  = claudeUpstreamReply{status: http.StatusBadRequest, body: claudeUnrelatedBadRequest}
	claudeAnthropicReply  = claudeUpstreamReply{status: http.StatusBadRequest, body: claudeAnthropicAdaptiveBody}
	claudeMaskedReply     = claudeUpstreamReply{status: http.StatusBadRequest, body: claudeMaskedAdaptiveBody}
	claudeShortAdaptive   = claudeUpstreamReply{status: http.StatusBadRequest, body: claudeShortAdaptiveBody}
	claudeReplyContentRaw = "application/json"
)

// startClaudeUpstream serves the scripted replies in order and returns the
// request bodies it received.
func startClaudeUpstream(t *testing.T, replies []claudeUpstreamReply) (string, func() [][]byte) {
	t.Helper()
	var (
		mu       sync.Mutex
		received [][]byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		mu.Lock()
		index := len(received)
		received = append(received, body)
		mu.Unlock()
		if index >= len(replies) {
			http.Error(w, "unexpected upstream call", http.StatusTeapot)
			return
		}
		reply := replies[index]
		contentType := reply.contentType
		if contentType == "" {
			contentType = claudeReplyContentRaw
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(reply.status)
		_, _ = io.WriteString(w, reply.body)
	}))
	t.Cleanup(server.Close)
	return server.URL, func() [][]byte {
		mu.Lock()
		defer mu.Unlock()
		return append([][]byte(nil), received...)
	}
}

// newClaudeThinkingCompatRelay prepares the context and relay state that the
// controller and ClaudeHelper build before sending a native Claude request.
func newClaudeThinkingCompatRelay(t *testing.T, upstreamURL string, channelType int, otherSettings dto.ChannelOtherSettings, modelName string, stream bool) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo, *dto.ClaudeRequest) {
	t.Helper()
	body := fmt.Sprintf(claudeManualThinkingRequest, modelName, stream)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	common.SetContextKey(c, constant.ContextKeyChannelId, claudeThinkingCompatChannel)
	common.SetContextKey(c, constant.ContextKeyChannelType, channelType)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstreamURL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, otherSettings)

	request := &dto.ClaudeRequest{}
	require.NoError(t, common.UnmarshalJsonStr(body, request))
	info := relaycommon.GenRelayInfoClaude(c, request)
	info.InitChannelMeta(c)
	return c, recorder, info, request
}

func TestSendClaudeRequestThinkingCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	t.Cleanup(claudeAdaptiveSupportCache.Purge)
	previousStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() { constant.StreamingTimeout = previousStreamingTimeout })

	for _, tc := range []struct {
		name          string
		channelType   int
		settings      dto.ChannelOtherSettings
		model         string
		cached        claudeAdaptiveSupport
		stream        bool
		responseBytes bool
		replies       []claudeUpstreamReply
		wantThinking  []string
		wantStatus    int
		wantCached    claudeAdaptiveSupport
		wantReason    string
	}{
		{
			name:         "auto rewrites and retries once after the adaptive-required 400",
			channelType:  constant.ChannelTypeAnthropic,
			replies:      []claudeUpstreamReply{claudeRequiredReply, claudeMessageReply},
			wantThinking: []string{"enabled", "adaptive"},
			wantCached:   claudeAdaptiveRequired,
			wantReason:   policyReasonClaudeAdaptiveRequired,
		},
		{
			name:         "auto retries streaming requests before the stream starts",
			channelType:  constant.ChannelTypeAnthropic,
			stream:       true,
			replies:      []claudeUpstreamReply{claudeRequiredReply, claudeStreamReply},
			wantThinking: []string{"enabled", "adaptive"},
			wantCached:   claudeAdaptiveRequired,
			wantReason:   policyReasonClaudeAdaptiveRequired,
		},
		{
			name:         "auto applies to Sub2API channels",
			channelType:  constant.ChannelTypeSub2API,
			replies:      []claudeUpstreamReply{claudeRequiredReply, claudeMessageReply},
			wantThinking: []string{"enabled", "adaptive"},
			wantCached:   claudeAdaptiveRequired,
			wantReason:   policyReasonClaudeAdaptiveRequired,
		},
		{
			name:         "Anthropic adaptive-only wording also triggers the retry",
			channelType:  constant.ChannelTypeAnthropic,
			replies:      []claudeUpstreamReply{claudeAnthropicReply, claudeMessageReply},
			wantThinking: []string{"enabled", "adaptive"},
			wantCached:   claudeAdaptiveRequired,
			wantReason:   policyReasonClaudeAdaptiveRequired,
		},
		{
			name:         "the Anthropic wording masked by a New API upstream also triggers the retry",
			channelType:  constant.ChannelTypeNewAPI,
			model:        "claude-opus-5",
			replies:      []claudeUpstreamReply{claudeMaskedReply, claudeMessageReply},
			wantThinking: []string{"enabled", "adaptive"},
			wantCached:   claudeAdaptiveRequired,
			wantReason:   policyReasonClaudeAdaptiveRequired,
		},
		{
			name:         "the short omit-thinking wording also triggers the retry",
			channelType:  constant.ChannelTypeAnthropic,
			replies:      []claudeUpstreamReply{claudeShortAdaptive, claudeMessageReply},
			wantThinking: []string{"enabled", "adaptive"},
			wantCached:   claudeAdaptiveRequired,
			wantReason:   policyReasonClaudeAdaptiveRequired,
		},
		{
			name:         "a remembered adaptive requirement rewrites up front",
			channelType:  constant.ChannelTypeAnthropic,
			cached:       claudeAdaptiveRequired,
			replies:      []claudeUpstreamReply{claudeMessageReply},
			wantThinking: []string{"adaptive"},
			wantCached:   claudeAdaptiveRequired,
		},
		{
			name:         "a rewritten request falls back once when adaptive is rejected",
			channelType:  constant.ChannelTypeAnthropic,
			cached:       claudeAdaptiveRequired,
			replies:      []claudeUpstreamReply{claudeRejectedReply, claudeMessageReply},
			wantThinking: []string{"adaptive", "enabled"},
			wantCached:   claudeAdaptiveUnsupported,
			wantReason:   policyReasonClaudeAdaptiveUnsupported,
		},
		{
			name:         "always rewrites adaptive-only models up front",
			channelType:  constant.ChannelTypeAnthropic,
			settings:     dto.ChannelOtherSettings{ClaudeThinkingCompatMode: dto.ClaudeThinkingCompatModeAlways},
			replies:      []claudeUpstreamReply{claudeMessageReply},
			wantThinking: []string{"adaptive"},
		},
		{
			name:         "the deprecated boolean opt-in behaves as always",
			channelType:  constant.ChannelTypeAnthropic,
			settings:     dto.ChannelOtherSettings{ClaudeAdaptiveThinkingCompatibility: true},
			replies:      []claudeUpstreamReply{claudeMessageReply},
			wantThinking: []string{"adaptive"},
		},
		{
			name:         "always falls back when adaptive is rejected",
			channelType:  constant.ChannelTypeAnthropic,
			settings:     dto.ChannelOtherSettings{ClaudeThinkingCompatMode: dto.ClaudeThinkingCompatModeAlways},
			replies:      []claudeUpstreamReply{claudeRejectedReply, claudeMessageReply},
			wantThinking: []string{"adaptive", "enabled"},
			wantCached:   claudeAdaptiveUnsupported,
			wantReason:   policyReasonClaudeAdaptiveUnsupported,
		},
		{
			name:         "a remembered adaptive rejection overrides always",
			channelType:  constant.ChannelTypeAnthropic,
			settings:     dto.ChannelOtherSettings{ClaudeThinkingCompatMode: dto.ClaudeThinkingCompatModeAlways},
			cached:       claudeAdaptiveUnsupported,
			replies:      []claudeUpstreamReply{claudeMessageReply},
			wantThinking: []string{"enabled"},
			wantCached:   claudeAdaptiveUnsupported,
		},
		{
			name:         "off never rewrites or retries",
			channelType:  constant.ChannelTypeAnthropic,
			settings:     dto.ChannelOtherSettings{ClaudeThinkingCompatMode: dto.ClaudeThinkingCompatModeOff, ClaudeAdaptiveThinkingCompatibility: true},
			cached:       claudeAdaptiveRequired,
			replies:      []claudeUpstreamReply{claudeRequiredReply},
			wantThinking: []string{"enabled"},
			wantStatus:   http.StatusBadRequest,
			wantCached:   claudeAdaptiveRequired,
		},
		{
			name:         "an unrelated 400 is returned without a retry",
			channelType:  constant.ChannelTypeAnthropic,
			replies:      []claudeUpstreamReply{claudeUnrelatedReply},
			wantThinking: []string{"enabled"},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:          "no retry once response bytes reached the client",
			channelType:   constant.ChannelTypeAnthropic,
			stream:        true,
			responseBytes: true,
			replies:       []claudeUpstreamReply{claudeRequiredReply},
			wantThinking:  []string{"enabled"},
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:         "the retry happens at most once",
			channelType:  constant.ChannelTypeAnthropic,
			replies:      []claudeUpstreamReply{claudeRequiredReply, claudeRequiredReply},
			wantThinking: []string{"enabled", "adaptive"},
			wantStatus:   http.StatusBadRequest,
			wantCached:   claudeAdaptiveRequired,
			wantReason:   policyReasonClaudeAdaptiveRequired,
		},
		{
			name:         "a model without an adaptive rendering is not resent",
			channelType:  constant.ChannelTypeAnthropic,
			model:        "claude-sonnet-4-5",
			replies:      []claudeUpstreamReply{claudeRequiredReply},
			wantThinking: []string{"enabled"},
			wantStatus:   http.StatusBadRequest,
			wantCached:   claudeAdaptiveRequired,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claudeAdaptiveSupportCache.Purge()
			modelName := tc.model
			if modelName == "" {
				modelName = claudeThinkingCompatOpusName
			}
			if tc.cached != 0 {
				claudeAdaptiveSupportCache.Set(claudeAdaptiveSupportKey(claudeThinkingCompatChannel, modelName), tc.cached)
			}
			upstreamURL, received := startClaudeUpstream(t, tc.replies)
			c, recorder, info, request := newClaudeThinkingCompatRelay(t, upstreamURL, tc.channelType, tc.settings, modelName, tc.stream)
			if tc.responseBytes {
				_, err := c.Writer.WriteString(": PING\n\n")
				require.NoError(t, err)
			}
			adaptor := GetAdaptor(info.ApiType)
			require.NotNil(t, adaptor)
			adaptor.Init(info)

			httpResp, closeBodies, apiErr := sendClaudeRequest(c, info, adaptor, request)
			defer closeBodies()

			bodies := received()
			require.Len(t, bodies, len(tc.wantThinking))
			for i, want := range tc.wantThinking {
				assert.Equal(t, want, gjson.GetBytes(bodies[i], "thinking.type").String(), "upstream call %d", i+1)
				if want == "adaptive" {
					assert.Equal(t, "medium", gjson.GetBytes(bodies[i], "output_config.effort").String(), "budget 2048 maps to medium effort")
					assert.False(t, gjson.GetBytes(bodies[i], "thinking.budget_tokens").Exists())
				} else {
					assert.Equal(t, int64(2048), gjson.GetBytes(bodies[i], "thinking.budget_tokens").Int(), "the client's manual controls are sent unchanged")
				}
			}

			support, found, err := claudeAdaptiveSupportCache.Get(claudeAdaptiveSupportKey(claudeThinkingCompatChannel, modelName))
			require.NoError(t, err)
			if tc.wantCached == 0 {
				assert.False(t, found)
			} else {
				assert.Equal(t, tc.wantCached, support)
			}

			var retries []service.PolicyEvent
			for _, event := range service.RequestPolicy(c).Events() {
				if event.Decision.Action == "retry" {
					retries = append(retries, event)
				}
			}
			if tc.wantReason == "" {
				assert.Empty(t, retries)
			} else {
				require.Len(t, retries, 1)
				assert.Equal(t, service.PolicyDecision{Action: "retry", Reason: tc.wantReason, Source: "same_channel"}, retries[0].Decision)
				assert.Equal(t, claudeThinkingCompatChannel, retries[0].ChannelID)
				assert.Equal(t, http.StatusBadRequest, retries[0].Status)
			}

			if tc.wantStatus != 0 {
				require.NotNil(t, apiErr)
				assert.Nil(t, httpResp)
				assert.Equal(t, tc.wantStatus, apiErr.StatusCode)
				return
			}
			require.Nil(t, apiErr)
			require.NotNil(t, httpResp)
			assert.False(t, c.Writer.Written(), "nothing reaches the client before the response handler runs")
			// The logged effort follows the controls actually sent: adaptive
			// output_config.effort or the manual 2048-token budget, both medium.
			assert.Equal(t, "medium", info.ReasoningEffort)

			usage, apiErr := adaptor.DoResponse(c, httpResp, info)
			require.Nil(t, apiErr)
			assert.Equal(t, 2, usage.(*dto.Usage).CompletionTokens)
			if tc.stream {
				assert.Contains(t, recorder.Body.String(), "message_stop")
			} else {
				assert.Equal(t, "ok", gjson.Get(recorder.Body.String(), "content.0.text").String())
			}
		})
	}
}

// The same-channel retry runs inside one relay attempt: the request is
// reserved once before it, the failed 400 is not settled, and the successful
// retry settles exactly once.
func TestClaudeHelperThinkingCompatRetrySettlesOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	claudeAdaptiveSupportCache.Purge()
	t.Cleanup(claudeAdaptiveSupportCache.Purge)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}, &model.QuotaNotificationState{}))
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousMemoryCache := common.RedisEnabled, common.MemoryCacheEnabled
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemoryCache
		require.NoError(t, sqlDB.Close())
	})

	const startingQuota = 2_000_000
	user := model.User{Username: "claude_thinking_compat", Quota: startingQuota, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "claude-thinking-compat", Name: "claude-thinking-compat", RemainQuota: startingQuota, Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(&token).Error)
	channel := model.Channel{Id: claudeThinkingCompatChannel, Name: "claude-thinking-compat", Key: "test-key", Status: common.ChannelStatusEnabled, Type: constant.ChannelTypeAnthropic}
	require.NoError(t, db.Create(&channel).Error)

	upstreamURL, received := startClaudeUpstream(t, []claudeUpstreamReply{claudeRequiredReply, claudeMessageReply})
	c, _, info, _ := newClaudeThinkingCompatRelay(t, upstreamURL, constant.ChannelTypeAnthropic, dto.ChannelOtherSettings{}, claudeThinkingCompatOpusName, false)
	const expression = `tier("request", fixed(0.01))`
	info.UserId, info.TokenId, info.TokenKey = user.Id, token.Id, token.Key
	info.UsingGroup, info.UserGroup = "default", "default"
	info.UserSetting = dto.UserSetting{BillingPreference: "wallet_only"}
	info.ForcePreConsume = true
	info.PriceData = hosttypes.PriceData{GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}
	info.BillingRequestInput = &billingexpr.RequestInput{Body: []byte(`{}`)}
	cost, trace, err := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, *info.BillingRequestInput)
	require.NoError(t, err)
	reservation, err := billingexpr.QuotaRoundStrict(cost / 1_000_000 * common.QuotaPerUnit)
	require.NoError(t, err)
	info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expression, ExprHash: billingexpr.ExprHashString(expression),
		QuotaPerUnit: common.QuotaPerUnit, GroupRatio: 1, EstimatedTier: trace.MatchedTier, EstimatedBillingUnit: trace.BillingUnit, EstimatedFixedPrice: trace.FixedPrice, EstimatedQuotaAfterGroup: reservation}
	require.Nil(t, service.PreConsumeBilling(c, reservation, info))
	c.Request.Body = io.NopCloser(strings.NewReader(fmt.Sprintf(claudeManualThinkingRequest, claudeThinkingCompatOpusName, false)))

	require.Nil(t, ClaudeHelper(c, info))

	assert.Len(t, received(), 2, "the adaptive-required 400 is retried once on the same channel")
	quota, err := model.GetUserQuota(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, startingQuota-reservation, quota, "the retried request is charged once")
	var logs []model.Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", user.Id, model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1, "one consume log for the retried request")
	assert.Equal(t, reservation, logs[0].Quota)
}

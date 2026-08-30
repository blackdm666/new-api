package sub2api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequestBodyNormalizesCanvasPayload(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewBufferString(`{
		"model":"grok-imagine-video-1.5-1080p",
		"prompt":"animate this",
		"duration":5,
		"size":"9:16",
		"images":["data:image/png;base64,AAAA"],
		"metadata":{"resolution":"720p"}
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(c)

	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{}}
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	info.OriginModelName = ModelGrokImagineVideo151080
	info.UpstreamModelName = ModelGrokImagineVideo15
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"grok-imagine-video-1.5",
		"prompt":"animate this",
		"duration":5,
		"aspect_ratio":"9:16",
		"resolution":"1080p",
		"image":{"url":"data:image/png;base64,AAAA"}
	}`, string(payload))
}

func TestValidateVideo15AllowsTextToVideo(t *testing.T) {
	for _, modelName := range []string{ModelGrokImagineVideo15, ModelGrokImagineVideo151080} {
		t.Run(modelName, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewBufferString(fmt.Sprintf(`{"model":%q,"prompt":"test","duration":5}`, modelName)))
			c.Request.Header.Set("Content-Type", "application/json")
			defer common.CleanupBodyStorage(c)

			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
			require.Nil(t, taskErr)
		})
	}
}

func TestArbitraryMappedAliasUsesResolvedModelConfig(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewBufferString(`{"model":"grok-sales-alias","prompt":"test","duration":5}`))
	c.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(c)
	info := &relaycommon.RelayInfo{
		OriginModelName: "grok-sales-alias",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: ModelGrokImagineVideo,
			IsModelMapped:     true,
		},
	}
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	assert.Equal(t, map[string]float64{"seconds": 5}, adaptor.EstimateBilling(c, info))
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"grok-imagine-video","prompt":"test","duration":5,"aspect_ratio":"16:9","resolution":"720p"}`, string(payload))
}

func TestValidateGrokVideoDefaultsAndLimits(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewBufferString(`{"model":"grok-imagine-video","prompt":"test","size":"3:2","metadata":{"resolution":"480p"}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(c)

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{}}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	info.OriginModelName = ModelGrokImagineVideo
	info.UpstreamModelName = ModelGrokImagineVideo
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"grok-imagine-video","prompt":"test","duration":8,"aspect_ratio":"3:2","resolution":"480p"}`, string(payload))
}

func TestEstimateBillingUsesGrokVideoDuration(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		body     string
		expected float64
	}{
		{name: "base four seconds", model: ModelGrokImagineVideo, body: `{"model":"grok-imagine-video","prompt":"test","duration":4}`, expected: 4},
		{name: "1.5 six seconds", model: ModelGrokImagineVideo15, body: `{"model":"grok-imagine-video-1.5","prompt":"test","duration":6}`, expected: 6},
		{name: "1.5 1080p fifteen seconds", model: ModelGrokImagineVideo151080, body: `{"model":"grok-imagine-video-1.5-1080p","prompt":"test","duration":15}`, expected: 15},
		{name: "default eight seconds", model: ModelGrokImagineVideo15, body: `{"model":"grok-imagine-video-1.5","prompt":"test"}`, expected: 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewBufferString(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			defer common.CleanupBodyStorage(c)

			adaptor := &TaskAdaptor{}
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{}}
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			info.OriginModelName = tt.model

			got := adaptor.EstimateBilling(c, info)

			assert.Equal(t, map[string]float64{"seconds": tt.expected}, got)
		})
	}
}

func TestParseTaskResultHandlesSub2APIStatesAndRelativeContentURL(t *testing.T) {
	adaptor := &TaskAdaptor{baseURL: "https://api.apikey.fun"}
	pending, err := adaptor.ParseTaskResult([]byte(`{"request_id":"upstream-1","status":"pending","progress":75}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, pending.Status)
	assert.Equal(t, "75%", pending.Progress)

	done, err := adaptor.ParseTaskResult([]byte(`{"request_id":"upstream-1","status":"done","progress":100,"video":{"url":"/v1/videos/upstream-1/content","duration":5}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, done.Status)
	assert.Equal(t, "https://api.apikey.fun/v1/videos/upstream-1/content", done.Url)
}

func TestDoResponseReturnsPublicTaskID(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{Body: io.NopCloser(bytes.NewBufferString(`{"request_id":"private-upstream-id","status":"pending","progress":0}`))}
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}, OriginModelName: ModelGrokImagineVideo}

	parsed, taskErr := (&TaskAdaptor{}).ParseResponse(c, resp, info)
	require.Nil(t, taskErr)
	require.NotNil(t, parsed)
	assert.Equal(t, "private-upstream-id", parsed.UpstreamTaskID)
	payload, err := common.Marshal(parsed.ClientResponse)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"task_public","object":"video","model":"grok-imagine-video","status":"queued"}`, string(payload))
}

func TestConvertToOpenAIVideoUsesPersistedTaskState(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_public",
		CreatedAt:  10,
		FinishTime: 20,
		Status:     model.TaskStatusSuccess,
		Progress:   "100%",
		Properties: model.Properties{OriginModelName: ModelGrokImagineVideo},
	}
	body, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"task_public","object":"video","model":"grok-imagine-video","status":"completed","progress":100,"created_at":10,"completed_at":20}`, string(body))
}

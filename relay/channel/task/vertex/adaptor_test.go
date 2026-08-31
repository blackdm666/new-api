package vertex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	omnitask "github.com/QuantumNous/new-api/relay/channel/task/omni"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOmniBuildRequestURL(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            `{"project_id":"vertex-project"}`,
			UpstreamModelName: omnitask.ModelGeminiOmniFlashPreview,
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://aiplatform.googleapis.com/v1beta1/projects/vertex-project/locations/global/interactions", requestURL)
	assert.Contains(t, adaptor.GetModelList(), omnitask.ModelGeminiOmniFlashPreview)
}

func TestOmniResolvedAliasRunsReferenceVideoValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/videos", strings.NewReader(`{
		"model":"gemini-omni-flash",
		"prompt":"edit this video",
		"duration":3,
		"video":"not-valid-base64"
	}`))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-omni-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: omnitask.ModelGeminiOmniFlashPreview,
			IsModelMapped:     true,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)

	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_omni_request", taskErr.Code)
	assert.Contains(t, taskErr.Message, "invalid base64 video data")
	assert.Equal(t, omnitask.ModelGeminiOmniFlashPreview, info.UpstreamModelName)
}

func TestConvertToOpenAIVideoIncludesInteractionFailure(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_failed",
		Status:     model.TaskStatusFailure,
		FailReason: "[content_blocked] Unable to show the generated video.",
		Progress:   "100%",
		CreatedAt:  100,
		UpdatedAt:  200,
	}

	data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	var video dto.OpenAIVideo
	require.NoError(t, common.Unmarshal(data, &video))
	require.NotNil(t, video.Error)
	assert.Equal(t, dto.VideoStatusFailed, video.Status)
	assert.Equal(t, "content_blocked", video.Error.Code)
	assert.Equal(t, "[content_blocked] Unable to show the generated video.", video.Error.Message)
}

func TestConvertToOpenAIVideoRecoversLegacyInteractionFailure(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_legacy_failed",
		Status:     model.TaskStatusFailure,
		FailReason: "interaction ended with status failed",
		Progress:   "100%",
		Data:       []byte(`{"status":"failed","errors":[{"code":"content_blocked","message":"Responsible AI blocked the output."}]}`),
	}

	data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	var video dto.OpenAIVideo
	require.NoError(t, common.Unmarshal(data, &video))
	require.NotNil(t, video.Error)
	assert.Equal(t, "content_blocked", video.Error.Code)
	assert.Equal(t, "[content_blocked] Responsible AI blocked the output.", video.Error.Message)
}

func TestParseTaskResultTreatsFilteredTerminalResponseAsFailure(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"name":"projects/project/locations/us-central1/publishers/google/models/veo-3.1-generate-001/operations/filtered",
		"done":true,
		"response":{"raiMediaFilteredCount":1,"raiMediaFilteredReasons":["1 video was filtered; support code: 15236754"]}
	}`))

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, result.Status)
	assert.Equal(t, "100%", result.Progress)
	assert.Contains(t, result.Reason, "15236754")
}

package gemini

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	omnitask "github.com/QuantumNous/new-api/relay/channel/task/omni"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOmniBuildRequestURL(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://generativelanguage.googleapis.com/",
			UpstreamModelName: omnitask.ModelGeminiOmniFlashPreview,
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://generativelanguage.googleapis.com/v1beta/interactions", requestURL)
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

func TestParseTaskResultTreatsFilteredTerminalResponseAsFailure(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"name":"models/veo-3.1/operations/filtered",
		"done":true,
		"response":{"generateVideoResponse":{"raiMediaFilteredCount":1,"raiMediaFilteredReasons":["blocked by Google safety policy; support code: 123"]}}
	}`))

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, result.Status)
	assert.Equal(t, "100%", result.Progress)
	assert.Contains(t, result.Reason, "support code: 123")
}

package relay

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskModelMappingRunsBeforeAdaptorValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		channelType int
		origin      string
		upstream    string
		mapping     string
		body        string
		wantCode    string
		wantMessage string
	}{
		{
			name:        "vertex omni",
			channelType: constant.ChannelTypeVertexAi,
			origin:      "omni-sales-alias",
			upstream:    "gemini-omni-flash-preview",
			mapping:     `{"omni-sales-alias":"gemini-omni-flash-preview"}`,
			body:        `{"model":"omni-sales-alias","prompt":"edit this video","duration":3,"video":"not-valid-base64"}`,
			wantCode:    "invalid_omni_request",
			wantMessage: "invalid base64 video data",
		},
		{
			name:        "sub2api",
			channelType: constant.ChannelTypeSub2API,
			origin:      "grok-sales-alias",
			upstream:    "grok-imagine-video",
			mapping:     `{"grok-sales-alias":"grok-imagine-video"}`,
			body:        `{"model":"grok-sales-alias","prompt":"test","duration":16}`,
			wantCode:    "invalid_request",
			wantMessage: "duration must be between 1 and 15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewBufferString(tt.body))
			request.Header.Set("Content-Type", "application/json")
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = request
			c.Set("channel_type", tt.channelType)
			c.Set("model_mapping", tt.mapping)
			defer common.CleanupBodyStorage(c)

			info := &relaycommon.RelayInfo{
				OriginModelName: tt.origin,
				TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
			}
			_, taskErr := RelayTaskSubmit(c, info)

			require.NotNil(t, taskErr)
			assert.Equal(t, tt.wantCode, taskErr.Code)
			assert.Contains(t, taskErr.Message, tt.wantMessage)
			assert.Equal(t, tt.origin, info.OriginModelName)
			assert.Equal(t, tt.upstream, info.UpstreamModelName)
			assert.True(t, info.IsModelMapped)
		})
	}
}

func TestTaskModelMappingRejectsCycleBeforeAdaptorValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewBufferString(`{"model":"sales-a","prompt":"test"}`))
	request.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = request
	c.Set("channel_type", constant.ChannelTypeSub2API)
	c.Set("model_mapping", `{"sales-a":"sales-b","sales-b":"sales-a"}`)

	info := &relaycommon.RelayInfo{OriginModelName: "sales-a", TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	_, taskErr := RelayTaskSubmit(c, info)

	require.NotNil(t, taskErr)
	assert.Equal(t, "model_mapping_failed", taskErr.Code)
	assert.Contains(t, taskErr.Message, "model_mapping_contains_cycle")
}

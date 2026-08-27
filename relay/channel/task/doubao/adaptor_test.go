package doubao

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestEstimateBillingFallsBackToResolvedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model: "doubao-sales-alias",
		Metadata: map[string]interface{}{
			"resolution": "1080p",
			"content": []interface{}{
				map[string]interface{}{"type": "video_url", "video_url": map[string]interface{}{"url": "https://example.com/input.mp4"}},
			},
		},
	})
	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-sales-alias",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "doubao-seedance-2-0-260128",
			IsModelMapped:     true,
		},
	}

	ratios := (&TaskAdaptor{}).EstimateBilling(c, info)

	assert.InDelta(t, 31.0/46.0, ratios["video_input"], 1e-9)
}

func TestEstimateBillingIncludesEffectiveDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:    "doubao-seedance-2-0-260128",
		Duration: 10,
	})

	ratios := (&TaskAdaptor{}).EstimateBilling(c, &relaycommon.RelayInfo{})

	assert.Equal(t, 10.0, ratios["seconds"])
}

func TestEstimateBillingUsesUpstreamFramesPrecedence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:    "doubao-seedance-2-0-260128",
		Duration: 10,
		Metadata: map[string]interface{}{"frames": 57},
	})

	ratios := (&TaskAdaptor{}).EstimateBilling(c, &relaycommon.RelayInfo{})

	assert.Equal(t, 2.375, ratios["seconds"])
}

func TestBuildRequestBodyForwardsStandardDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:    "doubao-seedance-2-0-260128",
		Prompt:   "test",
		Duration: 10,
	})

	body, err := (&TaskAdaptor{}).BuildRequestBody(c, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
	})
	assert.NoError(t, err)
	data, err := io.ReadAll(body)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(data), `"duration":10`))

	var payload requestPayload
	assert.NoError(t, common.Unmarshal(data, &payload))
	assert.NotNil(t, payload.Duration)
	assert.Equal(t, 10, int(*payload.Duration))
}

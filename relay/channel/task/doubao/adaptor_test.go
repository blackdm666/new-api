package doubao

import (
	"net/http/httptest"
	"testing"

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

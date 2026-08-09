package globalaiopc

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToRequestPayload(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt:   "A cinematic product shot",
		Model:    ModelSeedance25,
		Images:   []string{"https://example.com/reference.png"},
		Duration: 5,
		Size:     "1280x720",
		Metadata: map[string]interface{}{
			"reference_audios": []interface{}{"https://example.com/reference.mp3"},
		},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: ModelSeedance25,
		},
	}

	body, err := convertToRequestPayload(req, info)

	require.NoError(t, err)
	assert.Equal(t, ModelSeedance25, body.Model)
	assert.Equal(t, "A cinematic product shot", body.Prompt)
	assert.Equal(t, []string{"https://example.com/reference.png"}, body.ReferenceImages)
	assert.Equal(t, []string{"https://example.com/reference.mp3"}, body.ReferenceAudios)
	assert.Equal(t, 5, body.Duration)
	assert.Equal(t, "16:9", body.AspectRatio)
	assert.Equal(t, "720p", body.Resolution)
}

func TestConvertToRequestPayloadRejectsInvalidDuration(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt:   "A cinematic product shot",
		Model:    ModelSeedance25,
		Duration: 31,
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: ModelSeedance25,
		},
	}

	_, err := convertToRequestPayload(req, info)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duration must be between 4 and 30")
}

func TestParseTaskResultCompleted(t *testing.T) {
	body := []byte(`{
		"id": "mcp_example_123456",
		"status": "completed",
		"progress": 100,
		"result_url": "https://example.com/result.mp4",
		"video_url": "https://example.com/video.mp4"
	}`)

	info, err := (&TaskAdaptor{}).ParseTaskResult(body)

	require.NoError(t, err)
	assert.Equal(t, "mcp_example_123456", info.TaskID)
	assert.Equal(t, model.TaskStatusSuccess, model.TaskStatus(info.Status))
	assert.Equal(t, "100%", info.Progress)
	assert.Equal(t, "https://example.com/video.mp4", info.Url)
}

func TestParseTaskResultFailed(t *testing.T) {
	body := []byte(`{
		"id": "mcp_example_123456",
		"status": "failed",
		"error": {"message": "policy rejected"}
	}`)

	info, err := (&TaskAdaptor{}).ParseTaskResult(body)

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, model.TaskStatus(info.Status))
	assert.Equal(t, "policy rejected", info.Reason)
}

package globalaiopc

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToRequestPayloadSeedance25Variants(t *testing.T) {
	tests := []struct {
		name               string
		model              string
		expectedUpstream   string
		expectedResolution string
	}{
		{name: "480p", model: ModelSeedance25480P, expectedUpstream: ModelSeedance25, expectedResolution: "480p"},
		{name: "720p", model: ModelSeedance25720P, expectedUpstream: ModelSeedance25, expectedResolution: "720p"},
		{name: "c2 720p", model: ModelSeedance25C2720P, expectedUpstream: upstreamSeedance25C2, expectedResolution: "720p"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := relaycommon.TaskSubmitReq{
				Prompt:   "A cinematic product shot",
				Model:    tt.model,
				Duration: 4,
				Size:     "16:9",
			}
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tt.model},
			}

			body, err := convertToRequestPayload(req, info)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedUpstream, body.Model)
			assert.Equal(t, tt.expectedResolution, body.Resolution)
		})
	}
}

func TestConvertToRequestPayloadCannotOverrideBillableVariant(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt:   "A cinematic product shot",
		Model:    ModelSeedance25480P,
		Duration: 4,
		Metadata: map[string]interface{}{
			"model":      upstreamSeedance25C2,
			"resolution": "720p",
		},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: ModelSeedance25480P},
	}

	body, err := convertToRequestPayload(req, info)

	require.NoError(t, err)
	assert.Equal(t, ModelSeedance25, body.Model)
	assert.Equal(t, "480p", body.Resolution)
}

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

func TestConvertToRequestPayloadMiniMaxH3(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt:   "A cinematic product shot",
		Model:    ModelMiniMaxH3,
		Images:   []string{"https://example.com/reference.png"},
		Duration: 5,
		Size:     "2560x1440",
		Metadata: map[string]interface{}{
			"reference_audios": []interface{}{"https://example.com/reference.mp3"},
			"first_image":      "https://example.com/first.png",
			"last_image":       "https://example.com/last.png",
		},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: ModelMiniMaxH3,
		},
	}

	body, err := convertToRequestPayload(req, info)

	require.NoError(t, err)
	assert.Equal(t, ModelMiniMaxH3, body.Model)
	assert.Equal(t, []string{"https://example.com/reference.png"}, body.ReferenceImages)
	assert.Equal(t, []string{"https://example.com/reference.mp3"}, body.ReferenceAudios)
	assert.Equal(t, 5, body.Duration)
	assert.Equal(t, "16:9", body.AspectRatio)
	assert.Equal(t, "2k", body.Resolution)
	assert.Equal(t, "2560x1440", body.Size)
	assert.Equal(t, "https://example.com/first.png", body.FirstImage)
	assert.Equal(t, "https://example.com/last.png", body.LastImage)
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

func TestConvertToRequestPayloadRejectsInvalidMiniMaxH3Duration(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt:   "A cinematic product shot",
		Model:    ModelMiniMaxH3,
		Duration: 4,
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: ModelMiniMaxH3,
		},
	}

	_, err := convertToRequestPayload(req, info)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duration must be between 5 and 15")
}

func TestConvertToRequestPayloadRejectsMiniMaxH3AudioWithoutImage(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt:   "A cinematic product shot",
		Model:    ModelMiniMaxH3,
		Duration: 5,
		Metadata: map[string]interface{}{
			"reference_audios": []interface{}{"https://example.com/reference.mp3"},
		},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: ModelMiniMaxH3,
		},
	}

	_, err := convertToRequestPayload(req, info)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "reference_audios requires at least one image reference")
}

func TestParseTaskResultCompleted(t *testing.T) {
	body := []byte(`{
		"id": "mcp_example_123456",
		"status": "completed",
		"progress": 100,
		"actualDuration": 5.17,
		"result_url": "https://example.com/result.mp4",
		"video_url": "https://example.com/video.mp4"
	}`)

	info, err := (&TaskAdaptor{}).ParseTaskResult(body)

	require.NoError(t, err)
	assert.Equal(t, "mcp_example_123456", info.TaskID)
	assert.EqualValues(t, model.TaskStatusSuccess, info.Status)
	assert.Equal(t, "100%", info.Progress)
	assert.Equal(t, "https://example.com/video.mp4", info.Url)
}

func TestConvertToOpenAIVideoPreservesFractionalDuration(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_example_123456",
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Properties: model.Properties{
			OriginModelName: ModelMiniMaxH3,
		},
		Data: []byte(`{
			"id": "mcp_example_123456",
			"status": "completed",
			"progress": 100,
			"actualDuration": 5.17,
			"video_url": "https://example.com/video.mp4"
		}`),
	}

	data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)

	require.NoError(t, err)
	result := map[string]any{}
	require.NoError(t, common.Unmarshal(data, &result))
	assert.Equal(t, "5.17", result["seconds"])
}

func TestParseTaskResultFailed(t *testing.T) {
	body := []byte(`{
		"id": "mcp_example_123456",
		"status": "failed",
		"error": {"message": "policy rejected"}
	}`)

	info, err := (&TaskAdaptor{}).ParseTaskResult(body)

	require.NoError(t, err)
	assert.EqualValues(t, model.TaskStatusFailure, info.Status)
	assert.Equal(t, "policy rejected", info.Reason)
}

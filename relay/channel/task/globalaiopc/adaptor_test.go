package globalaiopc

import (
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

func TestGetModelListUsesRequestedPublicNames(t *testing.T) {
	models := (&TaskAdaptor{}).GetModelList()
	assert.Contains(t, models, ModelSeedance25)
	assert.Contains(t, models, ModelSeedance25C2)
	assert.NotContains(t, models, "seedance-2.5-720p")
	assert.NotContains(t, models, "seedance-2.5-c2-720p")
	assert.NotContains(t, models, "seedance-2.5-480p")
	assert.Contains(t, models, "seedance-2.0-1080p")
	assert.Contains(t, models, "happyhorse-1.0-r2v-720p")
	assert.Contains(t, models, "wan2.7-r2v")
	assert.Contains(t, models, ModelKlingO3)
	assert.Contains(t, models, ModelDigitalHuman)
}

func TestConvertToRequestPayloadSeedance25PublicNames(t *testing.T) {
	tests := []struct {
		name               string
		model              string
		expectedUpstream   string
		expectedResolution string
	}{
		{name: "official full model", model: ModelSeedance25, expectedUpstream: ModelSeedance25, expectedResolution: "720p"},
		{name: "c2", model: ModelSeedance25C2, expectedUpstream: ModelSeedance25C2, expectedResolution: "720p"},
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
		Model:    "seedance-2.0-1080p",
		Duration: 4,
		Metadata: map[string]interface{}{
			"model":      "sd_2.0_fast_special",
			"resolution": "4k",
		},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seedance-2.0-1080p"},
	}

	body, err := convertToRequestPayload(req, info)

	require.NoError(t, err)
	assert.Equal(t, "sd_2.0_special", body.Model)
	assert.Equal(t, "1080p", body.Resolution)
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

func TestConvertToRequestPayloadHappyHorseAlias(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt:   "A running horse",
		Model:    "happyhorse-1.0-r2v-720p",
		Images:   []string{"https://example.com/horse.png"},
		Duration: 5,
		Size:     "16:9",
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: req.Model}}

	body, err := convertToRequestPayload(req, info)

	require.NoError(t, err)
	assert.Equal(t, "happyhorse-1.0-r2v", body.Model)
	assert.Equal(t, "720P", body.Resolution)
	assert.Equal(t, req.Images, body.ReferenceImages)
}

func TestConvertToRequestPayloadWanOfficialNameLocks1080P(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt:   "A cinematic transition",
		Model:    "wan2.7-i2v",
		Images:   []string{"https://example.com/first.png", "https://example.com/last.png"},
		Duration: 5,
		Metadata: map[string]interface{}{"resolution": "720P"},
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: req.Model}}

	body, err := convertToRequestPayload(req, info)

	require.NoError(t, err)
	assert.Equal(t, "wan2.7-i2v", body.Model)
	assert.Equal(t, "1080P", body.Resolution)
	assert.Equal(t, 1, imageValueCount(body.FirstImage))
	assert.Equal(t, 1, imageValueCount(body.LastImage))
}

func TestConvertToRequestPayloadWanVideoEditRequiresVideo(t *testing.T) {
	req := relaycommon.TaskSubmitReq{Prompt: "Add a tracking shot", Model: "wan2.7-videoedit", Duration: 5}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: req.Model}}

	_, err := convertToRequestPayload(req, info)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "reference_videos must contain at least 1 items")
}

func TestConvertToDigitalHumanPayload(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt:   "digital human lip sync",
		Model:    ModelDigitalHuman,
		Duration: 12,
		Metadata: map[string]interface{}{
			"audioUrl":     "https://example.com/audio.mp3",
			"videoUrl":     "https://example.com/person.mp4",
			"modelVersion": 2,
		},
	}

	body, err := convertToDigitalHumanPayload(req, req.Model)

	require.NoError(t, err)
	assert.Equal(t, ModelDigitalHuman, body.Model)
	assert.Equal(t, "https://example.com/audio.mp3", body.AudioURL)
	assert.Equal(t, "https://example.com/person.mp4", body.VideoURL)
	assert.Equal(t, 2, body.ModelVersion)
}

func TestFetchTaskUsesDigitalHumanQueryPath(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	resp, err := (&TaskAdaptor{}).FetchTask(server.URL, "key", map[string]any{"task_id": "dh_123", "action": taskActionDigitalHuman}, "")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "/v1/result/dh_123", gotPath)
}

func TestEstimateBillingDistinguishesPerSecondAndPerCallModels(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		metadata map[string]interface{}
		want     map[string]float64
	}{
		{name: "seedance per second", model: ModelSeedance25, want: map[string]float64{"seconds": 6}},
		{name: "videos fixed per call", model: ModelVideosStable, want: nil},
		{name: "seedance 2 video reference surcharge", model: "seedance-2.0-720p", metadata: map[string]interface{}{"reference_videos": []interface{}{"https://example.com/ref.mp4"}}, want: map[string]float64{"seconds": 6, "video_reference": 1.27 / 0.58}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			req := relaycommon.TaskSubmitReq{Prompt: "test", Model: tt.model, Duration: 6, Metadata: tt.metadata}
			c.Set("task_request", req)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tt.model}}

			got := (&TaskAdaptor{}).EstimateBilling(c, info)

			assert.Equal(t, tt.want, got)
		})
	}
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
	assert.Contains(t, err.Error(), "reference_audios requires at least one image or video reference")
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

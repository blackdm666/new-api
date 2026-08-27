package xinmeng

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModelListContainsCurrentModels(t *testing.T) {
	assert.ElementsMatch(t, []string{
		ModelDVCSeedance25,
		ModelDVCSeedance20,
		ModelMiniMaxH3768P,
		ModelMiniMaxH31440P,
	}, (&TaskAdaptor{}).GetModelList())
}

func TestConvertDVCSeedance25Locks720p(t *testing.T) {
	generateAudio := false
	req := relaycommon.TaskSubmitReq{
		Model:    ModelDVCSeedance25,
		Prompt:   "A cinematic ocean sunset",
		Duration: 30,
		Size:     "9:16",
		Images:   []string{"https://example.com/character.png"},
		Metadata: map[string]interface{}{
			"resolution":     "480p",
			"generate_audio": generateAudio,
		},
	}

	body, err := convertToRequestPayload(req, nil)

	require.NoError(t, err)
	assert.Equal(t, ModelDVCSeedance25, body.Model)
	assert.Equal(t, "720p", body.Resolution)
	assert.Equal(t, "9:16", body.Ratio)
	assert.Equal(t, 30, body.Duration)
	assert.Equal(t, req.Images, body.ReferenceImages)
	require.NotNil(t, body.GenerateAudio)
	assert.False(t, *body.GenerateAudio)
	payload, err := common.Marshal(body)
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"referenceImages"`)
	assert.NotContains(t, string(payload), `"reference_images"`)
	assert.Contains(t, string(payload), `"resolution":"720p"`)
}

func TestConvertDVCSeedance20SupportsMixedReferencesAndLocks720p(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:    ModelDVCSeedance20,
		Prompt:   "Use the supplied references",
		Duration: 8,
		Metadata: map[string]interface{}{
			"aspect_ratio":     "16:9",
			"resolution":       "4k",
			"reference_images": []interface{}{"https://example.com/image.png"},
			"referenceVideos":  []interface{}{"https://example.com/video.mp4"},
			"audio_urls":       []interface{}{"https://example.com/audio.mp3"},
		},
	}

	body, err := convertToRequestPayload(req, nil)

	require.NoError(t, err)
	assert.Equal(t, "720p", body.Resolution)
	assert.Equal(t, []string{"https://example.com/image.png"}, body.ReferenceImages)
	assert.Equal(t, []string{"https://example.com/video.mp4"}, body.ReferenceVideos)
	assert.Equal(t, []string{"https://example.com/audio.mp3"}, body.ReferenceAudios)
}

func TestConvertDVCSeedance20RejectsTooManyMixedReferences(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  ModelDVCSeedance20,
		Prompt: "Too many references",
		Metadata: map[string]interface{}{
			"referenceImages": []interface{}{
				"https://example.com/1.png", "https://example.com/2.png", "https://example.com/3.png",
				"https://example.com/4.png", "https://example.com/5.png", "https://example.com/6.png",
				"https://example.com/7.png",
			},
			"referenceVideos": []interface{}{
				"https://example.com/1.mp4", "https://example.com/2.mp4", "https://example.com/3.mp4",
			},
			"referenceAudios": []interface{}{
				"https://example.com/1.mp3", "https://example.com/2.mp3", "https://example.com/3.mp3",
			},
		},
	}

	_, err := convertToRequestPayload(req, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most 12 items in total")
}

func TestConvertDVCSeedance20RejectsGenerateAudioOverride(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  ModelDVCSeedance20,
		Prompt: "The upstream model always includes audio",
		Metadata: map[string]interface{}{
			"generateAudio": false,
		},
	}

	_, err := convertToRequestPayload(req, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "generateAudio is not supported")
}

func TestConvertMiniMaxH3Variants(t *testing.T) {
	t.Run("locks each model to its upstream resolution", func(t *testing.T) {
		tests := []struct {
			model      string
			resolution string
		}{
			{model: ModelMiniMaxH3768P, resolution: "768p"},
			{model: ModelMiniMaxH31440P, resolution: "1440p"},
		}
		for _, tt := range tests {
			t.Run(tt.model, func(t *testing.T) {
				req := relaycommon.TaskSubmitReq{
					Model:    tt.model,
					Prompt:   "Animate the portrait to the music",
					Duration: 12,
					Images:   []string{"https://example.com/portrait.png"},
					Metadata: map[string]interface{}{
						"resolution":       "2k",
						"reference_audios": []interface{}{"https://example.com/music.mp3"},
					},
				}

				body, err := convertToRequestPayload(req, nil)

				require.NoError(t, err)
				assert.Equal(t, tt.model, body.Model)
				assert.Equal(t, tt.resolution, body.Resolution)
				assert.Equal(t, 12, body.Duration)
				assert.Equal(t, []string{"https://example.com/music.mp3"}, body.ReferenceAudios)
			})
		}
	})

	t.Run("uses the current four second default", func(t *testing.T) {
		req := relaycommon.TaskSubmitReq{
			Model:  ModelMiniMaxH3768P,
			Prompt: "A cinematic landscape",
		}

		body, err := convertToRequestPayload(req, nil)

		require.NoError(t, err)
		assert.Equal(t, 4, body.Duration)
	})

	t.Run("audio requires image", func(t *testing.T) {
		req := relaycommon.TaskSubmitReq{
			Model:  ModelMiniMaxH3768P,
			Prompt: "Audio only",
			Metadata: map[string]interface{}{
				"referenceAudios": []interface{}{"https://example.com/music.mp3"},
			},
		}

		_, err := convertToRequestPayload(req, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "require at least one image")
	})

	t.Run("reference video is rejected", func(t *testing.T) {
		req := relaycommon.TaskSubmitReq{
			Model:  ModelMiniMaxH31440P,
			Prompt: "Video reference",
			Metadata: map[string]interface{}{
				"referenceVideos": []interface{}{"https://example.com/video.mp4"},
			},
		}

		_, err := convertToRequestPayload(req, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "at most 0 items")
	})

	t.Run("legacy base model is rejected", func(t *testing.T) {
		_, err := convertToRequestPayload(relaycommon.TaskSubmitReq{
			Model:  "minimax-h3",
			Prompt: "Legacy model",
		}, nil)

		require.Error(t, err)
		assert.Equal(t, "model must be one of dvc-seedance-2.5, dvc-seedance-2.0, minimax-h3-768p, minimax-h3-1440p", err.Error())
	})

	t.Run("rejects ratios removed by the current upstream", func(t *testing.T) {
		_, err := convertToRequestPayload(relaycommon.TaskSubmitReq{
			Model:  ModelMiniMaxH31440P,
			Prompt: "Unsupported ratio",
			Metadata: map[string]interface{}{
				"ratio": "4:3",
			},
		}, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "ratio 4:3 is not supported")
	})
}

func TestEstimateBillingUsesRequestedSecondsForAllCurrentModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, modelName := range ModelList {
		t.Run(modelName, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("task_request", relaycommon.TaskSubmitReq{Model: modelName, Prompt: "test", Duration: 6})

			ratios := (&TaskAdaptor{}).EstimateBilling(c, &relaycommon.RelayInfo{})

			assert.Equal(t, map[string]float64{"seconds": 6}, ratios)
		})
	}
}

func TestResolvedSalesAliasesUseUpstreamModelForValidationBillingAndBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		alias    string
		upstream string
	}{
		{alias: "DC全能视频2.5 720P", upstream: ModelDVCSeedance25},
		{alias: "DC全能视频2.0 720P", upstream: ModelDVCSeedance20},
		{alias: "MiniMax H3 768P", upstream: ModelMiniMaxH3768P},
		{alias: "MiniMax H3 1440P", upstream: ModelMiniMaxH31440P},
	}
	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("task_request", relaycommon.TaskSubmitReq{
				Model:    tt.alias,
				Prompt:   "mapped sales model",
				Duration: 6,
				Size:     "16:9",
			})
			info := &relaycommon.RelayInfo{
				OriginModelName: tt.alias,
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: tt.upstream,
					IsModelMapped:     true,
				},
			}

			ratios := (&TaskAdaptor{}).EstimateBilling(c, info)
			assert.Equal(t, map[string]float64{"seconds": 6}, ratios)

			bodyReader, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
			require.NoError(t, err)
			bodyBytes, err := io.ReadAll(bodyReader)
			require.NoError(t, err)
			var body requestPayload
			require.NoError(t, common.Unmarshal(bodyBytes, &body))
			assert.Equal(t, tt.upstream, body.Model)
			assert.Equal(t, 6, body.Duration)
		})
	}
}

func TestXinMengChannelMetadata(t *testing.T) {
	endpoints := common.GetEndpointTypesByChannelType(constant.ChannelTypeXinMeng, ModelDVCSeedance25)
	assert.Equal(t, []constant.EndpointType{constant.EndpointTypeOpenAIVideo}, endpoints)

	task := model.InitTask(constant.TaskPlatform("62"), &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeXinMeng,
			ApiKey:      "selected-key",
		},
	})
	assert.Equal(t, "selected-key", task.PrivateData.Key)
}

func TestFetchTaskUsesXinMengTaskPathAndBearerAuth(t *testing.T) {
	var gotPath string
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	resp, err := (&TaskAdaptor{}).FetchTask(server.URL, "secret", map[string]any{"task_id": "vid_123"}, "")

	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "/v1/tasks/vid_123", gotPath)
	assert.Equal(t, "Bearer secret", gotAuthorization)
}

func TestParseTaskResultSupportsXinMengTerminalStatuses(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus model.TaskStatus
		wantURL    string
		wantReason string
	}{
		{name: "pending", body: `{"id":"vid_1","status":"pending"}`, wantStatus: model.TaskStatusQueued},
		{name: "success alias", body: `{"id":"vid_2","status":"success","result":"https://example.com/video.mp4"}`, wantStatus: model.TaskStatusSuccess, wantURL: "https://example.com/video.mp4"},
		{name: "cancelled", body: `{"id":"vid_3","status":"cancelled","error":{"message":"upstream timeout"}}`, wantStatus: model.TaskStatusFailure, wantReason: "upstream timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := (&TaskAdaptor{}).ParseTaskResult([]byte(tt.body))

			require.NoError(t, err)
			assert.EqualValues(t, tt.wantStatus, info.Status)
			assert.Equal(t, tt.wantURL, info.Url)
			assert.Equal(t, tt.wantReason, info.Reason)
		})
	}
}

func TestDoResponseReturnsPublicTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		OriginModelName: ModelDVCSeedance25,
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytesReader(`{"id":"upstream_private","status":"pending","object":"video"}`)),
	}

	upstreamID, _, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, info)

	require.Nil(t, taskErr)
	assert.Equal(t, "upstream_private", upstreamID)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.Equal(t, "task_public", payload["id"])
	assert.Equal(t, "task_public", payload["task_id"])
}

func TestConvertToOpenAIVideoUsesStoredResultURL(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_public",
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Properties: model.Properties{
			OriginModelName: ModelDVCSeedance20,
		},
		PrivateData: model.TaskPrivateData{ResultURL: "https://example.com/proxy.mp4"},
	}

	data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)

	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	metadata, ok := payload["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://example.com/proxy.mp4", metadata["url"])
}

func bytesReader(value string) io.Reader {
	return strings.NewReader(value)
}

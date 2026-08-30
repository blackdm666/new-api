package xinmeng

import (
	"fmt"
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
		ModelDoubaoSeedance25,
		ModelDoubaoSeedance20,
		ModelDoubaoSeedance20Fast,
		ModelSeedance20Mini480P,
		ModelSeedance20Mini720P,
		ModelWan30Video720P,
		ModelWan30Video1080P,
		ModelKling30Turbo720P,
		ModelKling30Turbo1080P,
		ModelKling30Turbo2K,
		ModelKling30Turbo4K,
	}, (&TaskAdaptor{}).GetModelList())
	assert.NotContains(t, (&TaskAdaptor{}).GetModelList(), ModelKling30Turbo)
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

func TestConvertMiniMaxH3768P(t *testing.T) {
	t.Run("locks the model to 768p", func(t *testing.T) {
		req := relaycommon.TaskSubmitReq{
			Model:    ModelMiniMaxH3768P,
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
		assert.Equal(t, ModelMiniMaxH3768P, body.Model)
		assert.Equal(t, "768p", body.Resolution)
		assert.Equal(t, 12, body.Duration)
		assert.Equal(t, []string{"https://example.com/music.mp3"}, body.ReferenceAudios)
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

	t.Run("accepts up to five reference videos", func(t *testing.T) {
		req := relaycommon.TaskSubmitReq{
			Model:  ModelMiniMaxH3768P,
			Prompt: "Video reference",
			Metadata: map[string]interface{}{
				"referenceVideos": []interface{}{
					"https://example.com/1.mp4", "https://example.com/2.mp4", "https://example.com/3.mp4",
					"https://example.com/4.mp4", "https://example.com/5.mp4",
				},
			},
		}

		_, err := convertToRequestPayload(req, nil)

		require.NoError(t, err)
	})

	t.Run("legacy base model is rejected", func(t *testing.T) {
		_, err := convertToRequestPayload(relaycommon.TaskSubmitReq{
			Model:  "minimax-h3",
			Prompt: "Legacy model",
		}, nil)

		require.Error(t, err)
		assert.Equal(t, "model minimax-h3 requires live XinMeng capability discovery", err.Error())
	})

	t.Run("rejects ratios removed by the current upstream", func(t *testing.T) {
		_, err := convertToRequestPayload(relaycommon.TaskSubmitReq{
			Model:  ModelMiniMaxH3768P,
			Prompt: "Unsupported ratio",
			Metadata: map[string]interface{}{
				"ratio": "4:3",
			},
		}, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "ratio 4:3 is not supported")
	})

	t.Run("accepts ten images, five videos, and five audios", func(t *testing.T) {
		images := make([]string, 10)
		for i := range images {
			images[i] = fmt.Sprintf("https://example.com/image-%d.png", i)
		}
		videos := make([]interface{}, 5)
		for i := range videos {
			videos[i] = fmt.Sprintf("https://example.com/video-%d.mp4", i)
		}
		audios := make([]interface{}, 5)
		for i := range audios {
			audios[i] = fmt.Sprintf("https://example.com/audio-%d.mp3", i)
		}

		_, err := convertToRequestPayload(relaycommon.TaskSubmitReq{
			Model: ModelMiniMaxH3768P, Prompt: "Documented maximum", Images: images,
			Metadata: map[string]interface{}{"referenceVideos": videos, "referenceAudios": audios},
		}, nil)

		require.NoError(t, err)
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

func TestNewFixedResolutionProfilesMatchXinMengDocumentation(t *testing.T) {
	tests := []struct {
		model      string
		resolution string
		minSeconds int
		maxSeconds int
	}{
		{model: ModelDoubaoSeedance25, resolution: "720p", minSeconds: 4, maxSeconds: 30},
		{model: ModelDoubaoSeedance20, resolution: "720p", minSeconds: 4, maxSeconds: 15},
		{model: ModelDoubaoSeedance20Fast, resolution: "720p", minSeconds: 4, maxSeconds: 15},
		{model: ModelSeedance20Mini480P, resolution: "480p", minSeconds: 4, maxSeconds: 15},
		{model: ModelSeedance20Mini720P, resolution: "720p", minSeconds: 4, maxSeconds: 15},
		{model: ModelWan30Video720P, resolution: "720p", minSeconds: 4, maxSeconds: 30},
		{model: ModelWan30Video1080P, resolution: "1080p", minSeconds: 4, maxSeconds: 30},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			body, err := convertToRequestPayload(relaycommon.TaskSubmitReq{
				Model: tt.model, Prompt: "documented request", Duration: tt.maxSeconds,
				Metadata: map[string]interface{}{"resolution": "4k"},
			}, nil)

			require.NoError(t, err)
			assert.Equal(t, tt.model, body.Model)
			assert.Equal(t, tt.resolution, body.Resolution)
			assert.Equal(t, tt.maxSeconds, body.Duration)

			_, err = convertToRequestPayload(relaycommon.TaskSubmitReq{
				Model: tt.model, Prompt: "too short", Duration: tt.minSeconds - 1,
			}, nil)
			require.Error(t, err)
		})
	}
}

func TestWan30VideoUsesDocumentedReferenceLimits(t *testing.T) {
	images := make([]interface{}, 10)
	videos := make([]interface{}, 5)
	audios := make([]interface{}, 5)
	for i := range images {
		images[i] = fmt.Sprintf("https://example.com/image-%d.png", i)
	}
	for i := range videos {
		videos[i] = fmt.Sprintf("https://example.com/video-%d.mp4", i)
	}
	for i := range audios {
		audios[i] = fmt.Sprintf("https://example.com/audio-%d.mp3", i)
	}

	_, err := convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: ModelWan30Video720P, Prompt: "Documented maximum references",
		Metadata: map[string]interface{}{
			"referenceImages": images,
			"referenceVideos": videos,
			"referenceAudios": audios,
		},
	}, nil)
	require.NoError(t, err)

	images = append(images, "https://example.com/image-10.png")
	_, err = convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: ModelWan30Video720P, Prompt: "Too many images",
		Metadata: map[string]interface{}{"referenceImages": images},
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most 10 items")

	_, err = convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: ModelWan30Video1080P, Prompt: "Mixed frame and references",
		Metadata: map[string]interface{}{
			"firstFrame":      "https://example.com/first.png",
			"referenceImages": []interface{}{"https://example.com/reference.png"},
		},
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "first/last frame mode cannot be mixed")
}

func TestKlingSalesAliasesLockResolutionAndUseNativeFields(t *testing.T) {
	generateAudio := true
	seed := 42
	tests := []struct {
		model      string
		resolution string
	}{
		{model: ModelKling30Turbo720P, resolution: "720p"},
		{model: ModelKling30Turbo1080P, resolution: "1080p"},
		{model: ModelKling30Turbo2K, resolution: "2k"},
		{model: ModelKling30Turbo4K, resolution: "4k"},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				OriginModelName: tt.model,
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: ModelKling30Turbo,
					IsModelMapped:     true,
				},
			}
			body, err := convertToRequestPayload(relaycommon.TaskSubmitReq{
				Model: tt.model, Prompt: "Kling native request", Duration: 6,
				NegativePrompt: "blur", Seed: &seed, GenerateAudio: &generateAudio,
				Metadata: map[string]interface{}{
					"resolution": "720p",
					"images": []interface{}{
						map[string]interface{}{"url": "https://example.com/first.png", "role": "first_frame"},
					},
					"camera_control": map[string]interface{}{"pan": 2},
				},
			}, info)

			require.NoError(t, err)
			assert.Equal(t, ModelKling30Turbo, body.Model)
			assert.Equal(t, tt.resolution, body.Resolution)
			assert.Equal(t, "blur", body.NegativePrompt)
			assert.Equal(t, &seed, body.Seed)
			assert.Equal(t, &generateAudio, body.GenerateAudioNative)
			assert.Len(t, body.Images, 1)
			assert.Empty(t, body.ReferenceImages)
			assert.EqualValues(t, 2, body.CameraControl["pan"])
		})
	}
}

func TestKlingSalesAliasFollowsRenamedMappedUpstreamModel(t *testing.T) {
	info := &relaycommon.RelayInfo{
		OriginModelName: ModelKling30Turbo720P,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "kling-v3-omni-renamed",
			IsModelMapped:     true,
		},
	}

	body, err := convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: ModelKling30Turbo720P, Prompt: "renamed upstream", Duration: 5,
	}, info)

	require.NoError(t, err)
	assert.Equal(t, "kling-v3-omni-renamed", body.Model)
	assert.Equal(t, "720p", body.Resolution)
}

func TestDynamicXinMengModelUsesLiveCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		assert.Equal(t, "/v1/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, err := io.WriteString(w, `{"object":"list","data":[{"id":"renamed-video-720p","category":"video","supportedRatios":["16:9","9:16"],"supportedQualities":["720p"],"supportedDurations":[4,6,8],"defaultRatio":"16:9","defaultQuality":"720p","defaultDuration":6,"videoModes":["text_only","reference_image"],"supportsVideoRef":0,"supportsAudioRef":0}]}`)
		assert.NoError(t, err)
	}))
	defer server.Close()

	request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"renamed-video-720p","prompt":"dynamic model","duration":8,"size":"9:16"}`))
	request.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = request
	info := &relaycommon.RelayInfo{
		OriginModelName: "renamed-video-720p",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    server.URL,
			ApiKey:            "dynamic-key",
			UpstreamModelName: "renamed-video-720p",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	taskErr := adaptor.ValidateRequestAndSetAction(c, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "Bearer dynamic-key", authorization)
	assert.Equal(t, map[string]float64{"seconds": 8}, adaptor.EstimateBilling(c, info))
	bodyReader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	bodyBytes, err := io.ReadAll(bodyReader)
	require.NoError(t, err)
	var body requestPayload
	require.NoError(t, common.Unmarshal(bodyBytes, &body))
	assert.Equal(t, "renamed-video-720p", body.Model)
	assert.Equal(t, "720p", body.Resolution)
	assert.Equal(t, "9:16", body.Ratio)
}

func TestDynamicXinMengModelRejectsUnsafeMultiResolutionAndDirectKling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"multi-quality","category":"video","supportedRatios":["16:9"],"supportedQualities":["720p","1080p"],"supportedDurations":[5],"defaultDuration":5}]}`)
	}))
	defer server.Close()

	for _, tt := range []struct {
		model       string
		wantMessage string
	}{
		{model: "multi-quality", wantMessage: "requires fixed-resolution sales aliases"},
		{model: ModelKling30Turbo, wantMessage: "must use a fixed-resolution sales alias"},
	} {
		t.Run(tt.model, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(fmt.Sprintf(`{"model":%q,"prompt":"test","duration":5}`, tt.model)))
			request.Header.Set("Content-Type", "application/json")
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = request
			info := &relaycommon.RelayInfo{
				OriginModelName: tt.model,
				TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelBaseUrl: server.URL, UpstreamModelName: tt.model,
				},
			}
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)

			taskErr := adaptor.ValidateRequestAndSetAction(c, info)
			require.NotNil(t, taskErr)
			assert.Contains(t, taskErr.Message, tt.wantMessage)
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
		{name: "Mini nested data URL", body: `{"task_id":"vid_mini","status":"succeeded","data":{"url":"https://example.com/mini.mp4"}}`, wantStatus: model.TaskStatusSuccess, wantURL: "https://example.com/mini.mp4"},
		{name: "Kling data array URL", body: `{"task_id":"vid_kling","status":"completed","data":[{"url":"https://example.com/kling.mp4"}]}`, wantStatus: model.TaskStatusSuccess, wantURL: "https://example.com/kling.mp4"},
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

	parsed, taskErr := (&TaskAdaptor{}).ParseResponse(c, resp, info)

	require.Nil(t, taskErr)
	require.NotNil(t, parsed)
	assert.Equal(t, "upstream_private", parsed.UpstreamTaskID)
	payloadBody, err := common.Marshal(parsed.ClientResponse)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(payloadBody, &payload))
	assert.Equal(t, "task_public", payload["id"])
	assert.Equal(t, "task_public", payload["task_id"])
}

func TestConvertToOpenAIVideoDoesNotExposeStoredResultURL(t *testing.T) {
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
	assert.Equal(t, "task_public", payload["id"])
	assert.NotContains(t, payload, "metadata")
}

func bytesReader(value string) io.Reader {
	return strings.NewReader(value)
}

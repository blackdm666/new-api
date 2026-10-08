package vertex

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	geminitask "github.com/QuantumNous/new-api/relay/channel/task/gemini"
	omnitask "github.com/QuantumNous/new-api/relay/channel/task/omni"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	vertexcore "github.com/QuantumNous/new-api/relay/channel/vertex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOmniBuildRequestURL(t *testing.T) {
	for _, tc := range []struct {
		model, wantURL string
	}{
		{"gemini-omni-flash-preview", "https://aiplatform.googleapis.com/v1beta1/projects/vertex-project/locations/global/interactions"},
		{"gemini-omni-1.1-flash-preview", "https://aiplatform.googleapis.com/v1beta1/projects/vertex-project/locations/global/interactions"},
		{"veo-3.1-fast-generate-001", "https://aiplatform.googleapis.com/v1/projects/vertex-project/locations/global/publishers/google/models/veo-3.1-fast-generate-001:predictLongRunning"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiKey:            `{"project_id":"vertex-project"}`,
					UpstreamModelName: tc.model,
				},
			}
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)
			requestURL, err := adaptor.BuildRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, tc.wantURL, requestURL)
		})
	}
}

func TestOmniMappedVideoUsesInteractionsAndRequestedSeconds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, provider := range []string{"vertex", "gemini"} {
		publicModel, upstreamModel := "gemini-omni-flash-1.1", "gemini-omni-1.1-flash-preview"
		if provider == "gemini" {
			publicModel, upstreamModel = "gemini-omni-flash", "gemini-omni-flash-preview"
		}
		for _, duration := range []int{3, 10, 11} {
			t.Run(fmt.Sprintf("%s/%ds", provider, duration), func(t *testing.T) {
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				raw, err := common.Marshal(map[string]any{
					"model": publicModel, "prompt": "A quiet pond.",
					"duration": duration, "size": "1280x720",
				})
				require.NoError(t, err)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(string(raw)))
				ctx.Request.Header.Set("Content-Type", "application/json")
				t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
				info := &relaycommon.RelayInfo{
					OriginModelName: publicModel,
					ChannelMeta: &relaycommon.ChannelMeta{
						UpstreamModelName: upstreamModel, IsModelMapped: true,
						ChannelBaseUrl: "https://generativelanguage.googleapis.com",
					},
					TaskRelayInfo: &relaycommon.TaskRelayInfo{},
				}
				var adaptor interface {
					Init(*relaycommon.RelayInfo)
					ValidateRequestAndSetAction(*gin.Context, *relaycommon.RelayInfo) *taskdto.TaskError
					BuildRequestBody(*gin.Context, *relaycommon.RelayInfo) (io.Reader, error)
					EstimateBilling(*gin.Context, *relaycommon.RelayInfo) map[string]float64
					ExtractUsageFacts(*gin.Context, *relaycommon.RelayInfo) map[string]any
					GetModelList() []string
				} = &TaskAdaptor{}
				if provider == "gemini" {
					adaptor = &geminitask.TaskAdaptor{}
				}
				adaptor.Init(info)
				taskErr := adaptor.ValidateRequestAndSetAction(ctx, info)
				if duration == 11 {
					require.NotNil(t, taskErr)
					assert.Equal(t, "invalid_omni_request", taskErr.Code)
					return
				}
				require.Nil(t, taskErr)
				body, err := adaptor.BuildRequestBody(ctx, info)
				require.NoError(t, err)
				data, err := io.ReadAll(body)
				require.NoError(t, err)
				var payload map[string]any
				require.NoError(t, common.Unmarshal(data, &payload))
				assert.Equal(t, upstreamModel, payload["model"])
				assert.Equal(t, true, payload["background"])
				assert.Equal(t, map[string]any{
					"type": "video", "aspect_ratio": "16:9",
					"duration": fmt.Sprintf("%ds", duration),
				}, payload["response_format"])
				assert.NotContains(t, payload, "instances")
				assert.NotContains(t, payload, "parameters")
				assert.Equal(t, float64(duration), adaptor.EstimateBilling(ctx, info)["seconds"])
				assert.Equal(t, float64(duration), adaptor.ExtractUsageFacts(ctx, info)["seconds"])
				assert.Contains(t, adaptor.GetModelList(), upstreamModel)
			})
		}
	}
}

type omniHTTPTransport func(*http.Request) (*http.Response, error)

func (f omniHTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestOmniSubmitAndPollRevisionPreserveLegacyOnly(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	credential, err := common.Marshal(vertexcore.Credentials{
		ProjectID: "vertex-project", ClientEmail: "test@example.test",
		PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	})
	require.NoError(t, err)
	service.InitHttpClient()
	client := service.GetHttpClient()
	previous := client.Transport
	t.Cleanup(func() { client.Transport = previous })
	for _, tc := range []struct {
		model, storedModel, wantRevision string
	}{
		{"gemini-omni-flash-preview", "gemini-omni-flash-preview", "2026-05-20"},
		{"gemini-omni-1.1-flash-preview", "gemini-omni-1.1-flash-preview", ""},
		{"gemini-omni-flash-preview", "", "2026-05-20"},
	} {
		t.Run(tc.model+"/"+tc.storedModel, func(t *testing.T) {
			polled := false
			client.Transport = omniHTTPTransport(func(req *http.Request) (*http.Response, error) {
				body := `{"access_token":"test-access-token"}`
				if req.URL.Host != "www.googleapis.com" {
					polled = true
					assert.Equal(t, http.MethodGet, req.Method)
					assert.Equal(t, "/v1beta1/projects/vertex-project/locations/global/interactions/interaction_test", req.URL.Path)
					assert.Equal(t, tc.wantRevision, req.Header.Get("Api-Revision"))
					body = `{"id":"interaction_test","status":"completed","outputs":[{"type":"video","mime_type":"video/mp4","data":"dmlkZW8="}]}`
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{},
					Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ApiKey: string(credential), UpstreamModelName: tc.model,
			}}
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)
			req := httptest.NewRequest(http.MethodPost, "https://vertex.test/interactions", nil)
			require.NoError(t, adaptor.BuildRequestHeader(nil, req, info))
			assert.Equal(t, tc.wantRevision, req.Header.Get("Api-Revision"))
			task := &model.Task{
				Properties: model.Properties{UpstreamModelName: tc.storedModel},
				PrivateData: model.TaskPrivateData{
					UpstreamTaskID: taskcommon.EncodeLocalTaskID("interactions/interaction_test"),
				},
			}
			response, err := adaptor.FetchTask("https://vertex.test", string(credential), task, "")
			require.NoError(t, err)
			defer response.Body.Close()
			assert.True(t, polled)
			raw, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			result, err := adaptor.ParseTaskResult(task, response, raw)
			require.NoError(t, err)
			assert.Equal(t, model.TaskStatusSuccess, result.Status)
			assert.Equal(t, "data:video/mp4;base64,dmlkZW8=", result.Url)
		})
	}
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
	result, err := (&TaskAdaptor{}).ParseTaskResult(nil, nil, []byte(`{
		"name":"projects/project/locations/us-central1/publishers/google/models/veo-3.1-generate-001/operations/filtered",
		"done":true,
		"response":{"raiMediaFilteredCount":1,"raiMediaFilteredReasons":["1 video was filtered; support code: 15236754"]}
	}`))

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, result.Status)
	assert.Equal(t, "100%", result.Progress)
	assert.Contains(t, result.Reason, "15236754")
}

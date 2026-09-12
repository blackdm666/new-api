package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"github.com/QuantumNous/new-api/common"
	"image"
	"image/png"
	"io"
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
	result, err := (&TaskAdaptor{}).ParseTaskResult(nil, nil, []byte(`{
		"name":"models/veo-3.1/operations/filtered",
		"done":true,
		"response":{"generateVideoResponse":{"raiMediaFilteredCount":1,"raiMediaFilteredReasons":["blocked by Google safety policy; support code: 123"]}}
	}`))

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, result.Status)
	assert.Equal(t, "100%", result.Progress)
	assert.Contains(t, result.Reason, "support code: 123")
}

func TestVeoExplicitImageModes(t *testing.T) {
	var buffer bytes.Buffer
	require.NoError(t, png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	source := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
	for _, tc := range []struct {
		name, mode          string
		count, duration     int
		wantFirst, wantLast bool
		wantRefs            int
		invalid             bool
	}{
		{name: "legacy first", count: 2, duration: 4, wantFirst: true},
		{name: "first frame", mode: "frames", count: 1, duration: 4, wantFirst: true},
		{name: "first and last", mode: "frames", count: 2, duration: 8, wantFirst: true, wantLast: true},
		{name: "one reference", mode: "reference", count: 1, duration: 8, wantRefs: 1},
		{name: "three references", mode: "reference", count: 3, duration: 8, wantRefs: 3},
		{name: "too many frames", mode: "frames", count: 3, duration: 8, invalid: true},
		{name: "too many references", mode: "reference", count: 4, duration: 8, invalid: true},
		{name: "wrong duration", mode: "reference", count: 1, duration: 4, invalid: true},
		{name: "unknown mode", mode: "typo", count: 1, duration: 8, invalid: true},
		{name: "text only", mode: "reference", duration: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := relaycommon.TaskSubmitReq{Model: "veo-3.1", Prompt: "A product on a table", Duration: tc.duration}
			if tc.mode != "" {
				req.Metadata = map[string]interface{}{"video_mode": tc.mode}
			}
			for i := 0; i < tc.count; i++ {
				req.Images = append(req.Images, source)
			}
			data, err := common.Marshal(req)
			require.NoError(t, err)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(data))
			ctx.Request.Header.Set("Content-Type", "application/json")
			info := &relaycommon.RelayInfo{OriginModelName: "veo-3.1", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "veo-3.1-generate-preview"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			adaptor := &TaskAdaptor{}
			taskErr := adaptor.ValidateRequestAndSetAction(ctx, info)
			if tc.invalid {
				require.NotNil(t, taskErr)
				assert.Equal(t, "invalid_veo_request", taskErr.Code)
				return
			}
			require.Nil(t, taskErr)
			reader, err := adaptor.BuildRequestBody(ctx, info)
			require.NoError(t, err)
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			var payload VeoRequestPayload
			require.NoError(t, common.Unmarshal(body, &payload))
			require.Len(t, payload.Instances, 1)
			instance := payload.Instances[0]
			assert.Equal(t, tc.wantFirst, instance.Image != nil)
			assert.Equal(t, tc.wantLast, instance.LastFrame != nil)
			assert.Len(t, instance.ReferenceImages, tc.wantRefs)
			for _, ref := range instance.ReferenceImages {
				assert.Equal(t, "asset", ref.ReferenceType)
				assert.Equal(t, "image/png", ref.Image.MimeType)
			}
		})
	}
}

func TestVeoRejectsMalformedExplicitImages(t *testing.T) {
	for _, source := range []string{"https://example.com/image.png", "data:image/png;base64,!!!", "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("not an image")), strings.Repeat("A", base64.StdEncoding.EncodedLen(maxVeoImageSize)+129)} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		ctx.Set("task_request", relaycommon.TaskSubmitReq{Prompt: "scene", Duration: 8, Images: []string{source}, Metadata: map[string]interface{}{"video_mode": "reference"}})
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "veo-3.1-generate-preview"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
		_, err := BuildVeoInstance(ctx, info)
		require.Error(t, err)
	}
}

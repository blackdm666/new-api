package gemini

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 1x1 PNG.
const testPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

func newGeminiImageInfo(relayMode int, model string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode:       relayMode,
		RelayFormat:     types.RelayFormatOpenAIImage,
		OriginModelName: model,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: model,
			ApiKey:            "test-key",
		},
	}
}

func TestResolveGeminiImageConfig(t *testing.T) {
	tests := []struct {
		name              string
		size, quality     string
		supportsImageSize bool
		want              geminiImageConfig
		wantErr           string
	}{
		{name: "empty", supportsImageSize: true},
		{name: "auto", size: "auto", supportsImageSize: true},
		{name: "square", size: "1024x1024", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "1:1"}},
		{name: "landscape", size: "1536x1024", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "3:2"}},
		{name: "portrait", size: "1024x1536", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "2:3"}},
		{name: "dalle wide", size: "1792x1024", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "16:9"}},
		{name: "dalle tall", size: "1024x1792", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "9:16"}},
		{name: "ratio", size: "21:9", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "21:9"}},
		{name: "unreduced ratio", size: "32:18", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "16:9"}},
		{name: "full hd implies 2K", size: "1920x1080", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "16:9", ImageSize: "2K"}},
		{name: "4096 implies 4K", size: "4096x4096", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "1:1", ImageSize: "4K"}},
		{name: "near ratio", size: "1365x1024", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "4:3"}},
		{name: "hd quality", size: "1024x1024", quality: "hd", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "1:1", ImageSize: "2K"}},
		{name: "explicit quality wins over size", size: "4096x4096", quality: "1k", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "1:1", ImageSize: "1K"}},
		{name: "4K quality", size: "16:9", quality: "4K", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "16:9", ImageSize: "4K"}},
		{name: "legacy model drops imageSize", size: "4096x4096", quality: "4K", want: geminiImageConfig{AspectRatio: "1:1"}},
		{name: "garbage size", size: "big", supportsImageSize: true, wantErr: `invalid size "big"`},
		{name: "reduced 21:9", size: "7:3", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "21:9"}},
		{name: "ultrawide pixels", size: "2560x1080", supportsImageSize: true, want: geminiImageConfig{AspectRatio: "21:9", ImageSize: "2K"}},
		{name: "unsupported ratio", size: "5:3", supportsImageSize: true, wantErr: `unsupported aspect ratio "5:3"`},
		{name: "unmatched pixel size", size: "1000x300", supportsImageSize: true, wantErr: `size "1000x300" has no matching Gemini aspect ratio`},
		{name: "unknown quality", quality: "ultra", supportsImageSize: true, wantErr: `unsupported quality "ultra"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, apiErr := resolveGeminiImageConfig(tt.size, tt.quality, tt.supportsImageSize)
			if tt.wantErr != "" {
				require.NotNil(t, apiErr)
				assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				assert.Contains(t, apiErr.Error(), tt.wantErr)
				return
			}
			require.Nil(t, apiErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGeminiImageModelDetection(t *testing.T) {
	tests := []struct {
		model    string
		relay    bool
		sizeable bool
	}{
		{model: "gemini-3-pro-image", relay: true, sizeable: true},
		{model: "gemini-3.1-flash-image", relay: true, sizeable: true},
		{model: "gemini-3.1-flash-lite-image", relay: true, sizeable: true},
		{model: "gemini-nano-banana-2.1", relay: true, sizeable: true},
		{model: "nano-banana-pro-preview", relay: true, sizeable: true},
		{model: "gemini-2.5-flash-image", relay: true, sizeable: false},
		{model: "imagen-4.0-generate-001", relay: false, sizeable: true},
		{model: "gemini-2.5-flash", relay: false, sizeable: false},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.relay, IsGeminiImageRelay(newGeminiImageInfo(relayconstant.RelayModeImagesGenerations, tt.model)))
			if tt.relay {
				assert.Equal(t, tt.sizeable, geminiImageModelSupportsImageSize(tt.model))
			}
		})
	}
	assert.False(t, IsGeminiImageRelay(newGeminiImageInfo(relayconstant.RelayModeChatCompletions, "gemini-3-pro-image")))

	mapped := newGeminiImageInfo(relayconstant.RelayModeImagesGenerations, "gemini-nano-banana-2.1")
	mapped.UpstreamModelName = "proxy-custom-alias"
	assert.True(t, IsGeminiImageRelay(mapped), "origin model name keeps an upstream alias on the generateContent path")
}

func TestConvertOpenAIImageGenerationToGemini(t *testing.T) {
	info := newGeminiImageInfo(relayconstant.RelayModeImagesGenerations, "gemini-3-pro-image-preview")
	converted, err := (&Adaptor{}).ConvertImageRequest(nil, info, dto.ImageRequest{
		Model:   "gemini-3-pro-image-preview",
		Prompt:  "a red bicycle",
		N:       common.GetPointer(uint(2)),
		Size:    "1792x1024",
		Quality: "hd",
		User:    []byte(`"end-user-1"`),
	})
	require.NoError(t, err)
	payload, err := common.Marshal(converted)
	require.NoError(t, err)

	assert.Equal(t, "user", gjson.GetBytes(payload, "contents.0.role").Str)
	assert.Equal(t, "a red bicycle", gjson.GetBytes(payload, "contents.0.parts.0.text").Str)
	assert.Equal(t, int64(1), gjson.GetBytes(payload, "contents.0.parts.#").Int())
	assert.JSONEq(t, `["TEXT","IMAGE"]`, gjson.GetBytes(payload, "generationConfig.responseModalities").Raw)
	assert.JSONEq(t, `{"aspectRatio":"16:9","imageSize":"2K"}`, gjson.GetBytes(payload, "generationConfig.imageConfig").Raw)
	assert.False(t, gjson.GetBytes(payload, "generationConfig.candidateCount").Exists(), "n is served by separate calls")
	assert.False(t, gjson.GetBytes(payload, "n").Exists())
	assert.NotContains(t, string(payload), "end-user-1")
	assert.Equal(t, int64(len(SafetySettingList)), gjson.GetBytes(payload, "safetySettings.#").Int())

	url, err := (&Adaptor{}).GetRequestURL(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://g.example", UpstreamModelName: "gemini-3-pro-image-preview"}})
	require.NoError(t, err)
	assert.Equal(t, "https://g.example/v1beta/models/gemini-3-pro-image-preview:generateContent", url)
}

func TestConvertOpenAIImageRequestToGeminiRejectsUnsupportedInput(t *testing.T) {
	tests := []struct {
		name    string
		mode    int
		request dto.ImageRequest
		wantErr string
	}{
		{name: "stream", request: dto.ImageRequest{Prompt: "p", Stream: common.GetPointer(true)}, wantErr: "stream is not supported"},
		{name: "n above fan-out bound", request: dto.ImageRequest{Prompt: "p", N: common.GetPointer(uint(11))}, wantErr: "n must be an integer between 1 and 10"},
		{name: "empty prompt", request: dto.ImageRequest{Prompt: " "}, wantErr: "prompt is required"},
		{name: "json mask", mode: relayconstant.RelayModeImagesEdits, request: dto.ImageRequest{Prompt: "p", Image: []byte(`"data:image/png;base64,` + testPNGBase64 + `"`), Mask: []byte(`{"image_url":"https://example.com/m.png"}`)}, wantErr: "mask is not supported"},
		{name: "file id", mode: relayconstant.RelayModeImagesEdits, request: dto.ImageRequest{Prompt: "p", Images: []byte(`[{"file_id":"file-1"}]`)}, wantErr: "file_id image inputs are not supported"},
		{name: "edit without image", mode: relayconstant.RelayModeImagesEdits, request: dto.ImageRequest{Prompt: "p"}, wantErr: "image is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode := tt.mode
			if mode == 0 {
				mode = relayconstant.RelayModeImagesGenerations
			}
			_, err := (&Adaptor{}).ConvertImageRequest(nil, newGeminiImageInfo(mode, "gemini-3.1-flash-image"), tt.request)
			var apiErr *types.NewAPIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.Contains(t, apiErr.Error(), tt.wantErr)
		})
	}
}

func TestConvertOpenAIImageEditJSONImagesToInlineData(t *testing.T) {
	info := newGeminiImageInfo(relayconstant.RelayModeImagesEdits, "gemini-3.1-flash-image")
	converted, err := (&Adaptor{}).ConvertImageRequest(nil, info, dto.ImageRequest{
		Prompt: "make it blue",
		Images: []byte(`[{"image_url":"data:image/png;base64,` + testPNGBase64 + `"},{"image_url":{"url":"data:image/png;base64,` + testPNGBase64 + `"}}]`),
	})
	require.NoError(t, err)
	parts := converted.(*dto.GeminiChatRequest).Contents[0].Parts
	require.Len(t, parts, 3)
	assert.Equal(t, "make it blue", parts[0].Text)
	for _, part := range parts[1:] {
		require.NotNil(t, part.InlineData)
		assert.Equal(t, "image/png", part.InlineData.MimeType)
		assert.Equal(t, testPNGBase64, part.InlineData.Data)
	}
}

func newMultipartImageEditContext(t *testing.T, withMask bool) *gin.Context {
	t.Helper()
	png, err := base64.StdEncoding.DecodeString(testPNGBase64)
	require.NoError(t, err)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gemini-3-pro-image"))
	require.NoError(t, writer.WriteField("prompt", "merge these"))
	require.NoError(t, writer.WriteField("response_format", "url"))
	files := []string{"image[]", "image[]"}
	if withMask {
		files = append(files, "mask")
	}
	for i, field := range files {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename="in%d.png"`, field, i))
		header.Set("Content-Type", "image/png")
		part, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = part.Write(png)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return c
}

func TestConvertOpenAIImageEditMultipartToGemini(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newMultipartImageEditContext(t, false)
	request, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
	require.NoError(t, err)
	info := newGeminiImageInfo(relayconstant.RelayModeImagesEdits, "gemini-3-pro-image")

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, *request)
	require.NoError(t, err)
	parts := converted.(*dto.GeminiChatRequest).Contents[0].Parts
	require.Len(t, parts, 3)
	assert.Equal(t, "merge these", parts[0].Text)
	for _, part := range parts[1:] {
		require.NotNil(t, part.InlineData)
		assert.Equal(t, "image/png", part.InlineData.MimeType)
		assert.Equal(t, testPNGBase64, part.InlineData.Data)
	}

	headers := http.Header{}
	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &headers, info))
	assert.Equal(t, "application/json", headers.Get("Content-Type"), "the multipart edit is sent upstream as JSON")

	masked := newMultipartImageEditContext(t, true)
	maskedRequest, err := helper.GetAndValidOpenAIImageRequest(masked, relayconstant.RelayModeImagesEdits)
	require.NoError(t, err)
	_, err = (&Adaptor{}).ConvertImageRequest(masked, info, *maskedRequest)
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Contains(t, apiErr.Error(), "mask is not supported")
}

func geminiImageResponseJSON(promptTokens, imageTokens int) string {
	return fmt.Sprintf(`{
		"candidates":[{"content":{"role":"model","parts":[
			{"text":"Here you go"},
			{"inlineData":{"mimeType":"image/png","data":%q}}
		]},"finishReason":"STOP"}],
		"usageMetadata":{
			"promptTokenCount":%d,"candidatesTokenCount":%d,"totalTokenCount":%d,
			"promptTokensDetails":[{"modality":"TEXT","tokenCount":%d}],
			"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":%d}]
		},
		"modelVersion":"gemini-3-pro-image-preview"
	}`, testPNGBase64, promptTokens, imageTokens, promptTokens+imageTokens, promptTokens, imageTokens)
}

func TestGeminiImageGenerateContentHandlerConvertsResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		format     string
		body       string
		wantStatus int
		wantCode   types.ErrorCode
		wantErr    string
	}{
		{name: "b64 default", body: geminiImageResponseJSON(12, 1120)},
		{name: "url as data url", format: "url", body: geminiImageResponseJSON(12, 1120)},
		{name: "prompt blocked", body: `{"promptFeedback":{"blockReason":"PROHIBITED_CONTENT"},"usageMetadata":{"promptTokenCount":9,"totalTokenCount":9}}`, wantStatus: http.StatusBadRequest, wantCode: types.ErrorCodePromptBlocked, wantErr: "PROHIBITED_CONTENT"},
		{name: "image safety", body: `{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"IMAGE_SAFETY"}]}`, wantStatus: http.StatusBadRequest, wantCode: types.ErrorCodePromptBlocked, wantErr: "IMAGE_SAFETY"},
		{name: "text only", body: `{"candidates":[{"content":{"role":"model","parts":[{"text":"I cannot draw that."}]},"finishReason":"STOP"}]}`, wantStatus: http.StatusBadRequest, wantCode: types.ErrorCodeBadResponse, wantErr: "I cannot draw that."},
		{name: "empty", body: `{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}]}`, wantStatus: http.StatusInternalServerError, wantCode: types.ErrorCodeEmptyResponse, wantErr: "no image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			info := newGeminiImageInfo(relayconstant.RelayModeImagesGenerations, "gemini-3-pro-image-preview")
			info.Request = &dto.ImageRequest{ResponseFormat: tt.format}
			info.PriceData = hosttypes.PriceData{UsePrice: true}
			info.PriceData.AddOtherRatio("n", 1)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tt.body))}

			usage, apiErr := (&Adaptor{}).DoResponse(c, resp, info)
			if tt.wantStatus != 0 {
				require.NotNil(t, apiErr)
				assert.Nil(t, usage, "a response without images is not settled as a success")
				assert.Equal(t, tt.wantStatus, apiErr.StatusCode)
				assert.Equal(t, tt.wantCode, apiErr.GetErrorCode())
				assert.Contains(t, apiErr.Error(), tt.wantErr)
				assert.Empty(t, recorder.Body.String())
				return
			}
			require.Nil(t, apiErr)
			body := recorder.Body.Bytes()
			assert.Equal(t, int64(1), gjson.GetBytes(body, "data.#").Int())
			if tt.format == "url" {
				assert.Equal(t, "data:image/png;base64,"+testPNGBase64, gjson.GetBytes(body, "data.0.url").Str)
				assert.Empty(t, gjson.GetBytes(body, "data.0.b64_json").Str)
			} else {
				assert.Equal(t, testPNGBase64, gjson.GetBytes(body, "data.0.b64_json").Str)
				assert.Empty(t, gjson.GetBytes(body, "data.0.url").Str)
			}
			geminiUsage := usage.(*dto.Usage)
			assert.Equal(t, 12, geminiUsage.PromptTokens)
			assert.Equal(t, 1120, geminiUsage.CompletionTokens)
			assert.Equal(t, 1120, geminiUsage.CompletionTokenDetails.ImageTokens)
			require.NotNil(t, geminiUsage.BillingUsage)
			assert.Equal(t, dto.BillingUsageSourceGeminiChat, geminiUsage.BillingUsage.Source)
			assert.Equal(t, 1.0, info.PriceData.OtherRatios()["n"])
		})
	}
}

// TestGeminiImageFanOutEndToEnd drives conversion, the n-call fan-out against a
// mocked generateContent upstream, and the OpenAI Images response, including
// partial failures that must bill only delivered images.
func TestGeminiImageFanOutEndToEnd(t *testing.T) {
	service.InitHttpClient()
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name         string
		n            int
		failCalls    map[int32]string
		wantStatus   int
		wantImages   int
		wantPrompt   int
		wantComplete int
	}{
		{name: "all succeed", n: 3, wantImages: 3, wantPrompt: 30, wantComplete: 3870},
		{name: "partial failure bills delivered images", n: 3, failCalls: map[int32]string{2: "429", 3: "safety"}, wantImages: 1, wantPrompt: 10, wantComplete: 1290},
		{name: "all fail returns upstream error", n: 2, failCalls: map[int32]string{1: "429", 2: "429"}, wantStatus: http.StatusTooManyRequests},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				body, _ := io.ReadAll(r.Body)
				assert.Equal(t, "/v1beta/models/gemini-3-pro-image-preview:generateContent", r.URL.Path)
				assert.Equal(t, "test-key", r.Header.Get("x-goog-api-key"))
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Equal(t, "a lighthouse", gjson.GetBytes(body, "contents.0.parts.0.text").Str)
				w.Header().Set("Content-Type", "application/json")
				switch tt.failCalls[call] {
				case "429":
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = io.WriteString(w, `{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED"}}`)
				case "safety":
					_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"IMAGE_SAFETY"}],"usageMetadata":{"promptTokenCount":10,"totalTokenCount":10}}`)
				default:
					_, _ = io.WriteString(w, geminiImageResponseJSON(10, 1290))
				}
			}))
			t.Cleanup(upstream.Close)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			c.Request.Header.Set("Content-Type", "application/json")
			request := &dto.ImageRequest{Model: "nano-banana-pro", Prompt: "a lighthouse", N: common.GetPointer(uint(tt.n)), ResponseFormat: "b64_json"}
			info := newGeminiImageInfo(relayconstant.RelayModeImagesGenerations, "gemini-3-pro-image-preview")
			info.OriginModelName = "nano-banana-pro"
			info.ChannelBaseUrl = upstream.URL
			info.Request = request
			info.ImageRequestCount = tt.n
			info.PriceData = hosttypes.PriceData{UsePrice: true}
			info.PriceData.AddOtherRatio("n", float64(tt.n))

			adaptor := &Adaptor{}
			converted, err := adaptor.ConvertImageRequest(c, info, *request)
			require.NoError(t, err)
			payload, err := common.Marshal(converted)
			require.NoError(t, err)
			respAny, err := adaptor.DoRequest(c, info, bytes.NewReader(payload))
			require.NoError(t, err)
			resp := respAny.(*http.Response)
			assert.Equal(t, int32(tt.n), calls.Load(), "one upstream call per requested image")
			if tt.wantStatus != 0 {
				assert.Equal(t, tt.wantStatus, resp.StatusCode, "ImageHelper maps this through RelayErrorHandler")
				return
			}
			require.Equal(t, http.StatusOK, resp.StatusCode)

			usage, apiErr := adaptor.DoResponse(c, resp, info)
			require.Nil(t, apiErr)
			body := recorder.Body.Bytes()
			assert.Equal(t, int64(tt.wantImages), gjson.GetBytes(body, "data.#").Int())
			for i := range tt.wantImages {
				assert.Equal(t, testPNGBase64, gjson.GetBytes(body, fmt.Sprintf("data.%d.b64_json", i)).Str)
			}
			geminiUsage := usage.(*dto.Usage)
			assert.Equal(t, tt.wantPrompt, geminiUsage.PromptTokens)
			assert.Equal(t, tt.wantComplete, geminiUsage.CompletionTokens)
			assert.Equal(t, tt.wantComplete, geminiUsage.CompletionTokenDetails.ImageTokens)
			assert.Equal(t, float64(tt.wantImages), info.PriceData.OtherRatios()["n"], "per-image price settles on delivered images")
		})
	}
}

func TestImagenImageRequestUnchanged(t *testing.T) {
	info := newGeminiImageInfo(relayconstant.RelayModeImagesGenerations, "imagen-4.0-generate-001")
	info.ChannelBaseUrl = "https://g.example"
	converted, err := (&Adaptor{}).ConvertImageRequest(nil, info, dto.ImageRequest{
		Prompt:  "a fox",
		N:       common.GetPointer(uint(2)),
		Size:    "1792x1024",
		Quality: "hd",
	})
	require.NoError(t, err)
	assert.Equal(t, dto.GeminiImageRequest{
		Instances: []dto.GeminiImageInstance{{Prompt: "a fox"}},
		Parameters: dto.GeminiImageParameters{
			SampleCount:      2,
			AspectRatio:      "16:9",
			PersonGeneration: "allow_adult",
			ImageSize:        "2K",
		},
	}, converted)
	url, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://g.example/v1beta/models/imagen-4.0-generate-001:predict", url)

	_, err = (&Adaptor{}).ConvertImageRequest(nil, newGeminiImageInfo(relayconstant.RelayModeImagesGenerations, "gemini-2.5-flash"), dto.ImageRequest{Prompt: "a fox"})
	require.EqualError(t, err, "not supported model for image generation, only imagen and Gemini image models are supported")
}

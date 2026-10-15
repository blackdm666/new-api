package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestConvertImageEditRequestMultipart verifies that ConvertImageRequest
// re-serializes multipart image edit requests with all fields (including
// stream) and the file intact, both when the form was already parsed and when
// it must be re-parsed from the reusable body.
func TestConvertImageEditRequestMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newMultipartContext := func(t *testing.T, prompt string) *gin.Context {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("model", "gpt-image-1"))
		require.NoError(t, writer.WriteField("prompt", prompt))
		require.NoError(t, writer.WriteField("stream", "true"))
		require.NoError(t, writer.WriteField("partial_images", "3"))
		part, err := writer.CreateFormFile("image", "input.png")
		require.NoError(t, err)
		_, err = part.Write([]byte("fake image"))
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return c
	}

	convertAndReplay := func(t *testing.T, c *gin.Context, prompt string) {
		info := &relaycommon.RelayInfo{
			RelayMode: relayconstant.RelayModeImagesEdits,
		}
		request := dto.ImageRequest{
			Model:  "gpt-image-1",
			Prompt: prompt,
			Stream: common.GetPointer(true),
		}

		converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
		require.NoError(t, err)
		convertedBody, ok := converted.(*bytes.Buffer)
		require.True(t, ok)

		replayedRequest := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(convertedBody.Bytes()))
		replayedRequest.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
		require.NoError(t, replayedRequest.ParseMultipartForm(32<<20))

		require.Equal(t, "gpt-image-1", replayedRequest.PostForm.Get("model"))
		require.Equal(t, prompt, replayedRequest.PostForm.Get("prompt"))
		require.Equal(t, "true", replayedRequest.PostForm.Get("stream"))
		require.Equal(t, "3", replayedRequest.PostForm.Get("partial_images"))
		require.Len(t, replayedRequest.MultipartForm.File["image"], 1)

		file, err := replayedRequest.MultipartForm.File["image"][0].Open()
		require.NoError(t, err)
		defer file.Close()
		fileBytes, err := io.ReadAll(file)
		require.NoError(t, err)
		require.Equal(t, []byte("fake image"), fileBytes)
	}

	t.Run("with pre-parsed form", func(t *testing.T) {
		prompt := "edit this image"
		c := newMultipartContext(t, prompt)
		require.NoError(t, c.Request.ParseMultipartForm(32<<20))

		convertAndReplay(t, c, prompt)
	})

	t.Run("re-parses reusable body when form is missing", func(t *testing.T) {
		prompt := "edit without pre-parsed form"
		c := newMultipartContext(t, prompt)

		storage, err := common.GetBodyStorage(c)
		require.NoError(t, err)
		c.Request.Body = io.NopCloser(storage)
		c.Request.MultipartForm = nil
		c.Request.PostForm = nil

		convertAndReplay(t, c, prompt)
	})
}

// stubXinMengMedia records reference uploads and serves result downloads
// without the Worker or the network.
func stubXinMengMedia(t *testing.T, results map[string][]byte) *[][]byte {
	t.Helper()
	var uploads [][]byte
	previousUpload, previousDownload := uploadXinMengReferenceImage, downloadXinMengResultImage
	uploadXinMengReferenceImage = func(_ context.Context, image io.ReadSeeker) (string, error) {
		content, err := io.ReadAll(image)
		require.NoError(t, err)
		uploads = append(uploads, content)
		return fmt.Sprintf("https://assets.example/media/reference-media/%d.png", len(uploads)), nil
	}
	downloadXinMengResultImage = func(_ context.Context, sourceURL string) ([]byte, error) {
		content, ok := results[sourceURL]
		require.True(t, ok, "unexpected download %s", sourceURL)
		return content, nil
	}
	t.Cleanup(func() {
		uploadXinMengReferenceImage, downloadXinMengResultImage = previousUpload, previousDownload
	})
	return &uploads
}

func newXinMengJSONContext(path string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func xinMengInfo(mode int, model string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode:   mode,
		RelayFormat: types.RelayFormatOpenAIImage,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: model},
	}
}

// XinMeng (Midjourney / Seedream behind an OpenAI-type channel) rejects GPT
// Image fields with "property ... should not exist", has no /images/edits and
// reads reference images only by URL.
func TestXinMengImageRequestsDropGPTOnlyFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stubXinMengMedia(t, nil)
	for _, mode := range []int{relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits} {
		request := dto.ImageRequest{
			Model:          "midjourney-v8.2",
			Prompt:         "a cat",
			N:              common.GetPointer(uint(2)),
			Size:           "1024x1024",
			Quality:        "standard",
			ResponseFormat: "url",
			Style:          json.RawMessage(`"vivid"`),
			Background:     json.RawMessage(`"transparent"`),
			Moderation:     json.RawMessage(`"low"`),
			OutputFormat:   json.RawMessage(`"png"`),
			PartialImages:  json.RawMessage(`2`),
			Stream:         common.GetPointer(true),
			Image:          json.RawMessage(`"https://88api.ai/cat.png"`),
		}

		converted, err := (&Adaptor{}).ConvertImageRequest(newXinMengJSONContext("/v1/images/generations"), xinMengInfo(mode, "midjourney-v8.2"), request)
		require.NoError(t, err)
		body, err := common.Marshal(converted)
		require.NoError(t, err)
		assert.JSONEq(t, `{
			"model":"midjourney-v8.2",
			"prompt":"a cat",
			"n":2,
			"size":"1024x1024",
			"quality":"standard",
			"response_format":"url",
			"image":"https://88api.ai/cat.png"
		}`, string(body), "relay mode %d", mode)
	}
}

func TestXinMengImageInputsBecomePublicURLs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	png := []byte("\x89PNG\r\n\x1a\nfake image")
	encoded := base64.StdEncoding.EncodeToString(png)

	for _, tc := range []struct {
		name, model, image, want string
		uploads                  int
	}{
		{"data URL", "midjourney-v8.2", `"data:image/png;base64,` + encoded + `"`, `"https://assets.example/media/reference-media/1.png"`, 1},
		{"bare base64", "midjourney-v8.2-fast", `"` + encoded + `"`, `"https://assets.example/media/reference-media/1.png"`, 1},
		{"URL kept", "midjourney-v8.2", `"https://88api.ai/cat.png"`, `"https://88api.ai/cat.png"`, 0},
		{"mixed array", "midjourney-v8.2-turbo", `["https://88api.ai/cat.png","data:image/png;base64,` + encoded + `"]`, `["https://88api.ai/cat.png","https://assets.example/media/reference-media/1.png"]`, 1},
		{"other models untouched", "gpt-image-1", `"data:image/png;base64,` + encoded + `"`, `"data:image/png;base64,` + encoded + `"`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uploads := stubXinMengMedia(t, nil)
			request := dto.ImageRequest{Model: tc.model, Prompt: "edit", Image: json.RawMessage(tc.image)}

			converted, err := (&Adaptor{}).ConvertImageRequest(newXinMengJSONContext("/v1/images/edits"), xinMengInfo(relayconstant.RelayModeImagesEdits, tc.model), request)
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, gjson.GetBytes(body, "image").Raw)
			require.Len(t, *uploads, tc.uploads)
			for _, upload := range *uploads {
				assert.Equal(t, png, upload)
			}
		})
	}

	t.Run("invalid image is a client error", func(t *testing.T) {
		uploads := stubXinMengMedia(t, nil)
		request := dto.ImageRequest{Model: "midjourney-v8.2", Prompt: "edit", Image: json.RawMessage(`"not an image!"`)}
		_, err := (&Adaptor{}).ConvertImageRequest(newXinMengJSONContext("/v1/images/edits"), xinMengInfo(relayconstant.RelayModeImagesEdits, "midjourney-v8.2"), request)
		var apiErr *types.NewAPIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
		assert.Empty(t, *uploads)
	})
}

func TestXinMengMultipartImageEditBecomesJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	uploads := stubXinMengMedia(t, nil)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "midjourney-v8.2"))
	require.NoError(t, writer.WriteField("prompt", "edit"))
	require.NoError(t, writer.WriteField("n", "1"))
	require.NoError(t, writer.WriteField("response_format", "url"))
	require.NoError(t, writer.WriteField("stream", "true"))
	require.NoError(t, writer.WriteField("output_format", "png"))
	for _, name := range []string{"first.png", "second.png"} {
		part, err := writer.CreateFormFile("image[]", name)
		require.NoError(t, err)
		_, err = part.Write([]byte(name))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	info := xinMengInfo(relayconstant.RelayModeImagesEdits, "midjourney-v8.2")
	request := dto.ImageRequest{Model: "midjourney-v8.2", Prompt: "edit", N: common.GetPointer(uint(1)), Stream: common.GetPointer(true)}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)
	_, isMultipart := converted.(*bytes.Buffer)
	require.False(t, isMultipart)
	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"midjourney-v8.2",
		"prompt":"edit",
		"n":1,
		"response_format":"url",
		"image":["https://assets.example/media/reference-media/1.png","https://assets.example/media/reference-media/2.png"]
	}`, string(encoded))
	assert.Equal(t, [][]byte{[]byte("first.png"), []byte("second.png")}, *uploads)

	header := http.Header{}
	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &header, info))
	assert.Equal(t, "application/json", header.Get("Content-Type"))
}

// XinMeng result links name its private image host, so clients receive our
// own copy (or inline bytes) while the billed count still follows the payloads.
func TestXinMengImageResultsAreRehosted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := `{"created":1792057170,"provider":"xinmeng","data":[{"url":"https://getapib.org/image/a.png"},{"url":"https://getapib.org/image/b.png","revised_prompt":"cat"}],"usage":{"total_tokens":1}}`
	results := map[string][]byte{"https://getapib.org/image/a.png": []byte("first"), "https://getapib.org/image/b.png": []byte("second")}

	for _, tc := range []struct {
		name, responseFormat, want string
	}{
		{"url", "url", `{"created":1792057170,"data":[
			{"url":"https://assets.example/media/reference-media/1.png","b64_json":"","revised_prompt":""},
			{"url":"https://assets.example/media/reference-media/2.png","b64_json":"","revised_prompt":"cat"}],"usage":{"total_tokens":1}}`},
		{"b64_json", "b64_json", `{"created":1792057170,"data":[
			{"url":"","b64_json":"Zmlyc3Q=","revised_prompt":""},
			{"url":"","b64_json":"c2Vjb25k","revised_prompt":"cat"}],"usage":{"total_tokens":1}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubXinMengMedia(t, results)
			c, recorder, resp, info := newImageTestContext(t, upstream, "application/json", false)
			info.UpstreamModelName = "midjourney-v8.2"
			info.Request = &dto.ImageRequest{Model: "midjourney-v8.2", ResponseFormat: tc.responseFormat}
			info.PriceData.UsePrice = true
			info.PriceData.AddOtherRatio("n", 1)

			_, apiErr := OpenaiImageHandler(c, info, resp)
			require.Nil(t, apiErr)
			assert.JSONEq(t, tc.want, recorder.Body.String())
			assert.NotContains(t, recorder.Body.String(), "getapib")
			assert.Equal(t, float64(2), info.PriceData.OtherRatios()["n"])
		})
	}

	t.Run("delivery failure hides the upstream host", func(t *testing.T) {
		stubXinMengMedia(t, results)
		downloadXinMengResultImage = func(_ context.Context, sourceURL string) ([]byte, error) {
			return nil, fmt.Errorf("Get %q: timeout", sourceURL)
		}
		c, recorder, resp, info := newImageTestContext(t, upstream, "application/json", false)
		info.UpstreamModelName = "midjourney-v8.2"
		info.Request = &dto.ImageRequest{Model: "midjourney-v8.2"}

		_, apiErr := OpenaiImageHandler(c, info, resp)
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
		assert.NotContains(t, apiErr.Error(), "getapib")
		assert.Empty(t, recorder.Body.String())
	})
}

func TestImageEditRequestURL(t *testing.T) {
	for _, tc := range []struct {
		model, baseURL, want string
	}{
		{"doubao-seedream-5-0-pro", "https://www.jimengvip.online", "https://www.jimengvip.online/v1/images/generations"},
		{"midjourney-v8.2", "https://www.jimengvip.online", "https://www.jimengvip.online/v1/images/generations"},
		{"gpt-image-1", "https://api.openai.com", "https://api.openai.com/v1/images/edits"},
	} {
		info := &relaycommon.RelayInfo{
			RelayMode:      relayconstant.RelayModeImagesEdits,
			RelayFormat:    types.RelayFormatOpenAIImage,
			RequestURLPath: "/v1/images/edits",
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl:    tc.baseURL,
				ChannelType:       1,
				UpstreamModelName: tc.model,
			},
		}
		url, err := (&Adaptor{}).GetRequestURL(info)
		require.NoError(t, err)
		assert.Equal(t, tc.want, url, tc.model)
	}
}

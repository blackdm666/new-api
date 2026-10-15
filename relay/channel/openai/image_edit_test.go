package openai

import (
	"bytes"
	"encoding/json"
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

// XinMeng (Midjourney / Seedream behind an OpenAI-type channel) rejects GPT
// Image fields with "property ... should not exist" and has no /images/edits.
func TestXinMengImageRequestsDropGPTOnlyFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []int{relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(`{}`))
		c.Request.Header.Set("Content-Type", "application/json")
		info := &relaycommon.RelayInfo{
			RelayMode:   mode,
			RelayFormat: types.RelayFormatOpenAIImage,
			ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "midjourney-v8.2"},
		}
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
			Image:          json.RawMessage(`"data:image/png;base64,abc"`),
		}

		converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
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
			"image":"data:image/png;base64,abc"
		}`, string(body), "relay mode %d", mode)
	}
}

func TestXinMengMultipartImageEditDropsGPTOnlyFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "midjourney-v8.2"))
	require.NoError(t, writer.WriteField("prompt", "edit"))
	require.NoError(t, writer.WriteField("stream", "true"))
	require.NoError(t, writer.WriteField("partial_images", "3"))
	require.NoError(t, writer.WriteField("output_format", "png"))
	part, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake image"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "midjourney-v8.2"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{Model: "midjourney-v8.2", Prompt: "edit"})
	require.NoError(t, err)
	convertedBody, ok := converted.(*bytes.Buffer)
	require.True(t, ok)
	replayed := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(convertedBody.Bytes()))
	replayed.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	require.NoError(t, replayed.ParseMultipartForm(32<<20))

	assert.Equal(t, "edit", replayed.PostForm.Get("prompt"))
	for _, field := range []string{"stream", "partial_images", "output_format"} {
		assert.NotContains(t, replayed.PostForm, field)
	}
	assert.Len(t, replayed.MultipartForm.File["image"], 1)
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

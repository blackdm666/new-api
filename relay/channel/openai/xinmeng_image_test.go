package openai

import (
	"bytes"
	"encoding/json"
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

func TestXinMengImageGenerationRequestDropsUnsupportedFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesGenerations,
		RelayFormat: types.RelayFormatOpenAIImage,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "midjourney-v8.2",
		},
	}
	request := dto.ImageRequest{
		Model:          "midjourney-v8.2",
		Prompt:         "a cat",
		Size:           "1024x1024",
		Quality:        "standard",
		ResponseFormat: "url",
		Background:     json.RawMessage(`true`),
		OutputFormat:   json.RawMessage(`"png"`),
		PartialImages:  json.RawMessage(`2`),
		Stream:         common.GetPointer(true),
		Image:          json.RawMessage(`"data:image/png;base64,abc"`),
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)
	convertedRequest, ok := converted.(dto.ImageRequest)
	require.True(t, ok)
	body, err := common.Marshal(convertedRequest)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"model":"midjourney-v8.2",
		"prompt":"a cat",
		"size":"1024x1024",
		"quality":"standard",
		"response_format":"url",
		"image":"data:image/png;base64,abc"
	}`, string(body))
}

func TestXinMengImageEditsUseGenerationsEndpoint(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesEdits,
		RelayFormat: types.RelayFormatOpenAIImage,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://www.jimengvip.online",
			ChannelType:       1,
			UpstreamModelName: "doubao-seedream-5-0-pro",
		},
	}

	url, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://www.jimengvip.online/v1/images/generations", url)
}

func TestNonXinMengImageEditsKeepTheirEndpoint(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesEdits,
		RelayFormat: types.RelayFormatOpenAIImage,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://api.openai.com",
			ChannelType:       1,
			UpstreamModelName: "gpt-image-1",
		},
		RequestURLPath: "/v1/images/edits",
	}

	url, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://api.openai.com/v1/images/edits", url)
}

package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsImageGenerationModelRecognizesGPTImageFamily(t *testing.T) {
	tests := []struct {
		modelName string
		want      bool
	}{
		{modelName: "gpt-image-1", want: true},
		{modelName: "gpt-image-1.5", want: true},
		{modelName: "gpt-image-2", want: true},
		{modelName: "openai/gpt-image-3", want: true},
		{modelName: "GPT-IMAGE-2", want: true},
		{modelName: "gpt-5.6-terra", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.modelName, func(t *testing.T) {
			require.Equal(t, tt.want, IsImageGenerationModel(tt.modelName))
		})
	}
}

func TestGPTImageFamilyPrefersImageGenerationEndpoint(t *testing.T) {
	endpoints := GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "gpt-image-2")
	require.Equal(t, []constant.EndpointType{
		constant.EndpointTypeImageGeneration,
		constant.EndpointTypeOpenAI,
	}, endpoints)
}

func TestVideoTaskModelsUseOpenAIVideoEndpoint(t *testing.T) {
	for _, test := range []struct {
		channelType int
		model       string
	}{
		{channelType: constant.ChannelTypeVertexAi, model: "veo-3.1-generate-001"},
		{channelType: constant.ChannelTypeVertexAi, model: "gemini-omni-flash-preview"},
		{channelType: constant.ChannelTypeSub2API, model: "grok-imagine-video-1.5"},
	} {
		t.Run(test.model, func(t *testing.T) {
			require.Equal(t, []constant.EndpointType{constant.EndpointTypeOpenAIVideo}, GetEndpointTypesByChannelType(test.channelType, test.model))
		})
	}
}

func TestWanEndpointsDistinguishImagesFromVideos(t *testing.T) {
	for _, name := range []string{
		"wan2.7-image-pro", "wan2.7-image", "wan2.6-image", "wan2.6-t2i",
		"wan2.5-t2i-preview", "wan2.2-t2i-flash", "wan2.2-t2i-plus",
		"wanx2.1-t2i-turbo", "wanx2.1-t2i-plus", "wanx2.0-t2i-turbo",
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, GetEndpointTypesByChannelType(constant.ChannelTypeAli, name), constant.EndpointTypeImageGeneration)
		})
	}
	for _, name := range []string{
		"wanx2.1-t2v-plus", "wanx2.1-t2v-turbo", "wanx2.1-i2v-plus", "wanx2.1-i2v-turbo",
	} {
		t.Run(name, func(t *testing.T) {
			assert.NotContains(t, GetEndpointTypesByChannelType(constant.ChannelTypeAli, name), constant.EndpointTypeImageGeneration)
		})
	}
}

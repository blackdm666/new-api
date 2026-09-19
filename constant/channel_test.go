package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetChannelBaseURLIsBoundsSafe(t *testing.T) {
	assert.Empty(t, GetChannelBaseURL(ChannelTypeTaskPlugin))
	assert.Empty(t, GetChannelBaseURL(9999))
}

func TestDeployedChannelIdentitiesRemainDistinct(t *testing.T) {
	assert.Equal(t, 63, ChannelTypeTaskPlugin)
	assert.Empty(t, ChannelTypeNames[61])
	assert.Empty(t, ChannelTypeNames[62])
	assert.Equal(t, "vLLM", GetChannelTypeName(ChannelTypeVLLM))
	assert.Equal(t, "SGLang", GetChannelTypeName(ChannelTypeSGLang))
	assert.NotEqual(t, ChannelTypeTaskPlugin, ChannelTypeVLLM)
	assert.NotEqual(t, ChannelTypeTaskPlugin, ChannelTypeSGLang)
	assert.Greater(t, ChannelTypeDummy, ChannelTypeSGLang)
}

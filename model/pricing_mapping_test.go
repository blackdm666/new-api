package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestPricingEndpointDiscoveryUsesMappedModel(t *testing.T) {
	ability := AbilityWithChannel{
		Ability:             Ability{Model: "image-sales-alias"},
		ChannelType:         constant.ChannelTypeOpenAI,
		ChannelModelMapping: `{"image-sales-alias":"gpt-image-2"}`,
	}

	endpoints := getPricingEndpointTypesForAbility(ability, nil)

	assert.Equal(t, constant.EndpointTypeImageGeneration, endpoints[0])
}

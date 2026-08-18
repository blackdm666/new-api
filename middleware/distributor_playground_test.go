package middleware

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func TestChannelSupportsCanonicalPlaygroundPathForModel(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeAdvancedCustom}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{
			Routes: []dto.AdvancedCustomRoute{
				{
					IncomingPath: "/v1/chat/completions",
					UpstreamPath: "/v1/responses",
					Models:       []string{"deepseek-v4-pro"},
				},
			},
		},
	})

	if !channelSupportsRequestPath(channel, "/pg/chat/completions", "deepseek-v4-pro") {
		t.Fatal("Playground path should match the channel's canonical /v1 route")
	}
	if channelSupportsRequestPath(channel, "/pg/chat/completions", "other-model") {
		t.Fatal("canonical path matching must still enforce route model restrictions")
	}
}

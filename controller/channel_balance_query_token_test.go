package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelBalanceQueryToken(t *testing.T) {
	channel := &model.Channel{}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		BalanceQuery: &dto.ChannelBalanceQueryConfig{
			Mode: dto.ChannelBalanceQueryModeNewAPI,
			Auth: &dto.AdvancedCustomRouteAuth{
				Type:  dto.AdvancedCustomAuthTypeHeader,
				Name:  "Authorization",
				Value: "account-access-token",
			},
		},
	})

	token, err := channelBalanceQueryToken(channel)
	require.NoError(t, err)
	assert.Equal(t, "account-access-token", token)
}

func TestChannelBalanceQueryTokenRejectsMissingOrPlaceholderSecret(t *testing.T) {
	for name, value := range map[string]string{
		"missing":     "",
		"api-key-ref": "Bearer {api_key}",
	} {
		t.Run(name, func(t *testing.T) {
			channel := &model.Channel{}
			channel.SetOtherSettings(dto.ChannelOtherSettings{
				BalanceQuery: &dto.ChannelBalanceQueryConfig{
					Mode: dto.ChannelBalanceQueryModeCustom,
					Auth: &dto.AdvancedCustomRouteAuth{
						Type:  dto.AdvancedCustomAuthTypeHeader,
						Name:  "Authorization",
						Value: value,
					},
				},
			})

			_, err := channelBalanceQueryToken(channel)
			require.ErrorContains(t, err, "not configured")
		})
	}
}

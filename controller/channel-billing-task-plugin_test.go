package controller

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestTaskPluginCustomBalanceQueryEnabled(t *testing.T) {
	baseURL := "https://example.com"

	require.False(t, taskPluginCustomBalanceQueryEnabled(nil))
	require.False(t, taskPluginCustomBalanceQueryEnabled(&model.Channel{
		Type:    constant.ChannelTypeTaskPlugin,
		BaseURL: &baseURL,
	}))

	disabled := dto.ChannelOtherSettings{
		BalanceQuery: &dto.ChannelBalanceQueryConfig{Mode: dto.ChannelBalanceQueryModeDisabled},
	}
	disabledJSON, err := json.Marshal(disabled)
	require.NoError(t, err)
	require.False(t, taskPluginCustomBalanceQueryEnabled(&model.Channel{
		Type:          constant.ChannelTypeTaskPlugin,
		BaseURL:       &baseURL,
		OtherSettings: string(disabledJSON),
	}))

	custom := dto.ChannelOtherSettings{
		BalanceQuery: &dto.ChannelBalanceQueryConfig{Mode: dto.ChannelBalanceQueryModeCustom},
	}
	customJSON, err := json.Marshal(custom)
	require.NoError(t, err)
	require.True(t, taskPluginCustomBalanceQueryEnabled(&model.Channel{
		Type:          constant.ChannelTypeTaskPlugin,
		BaseURL:       &baseURL,
		OtherSettings: string(customJSON),
	}))
}

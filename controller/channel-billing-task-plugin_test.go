package controller

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestTaskPluginBalanceQueryEnabled(t *testing.T) {
	baseURL := "https://example.com"

	require.False(t, taskPluginBalanceQueryEnabled(nil))
	require.False(t, taskPluginBalanceQueryEnabled(&model.Channel{
		Type:    constant.ChannelTypeTaskPlugin,
		BaseURL: &baseURL,
	}))

	testCases := []struct {
		name    string
		mode    string
		enabled bool
	}{
		{name: "disabled", mode: dto.ChannelBalanceQueryModeDisabled, enabled: false},
		{name: "auto", mode: dto.ChannelBalanceQueryModeAuto, enabled: false},
		{name: "new api account", mode: dto.ChannelBalanceQueryModeNewAPI, enabled: true},
		{name: "one api account", mode: dto.ChannelBalanceQueryModeOneAPI, enabled: true},
		{name: "sub2api", mode: dto.ChannelBalanceQueryModeSub2API, enabled: true},
		{name: "vertex trial credit", mode: dto.ChannelBalanceQueryModeGCPTrial, enabled: true},
		{name: "custom", mode: dto.ChannelBalanceQueryModeCustom, enabled: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			settings := dto.ChannelOtherSettings{
				BalanceQuery: &dto.ChannelBalanceQueryConfig{Mode: testCase.mode},
			}
			settingsJSON, err := json.Marshal(settings)
			require.NoError(t, err)
			require.Equal(t, testCase.enabled, taskPluginBalanceQueryEnabled(&model.Channel{
				Type:          constant.ChannelTypeTaskPlugin,
				BaseURL:       &baseURL,
				OtherSettings: string(settingsJSON),
			}))
		})
	}
}

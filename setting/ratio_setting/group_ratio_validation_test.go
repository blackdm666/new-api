package ratio_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupRatioValidationRejectsNullWithoutReplacingCurrentValues(t *testing.T) {
	savedGroupRatios := GroupRatio2JSONString()
	savedGroupGroupRatios := GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupRatioByJSONString(savedGroupRatios))
		require.NoError(t, UpdateGroupGroupRatioByJSONString(savedGroupGroupRatios))
	})

	require.NoError(t, UpdateGroupRatioByJSONString(`{"paid":1}`))
	assert.Error(t, UpdateGroupRatioByJSONString(`{"paid":null}`))
	assert.Equal(t, 1.0, GetGroupRatio("paid"))

	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{"vip":{"paid":0.8}}`))
	assert.Error(t, UpdateGroupGroupRatioByJSONString(`{"vip":{"paid":null}}`))
	ratio, ok := GetGroupGroupRatio("vip", "paid")
	assert.True(t, ok)
	assert.Equal(t, 0.8, ratio)
}

func TestGroupPinBottomKeepsConfiguredOrderAndOnlyUsableGroups(t *testing.T) {
	setting := GetGroupRatioSetting()
	saved := setting.GroupPinBottom
	t.Cleanup(func() { setting.GroupPinBottom = saved })

	exported, err := config.ConfigToMap(setting)
	require.NoError(t, err)
	assert.Equal(t, "[]", exported["group_pin_bottom"])

	require.NoError(t, config.UpdateConfigFromMap(setting, map[string]string{
		"group_pin_bottom": `["AZ-GPT可蒸馏","hidden","AZ-GPT-T6号池","AZ-GPT可蒸馏"]`,
	}))
	usable := map[string]string{"AZ-GPT-T6号池": "", "AZ-GPT可蒸馏": "", "GPT-Plus": ""}
	assert.Equal(t, []string{"AZ-GPT可蒸馏", "AZ-GPT-T6号池"}, GetGroupPinBottomIn(usable))
	assert.Equal(t, []string{}, GetGroupPinBottomIn(map[string]any{"GPT-Plus": nil}))
}

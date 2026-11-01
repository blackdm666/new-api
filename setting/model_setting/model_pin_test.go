package model_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useModelPins(t *testing.T, top string, bottom string) {
	t.Helper()
	saved := modelPinSetting
	t.Cleanup(func() { modelPinSetting = saved })
	require.NoError(t, config.UpdateConfigFromMap(&modelPinSetting, map[string]string{
		"pin_top":    top,
		"pin_bottom": bottom,
	}))
}

func TestPinnedModelListsMovesModelBetweenTopAndBottom(t *testing.T) {
	useModelPins(t, `["a","b"]`, `["c","d"]`)

	top, bottom := PinnedModelLists("c", ModelPinTop)
	assert.Equal(t, []string{"c", "a", "b"}, top)
	assert.Equal(t, []string{"d"}, bottom)

	top, bottom = PinnedModelLists("a", ModelPinBottom)
	assert.Equal(t, []string{"b"}, top)
	assert.Equal(t, []string{"c", "d", "a"}, bottom)

	top, bottom = PinnedModelLists("b", "")
	assert.Equal(t, []string{"a"}, top)
	assert.Equal(t, []string{"c", "d"}, bottom)
}

func TestModelPinsOnlyExposeVisibleModelsInConfiguredOrder(t *testing.T) {
	useModelPins(t, `["hidden","b","a"]`, `["z","hidden-2","y"]`)

	top, bottom := GetModelPinsIn(map[string]struct{}{"a": {}, "b": {}, "x": {}, "y": {}, "z": {}})
	assert.Equal(t, []string{"b", "a"}, top)
	assert.Equal(t, []string{"z", "y"}, bottom)
	assert.Equal(t, ModelPinTop, GetModelPin("a"))
	assert.Equal(t, ModelPinBottom, GetModelPin("y"))
	assert.Empty(t, GetModelPin("x"))
}

func TestOrderModelsPinnedKeepsUnpinnedOrder(t *testing.T) {
	useModelPins(t, `["c"]`, `["a"]`)

	assert.Equal(t, []string{"c", "b", "d", "a"}, OrderModelsPinned([]string{"a", "b", "c", "d"}))
	assert.Equal(t, []string{"b", "d"}, OrderModelsPinned([]string{"b", "d"}))
}

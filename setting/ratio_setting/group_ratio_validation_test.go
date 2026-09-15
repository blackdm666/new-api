package ratio_setting

import (
	"testing"

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

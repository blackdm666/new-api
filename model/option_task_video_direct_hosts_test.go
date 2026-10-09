package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskVideoDirectHostsOptionHotReloadsWithoutRestart(t *testing.T) {
	db := useFrontendOptionMigrationDB(t)
	previousMap := common.OptionMap
	t.Cleanup(func() { common.OptionMap = previousMap })
	common.OptionMap = map[string]string{}
	t.Setenv(system_setting.TaskVideoDirectHostsEnv, "*.env.example")
	key := system_setting.TaskVideoDirectHostsOptionKey

	// Upgraded instances without a saved option keep the environment list.
	assert.True(t, system_setting.TaskVideoDirectHostAllowed("cdn.env.example"))

	// An administrator save applies immediately on the writing node.
	require.NoError(t, UpdateOption(key, "*.saved.example"))
	assert.Equal(t, "*.saved.example", requireOptionValue(t, db, key))
	assert.True(t, system_setting.TaskVideoDirectHostAllowed("cdn.saved.example"))
	assert.False(t, system_setting.TaskVideoDirectHostAllowed("cdn.env.example"))

	// Other nodes pick up the persisted row through the periodic option sync.
	common.OptionMap = map[string]string{}
	require.NoError(t, db.Save(&Option{Key: key, Value: "cdn.other.example"}).Error)
	assert.True(t, system_setting.TaskVideoDirectHostAllowed("cdn.env.example"), "before sync the node still uses its seed")
	loadOptionsFromDatabase()
	assert.True(t, system_setting.TaskVideoDirectHostAllowed("cdn.other.example"))
	assert.False(t, system_setting.TaskVideoDirectHostAllowed("cdn.env.example"))

	require.NoError(t, UpdateOption(key, ""))
	assert.False(t, system_setting.TaskVideoDirectHostAllowed("cdn.env.example"), "a saved empty list archives everything")

	require.Error(t, UpdateOption(key, "*"))
	require.Error(t, UpdateOption(key, "https://cdn.example.com"))
	assert.Equal(t, "", requireOptionValue(t, db, key))
	assert.Equal(t, "", system_setting.TaskVideoDirectHosts())
}

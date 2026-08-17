package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdatingBackupSMTPConfigurationDeactivatesFailover(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	keys := []string{"SMTPBackupEnabled", "SMTPBackupServer"}
	require.NoError(t, DB.Where("key IN ?", keys).Delete(&Option{}).Error)
	t.Cleanup(func() { _ = DB.Where("key IN ?", keys).Delete(&Option{}).Error })

	common.OptionMapRWMutex.Lock()
	originalMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	originalEnabled := common.SMTPBackupEnabled
	originalServer := common.SMTPBackupServer
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalMap
		common.OptionMapRWMutex.Unlock()
		common.SMTPBackupEnabled = originalEnabled
		common.SMTPBackupServer = originalServer
	})

	require.NoError(t, UpdateOption("SMTPBackupEnabled", "true"))
	require.True(t, common.SMTPBackupEnabled)

	require.NoError(t, UpdateOption("SMTPBackupServer", "smtp.backup.example"))
	assert.False(t, common.SMTPBackupEnabled)
	assert.Equal(t, "false", common.OptionMap["SMTPBackupEnabled"])

	var stored Option
	require.NoError(t, DB.First(&stored, "key = ?", "SMTPBackupEnabled").Error)
	assert.Equal(t, "false", stored.Value)
}

func TestBulkBackupSMTPConfigurationCannotBypassVerification(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	keys := []string{"SMTPBackupEnabled", "SMTPBackupServer"}
	require.NoError(t, DB.Where("key IN ?", keys).Delete(&Option{}).Error)
	t.Cleanup(func() { _ = DB.Where("key IN ?", keys).Delete(&Option{}).Error })

	common.OptionMapRWMutex.Lock()
	originalMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	originalEnabled := common.SMTPBackupEnabled
	originalServer := common.SMTPBackupServer
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalMap
		common.OptionMapRWMutex.Unlock()
		common.SMTPBackupEnabled = originalEnabled
		common.SMTPBackupServer = originalServer
	})

	require.NoError(t, UpdateOptionsBulk(map[string]string{
		"SMTPBackupEnabled": "true",
		"SMTPBackupServer":  "smtp.backup.example",
	}))
	assert.False(t, common.SMTPBackupEnabled)

	var stored Option
	require.NoError(t, DB.First(&stored, "key = ?", "SMTPBackupEnabled").Error)
	assert.Equal(t, "false", stored.Value)
}

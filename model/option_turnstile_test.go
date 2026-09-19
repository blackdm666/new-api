package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var turnstileOptionKeys = []string{
	"TurnstileCheckEnabled",
	"TurnstileProvider",
	"TurnstileSiteKey",
	"TurnstileSecretKey",
	"TurnstileWidgetScriptURL",
	"TurnstileWidgetEndpoint",
	"TurnstileVerifyURL",
	"TurnstileAction",
}

func TestApplyLegacyTurnstileCompatibilityDerivesExplicitCustomConfig(t *testing.T) {
	t.Setenv("TURNSTILE_VERIFY_URL", "")
	previous := common.CurrentTurnstileConfig()
	common.OptionMapRWMutex.RLock()
	previousOptionMap := common.OptionMap
	common.OptionMapRWMutex.RUnlock()
	common.TurnstileSiteKey = "https://captcha.example/"
	common.TurnstileProvider = common.TurnstileProviderCloudflare
	common.TurnstileWidgetScriptURL = ""
	common.TurnstileWidgetEndpoint = ""
	common.TurnstileVerifyURL = ""
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.TurnstileCheckEnabled = previous.Enabled
		common.TurnstileProvider = previous.Provider
		common.TurnstileSiteKey = previous.SiteKey
		common.TurnstileSecretKey = previous.SecretKey
		common.TurnstileWidgetScriptURL = previous.WidgetScriptURL
		common.TurnstileWidgetEndpoint = previous.WidgetEndpoint
		common.TurnstileVerifyURL = previous.VerifyURL
		common.TurnstileAction = previous.Action
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	migrationValues := applyLegacyTurnstileCompatibility(map[string]struct{}{})

	assert.Equal(t, common.TurnstileProviderCustom, common.TurnstileProvider)
	assert.Equal(t, "https://captcha.example", common.CurrentTurnstileConfig().SiteKey)
	assert.Equal(t, "https://captcha.example/widget.js", common.TurnstileWidgetScriptURL)
	assert.Equal(t, "https://captcha.example", common.TurnstileWidgetEndpoint)
	assert.Equal(t, "https://captcha.example/turnstile/v0/siteverify", common.TurnstileVerifyURL)
	assert.Equal(t, "https://captcha.example", migrationValues["TurnstileSiteKey"])
}

func TestApplyLegacyTurnstileCompatibilityPreservesLegacyVerifyURL(t *testing.T) {
	t.Setenv("TURNSTILE_VERIFY_URL", "https://verify.example/custom/siteverify")
	previous := common.CurrentTurnstileConfig()
	common.OptionMapRWMutex.RLock()
	previousOptionMap := common.OptionMap
	common.OptionMapRWMutex.RUnlock()
	common.ApplyTurnstileConfig(common.TurnstileConfig{
		Provider: common.TurnstileProviderCloudflare,
		SiteKey:  "https://captcha.example",
		Action:   "register",
	})
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.ApplyTurnstileConfig(previous)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	applyLegacyTurnstileCompatibility(map[string]struct{}{})

	assert.Equal(t, "https://verify.example/custom/siteverify", common.CurrentTurnstileConfig().VerifyURL)
}

func TestLoadOptionsPersistsLegacyTurnstileMigration(t *testing.T) {
	t.Setenv("TURNSTILE_VERIFY_URL", "https://verify.example/custom/siteverify")
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key IN ?", turnstileOptionKeys).Delete(&Option{}).Error)
	require.NoError(t, DB.Create([]Option{
		{Key: "TurnstileCheckEnabled", Value: "true"},
		{Key: "TurnstileSiteKey", Value: "https://captcha.example"},
	}).Error)

	previous := common.CurrentTurnstileConfig()
	common.OptionMapRWMutex.RLock()
	previousOptionMap := common.OptionMap
	common.OptionMapRWMutex.RUnlock()
	common.ApplyTurnstileConfig(common.TurnstileConfig{
		Provider: common.TurnstileProviderCloudflare,
		Action:   "register",
	})
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.ApplyTurnstileConfig(previous)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
		_ = DB.Where("key IN ?", turnstileOptionKeys).Delete(&Option{}).Error
	})

	loadOptionsFromDatabase()

	config := common.CurrentTurnstileConfig()
	assert.True(t, config.Enabled)
	assert.Equal(t, common.TurnstileProviderCustom, config.Provider)
	assert.Equal(t, "https://captcha.example", config.SiteKey)
	assert.Empty(t, config.SecretKey)
	assert.Equal(t, "https://verify.example/custom/siteverify", config.VerifyURL)
	var provider Option
	require.NoError(t, DB.First(&provider, "key = ?", "TurnstileProvider").Error)
	assert.Equal(t, common.TurnstileProviderCustom, provider.Value)
	var siteKey Option
	require.NoError(t, DB.First(&siteKey, "key = ?", "TurnstileSiteKey").Error)
	assert.Equal(t, "https://captcha.example", siteKey.Value)
}

func TestApplyLegacyTurnstileCompatibilityDoesNotOverrideExplicitProvider(t *testing.T) {
	previous := common.CurrentTurnstileConfig()
	common.TurnstileSiteKey = "https://captcha.example"
	common.TurnstileProvider = common.TurnstileProviderCloudflare
	t.Cleanup(func() {
		common.TurnstileCheckEnabled = previous.Enabled
		common.TurnstileProvider = previous.Provider
		common.TurnstileSiteKey = previous.SiteKey
		common.TurnstileSecretKey = previous.SecretKey
		common.TurnstileWidgetScriptURL = previous.WidgetScriptURL
		common.TurnstileWidgetEndpoint = previous.WidgetEndpoint
		common.TurnstileVerifyURL = previous.VerifyURL
		common.TurnstileAction = previous.Action
	})

	applyLegacyTurnstileCompatibility(map[string]struct{}{"TurnstileProvider": {}})

	assert.Equal(t, common.TurnstileProviderCloudflare, common.TurnstileProvider)
}

func TestValidateTurnstileOptionValuesRejectsIncompleteEnabledUpdate(t *testing.T) {
	previous := common.CurrentTurnstileConfig()
	common.TurnstileCheckEnabled = true
	common.TurnstileProvider = common.TurnstileProviderCloudflare
	common.TurnstileSiteKey = "cloudflare-site"
	common.TurnstileSecretKey = "secret"
	t.Cleanup(func() {
		common.TurnstileCheckEnabled = previous.Enabled
		common.TurnstileProvider = previous.Provider
		common.TurnstileSiteKey = previous.SiteKey
		common.TurnstileSecretKey = previous.SecretKey
		common.TurnstileWidgetScriptURL = previous.WidgetScriptURL
		common.TurnstileWidgetEndpoint = previous.WidgetEndpoint
		common.TurnstileVerifyURL = previous.VerifyURL
		common.TurnstileAction = previous.Action
	})

	err := validateTurnstileOptionValues(map[string]string{
		"TurnstileProvider": common.TurnstileProviderCustom,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "widget script URL")
}

func TestValidateTurnstileOptionValuesAcceptsAtomicProviderSwitch(t *testing.T) {
	previous := common.CurrentTurnstileConfig()
	common.TurnstileCheckEnabled = true
	common.TurnstileProvider = common.TurnstileProviderCloudflare
	common.TurnstileSiteKey = "cloudflare-site"
	common.TurnstileSecretKey = "secret"
	t.Cleanup(func() {
		common.TurnstileCheckEnabled = previous.Enabled
		common.TurnstileProvider = previous.Provider
		common.TurnstileSiteKey = previous.SiteKey
		common.TurnstileSecretKey = previous.SecretKey
		common.TurnstileWidgetScriptURL = previous.WidgetScriptURL
		common.TurnstileWidgetEndpoint = previous.WidgetEndpoint
		common.TurnstileVerifyURL = previous.VerifyURL
		common.TurnstileAction = previous.Action
	})

	err := validateTurnstileOptionValues(map[string]string{
		"TurnstileProvider":        common.TurnstileProviderCustom,
		"TurnstileWidgetScriptURL": "https://captcha.example/widget.js",
		"TurnstileWidgetEndpoint":  "https://captcha.example",
		"TurnstileVerifyURL":       "https://captcha.example/siteverify",
	})

	require.NoError(t, err)
}

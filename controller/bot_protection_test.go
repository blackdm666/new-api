package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveBotProtectionUpdatePreservesConfiguredSecret(t *testing.T) {
	previousSecret := common.TurnstileSecretKey
	common.TurnstileSecretKey = "saved-secret"
	t.Cleanup(func() { common.TurnstileSecretKey = previousSecret })

	config, values, err := resolveBotProtectionUpdate(botProtectionUpdateRequest{
		Enabled:  true,
		Provider: common.TurnstileProviderCloudflare,
		SiteKey:  "site-key",
	})

	require.NoError(t, err)
	assert.Equal(t, "saved-secret", config.SecretKey)
	_, overwritesSecret := values["TurnstileSecretKey"]
	assert.False(t, overwritesSecret)
}

func TestResolveBotProtectionUpdateBuildsAtomicCustomConfig(t *testing.T) {
	config, values, err := resolveBotProtectionUpdate(botProtectionUpdateRequest{
		Enabled:         true,
		Provider:        common.TurnstileProviderCustom,
		SecretKey:       "new-secret",
		WidgetScriptURL: "https://captcha.example/widget.js",
		WidgetEndpoint:  "https://captcha.example/",
		VerifyURL:       "https://captcha.example/siteverify",
		Action:          "login",
	})

	require.NoError(t, err)
	assert.Equal(t, "https://captcha.example", config.WidgetEndpoint)
	assert.Equal(t, "custom", values["TurnstileProvider"])
	assert.Equal(t, "true", values["TurnstileCheckEnabled"])
	assert.Equal(t, "new-secret", values["TurnstileSecretKey"])
	assert.Len(t, values, 8)
}

func TestResolveBotProtectionUpdateRejectsUnsafeVerifyURL(t *testing.T) {
	_, _, err := resolveBotProtectionUpdate(botProtectionUpdateRequest{
		Enabled:         true,
		Provider:        common.TurnstileProviderCustom,
		SecretKey:       "secret",
		WidgetScriptURL: "https://captcha.example/widget.js",
		WidgetEndpoint:  "https://captcha.example",
		VerifyURL:       "file:///etc/passwd",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification URL")
}

func TestResolveBotProtectionUpdateClearsOptionalSecret(t *testing.T) {
	config, values, err := resolveBotProtectionUpdate(botProtectionUpdateRequest{
		Provider:    common.TurnstileProviderCloudflare,
		ClearSecret: true,
	})

	require.NoError(t, err)
	assert.Empty(t, config.SecretKey)
	assert.Equal(t, "", values["TurnstileSecretKey"])

	_, _, err = resolveBotProtectionUpdate(botProtectionUpdateRequest{
		Enabled: true, Provider: common.TurnstileProviderCloudflare,
		SiteKey: "site", ClearSecret: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secret key is required")

	config, values, err = resolveBotProtectionUpdate(botProtectionUpdateRequest{
		Enabled: true, Provider: common.TurnstileProviderCustom,
		WidgetScriptURL: "https://captcha.example/widget.js",
		WidgetEndpoint:  "https://captcha.example",
		VerifyURL:       "https://captcha.example/siteverify",
		ClearSecret:     true,
	})
	require.NoError(t, err)
	assert.Empty(t, config.SecretKey)
	assert.Equal(t, "", values["TurnstileSecretKey"])
}

func TestResolveBotProtectionUpdateRejectsConflictingSecretActions(t *testing.T) {
	_, _, err := resolveBotProtectionUpdate(botProtectionUpdateRequest{
		Provider:  common.TurnstileProviderCloudflare,
		SecretKey: "replacement", ClearSecret: true,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "entered and cleared")
}

func TestTurnstileSiteKeyIsPublicButSecretRemainsSensitive(t *testing.T) {
	assert.False(t, isSensitiveOptionKey("TurnstileSiteKey"))
	assert.True(t, isSensitiveOptionKey("TurnstileSecretKey"))
}

package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func affiliateAlipaySettingOptionKeys() []string {
	return []string{
		AffiliateAlipayPayoutEnabledOptionKey,
		AffiliateAlipayAppIdOptionKey,
		AffiliateAlipayPrivateKeyOptionKey,
		AffiliateAlipayAppCertificateOptionKey,
		AffiliateAlipayPublicCertificateOptionKey,
		AffiliateAlipayRootCertificateOptionKey,
		AffiliateAlipayTransferTitleOptionKey,
	}
}

func TestAffiliateAlipayPayoutSettingsEncryptCertificateCredentialsAndNeverReturnThem(t *testing.T) {
	setupAffiliatePayoutTest(t, 100)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	keys := affiliateAlipaySettingOptionKeys()
	for _, key := range keys {
		setAffiliateOptionForTest(t, key, "")
	}
	t.Cleanup(func() { _ = DB.Where("key IN ?", keys).Delete(&Option{}).Error })
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "affiliate-certificate-setting-test-secret"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })

	require.NoError(t, UpdateAffiliateAlipayPayoutSettings(UpdateAffiliateAlipayPayoutSettingsParams{
		Enabled:                 true,
		AppId:                   "2026000000000000",
		PrivateKey:              "private-key-material",
		AppCertificate:          "application-certificate-material",
		AlipayPublicCertificate: "alipay-public-certificate-material",
		AlipayRootCertificate:   "alipay-root-certificate-material",
		TransferTitle:           "推广佣金",
	}))

	settings, err := GetAffiliateAlipayPayoutSettings()
	require.NoError(t, err)
	assert.True(t, settings.Enabled)
	assert.True(t, settings.Configured)
	assert.True(t, settings.PrivateKeyConfigured)
	assert.True(t, settings.AppCertificateConfigured)
	assert.True(t, settings.AlipayPublicCertificateConfigured)
	assert.True(t, settings.AlipayRootCertificateConfigured)
	assert.Equal(t, "推广佣金", settings.TransferTitle)

	stored := []Option{}
	require.NoError(t, DB.Where("key IN ?", []string{
		AffiliateAlipayPrivateKeyOptionKey,
		AffiliateAlipayAppCertificateOptionKey,
		AffiliateAlipayPublicCertificateOptionKey,
		AffiliateAlipayRootCertificateOptionKey,
	}).Find(&stored).Error)
	require.Len(t, stored, 4)
	for _, option := range stored {
		assert.NotContains(t, option.Value, "key-material")
		assert.NotContains(t, option.Value, "certificate-material")
		assert.Contains(t, option.Value, "v1:")
	}

	config, err := GetAffiliateAlipayPayoutConfig()
	require.NoError(t, err)
	assert.Equal(t, "private-key-material", config.PrivateKey)
	assert.Equal(t, "application-certificate-material", config.AppCertificate)
	assert.Equal(t, "alipay-public-certificate-material", config.AlipayPublicCertificate)
	assert.Equal(t, "alipay-root-certificate-material", config.AlipayRootCertificate)
	assert.Equal(t, "推广佣金", config.TransferTitle)
}

func TestAffiliateAlipayPayoutCannotEnableIncompleteCertificateConfiguration(t *testing.T) {
	setupAffiliatePayoutTest(t, 100)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	keys := affiliateAlipaySettingOptionKeys()
	for _, key := range keys {
		setAffiliateOptionForTest(t, key, "")
	}
	t.Cleanup(func() { _ = DB.Where("key IN ?", keys).Delete(&Option{}).Error })
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "affiliate-certificate-incomplete-test-secret"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })

	err := UpdateAffiliateAlipayPayoutSettings(UpdateAffiliateAlipayPayoutSettingsParams{
		Enabled:    true,
		AppId:      "2026000000000000",
		PrivateKey: "private-key-material",
	})
	assert.ErrorIs(t, err, ErrAffiliateAlipayNotConfigured)
}

func TestAffiliateAlipayPayoutRequiresSixteenDigitAppId(t *testing.T) {
	setupAffiliatePayoutTest(t, 100)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	keys := affiliateAlipaySettingOptionKeys()
	for _, key := range keys {
		setAffiliateOptionForTest(t, key, "")
	}
	t.Cleanup(func() { _ = DB.Where("key IN ?", keys).Delete(&Option{}).Error })

	err := UpdateAffiliateAlipayPayoutSettings(UpdateAffiliateAlipayPayoutSettingsParams{
		AppId: "12345",
	})
	assert.ErrorIs(t, err, ErrAffiliateAlipayConfigInvalid)
}

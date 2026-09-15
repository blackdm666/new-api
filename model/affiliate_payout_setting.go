package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	AffiliateAlipayPayoutEnabledOptionKey     = "AffiliateAlipayPayoutEnabled"
	AffiliateAlipayAppIdOptionKey             = "AffiliateAlipayAppId"
	AffiliateAlipayPrivateKeyOptionKey        = "AffiliateAlipayPrivateKey"
	AffiliateAlipayAppCertificateOptionKey    = "AffiliateAlipayAppCertificate"
	AffiliateAlipayPublicCertificateOptionKey = "AffiliateAlipayPublicCertificate"
	AffiliateAlipayRootCertificateOptionKey   = "AffiliateAlipayRootCertificate"
	AffiliateAlipayTransferTitleOptionKey     = "AffiliateAlipayTransferTitle"
	AffiliateAlipayDefaultTransferTitle       = "88API 推广佣金结算"
	affiliateAlipayPrivateKeyPurpose          = "affiliate-alipay-private-key"
	affiliateAlipayAppCertificatePurpose      = "affiliate-alipay-app-certificate"
	affiliateAlipayPublicCertificatePurpose   = "affiliate-alipay-public-certificate"
	affiliateAlipayRootCertificatePurpose     = "affiliate-alipay-root-certificate"
)

var (
	ErrAffiliateAlipayConfigInvalid = errors.New("affiliate alipay payout configuration is invalid")
	ErrAffiliateAlipayNotConfigured = errors.New("affiliate alipay payout is not configured")
)

type AffiliateAlipayPayoutConfig struct {
	Enabled                 bool
	AppId                   string
	PrivateKey              string
	AppCertificate          string
	AlipayPublicCertificate string
	AlipayRootCertificate   string
	TransferTitle           string
}

type AffiliateAlipayPayoutSettings struct {
	Enabled                           bool   `json:"enabled"`
	AppId                             string `json:"app_id"`
	TransferTitle                     string `json:"transfer_title"`
	PrivateKeyConfigured              bool   `json:"private_key_configured"`
	AppCertificateConfigured          bool   `json:"app_certificate_configured"`
	AlipayPublicCertificateConfigured bool   `json:"alipay_public_certificate_configured"`
	AlipayRootCertificateConfigured   bool   `json:"alipay_root_certificate_configured"`
	Configured                        bool   `json:"configured"`
}

type UpdateAffiliateAlipayPayoutSettingsParams struct {
	Enabled                 bool
	AppId                   string
	PrivateKey              string
	AppCertificate          string
	AlipayPublicCertificate string
	AlipayRootCertificate   string
	TransferTitle           string
	ClearKeys               bool
}

func GetAffiliateAlipayPayoutConfig() (*AffiliateAlipayPayoutConfig, error) {
	common.OptionMapRWMutex.RLock()
	enabled := common.OptionMap[AffiliateAlipayPayoutEnabledOptionKey] == "true"
	appId := strings.TrimSpace(common.OptionMap[AffiliateAlipayAppIdOptionKey])
	privateEncrypted := common.OptionMap[AffiliateAlipayPrivateKeyOptionKey]
	appCertificateEncrypted := common.OptionMap[AffiliateAlipayAppCertificateOptionKey]
	alipayPublicCertificateEncrypted := common.OptionMap[AffiliateAlipayPublicCertificateOptionKey]
	alipayRootCertificateEncrypted := common.OptionMap[AffiliateAlipayRootCertificateOptionKey]
	title := strings.TrimSpace(common.OptionMap[AffiliateAlipayTransferTitleOptionKey])
	common.OptionMapRWMutex.RUnlock()

	if title == "" {
		title = AffiliateAlipayDefaultTransferTitle
	}
	config := &AffiliateAlipayPayoutConfig{
		Enabled:       enabled,
		AppId:         appId,
		TransferTitle: title,
	}
	var err error
	if privateEncrypted != "" {
		plain, decryptErr := common.DecryptSensitiveValue(affiliateAlipayPrivateKeyPurpose, privateEncrypted)
		if decryptErr != nil {
			return nil, decryptErr
		}
		config.PrivateKey = strings.TrimSpace(string(plain))
	}
	if appCertificateEncrypted != "" {
		plain, decryptErr := common.DecryptSensitiveValue(affiliateAlipayAppCertificatePurpose, appCertificateEncrypted)
		if decryptErr != nil {
			return nil, decryptErr
		}
		config.AppCertificate = strings.TrimSpace(string(plain))
	}
	if alipayPublicCertificateEncrypted != "" {
		plain, decryptErr := common.DecryptSensitiveValue(affiliateAlipayPublicCertificatePurpose, alipayPublicCertificateEncrypted)
		if decryptErr != nil {
			return nil, decryptErr
		}
		config.AlipayPublicCertificate = strings.TrimSpace(string(plain))
	}
	if alipayRootCertificateEncrypted != "" {
		plain, decryptErr := common.DecryptSensitiveValue(affiliateAlipayRootCertificatePurpose, alipayRootCertificateEncrypted)
		if decryptErr != nil {
			return nil, decryptErr
		}
		config.AlipayRootCertificate = strings.TrimSpace(string(plain))
	}
	if config.Enabled && !config.Configured() {
		err = ErrAffiliateAlipayNotConfigured
	}
	return config, err
}

func (config *AffiliateAlipayPayoutConfig) Configured() bool {
	return config != nil && config.AppId != "" && config.PrivateKey != "" && config.AppCertificate != "" && config.AlipayPublicCertificate != "" && config.AlipayRootCertificate != ""
}

func GetAffiliateAlipayPayoutSettings() (*AffiliateAlipayPayoutSettings, error) {
	config, err := GetAffiliateAlipayPayoutConfig()
	if err != nil && !errors.Is(err, ErrAffiliateAlipayNotConfigured) {
		return nil, err
	}
	return &AffiliateAlipayPayoutSettings{
		Enabled:                           config.Enabled,
		AppId:                             config.AppId,
		TransferTitle:                     config.TransferTitle,
		PrivateKeyConfigured:              config.PrivateKey != "",
		AppCertificateConfigured:          config.AppCertificate != "",
		AlipayPublicCertificateConfigured: config.AlipayPublicCertificate != "",
		AlipayRootCertificateConfigured:   config.AlipayRootCertificate != "",
		Configured:                        config.Configured(),
	}, nil
}

func UpdateAffiliateAlipayPayoutSettings(params UpdateAffiliateAlipayPayoutSettingsParams) error {
	params.AppId = strings.TrimSpace(params.AppId)
	params.PrivateKey = strings.TrimSpace(params.PrivateKey)
	params.AppCertificate = strings.TrimSpace(params.AppCertificate)
	params.AlipayPublicCertificate = strings.TrimSpace(params.AlipayPublicCertificate)
	params.AlipayRootCertificate = strings.TrimSpace(params.AlipayRootCertificate)
	params.TransferTitle = strings.TrimSpace(params.TransferTitle)
	if params.TransferTitle == "" {
		params.TransferTitle = AffiliateAlipayDefaultTransferTitle
	}
	if (params.AppId != "" && len(params.AppId) != 16) || len(params.TransferTitle) > 64 || len(params.PrivateKey) > 16384 || len(params.AppCertificate) > 32768 || len(params.AlipayPublicCertificate) > 32768 || len(params.AlipayRootCertificate) > 65536 {
		return ErrAffiliateAlipayConfigInvalid
	}
	for _, character := range params.AppId {
		if character < '0' || character > '9' {
			return ErrAffiliateAlipayConfigInvalid
		}
	}

	common.OptionMapRWMutex.RLock()
	privateEncrypted := common.OptionMap[AffiliateAlipayPrivateKeyOptionKey]
	appCertificateEncrypted := common.OptionMap[AffiliateAlipayAppCertificateOptionKey]
	alipayPublicCertificateEncrypted := common.OptionMap[AffiliateAlipayPublicCertificateOptionKey]
	alipayRootCertificateEncrypted := common.OptionMap[AffiliateAlipayRootCertificateOptionKey]
	common.OptionMapRWMutex.RUnlock()
	if params.ClearKeys {
		privateEncrypted = ""
		appCertificateEncrypted = ""
		alipayPublicCertificateEncrypted = ""
		alipayRootCertificateEncrypted = ""
	}
	if params.PrivateKey != "" {
		value, err := common.EncryptSensitiveValue(affiliateAlipayPrivateKeyPurpose, []byte(params.PrivateKey))
		if err != nil {
			return err
		}
		privateEncrypted = value
	}
	if params.AppCertificate != "" {
		value, err := common.EncryptSensitiveValue(affiliateAlipayAppCertificatePurpose, []byte(params.AppCertificate))
		if err != nil {
			return err
		}
		appCertificateEncrypted = value
	}
	if params.AlipayPublicCertificate != "" {
		value, err := common.EncryptSensitiveValue(affiliateAlipayPublicCertificatePurpose, []byte(params.AlipayPublicCertificate))
		if err != nil {
			return err
		}
		alipayPublicCertificateEncrypted = value
	}
	if params.AlipayRootCertificate != "" {
		value, err := common.EncryptSensitiveValue(affiliateAlipayRootCertificatePurpose, []byte(params.AlipayRootCertificate))
		if err != nil {
			return err
		}
		alipayRootCertificateEncrypted = value
	}
	configured := params.AppId != "" && privateEncrypted != "" && appCertificateEncrypted != "" && alipayPublicCertificateEncrypted != "" && alipayRootCertificateEncrypted != ""
	if params.Enabled && !configured {
		return ErrAffiliateAlipayNotConfigured
	}
	return UpdateOptionsBulk(map[string]string{
		AffiliateAlipayPayoutEnabledOptionKey:     strconv.FormatBool(params.Enabled),
		AffiliateAlipayAppIdOptionKey:             params.AppId,
		AffiliateAlipayPrivateKeyOptionKey:        privateEncrypted,
		AffiliateAlipayAppCertificateOptionKey:    appCertificateEncrypted,
		AffiliateAlipayPublicCertificateOptionKey: alipayPublicCertificateEncrypted,
		AffiliateAlipayRootCertificateOptionKey:   alipayRootCertificateEncrypted,
		AffiliateAlipayTransferTitleOptionKey:     params.TransferTitle,
	})
}

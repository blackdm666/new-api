package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoreEmailTemplatesRenderForEverySupportedLanguage(t *testing.T) {
	keys := []string{
		constant.EmailTemplateKeyAccountVerificationUser,
		constant.EmailTemplateKeyPasswordResetUser,
		constant.EmailTemplateKeyQuotaWarningUser,
		constant.EmailTemplateKeyChannelStatusAdmin,
		constant.EmailTemplateKeyInspectionAlertAdmin,
	}
	for _, lang := range appI18n.SupportedLanguages() {
		for _, key := range keys {
			t.Run(lang+"/"+key, func(t *testing.T) {
				subject, body, err := PreviewEmailTemplateForLang(key, lang, "", "")
				require.NoError(t, err)
				require.NotEmpty(t, subject)
				require.NotEmpty(t, body)
				assert.NotContains(t, subject, "{{")
				assert.NotContains(t, body, "{{")
				assert.NotContains(t, subject, "core_email.")
				assert.NotContains(t, body, "core_email.")
			})
		}
	}
}

func TestCoreEmailTemplatePreviewsAreLocalizedAndComplete(t *testing.T) {
	tests := []struct {
		name            string
		key             string
		lang            string
		subjectContains string
		bodyContains    []string
	}{
		{
			name:            "verification-zh",
			key:             constant.EmailTemplateKeyAccountVerificationUser,
			lang:            "zh-CN",
			subjectContains: "邮箱验证码",
			bodyContains:    []string{"验证你的邮箱", "428615", "分钟后失效"},
		},
		{
			name:            "password-reset-en",
			key:             constant.EmailTemplateKeyPasswordResetUser,
			lang:            "en",
			subjectContains: "Reset your",
			bodyContains:    []string{"Reset your password", "Reset password", "user%40example.com"},
		},
		{
			name:            "quota-warning-zh",
			key:             constant.EmailTemplateKeyQuotaWarningUser,
			lang:            "zh-CN",
			subjectContains: "额度预警",
			bodyContains:    []string{"低于预警阈值", "$5.00", "自动发送"},
		},
		{
			name:            "channel-status-en",
			key:             constant.EmailTemplateKeyChannelStatusAdmin,
			lang:            "en",
			subjectContains: "Channel status changed",
			bodyContains:    []string{"status change", "Example Channel", "sent automatically"},
		},
		{
			name:            "inspection-alert-zh",
			key:             constant.EmailTemplateKeyInspectionAlertAdmin,
			lang:            "zh-CN",
			subjectContains: "巡检告警",
			bodyContains:    []string{"巡检已完成", "1 个渠道", "自动发送"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			subject, body, err := PreviewEmailTemplateForLang(test.key, test.lang, "", "")
			require.NoError(t, err)
			assert.Contains(t, subject, test.subjectContains)
			assert.NotContains(t, subject, "{{")
			assert.NotContains(t, body, "{{")
			for _, expected := range test.bodyContains {
				assert.Contains(t, body, expected)
			}
		})
	}
}

func TestQuotaWarningEmailUsesConfiguredDisplayCurrency(t *testing.T) {
	settings := operation_setting.GetGeneralSetting()
	originalType := settings.QuotaDisplayType
	originalSymbol := settings.CustomCurrencySymbol
	originalRate := settings.CustomCurrencyExchangeRate
	originalUSDExchangeRate := operation_setting.USDExchangeRate
	t.Cleanup(func() {
		settings.QuotaDisplayType = originalType
		settings.CustomCurrencySymbol = originalSymbol
		settings.CustomCurrencyExchangeRate = originalRate
		operation_setting.USDExchangeRate = originalUSDExchangeRate
	})

	spec, ok := constant.FindEmailTemplateSpec(constant.EmailTemplateKeyQuotaWarningUser)
	require.True(t, ok)
	sampleQuota := int(5 * common.QuotaPerUnit)

	settings.QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	_, usdBody, err := PreviewEmailTemplateForLang(
		constant.EmailTemplateKeyQuotaWarningUser,
		"en",
		spec.DefaultSubject,
		spec.DefaultBody,
	)
	require.NoError(t, err)
	assert.Contains(t, usdBody, "$5.00")

	settings.QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	operation_setting.USDExchangeRate = 7.2
	_, cnyBody, err := PreviewEmailTemplateForLang(
		constant.EmailTemplateKeyQuotaWarningUser,
		"zh-CN",
		spec.DefaultSubject,
		spec.DefaultBody,
	)
	require.NoError(t, err)
	assert.Contains(t, cnyBody, "¥36.00")

	settings.QuotaDisplayType = operation_setting.QuotaDisplayTypeCustom
	settings.CustomCurrencySymbol = "¤"
	settings.CustomCurrencyExchangeRate = 2.5
	notification := buildQuotaWarningNotification(
		dto.UserSetting{Language: "en", NotifyType: dto.NotifyTypeEmail},
		sampleQuota,
		"https://example.com/wallet",
	)
	assert.Contains(t, notification.Content, "¤12.50")
	assert.Contains(t, notification.Content, "Top up now")
	assert.Contains(t, notification.Content, `href="https://example.com/wallet"`)
}

func TestBuildAccountVerificationEmailUsesLocalizedBrandLayout(t *testing.T) {
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://88api.ai/"
	t.Cleanup(func() { system_setting.ServerAddress = previousAddress })

	subject, body := BuildAccountVerificationEmail("zh-CN", "935107")
	assert.Contains(t, subject, "邮箱验证码")
	assert.Contains(t, body, "935107")
	assert.Contains(t, body, "验证你的邮箱")
	assert.Contains(t, body, "border-radius:14px")
	assert.Contains(t, body, `href="https://88api.ai"`)
	assert.NotContains(t, body, "{{")
}

func TestBuildPasswordResetEmailEscapesQueryValues(t *testing.T) {
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://example.com/"
	t.Cleanup(func() { system_setting.ServerAddress = previousAddress })

	subject, body := BuildPasswordResetEmail("en", "alice+ops@example.com", "token/with space")
	assert.Contains(t, subject, "Reset your")
	assert.Contains(t, body, "alice%2Bops%40example.com")
	assert.Contains(t, body, "token%2Fwith+space")
	assert.Contains(t, body, "Reset password")
	assert.NotContains(t, body, "{{")
}

func TestBuildSystemAlertEmailWrapsDynamicNotificationContent(t *testing.T) {
	data := dto.NewNotify(
		dto.NotifyTypeQuotaExceed,
		"Quota warning",
		"Remaining quota: {{value}}\nTop up now",
		[]interface{}{"$5"},
	)
	subject, body := BuildSystemAlertEmail("en", data)
	require.Equal(t, "Quota warning", subject)
	assert.Contains(t, body, "Remaining quota: $5<br/>Top up now")
	assert.Contains(t, body, "sent automatically")
	assert.False(t, strings.Contains(body, "{{value}}"))
}

func TestBuildSystemAlertEmailEscapesUntrustedProviderContent(t *testing.T) {
	data := dto.NewNotify(
		dto.NotifyTypeChannelUpdate,
		"Channel disabled",
		`provider error: <a href="https://attacker.example">re-authenticate</a><script>alert(1)</script>`,
		nil,
	)
	_, body := BuildSystemAlertEmail("en", data)
	assert.Contains(t, body, `&lt;a href=&#34;https://attacker.example&#34;&gt;`)
	assert.Contains(t, body, `&lt;script&gt;alert(1)&lt;/script&gt;`)
	assert.NotContains(t, body, `<script>alert(1)</script>`)
}

func TestNotificationEmailTemplateKeyRoutesBusinessNotifications(t *testing.T) {
	tests := map[string]string{
		dto.NotifyTypeQuotaExceed:          constant.EmailTemplateKeyQuotaWarningUser,
		dto.NotifyTypeChannelUpdate:        constant.EmailTemplateKeyChannelStatusAdmin,
		"channel_update_12_3":              constant.EmailTemplateKeyChannelStatusAdmin,
		dto.NotifyTypeChannelTest:          constant.EmailTemplateKeyInspectionAlertAdmin,
		dto.NotifyTypeInspectionAlert:      constant.EmailTemplateKeyInspectionAlertAdmin,
		"future_unclassified_notification": constant.EmailTemplateKeySystemAlertUser,
	}
	for notificationType, expected := range tests {
		t.Run(notificationType, func(t *testing.T) {
			assert.Equal(t, expected, NotificationEmailTemplateKey(notificationType))
		})
	}
}

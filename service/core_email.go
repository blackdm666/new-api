package service

import (
	"fmt"
	"html"
	"math"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

func coreEmailText(lang string, key string, data ...map[string]any) string {
	_ = appI18n.Init()
	return appI18n.Translate(lang, key, data...)
}

func systemEmailSiteURL() string {
	return strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/")
}

func formatQuotaForEmail(quota int) string {
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		return fmt.Sprintf("%d", quota)
	}

	value := float64(quota) / common.QuotaPerUnit
	symbol := operation_setting.GetCurrencySymbol()
	value *= operation_setting.GetUsdToCurrencyRate(operation_setting.USDExchangeRate)
	digits := 2
	if absolute := math.Abs(value); absolute > 0 && absolute < 0.01 {
		digits = 4
	}
	return fmt.Sprintf("%s%.*f", symbol, digits, value)
}

func BuildAccountVerificationEmail(lang string, code string) (string, string) {
	systemName := common.SystemNameOrDefault()
	minutes := common.VerificationValidMinutes
	vars := map[string]string{
		"system_name":       html.EscapeString(systemName),
		"server_address":    html.EscapeString(systemEmailSiteURL()),
		"email_subject":     coreEmailText(lang, "core_email.verification.subject", map[string]any{"SystemName": systemName}),
		"heading":           coreEmailText(lang, "core_email.verification.heading"),
		"intro":             coreEmailText(lang, "core_email.verification.intro"),
		"verification_code": html.EscapeString(code),
		"expires_minutes":   fmt.Sprintf("%d", minutes),
		"validity_note":     coreEmailText(lang, "core_email.verification.validity", map[string]any{"Minutes": minutes}),
		"security_note":     coreEmailText(lang, "core_email.verification.security"),
	}
	return RenderEmailByKeyForLang(constant.EmailTemplateKeyAccountVerificationUser, lang, vars)
}

func BuildPasswordResetEmail(lang string, email string, token string) (string, string) {
	systemName := common.SystemNameOrDefault()
	minutes := common.VerificationValidMinutes
	baseURL := systemEmailSiteURL()
	resetURL := fmt.Sprintf(
		"%s/user/reset?email=%s&token=%s",
		baseURL,
		url.QueryEscape(email),
		url.QueryEscape(token),
	)
	rows := common.RenderInfoTableHTML([]common.EmailTemplateRow{
		{
			Label: coreEmailText(lang, "core_email.password_reset.validity_label"),
			Value: html.EscapeString(coreEmailText(lang, "core_email.password_reset.validity_value", map[string]any{
				"Minutes": minutes,
			})),
		},
		{
			Label: coreEmailText(lang, "core_email.password_reset.link_label"),
			Value: fmt.Sprintf(`<a href="%s" style="color:#0066cc;text-decoration:none;">%s</a>`, html.EscapeString(resetURL), html.EscapeString(resetURL)),
		},
	})
	vars := map[string]string{
		"system_name":     html.EscapeString(systemName),
		"server_address":  html.EscapeString(baseURL),
		"email_subject":   coreEmailText(lang, "core_email.password_reset.subject", map[string]any{"SystemName": systemName}),
		"heading":         coreEmailText(lang, "core_email.password_reset.heading"),
		"intro":           coreEmailText(lang, "core_email.password_reset.intro"),
		"expires_minutes": fmt.Sprintf("%d", minutes),
		"reset_url":       html.EscapeString(resetURL),
		"security_note":   coreEmailText(lang, "core_email.password_reset.security"),
		"info_table":      rows,
		"action_url":      html.EscapeString(resetURL),
		"action_label":    coreEmailText(lang, "core_email.password_reset.action"),
	}
	return RenderEmailByKeyForLang(constant.EmailTemplateKeyPasswordResetUser, lang, vars)
}

func renderNotificationContent(data dto.Notify) string {
	content := data.Content
	for _, value := range data.Values {
		content = strings.Replace(content, dto.ContentValueParam, fmt.Sprintf("%v", value), 1)
	}
	// Quota warnings are assembled entirely by this service and may contain the
	// trusted, escaped top-up link. All other notification bodies can include
	// upstream/provider error text and must be treated as untrusted plain text.
	if data.Type != dto.NotifyTypeQuotaExceed {
		content = html.EscapeString(content)
	}
	return strings.ReplaceAll(content, "\n", "<br/>")
}

// NotificationEmailTemplateKey maps a business notification to the template
// administrators can edit for that exact operation. Dynamic channel status
// types include the channel id and target status after the shared prefix.
func NotificationEmailTemplateKey(notificationType string) string {
	switch {
	case notificationType == dto.NotifyTypeQuotaExceed:
		return constant.EmailTemplateKeyQuotaWarningUser
	case notificationType == dto.NotifyTypeChannelUpdate,
		strings.HasPrefix(notificationType, dto.NotifyTypeChannelUpdate+"_"):
		return constant.EmailTemplateKeyChannelStatusAdmin
	case notificationType == dto.NotifyTypeChannelTest,
		notificationType == dto.NotifyTypeInspectionAlert:
		return constant.EmailTemplateKeyInspectionAlertAdmin
	default:
		return constant.EmailTemplateKeySystemAlertUser
	}
}

func notificationEmailIntroKey(templateKey string) string {
	switch templateKey {
	case constant.EmailTemplateKeyQuotaWarningUser:
		return "core_email.quota_warning.intro"
	case constant.EmailTemplateKeyChannelStatusAdmin:
		return "core_email.channel_status.intro"
	case constant.EmailTemplateKeyInspectionAlertAdmin:
		return "core_email.inspection_alert.intro"
	default:
		return "core_email.system_alert.intro"
	}
}

func BuildSystemAlertEmail(lang string, data dto.Notify) (string, string) {
	systemName := common.SystemNameOrDefault()
	messageBody := renderNotificationContent(data)
	templateKey := NotificationEmailTemplateKey(data.Type)
	vars := map[string]string{
		"system_name":       html.EscapeString(systemName),
		"server_address":    html.EscapeString(systemEmailSiteURL()),
		"email_subject":     data.Title,
		"heading":           html.EscapeString(data.Title),
		"intro":             coreEmailText(lang, notificationEmailIntroKey(templateKey)),
		"message_body":      messageBody,
		"notification_type": html.EscapeString(data.Type),
		"security_note":     coreEmailText(lang, "core_email.system_alert.security"),
	}
	return RenderEmailByKeyForLang(templateKey, lang, vars)
}

package service

import (
	"fmt"
	"html"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
)

// GetEmailTemplate 读取保存的自定义主题/正文；若 OptionMap 中对应 key 为空，回落到 spec 默认值。
//
// 返回 subject / body 都是未渲染的模板字符串（仍含 {{var}} 占位）。
func GetEmailTemplate(key string) (subject string, body string, spec constant.EmailTemplateSpec, ok bool) {
	return GetEmailTemplateForLang(key, "")
}

func GetEmailTemplateForLang(key string, lang string) (subject string, body string, spec constant.EmailTemplateSpec, ok bool) {
	spec, ok = constant.FindEmailTemplateSpec(key)
	if !ok {
		return "", "", spec, false
	}

	common.OptionMapRWMutex.RLock()
	savedSubject := common.OptionMap[constant.EmailTemplateSubjectLangKey(key, lang)]
	savedBody := common.OptionMap[constant.EmailTemplateBodyLangKey(key, lang)]
	if lang != "" {
		if savedSubject == "" {
			savedSubject = common.OptionMap[constant.EmailTemplateSubjectKey(key)]
		}
		if savedBody == "" {
			savedBody = common.OptionMap[constant.EmailTemplateBodyKey(key)]
		}
	}
	common.OptionMapRWMutex.RUnlock()

	subject = savedSubject
	if subject == "" {
		subject = spec.DefaultSubject
	}
	body = savedBody
	if body == "" {
		body = spec.DefaultBody
	}
	return subject, body, spec, true
}

// RenderEmailByKey 读取模板并用 vars 渲染。返回 (subject, body)。
// 若 key 不存在，返回两个空串 —— 调用方应先判断。
func RenderEmailByKey(key string, vars map[string]string) (string, string) {
	return RenderEmailByKeyForLang(key, "", vars)
}

func RenderEmailByKeyForLang(key string, lang string, vars map[string]string) (string, string) {
	subject, body, _, ok := GetEmailTemplateForLang(key, lang)
	if !ok {
		return "", ""
	}
	return common.RenderPlaceholders(subject, vars), common.RenderPlaceholders(body, vars)
}

// PreviewEmailTemplate 用 spec.Variables 中的 Sample 作为占位变量，渲染一份预览。
//
// 传入的 subject/body 若为空则使用已保存（或默认）的模板 —— 主要用于"未保存先预览"。
func PreviewEmailTemplate(key, subject, body string) (renderedSubject, renderedBody string, err error) {
	return PreviewEmailTemplateForLang(key, "", subject, body)
}

func PreviewEmailTemplateForLang(key, lang, subject, body string) (renderedSubject, renderedBody string, err error) {
	spec, ok := constant.FindEmailTemplateSpec(key)
	if !ok {
		return "", "", fmt.Errorf("unknown email template key: %s", key)
	}

	if subject == "" || body == "" {
		savedSubject, savedBody, _, _ := GetEmailTemplateForLang(key, lang)
		if subject == "" {
			subject = savedSubject
		}
		if body == "" {
			body = savedBody
		}
	}

	vars := sampleVarsFromSpecForLang(spec, lang)
	return common.RenderPlaceholders(subject, vars), common.RenderPlaceholders(body, vars), nil
}

func sampleVarsFromSpec(spec constant.EmailTemplateSpec) map[string]string {
	return sampleVarsFromSpecForLang(spec, appI18n.LangZhCN)
}

func sampleVarsFromSpecForLang(spec constant.EmailTemplateSpec, lang string) map[string]string {
	if lang == "" {
		lang = appI18n.LangZhCN
	}
	vars := make(map[string]string, len(spec.Variables))
	for _, v := range spec.Variables {
		vars[v.Name] = v.Sample
	}
	vars["server_address"] = html.EscapeString(emailTemplatePreviewSiteURL())
	applyCoreEmailTemplatePreviewSamples(spec.Key, lang, vars)
	applyInvoiceTemplatePreviewSamples(spec.Key, lang, vars)
	applyAffiliateTemplatePreviewSamples(spec.Key, lang, vars)
	return vars
}

func emailTemplatePreviewSiteURL() string {
	if siteURL := systemEmailSiteURL(); siteURL != "" {
		return siteURL
	}
	return "https://example.com"
}

func applyCoreEmailTemplatePreviewSamples(key string, lang string, vars map[string]string) {
	baseURL := html.EscapeString(emailTemplatePreviewSiteURL())
	previewURL := baseURL + "/user/reset?email=user%40example.com&token=example"
	systemName := common.SystemNameOrDefault()
	minutes := common.VerificationValidMinutes
	vars["system_name"] = html.EscapeString(systemName)

	switch key {
	case constant.EmailTemplateKeyAccountVerificationUser:
		vars["email_subject"] = coreEmailText(lang, "core_email.verification.subject", map[string]any{"SystemName": systemName})
		vars["heading"] = coreEmailText(lang, "core_email.verification.heading")
		vars["intro"] = coreEmailText(lang, "core_email.verification.intro")
		vars["verification_code"] = "428615"
		vars["expires_minutes"] = fmt.Sprintf("%d", minutes)
		vars["validity_note"] = coreEmailText(lang, "core_email.verification.validity", map[string]any{"Minutes": minutes})
		vars["security_note"] = coreEmailText(lang, "core_email.verification.security")
	case constant.EmailTemplateKeyPasswordResetUser:
		vars["email_subject"] = coreEmailText(lang, "core_email.password_reset.subject", map[string]any{"SystemName": systemName})
		vars["heading"] = coreEmailText(lang, "core_email.password_reset.heading")
		vars["intro"] = coreEmailText(lang, "core_email.password_reset.intro")
		vars["expires_minutes"] = fmt.Sprintf("%d", minutes)
		vars["reset_url"] = previewURL
		vars["security_note"] = coreEmailText(lang, "core_email.password_reset.security")
		vars["action_url"] = previewURL
		vars["action_label"] = coreEmailText(lang, "core_email.password_reset.action")
		vars["info_table"] = common.RenderInfoTableHTML([]common.EmailTemplateRow{
			{Label: coreEmailText(lang, "core_email.password_reset.validity_label"), Value: coreEmailText(lang, "core_email.password_reset.validity_value", map[string]any{"Minutes": minutes})},
			{Label: coreEmailText(lang, "core_email.password_reset.link_label"), Value: previewURL},
		})
	case constant.EmailTemplateKeyQuotaWarningUser:
		vars["email_subject"] = coreEmailText(lang, "core_email.quota_warning.preview_subject")
		vars["heading"] = vars["email_subject"]
		vars["intro"] = coreEmailText(lang, "core_email.quota_warning.intro")
		vars["message_body"] = coreEmailText(lang, "core_email.quota_warning.preview_body", map[string]any{
			"Amount": formatQuotaForEmail(int(5 * common.QuotaPerUnit)),
		})
		vars["notification_type"] = "quota_exceed"
		vars["security_note"] = coreEmailText(lang, "core_email.system_alert.security")
	case constant.EmailTemplateKeyChannelStatusAdmin:
		vars["email_subject"] = coreEmailText(lang, "core_email.channel_status.preview_subject")
		vars["heading"] = vars["email_subject"]
		vars["intro"] = coreEmailText(lang, "core_email.channel_status.intro")
		vars["message_body"] = coreEmailText(lang, "core_email.channel_status.preview_body")
		vars["notification_type"] = "channel_update_12_3"
		vars["security_note"] = coreEmailText(lang, "core_email.system_alert.security")
	case constant.EmailTemplateKeyInspectionAlertAdmin:
		vars["email_subject"] = coreEmailText(lang, "core_email.inspection_alert.preview_subject")
		vars["heading"] = vars["email_subject"]
		vars["intro"] = coreEmailText(lang, "core_email.inspection_alert.intro")
		vars["message_body"] = coreEmailText(lang, "core_email.inspection_alert.preview_body")
		vars["notification_type"] = "inspection_alert"
		vars["security_note"] = coreEmailText(lang, "core_email.system_alert.security")
	case constant.EmailTemplateKeySystemAlertUser:
		vars["email_subject"] = "System notification"
		vars["heading"] = vars["email_subject"]
		vars["intro"] = coreEmailText(lang, "core_email.system_alert.intro")
		vars["message_body"] = "The system detected an event that requires attention."
		vars["notification_type"] = "system_alert"
		vars["security_note"] = coreEmailText(lang, "core_email.system_alert.security")
	}
}

func applyAffiliateTemplatePreviewSamples(key string, lang string, vars map[string]string) {
	if key != constant.EmailTemplateKeyAffiliateUpgradeAdmin &&
		key != constant.EmailTemplateKeyAffiliateUpgradeUser &&
		key != constant.EmailTemplateKeyAffiliateCommissionUser &&
		key != constant.EmailTemplateKeyAffiliatePayoutUser {
		return
	}
	baseURL := html.EscapeString(emailTemplatePreviewSiteURL())
	vars["system_name"] = common.SystemNameOrDefault()
	vars["server_address"] = baseURL
	if key == constant.EmailTemplateKeyAffiliateUpgradeAdmin {
		vars["email_subject"] = affiliateEmailText(lang, "affiliate.email.upgrade.subject")
		vars["heading"] = affiliateEmailText(lang, "affiliate.email.upgrade.heading")
		vars["intro"] = affiliateEmailText(lang, "affiliate.email.upgrade.intro", map[string]any{"Username": "alice", "NextGroup": affiliateTierLabel("高级推广", lang), "Rate": "10%"})
		vars["action_url"] = baseURL + "/admin-affiliates"
		vars["action_label"] = affiliateEmailText(lang, "affiliate.email.upgrade.action")
		vars["info_table"] = common.RenderInfoTableHTML([]common.EmailTemplateRow{
			{Label: affiliateEmailText(lang, "affiliate.email.label.username"), Value: "alice"},
			{Label: affiliateEmailText(lang, "affiliate.email.label.effective_invitees"), Value: "50 / 50"},
			{Label: affiliateEmailText(lang, "affiliate.email.label.effective_topup_amount"), Value: "¥2,000.00 / ¥2,000.00"},
			{Label: affiliateEmailText(lang, "affiliate.email.label.next_group"), Value: affiliateTierLabel("高级推广", lang)},
			{Label: affiliateEmailText(lang, "affiliate.email.label.rate"), Value: "10%"},
		})
		return
	}
	if key == constant.EmailTemplateKeyAffiliateUpgradeUser {
		vars["email_subject"] = affiliateEmailText(lang, "affiliate.email.upgrade_approved.subject")
		vars["heading"] = affiliateEmailText(lang, "affiliate.email.upgrade_approved.heading")
		vars["intro"] = affiliateEmailText(lang, "affiliate.email.upgrade_approved.intro", map[string]any{"Group": affiliateTierLabel("高级推广", lang), "Rate": "10%"})
		vars["action_url"] = baseURL + "/referral"
		vars["action_label"] = affiliateEmailText(lang, "affiliate.email.upgrade_approved.action")
		vars["info_table"] = common.RenderInfoTableHTML([]common.EmailTemplateRow{
			{Label: affiliateEmailText(lang, "affiliate.email.label.username"), Value: "alice"},
			{Label: affiliateEmailText(lang, "affiliate.email.label.new_group"), Value: affiliateTierLabel("高级推广", lang)},
			{Label: affiliateEmailText(lang, "affiliate.email.label.rate"), Value: "10%"},
		})
		return
	}
	if key == constant.EmailTemplateKeyAffiliatePayoutUser {
		vars["email_subject"] = affiliateEmailText(lang, "affiliate.email.payout.approved.subject")
		vars["heading"] = affiliateEmailText(lang, "affiliate.email.payout.approved.heading")
		vars["intro"] = affiliateEmailText(lang, "affiliate.email.payout.approved.intro")
		vars["action_url"] = baseURL + "/referral?tab=payouts"
		vars["action_label"] = affiliateEmailText(lang, "affiliate.email.payout.action")
		vars["info_table"] = common.RenderInfoTableHTML([]common.EmailTemplateRow{
			{Label: affiliateEmailText(lang, "affiliate.email.label.payout_id"), Value: "#1024"},
			{Label: affiliateEmailText(lang, "affiliate.email.label.amount"), Value: "¥100.00"},
			{Label: affiliateEmailText(lang, "affiliate.email.label.status"), Value: affiliateEmailText(lang, "affiliate.email.payout.status.approved")},
			{Label: affiliateEmailText(lang, "affiliate.email.label.alipay_recipient"), Value: "张三"},
			{Label: affiliateEmailText(lang, "affiliate.email.label.settlement_time"), Value: "2026-09-10"},
		})
		return
	}
	vars["email_subject"] = affiliateEmailText(lang, "affiliate.email.commission.approved.subject")
	vars["heading"] = affiliateEmailText(lang, "affiliate.email.commission.approved.heading")
	vars["intro"] = affiliateEmailText(lang, "affiliate.email.commission.approved.intro")
	vars["action_url"] = baseURL + "/referral"
	vars["action_label"] = affiliateEmailText(lang, "affiliate.email.commission.action")
	vars["info_table"] = common.RenderInfoTableHTML([]common.EmailTemplateRow{
		{Label: affiliateEmailText(lang, "affiliate.email.label.order"), Value: "ORDER-202608-001"},
		{Label: affiliateEmailText(lang, "affiliate.email.label.commission"), Value: "5.00"},
		{Label: affiliateEmailText(lang, "affiliate.email.label.status"), Value: affiliateEmailText(lang, "affiliate.email.status.approved")},
	})
}

func applyInvoiceTemplatePreviewSamples(key string, lang string, vars map[string]string) {
	baseURL := html.EscapeString(emailTemplatePreviewSiteURL())
	companyName := "Example Technology Co., Ltd."
	if lang == appI18n.LangZhCN || lang == appI18n.LangZhTW {
		companyName = "示例科技有限公司"
	}
	rows := []common.EmailTemplateRow{
		{Label: invoiceEmailText(lang, "invoice.email.label.request_id"), Value: "#1024"},
		{Label: invoiceEmailText(lang, "invoice.email.label.username"), Value: "alice"},
		{Label: invoiceEmailText(lang, "invoice.email.label.company_name"), Value: companyName},
		{Label: invoiceEmailText(lang, "invoice.email.label.tax_number"), Value: "91310000EXAMPLE"},
		{Label: invoiceEmailText(lang, "invoice.email.label.amount"), Value: "CNY 701.25"},
		{Label: invoiceEmailText(lang, "invoice.email.label.status"), Value: invoiceRequestStatusLabel(1, lang)},
		{Label: invoiceEmailText(lang, "invoice.email.label.created_at"), Value: "2026-08-14 12:00:00"},
		{Label: invoiceEmailText(lang, "invoice.email.label.orders"), Value: "INV-202608-001"},
	}

	switch key {
	case constant.EmailTemplateKeyInvoiceRequestAdmin:
		vars["email_subject"] = invoiceEmailText(lang, "invoice.email.admin.created.subject", map[string]any{"ID": 1024, "CompanyName": companyName})
		vars["heading"] = invoiceEmailText(lang, "invoice.email.admin.created.heading")
		vars["intro"] = invoiceEmailText(lang, "invoice.email.admin.created.intro", map[string]any{"Username": "alice"})
		vars["action_url"] = baseURL + "/admin-invoices/1024"
		vars["action_label"] = invoiceEmailText(lang, "invoice.email.admin.created.action")
	case constant.EmailTemplateKeyInvoiceIssuedUser:
		rows[5].Value = invoiceRequestStatusLabel(2, lang)
		vars["email_subject"] = invoiceEmailText(lang, "invoice.email.user.issued.subject", map[string]any{"ID": 1024})
		vars["heading"] = invoiceEmailText(lang, "invoice.email.user.issued.heading")
		vars["intro"] = invoiceEmailText(lang, "invoice.email.user.issued.intro")
		vars["action_url"] = baseURL + "/invoices/1024"
		vars["action_label"] = invoiceEmailText(lang, "invoice.email.user.issued.action")
	case constant.EmailTemplateKeyInvoiceExpiryAdmin:
		rows = append(rows, common.EmailTemplateRow{Label: invoiceEmailText(lang, "invoice.email.label.expires_at"), Value: "2026-08-15 12:00:00"})
		vars["email_subject"] = invoiceEmailText(lang, "invoice.email.admin.expiry.subject", map[string]any{"ID": 1024})
		vars["heading"] = invoiceEmailText(lang, "invoice.email.admin.expiry.heading")
		vars["intro"] = invoiceEmailText(lang, "invoice.email.admin.expiry.intro")
		vars["action_url"] = baseURL + "/admin-invoices/1024"
		vars["action_label"] = invoiceEmailText(lang, "invoice.email.admin.expiry.action")
	default:
		return
	}
	vars["info_table"] = common.RenderInfoTableHTML(rows)
}

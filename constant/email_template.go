package constant

const (
	EmailTemplateKeyInvoiceRequestAdmin     = "invoice_request_admin"
	EmailTemplateKeyInvoiceIssuedUser       = "invoice_issued_user"
	EmailTemplateKeyInvoiceExpiryAdmin      = "invoice_expiry_admin"
	EmailTemplateKeyAffiliateUpgradeAdmin   = "affiliate_upgrade_admin"
	EmailTemplateKeyAffiliateUpgradeUser    = "affiliate_upgrade_user"
	EmailTemplateKeyAffiliateCommissionUser = "affiliate_commission_user"
	EmailTemplateKeyAffiliatePayoutUser     = "affiliate_payout_user"
	EmailTemplateKeyAccountVerificationUser = "account_verification_user"
	EmailTemplateKeyPasswordResetUser       = "password_reset_user"
	EmailTemplateKeyQuotaWarningUser        = "quota_warning_user"
	EmailTemplateKeyChannelStatusAdmin      = "channel_status_admin"
	EmailTemplateKeyInspectionAlertAdmin    = "inspection_alert_admin"
	// EmailTemplateKeySystemAlertUser is retained for unknown notification types
	// and installations that saved the former shared operations template. It is
	// intentionally not exposed in EmailTemplateSpecs: new operations emails use
	// the three business-specific templates above.
	EmailTemplateKeySystemAlertUser = "system_alert_user"
)

const EmailTemplateOptionPrefix = "EmailTemplate."

type EmailTemplateVariable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Sample      string `json:"sample"`
}

type EmailTemplateSpec struct {
	Key            string                  `json:"key"`
	Name           string                  `json:"name"`
	Description    string                  `json:"description"`
	Variables      []EmailTemplateVariable `json:"variables"`
	DefaultSubject string                  `json:"default_subject"`
	DefaultBody    string                  `json:"default_body"`
}

var (
	varSystemName            = EmailTemplateVariable{Name: "system_name", Description: "系统名称", Sample: "New API"}
	varServerAddr            = EmailTemplateVariable{Name: "server_address", Description: "网站地址（ServerAddress）", Sample: "https://example.com"}
	varHeading               = EmailTemplateVariable{Name: "heading", Description: "邮件大标题", Sample: "通知"}
	varInfoTable             = EmailTemplateVariable{Name: "info_table", Description: "由系统拼装的信息表格 HTML", Sample: `<table style="width:100%;border-collapse:collapse;font-size:14px;"><tr><td style="padding:10px 0;color:#8e8e93;">示例字段</td><td style="padding:10px 0;color:#1d1d1f;text-align:right;">示例值</td></tr></table>`}
	varActionURL             = EmailTemplateVariable{Name: "action_url", Description: "跳转链接", Sample: "https://example.com/invoices/1024"}
	varActionLabel           = EmailTemplateVariable{Name: "action_label", Description: "跳转按钮文字", Sample: "前往查看"}
	varEmailSubject          = EmailTemplateVariable{Name: "email_subject", Description: "按收件人语言生成的邮件主题", Sample: "发票通知"}
	varNotificationVariables = []EmailTemplateVariable{
		varSystemName, varServerAddr, varEmailSubject, varHeading,
		{Name: "intro", Description: "通知说明", Sample: "系统检测到一项需要关注的事件。"},
		{Name: "message_body", Description: "通知正文 HTML", Sample: "请查看以下详情。"},
		{Name: "notification_type", Description: "通知类型", Sample: "system_alert"},
		{Name: "security_note", Description: "邮件来源提示", Sample: "这是一封由系统自动发送的通知邮件。"},
	}
)

const defaultEmailShellStart = `<div style="background-color:#f4f7ff;padding:40px 16px;font-family:-apple-system,BlinkMacSystemFont,'SF Pro Text','Helvetica Neue',Arial,'PingFang SC','Microsoft YaHei',sans-serif;color:#172033;line-height:1.55;">
  <div style="max-width:560px;margin:0 auto;overflow:hidden;background-color:#ffffff;border:1px solid #dfe7ff;border-radius:18px;padding:40px 40px 32px;box-shadow:0 16px 40px rgba(37,57,128,0.10);">
    <div style="margin:-40px -40px 32px;padding:25px 32px;background-color:#5b5ce2;background-image:linear-gradient(135deg,#0891b2 0%,#4f46e5 55%,#7c3aed 100%);">
      <a href="{{server_address}}" style="display:inline-block;color:#ffffff;text-decoration:none;font-size:20px;font-weight:750;letter-spacing:-0.01em;"><span style="display:inline-block;width:10px;height:10px;margin-right:11px;border-radius:999px;background-color:#a5f3fc;box-shadow:0 0 0 5px rgba(255,255,255,0.16);vertical-align:1px;"></span>{{system_name}}</a>
    </div>`

const defaultEmailShellEnd = `
  </div>
  <p style="max-width:560px;margin:20px auto 0;text-align:center;color:#8490ad;font-size:12px;letter-spacing:0.02em;"><a href="{{server_address}}" style="color:#5b5ce2;text-decoration:none;font-weight:600;">{{system_name}}</a></p>
</div>`

const defaultBody = defaultEmailShellStart + `
    <h1 style="margin:0 0 8px;font-size:24px;font-weight:650;letter-spacing:-0.01em;color:#172033;">{{heading}}</h1>
    <p style="margin:0 0 28px;font-size:14px;color:#66728f;">{{intro}}</p>
    {{info_table}}
    <div style="margin:32px 0 8px;text-align:center;">
      <a href="{{action_url}}" style="display:inline-block;padding:11px 24px;background-color:#4f46e5;background-image:linear-gradient(135deg,#0891b2 0%,#4f46e5 58%,#7c3aed 100%);color:#ffffff;text-decoration:none;border-radius:11px;box-shadow:0 8px 18px rgba(79,70,229,0.22);font-size:14px;font-weight:650;letter-spacing:0.01em;">{{action_label}}</a>
    </div>` + defaultEmailShellEnd

const defaultCodeBody = defaultEmailShellStart + `
    <h1 style="margin:0 0 8px;font-size:24px;font-weight:650;letter-spacing:-0.01em;color:#172033;">{{heading}}</h1>
    <p style="margin:0 0 28px;font-size:14px;color:#66728f;">{{intro}}</p>
    <div style="padding:22px 24px;background-color:#f1f5ff;background-image:linear-gradient(135deg,#ecfeff 0%,#eef2ff 55%,#f5f3ff 100%);border:1px solid #dce5ff;border-radius:14px;text-align:center;">
      <div style="font-family:'SFMono-Regular',Consolas,'Liberation Mono',monospace;font-size:32px;font-weight:750;letter-spacing:0.22em;color:#4338ca;">{{verification_code}}</div>
    </div>
    <p style="margin:20px 0 0;font-size:13px;color:#66728f;">{{validity_note}}</p>
    <p style="margin:8px 0 0;font-size:13px;color:#66728f;">{{security_note}}</p>` + defaultEmailShellEnd

const defaultActionBody = defaultEmailShellStart + `
    <h1 style="margin:0 0 8px;font-size:24px;font-weight:650;letter-spacing:-0.01em;color:#172033;">{{heading}}</h1>
    <p style="margin:0 0 28px;font-size:14px;color:#66728f;">{{intro}}</p>
    {{info_table}}
    <div style="margin:28px 0 16px;text-align:center;">
      <a href="{{action_url}}" style="display:inline-block;padding:11px 24px;background-color:#4f46e5;background-image:linear-gradient(135deg,#0891b2 0%,#4f46e5 58%,#7c3aed 100%);color:#ffffff;text-decoration:none;border-radius:11px;box-shadow:0 8px 18px rgba(79,70,229,0.22);font-size:14px;font-weight:650;letter-spacing:0.01em;">{{action_label}}</a>
    </div>
    <p style="margin:0;font-size:13px;color:#66728f;">{{security_note}}</p>` + defaultEmailShellEnd

const defaultMessageBody = defaultEmailShellStart + `
    <h1 style="margin:0 0 8px;font-size:24px;font-weight:650;letter-spacing:-0.01em;color:#172033;">{{heading}}</h1>
    <p style="margin:0 0 28px;font-size:14px;color:#66728f;">{{intro}}</p>
    <div style="padding:18px 20px;background-color:#f4f6ff;border:1px solid #e0e5ff;border-left:4px solid #5b5ce2;border-radius:12px;line-height:1.7;color:#27324d;font-size:14px;word-break:break-word;">{{message_body}}</div>
    <p style="margin:20px 0 0;font-size:13px;color:#66728f;">{{security_note}}</p>` + defaultEmailShellEnd

func EmailTemplateSpecs() []EmailTemplateSpec {
	return []EmailTemplateSpec{
		{
			Key:         EmailTemplateKeyAccountVerificationUser,
			Name:        "账号注册-邮箱验证码",
			Description: "用户注册或绑定邮箱时发送验证码。",
			Variables: []EmailTemplateVariable{
				varSystemName, varServerAddr, varEmailSubject, varHeading,
				{Name: "intro", Description: "验证码用途说明", Sample: "请使用以下验证码完成邮箱验证。"},
				{Name: "verification_code", Description: "六位邮箱验证码", Sample: "428615"},
				{Name: "expires_minutes", Description: "验证码有效分钟数", Sample: "10"},
				{Name: "validity_note", Description: "有效期提示", Sample: "验证码将在 10 分钟后失效。"},
				{Name: "security_note", Description: "安全提示", Sample: "如果不是你本人操作，请忽略此邮件。"},
			},
			DefaultSubject: "{{email_subject}}",
			DefaultBody:    defaultCodeBody,
		},
		{
			Key:         EmailTemplateKeyPasswordResetUser,
			Name:        "账号安全-密码重置",
			Description: "用户申请重置密码时发送安全链接。",
			Variables: []EmailTemplateVariable{
				varSystemName, varServerAddr, varEmailSubject, varHeading,
				{Name: "intro", Description: "正文开头说明", Sample: "我们收到了你的密码重置请求。"},
				{Name: "expires_minutes", Description: "重置链接有效分钟数", Sample: "10"},
				{Name: "reset_url", Description: "密码重置链接", Sample: "https://example.com/user/reset?email=user%40example.com&token=example"},
				{Name: "security_note", Description: "安全提示", Sample: "链接将在 10 分钟后失效；如果不是你本人操作，请忽略此邮件。"},
				varInfoTable, varActionURL, varActionLabel,
			},
			DefaultSubject: "{{email_subject}}",
			DefaultBody:    defaultActionBody,
		},
		operationsEmailTemplateSpec(
			EmailTemplateKeyQuotaWarningUser,
			"额度预警",
			"用户额度低于预警阈值时发送。",
		),
		operationsEmailTemplateSpec(
			EmailTemplateKeyChannelStatusAdmin,
			"通道状态",
			"通道自动禁用、恢复或状态变化时发送给管理员。",
		),
		operationsEmailTemplateSpec(
			EmailTemplateKeyInspectionAlertAdmin,
			"巡检告警",
			"通道测试或上游模型巡检发现结果时发送给管理员。",
		),
		{
			Key:         EmailTemplateKeyInvoiceRequestAdmin,
			Name:        "发票申请-通知管理员",
			Description: "用户提交发票申请后，发送给财务管理员。",
			Variables: []EmailTemplateVariable{
				varSystemName, varServerAddr, varEmailSubject, varHeading,
				{Name: "intro", Description: "正文开头说明", Sample: "alice 提交了一份新的发票申请。"},
				{Name: "invoice_id", Description: "发票申请编号", Sample: "1024"},
				{Name: "username", Description: "申请用户名", Sample: "alice"},
				{Name: "company_name", Description: "发票抬头", Sample: "示例科技有限公司"},
				{Name: "tax_number", Description: "税号", Sample: "91310000EXAMPLE"},
				{Name: "total_money", Description: "申请金额", Sample: "701.25"},
				{Name: "invoice_status", Description: "发票状态", Sample: "待开具"},
				{Name: "created_at", Description: "申请时间", Sample: "2026-08-14 12:00:00"},
				{Name: "order_numbers", Description: "关联充值订单号", Sample: "INV-202608-001"},
				varInfoTable, varActionURL, varActionLabel,
			},
			DefaultSubject: "{{email_subject}}",
			DefaultBody:    defaultBody,
		},
		{
			Key:         EmailTemplateKeyInvoiceIssuedUser,
			Name:        "发票已开具-通知用户",
			Description: "发票开具完成后，发送给申请用户。",
			Variables: []EmailTemplateVariable{
				varSystemName, varServerAddr, varEmailSubject, varHeading,
				{Name: "intro", Description: "正文开头说明", Sample: "你的发票申请已开具，可在申请详情中下载。"},
				{Name: "invoice_id", Description: "发票申请编号", Sample: "1024"},
				{Name: "username", Description: "申请用户名", Sample: "alice"},
				{Name: "company_name", Description: "发票抬头", Sample: "示例科技有限公司"},
				{Name: "tax_number", Description: "税号", Sample: "91310000EXAMPLE"},
				{Name: "total_money", Description: "申请金额", Sample: "701.25"},
				{Name: "invoice_status", Description: "发票状态", Sample: "已开具"},
				{Name: "previous_status", Description: "变更前状态", Sample: "待开具"},
				{Name: "created_at", Description: "申请时间", Sample: "2026-08-14 12:00:00"},
				{Name: "order_numbers", Description: "关联充值订单号", Sample: "INV-202608-001"},
				varInfoTable, varActionURL, varActionLabel,
			},
			DefaultSubject: "{{email_subject}}",
			DefaultBody:    defaultBody,
		},
		{
			Key:         EmailTemplateKeyInvoiceExpiryAdmin,
			Name:        "发票申请即将过期-通知管理员",
			Description: "待处理发票申请将在 24 小时内自动过期时，发送给财务管理员。",
			Variables: []EmailTemplateVariable{
				varSystemName, varServerAddr, varEmailSubject, varHeading,
				{Name: "intro", Description: "正文开头说明", Sample: "一份待处理发票申请将在 24 小时内自动过期。"},
				{Name: "invoice_id", Description: "发票申请编号", Sample: "1024"},
				{Name: "username", Description: "申请用户名", Sample: "alice"},
				{Name: "company_name", Description: "发票抬头", Sample: "示例科技有限公司"},
				{Name: "tax_number", Description: "税号", Sample: "91310000EXAMPLE"},
				{Name: "total_money", Description: "申请金额", Sample: "701.25"},
				{Name: "invoice_status", Description: "发票状态", Sample: "待开具"},
				{Name: "created_at", Description: "申请时间", Sample: "2026-08-14 12:00:00"},
				{Name: "expires_at", Description: "预计过期时间", Sample: "2026-08-15 12:00:00"},
				{Name: "order_numbers", Description: "关联充值订单号", Sample: "INV-202608-001"},
				varInfoTable, varActionURL, varActionLabel,
			},
			DefaultSubject: "{{email_subject}}",
			DefaultBody:    defaultBody,
		},
		{
			Key:         EmailTemplateKeyAffiliateUpgradeAdmin,
			Name:        "推广员达到升级条件-通知管理员",
			Description: "推广员达到有效充值人数或有效充值金额门槛后，发送给系统通知邮箱。",
			Variables: []EmailTemplateVariable{
				varSystemName, varServerAddr, varEmailSubject, varHeading,
				{Name: "intro", Description: "正文开头说明", Sample: "alice 已达到高级推广升级条件。"},
				{Name: "username", Description: "推广员用户名", Sample: "alice"},
				{Name: "effective_invitees", Description: "有效充值人数", Sample: "50"},
				{Name: "upgrade_threshold", Description: "升级门槛", Sample: "50"},
				{Name: "effective_topup_amount", Description: "有效充值金额", Sample: "¥2,000.00"},
				{Name: "topup_amount_threshold", Description: "有效充值金额门槛", Sample: "¥2,000.00"},
				{Name: "next_group", Description: "目标推广等级", Sample: "高级推广"},
				{Name: "next_rate", Description: "目标返佣比例", Sample: "10%"},
				varInfoTable, varActionURL, varActionLabel,
			},
			DefaultSubject: "{{email_subject}}",
			DefaultBody:    defaultBody,
		},
		{
			Key:         EmailTemplateKeyAffiliateCommissionUser,
			Name:        "返佣审核结果-通知用户",
			Description: "返佣通过或驳回后，发送给推广员注册邮箱。",
			Variables: []EmailTemplateVariable{
				varSystemName, varServerAddr, varEmailSubject, varHeading,
				{Name: "intro", Description: "正文开头说明", Sample: "你的返佣已通过审核。"},
				{Name: "order_number", Description: "充值订单号", Sample: "ORDER-202608-001"},
				{Name: "topup_amount", Description: "受邀用户充值金额", Sample: "100.00"},
				{Name: "commission_amount", Description: "返佣金额", Sample: "5.00"},
				{Name: "commission_status", Description: "审核状态", Sample: "已通过"},
				{Name: "reject_reason", Description: "驳回原因", Sample: "订单需要进一步核对"},
				varInfoTable, varActionURL, varActionLabel,
			},
			DefaultSubject: "{{email_subject}}",
			DefaultBody:    defaultBody,
		},
		{
			Key:         EmailTemplateKeyAffiliateUpgradeUser,
			Name:        "推广等级升级成功-通知用户",
			Description: "管理员通过推广等级升级后，发送给推广员注册邮箱。",
			Variables: []EmailTemplateVariable{
				varSystemName, varServerAddr, varEmailSubject, varHeading,
				{Name: "intro", Description: "正文开头说明", Sample: "你的推广等级已升级为高级推广。"},
				{Name: "username", Description: "推广员用户名", Sample: "alice"},
				{Name: "new_group", Description: "新的推广等级", Sample: "高级推广"},
				{Name: "new_rate", Description: "新的返佣比例", Sample: "10%"},
				varInfoTable, varActionURL, varActionLabel,
			},
			DefaultSubject: "{{email_subject}}",
			DefaultBody:    defaultBody,
		},
		{
			Key:         EmailTemplateKeyAffiliatePayoutUser,
			Name:        "佣金结算状态-通知用户",
			Description: "结算申请通过、驳回或完成打款后，发送给推广员注册邮箱。",
			Variables: []EmailTemplateVariable{
				varSystemName, varServerAddr, varEmailSubject, varHeading,
				{Name: "intro", Description: "正文开头说明", Sample: "你的佣金结算申请已通过审核。"},
				{Name: "payout_id", Description: "结算申请编号", Sample: "1024"},
				{Name: "amount", Description: "结算金额", Sample: "¥100.00"},
				{Name: "payout_status", Description: "结算状态", Sample: "已审核待打款"},
				{Name: "account_name", Description: "支付宝收款人", Sample: "张三"},
				{Name: "account_last4", Description: "支付宝账号后四位", Sample: "1234"},
				{Name: "settlement_time", Description: "计划打款时间", Sample: "2026-09-10"},
				{Name: "reject_reason", Description: "驳回原因", Sample: "收款信息不一致"},
				varInfoTable, varActionURL, varActionLabel,
			},
			DefaultSubject: "{{email_subject}}",
			DefaultBody:    defaultBody,
		},
	}
}

func operationsEmailTemplateSpec(key string, name string, description string) EmailTemplateSpec {
	variables := make([]EmailTemplateVariable, len(varNotificationVariables))
	copy(variables, varNotificationVariables)
	return EmailTemplateSpec{
		Key:            key,
		Name:           name,
		Description:    description,
		Variables:      variables,
		DefaultSubject: "{{email_subject}}",
		DefaultBody:    defaultMessageBody,
	}
}

func legacySystemAlertEmailTemplateSpec() EmailTemplateSpec {
	return operationsEmailTemplateSpec(
		EmailTemplateKeySystemAlertUser,
		"系统运维-其他通知",
		"未分类的系统通知；仅作为旧版本兼容模板。",
	)
}

func FindEmailTemplateSpec(key string) (EmailTemplateSpec, bool) {
	for _, spec := range EmailTemplateSpecs() {
		if spec.Key == key {
			return spec, true
		}
	}
	if key == EmailTemplateKeySystemAlertUser {
		return legacySystemAlertEmailTemplateSpec(), true
	}
	return EmailTemplateSpec{}, false
}

func EmailTemplateSubjectKey(key string) string {
	return EmailTemplateOptionPrefix + key + ".subject"
}

func EmailTemplateBodyKey(key string) string {
	return EmailTemplateOptionPrefix + key + ".body"
}

func EmailTemplateSubjectLangKey(key string, lang string) string {
	if lang == "" {
		return EmailTemplateSubjectKey(key)
	}
	return EmailTemplateOptionPrefix + key + "." + lang + ".subject"
}

func EmailTemplateBodyLangKey(key string, lang string) string {
	if lang == "" {
		return EmailTemplateBodyKey(key)
	}
	return EmailTemplateOptionPrefix + key + "." + lang + ".body"
}

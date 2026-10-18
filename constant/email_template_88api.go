package constant

// 88api cyberguard: the account-restriction notice CyberGuard sends after it
// moves a user into the ban group. Unbans intentionally send no email.
const EmailTemplateKeyCyberGuardBan = "cyber_guard_ban"

const cyberGuardBanDefaultBody = defaultEmailShellStart + `
    <h1 style="margin:0 0 8px;font-size:24px;font-weight:650;letter-spacing:-0.01em;color:#172033;">{{heading}}</h1>
    <p style="margin:0 0 12px;font-size:14px;color:#27324d;">{{greeting}}</p>
    <p style="margin:0 0 24px;font-size:14px;color:#66728f;">{{intro}}</p>
    {{info_table}}
    <p style="margin:20px 0 0;font-size:13px;color:#66728f;">{{review_note}}</p>` + defaultEmailShellEnd

func cyberGuardBanEmailTemplateSpec() EmailTemplateSpec {
	return EmailTemplateSpec{
		Key:         EmailTemplateKeyCyberGuardBan,
		Name:        "账号限制通知",
		Description: "风控中心将用户移入封禁分组（暂停 GPT 分组）后发送给用户；解封不发送邮件。",
		Variables: []EmailTemplateVariable{
			varSystemName, varServerAddr, varEmailSubject, varHeading,
			{Name: "greeting", Description: "称呼（含用户名）", Sample: "你好 alice："},
			{Name: "intro", Description: "正文说明", Sample: "我们检测到你的账号在使用 GPT 分组时多次触发上游（OpenAI）的网络安全内容策略。根据平台使用规范，你的账号已被暂停 GPT 分组（GPT-Plus、GPT-Pro 等）的使用权限，其他模型仍可正常使用。"},
			{Name: "username", Description: "用户名", Sample: "alice"},
			{Name: "case_id", Description: "案例号", Sample: "CG-12"},
			{Name: "ban_time", Description: "暂停时间（北京时间）", Sample: "2026-10-13 21:30"},
			{Name: "expires_text", Description: "恢复方式", Sample: "2026-10-16 21:30（北京时间）自动恢复"},
			{Name: "reason", Description: "封禁原因（后台记录）", Sample: "自动：第 1 档（7 天内 ≥2 个被标记会话）"},
			{Name: "review_note", Description: "复核说明", Sample: "如你认为判断有误，请通过网站客服提交说明并附上案例号，我们会进一步复核。"},
			varInfoTable,
		},
		DefaultSubject: "{{email_subject}}",
		DefaultBody:    cyberGuardBanDefaultBody,
	}
}

// 88api referral: the confirmation sent to the applicant of the official
// site's referral program form. The program needs no manual approval, so the
// email tells the applicant where to find the referral link.
const EmailTemplateKeyReferralApplicationUser = "referral_application_user"

const referralApplicationUserDefaultBody = defaultEmailShellStart + `
    <h1 style="margin:0 0 8px;font-size:24px;font-weight:650;letter-spacing:-0.01em;color:#172033;">{{heading}}</h1>
    <p style="margin:0 0 24px;font-size:14px;color:#66728f;">{{intro}}</p>
    {{info_table}}
    <p style="margin:24px 0 8px;font-size:15px;font-weight:650;color:#172033;">{{steps_title}}</p>
    {{next_steps}}
    <div style="margin:28px 0 20px;text-align:center;">
      <a href="{{action_url}}" style="display:inline-block;padding:11px 24px;background-color:#4f46e5;background-image:linear-gradient(135deg,#0891b2 0%,#4f46e5 58%,#7c3aed 100%);color:#ffffff;text-decoration:none;border-radius:11px;box-shadow:0 8px 18px rgba(79,70,229,0.22);font-size:14px;font-weight:650;letter-spacing:0.01em;">{{action_label}}</a>
    </div>
    <p style="margin:0 0 8px;font-size:13px;color:#66728f;">{{support_note}}</p>
    <p style="margin:0 0 8px;font-size:13px;color:#66728f;">{{rules_note}}</p>
    <p style="margin:0;font-size:13px;color:#66728f;">{{security_note}}</p>` + defaultEmailShellEnd

func referralApplicationUserEmailTemplateSpec() EmailTemplateSpec {
	return EmailTemplateSpec{
		Key:         EmailTemplateKeyReferralApplicationUser,
		Name:        "推广返佣申请已通过-通知申请人",
		Description: "官网推广返佣申请表提交后自动发送给申请人：收件人依次取申请表里的邮箱、填写为邮箱的 88API 账号、填写的 UID 的注册邮箱；都没有则不发。不含申请人填写的名称、说明等内容。",
		Variables: []EmailTemplateVariable{
			varSystemName, varServerAddr, varEmailSubject, varHeading,
			{Name: "intro", Description: "正文开头说明", Sample: "你的 88API 推广返佣计划申请已通过。登录控制台，在「推荐计划」复制专属推荐链接，就可以开始推广。"},
			{Name: "reference", Description: "申请编号", Sample: "RA-7K2M9QXD"},
			{Name: "start_rate", Description: "起始返佣比例", Sample: "5%"},
			{Name: "next_group", Description: "下一推广等级", Sample: "高级推广"},
			{Name: "next_rate", Description: "下一等级返佣比例", Sample: "10%"},
			{Name: "upgrade_invitees", Description: "升级所需有效充值用户数", Sample: "50"},
			{Name: "upgrade_amount", Description: "升级所需有效充值累计", Sample: "¥2000.00"},
			{Name: "payout_minimum", Description: "支付宝现金结算起点", Sample: "¥100.00"},
			{Name: "settlement_day", Description: "每月打款日", Sample: "10"},
			{Name: "steps_title", Description: "步骤标题", Sample: "接下来"},
			{Name: "next_steps", Description: "由系统拼装的步骤列表 HTML", Sample: "<ol><li>登录控制台，打开「推荐计划」。</li></ol>"},
			{Name: "support_note", Description: "客服说明", Sample: "需要推广素材，或想申请更高的返佣比例？联系在线客服即可。"},
			{Name: "rules_note", Description: "规则提醒", Sample: "请遵守推广计划规则。"},
			{Name: "security_note", Description: "非本人提交提示", Sample: "如果你没有提交过这份申请，请忽略这封邮件。"},
			varInfoTable, varActionURL, varActionLabel,
		},
		DefaultSubject: "{{email_subject}}",
		DefaultBody:    referralApplicationUserDefaultBody,
	}
}

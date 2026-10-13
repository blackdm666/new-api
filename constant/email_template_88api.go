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

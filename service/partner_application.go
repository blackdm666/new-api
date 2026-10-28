package service

import (
	"errors"
	"fmt"
	"html"
	"slices"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

// PartnerApplicationEmailCategory is the email outbox category of the
// official site's partner (reseller) application notification.
const PartnerApplicationEmailCategory = "partner_application_admin"

// Partner application validation error codes returned to the site form, in
// addition to the referral application codes.
const (
	PartnerApplicationErrorNotEligible     = "not_eligible"
	PartnerApplicationErrorAccountNotFound = "account_not_found"
)

// partnerApplicationForm is the single definition of the official site's
// partner application form. Validation and the notification email both follow
// its field table, like referralApplicationFields.
var partnerApplicationForm = applicationForm{
	Fields: []referralApplicationField{
		{Key: "account", Label: "88API UID", Kind: referralFieldUID, Required: true, MaxLength: 254},
		{Key: "display_name", Label: "称呼或公司", Kind: referralFieldText, Required: true, MaxLength: 50},
		{Key: "business_type", Label: "业务类型", Kind: referralFieldChoice, Required: true, Options: []referralApplicationOption{
			{Value: "relay_running", Label: "已在运营 API 中转站"},
			{Value: "relay_building", Label: "正在搭建 API 站点"},
			{Value: "saas", Label: "产品或 SaaS 集成"},
			{Value: "reseller", Label: "渠道商（转售给客户）"},
			{Value: "studio", Label: "公司或团队自用"},
			{Value: "personal", Label: "个人使用"},
			{Value: "other", Label: "其他"},
		}},
		{Key: "site_url", Label: "站点或产品链接", Kind: referralFieldText, MaxLength: 500, MaxLinks: 3,
			RequiredWhen: &applicationFieldCondition{Key: "business_type", Values: []string{"relay_running", "saas"}}},
		{Key: "system", Label: "站点系统", Kind: referralFieldChoice, Required: true, Options: []referralApplicationOption{
			{Value: "new_api", Label: "New API"},
			{Value: "one_api", Label: "One API"},
			{Value: "sub2api", Label: "Sub2API"},
			{Value: "custom", Label: "自研"},
			{Value: "other", Label: "其他"},
			{Value: "undecided", Label: "还没确定"},
		}},
		{Key: "monthly_spend", Label: "预计月消耗", Kind: referralFieldChoice, Required: true, Options: []referralApplicationOption{
			{Value: "lt5k", Label: "¥5,000 以下"},
			{Value: "5k-10k", Label: "¥5,000–1 万"},
			{Value: "10k-50k", Label: "¥1 万–5 万"},
			{Value: "gt50k", Label: "¥5 万以上"},
		}},
		{Key: "prepay", Label: "单次预存不少于 ¥2,000", Kind: referralFieldChoice, Required: true, Options: []referralApplicationOption{
			{Value: "paid", Label: "已预存 ¥2,000 以上"},
			{Value: "willing", Label: "还没预存，接受预存"},
			{Value: "decline", Label: "不接受"},
		}},
		{Key: "models", Label: "主要模型", Kind: referralFieldMultiChoice, Required: true, Options: []referralApplicationOption{
			{Value: "claude", Label: "Claude"},
			{Value: "gpt", Label: "GPT / Codex"},
			{Value: "gemini", Label: "Gemini"},
			{Value: "image", Label: "生图"},
			{Value: "video", Label: "视频"},
			{Value: "other", Label: "其他"},
		}},
		{Key: "needs", Label: "主要需求", Kind: referralFieldMultiChoice, Options: []referralApplicationOption{
			{Value: "price", Label: "专属价格"},
			{Value: "rpm", Label: "提高速率上限"},
			{Value: "passthrough", Label: "参数透传"},
			{Value: "fapiao", Label: "开发票"},
			{Value: "other", Label: "其他"},
		}},
		{Key: "telegram", Label: "Telegram", Kind: referralFieldTelegram, MaxLength: 64},
		{Key: "wechat", Label: "微信", Kind: referralFieldText, MaxLength: 64},
		{Key: "email", Label: "邮箱", Kind: referralFieldEmail, MaxLength: 254},
		{Key: "message", Label: "补充说明", Kind: referralFieldMultiline, MaxLength: 1000, MaxLinks: 3},
		{Key: "consent_privacy", Label: "已阅读《隐私政策》并同意就本次申请联系", Kind: referralFieldConsent, Required: true},
	},
	ContactKeys:     referralApplicationContactKeys,
	ReferencePrefix: "PA-",
	Title:           "代理合作申请",
	Intro:           "官网代理合作页收到一份新申请；评级只按申请人自报的月消耗，联系前请结合账号概况核实。",
	EmailCategory:   PartnerApplicationEmailCategory,
	DeliveryPrefix:  "partner-application",
}

// PartnerApplication is a validated partner application ready to be emailed.
type PartnerApplication struct {
	applicationSubmission
	// Grade is "A" or "B", from the self-reported monthly spend only.
	Grade string
	// User is the enabled account whose UID the applicant entered.
	User *model.User
}

// ValidatePartnerApplication checks a decoded form payload against the
// partner application form, then the eligibility threshold, and finally looks
// up the entered UID, so malformed or ineligible submissions never reach the
// database. Only a numeric UID of an enabled user is accepted; the form does
// not take emails, so it cannot be used to probe which emails are registered.
func ValidatePartnerApplication(payload map[string]any, request ReferralApplicationContext) (*PartnerApplication, error) {
	submission, err := validateApplicationForm(partnerApplicationForm, payload, request)
	if err != nil {
		return nil, err
	}
	// The thresholds (2026-10-28): monthly spend of ¥5,000 or more and a single
	// prepayment of at least ¥2,000. Personal and in-house use stay on regular
	// top-ups. The site form blocks all of these first; this only enforces it.
	if slices.Contains([]string{"personal", "studio"}, submission.Values["business_type"]) {
		return nil, &ReferralApplicationFieldError{Field: "business_type", Code: PartnerApplicationErrorNotEligible}
	}
	grade := ""
	switch submission.Values["monthly_spend"] {
	case "10k-50k", "gt50k":
		grade = "A"
	case "5k-10k":
		grade = "B"
	default:
		return nil, &ReferralApplicationFieldError{Field: "monthly_spend", Code: PartnerApplicationErrorNotEligible}
	}
	if submission.Values["prepay"] == "decline" {
		return nil, &ReferralApplicationFieldError{Field: "prepay", Code: PartnerApplicationErrorNotEligible}
	}
	notFound := &ReferralApplicationFieldError{Field: "account", Code: PartnerApplicationErrorAccountNotFound}
	userId, err := strconv.Atoi(submission.Values["account"])
	if err != nil || userId <= 0 {
		return nil, notFound
	}
	user, err := model.GetUserById(userId, false)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notFound
	}
	if err != nil {
		return nil, err
	}
	if user.Status != common.UserStatusEnabled {
		return nil, notFound
	}
	return &PartnerApplication{applicationSubmission: *submission, Grade: grade, User: user}, nil
}

// RenderPartnerApplicationEmail builds the administrator notification: the
// form answers, an overview of the applicant's account read from its users
// row, and the submission facts. The subject carries only the grade and the
// reference so no personal data remains once the outbox clears the body.
func RenderPartnerApplicationEmail(application *PartnerApplication) (string, string) {
	user := application.User
	quotaUSD := func(quota int) string {
		return html.EscapeString(fmt.Sprintf("$%.2f", float64(quota)/common.QuotaPerUnit))
	}
	account := []common.EmailTemplateRow{
		{Label: "用户名", Value: html.EscapeString(user.Username)},
		{Label: "分组", Value: html.EscapeString(user.Group)},
		{Label: "已用额度", Value: quotaUSD(user.UsedQuota)},
		{Label: "剩余额度", Value: quotaUSD(user.Quota)},
		{Label: "请求次数", Value: strconv.Itoa(user.RequestCount)},
	}
	if user.CreatedAt > 0 {
		account = append(account, common.EmailTemplateRow{
			Label: "注册时间",
			Value: html.EscapeString(time.Unix(user.CreatedAt, 0).In(applicationEmailLocation).Format("2006-01-02 15:04:05") + "（北京时间）"),
		})
	}
	return renderApplicationEmail(partnerApplicationForm, &application.applicationSubmission, application.Grade+"级",
		[]applicationEmailSection{{Title: "账号概况", Rows: account}})
}

// QueuePartnerApplicationEmail stores the notification in the email outbox
// for each configured recipient (the referral application recipients). No
// confirmation is sent to the applicant. It fails only when no recipient
// could be queued.
func QueuePartnerApplicationEmail(application *PartnerApplication) error {
	if !applicationEmailConfigured() {
		return ErrReferralApplicationEmailNotConfigured
	}
	subject, body := RenderPartnerApplicationEmail(application)
	expires := application.SubmittedAt.Add(7 * 24 * time.Hour).Unix()
	return queueApplicationNotification(partnerApplicationForm, ReferralApplicationRecipients(), application.Reference, subject, body, expires)
}

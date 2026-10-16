package service

import (
	"errors"
	"fmt"
	"html"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

// ReferralApplicationEmailCategory is the email outbox category of the
// official site's referral program application notification.
const ReferralApplicationEmailCategory = "referral_application_admin"

type referralApplicationFieldKind int

const (
	referralFieldText referralApplicationFieldKind = iota
	referralFieldMultiline
	referralFieldAccount
	referralFieldEmail
	referralFieldTelegram
	referralFieldChoice
	referralFieldMultiChoice
	referralFieldConsent
)

type referralApplicationOption struct {
	Value string
	Label string
}

type referralApplicationField struct {
	Key       string
	Label     string
	Kind      referralApplicationFieldKind
	Required  bool
	MaxLength int
	MaxLinks  int
	Options   []referralApplicationOption
}

// referralApplicationFields is the single definition of the official site's
// referral program application form (88api-home ReferralApplyForm.astro).
// Validation and the notification email both follow this table, so adding,
// removing or relabelling a form field only needs a change here and in the
// site form. Lengths count characters, matching the site's maxlength.
var referralApplicationFields = []referralApplicationField{
	{Key: "account", Label: "88API 账号（申请人填写）", Kind: referralFieldAccount, Required: true, MaxLength: 254},
	{Key: "display_name", Label: "名称或 ID", Kind: referralFieldText, Required: true, MaxLength: 50},
	{Key: "telegram", Label: "Telegram", Kind: referralFieldTelegram, MaxLength: 64},
	{Key: "wechat", Label: "微信", Kind: referralFieldText, MaxLength: 64},
	{Key: "email", Label: "邮箱", Kind: referralFieldEmail, MaxLength: 254},
	{Key: "channels", Label: "主要推广渠道", Kind: referralFieldMultiChoice, Required: true, Options: []referralApplicationOption{
		{Value: "bilibili", Label: "B站"},
		{Value: "douyin", Label: "抖音"},
		{Value: "xiaohongshu", Label: "小红书"},
		{Value: "wechat_channels", Label: "视频号"},
		{Value: "youtube", Label: "YouTube"},
		{Value: "x", Label: "X"},
		{Value: "zhihu", Label: "知乎"},
		{Value: "wechat_mp", Label: "公众号"},
		{Value: "blog", Label: "独立博客"},
		{Value: "tg_group", Label: "Telegram 群"},
		{Value: "wechat_group", Label: "微信群"},
		{Value: "qq_group", Label: "QQ 群"},
		{Value: "discord", Label: "Discord"},
		{Value: "offline_training", Label: "线下培训"},
		{Value: "tool_plugin", Label: "工具或插件"},
		{Value: "other", Label: "其他"},
	}},
	{Key: "channel_url", Label: "主页或社群链接", Kind: referralFieldText, MaxLength: 500, MaxLinks: 3},
	{Key: "audience_size", Label: "粉丝或社群规模", Kind: referralFieldChoice, Required: true, Options: []referralApplicationOption{
		{Value: "lt1k", Label: "1,000 以下"},
		{Value: "1k-10k", Label: "1,000–1 万"},
		{Value: "10k-100k", Label: "1 万–10 万"},
		{Value: "gt100k", Label: "10 万以上"},
	}},
	{Key: "message", Label: "申请说明", Kind: referralFieldMultiline, MaxLength: 1000, MaxLinks: 3},
	{Key: "consent_rules", Label: "同意《88API 推广计划规则》", Kind: referralFieldConsent, Required: true},
	{Key: "consent_privacy", Label: "已阅读《隐私政策》并同意就本次申请联系", Kind: referralFieldConsent, Required: true},
}

// referralApplicationContactKeys must contain at least one filled value.
var referralApplicationContactKeys = []string{"telegram", "wechat", "email"}

const (
	referralApplicationMaxSourceLength = 500
	referralApplicationEmailMaxLength  = 254
)

var (
	referralApplicationUIDPattern      = regexp.MustCompile(`^\d{1,12}$`)
	referralApplicationTelegramPattern = regexp.MustCompile(`^@?[A-Za-z][A-Za-z0-9_]{4,31}$`)
	referralApplicationPhonePattern    = regexp.MustCompile(`^\+\d{6,15}$`)
	referralApplicationLinkPattern     = regexp.MustCompile(`(?i)https?://`)
	referralApplicationLocalePattern   = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})?$`)
)

// Referral application validation error codes returned to the site form.
const (
	ReferralApplicationErrorRequired        = "required"
	ReferralApplicationErrorInvalid         = "invalid"
	ReferralApplicationErrorTooLong         = "too_long"
	ReferralApplicationErrorTooManyLinks    = "too_many_links"
	ReferralApplicationErrorContactRequired = "contact_required"
)

// ReferralApplicationFieldError identifies the first invalid field so the
// site can show its own localized message next to that field.
type ReferralApplicationFieldError struct {
	Field string
	Code  string
}

func (e *ReferralApplicationFieldError) Error() string {
	return fmt.Sprintf("referral application field %s is %s", e.Field, e.Code)
}

type referralApplicationEntry struct {
	Label string
	Value string
}

// ReferralApplication is a validated submission ready to be emailed.
type ReferralApplication struct {
	Reference   string
	Entries     []referralApplicationEntry
	SourcePage  string
	Locale      string
	ClientIP    string
	SubmittedAt time.Time
}

// ReferralApplicationContext carries request facts that are not form fields.
type ReferralApplicationContext struct {
	SourcePage string
	Referer    string
	Locale     string
	ClientIP   string
	Now        time.Time
}

// IsReferralApplicationSpam reports whether the hidden honeypot field was
// filled. Such submissions are accepted silently and never emailed.
func IsReferralApplicationSpam(payload map[string]any) bool {
	value, _ := payload["website"].(string)
	return strings.TrimSpace(value) != ""
}

// ValidateReferralApplication checks a decoded form payload against
// referralApplicationFields and returns the normalized submission.
func ValidateReferralApplication(payload map[string]any, request ReferralApplicationContext) (*ReferralApplication, error) {
	entries := make([]referralApplicationEntry, 0, len(referralApplicationFields))
	filled := make(map[string]bool, len(referralApplicationFields))
	for _, field := range referralApplicationFields {
		value, err := normalizeReferralApplicationField(field, payload[field.Key])
		if err != nil {
			return nil, err
		}
		filled[field.Key] = value != ""
		if value == "" {
			value = "-"
		}
		entries = append(entries, referralApplicationEntry{Label: field.Label, Value: value})
	}
	contactFilled := false
	for _, key := range referralApplicationContactKeys {
		contactFilled = contactFilled || filled[key]
	}
	if !contactFilled {
		return nil, &ReferralApplicationFieldError{Field: "contact", Code: ReferralApplicationErrorContactRequired}
	}

	sourcePage := referralApplicationSourcePage(request.SourcePage)
	if sourcePage == "" {
		sourcePage = referralApplicationSourcePage(request.Referer)
	}
	locale := strings.TrimSpace(request.Locale)
	if !referralApplicationLocalePattern.MatchString(locale) {
		locale = ""
	}
	submittedAt := request.Now
	if submittedAt.IsZero() {
		submittedAt = time.Now()
	}
	return &ReferralApplication{
		Reference:   "RA-" + strings.ToUpper(common.GetRandomString(8)),
		Entries:     entries,
		SourcePage:  sourcePage,
		Locale:      locale,
		ClientIP:    strings.TrimSpace(request.ClientIP),
		SubmittedAt: submittedAt,
	}, nil
}

func normalizeReferralApplicationField(field referralApplicationField, raw any) (string, error) {
	fieldError := func(code string) error {
		return &ReferralApplicationFieldError{Field: field.Key, Code: code}
	}
	switch field.Kind {
	case referralFieldConsent:
		accepted, _ := raw.(bool)
		if field.Required && !accepted {
			return "", fieldError(ReferralApplicationErrorRequired)
		}
		if accepted {
			return "是", nil
		}
		return "", nil
	case referralFieldMultiChoice:
		values, ok := raw.([]any)
		if raw != nil && !ok {
			return "", fieldError(ReferralApplicationErrorInvalid)
		}
		labels := make([]string, 0, len(values))
		seen := make(map[string]bool, len(values))
		for _, item := range values {
			value, _ := item.(string)
			label, valid := referralApplicationOptionLabel(field, value)
			if !valid {
				return "", fieldError(ReferralApplicationErrorInvalid)
			}
			if seen[value] {
				continue
			}
			seen[value] = true
			labels = append(labels, label)
		}
		if field.Required && len(labels) == 0 {
			return "", fieldError(ReferralApplicationErrorRequired)
		}
		return strings.Join(labels, "、"), nil
	}

	if raw != nil {
		if _, ok := raw.(string); !ok {
			return "", fieldError(ReferralApplicationErrorInvalid)
		}
	}
	value, _ := raw.(string)
	value = strings.TrimSpace(value)
	if value == "" {
		if field.Required {
			return "", fieldError(ReferralApplicationErrorRequired)
		}
		return "", nil
	}
	if field.MaxLength > 0 && utf8.RuneCountInString(value) > field.MaxLength {
		return "", fieldError(ReferralApplicationErrorTooLong)
	}
	if strings.ContainsFunc(value, func(r rune) bool {
		return unicode.IsControl(r) && !(field.Kind == referralFieldMultiline && (r == '\n' || r == '\r' || r == '\t'))
	}) {
		return "", fieldError(ReferralApplicationErrorInvalid)
	}
	if field.MaxLinks > 0 && len(referralApplicationLinkPattern.FindAllStringIndex(value, -1)) > field.MaxLinks {
		return "", fieldError(ReferralApplicationErrorTooManyLinks)
	}
	switch field.Kind {
	case referralFieldAccount:
		if !referralApplicationUIDPattern.MatchString(value) && !isReferralApplicationEmail(value) {
			return "", fieldError(ReferralApplicationErrorInvalid)
		}
	case referralFieldEmail:
		if !isReferralApplicationEmail(value) {
			return "", fieldError(ReferralApplicationErrorInvalid)
		}
	case referralFieldTelegram:
		normalized, ok := normalizeReferralApplicationTelegram(value)
		if !ok {
			return "", fieldError(ReferralApplicationErrorInvalid)
		}
		value = normalized
	case referralFieldChoice:
		label, valid := referralApplicationOptionLabel(field, value)
		if !valid {
			return "", fieldError(ReferralApplicationErrorInvalid)
		}
		value = label
	}
	return value, nil
}

func referralApplicationOptionLabel(field referralApplicationField, value string) (string, bool) {
	for _, option := range field.Options {
		if option.Value == value {
			return option.Label, true
		}
	}
	return "", false
}

func isReferralApplicationEmail(value string) bool {
	if len(value) > referralApplicationEmailMaxLength || strings.ContainsAny(value, " <>\"") {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value {
		return false
	}
	_, domain, ok := strings.Cut(value, "@")
	return ok && strings.Contains(domain, ".") && !strings.HasSuffix(domain, ".")
}

// normalizeReferralApplicationTelegram accepts @username, username,
// t.me/username links and international phone numbers, and returns the
// value shown in the notification email.
func normalizeReferralApplicationTelegram(value string) (string, bool) {
	if referralApplicationPhonePattern.MatchString(value) {
		return value, true
	}
	handle := value
	for _, prefix := range []string{"https://t.me/", "http://t.me/", "t.me/"} {
		if len(handle) > len(prefix) && strings.EqualFold(handle[:len(prefix)], prefix) {
			handle = handle[len(prefix):]
			break
		}
	}
	handle = strings.TrimSuffix(handle, "/")
	if !referralApplicationTelegramPattern.MatchString(handle) {
		return "", false
	}
	return "@" + strings.TrimPrefix(handle, "@"), true
}

func referralApplicationSourcePage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > referralApplicationMaxSourceLength {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	// Query strings and fragments can carry tracking or personal data; the
	// page path is enough to know which page the applicant used.
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// ReferralApplicationRecipients returns the configured recipient list, or
// the default recipient when the option is empty.
func ReferralApplicationRecipients() []string {
	common.OptionMapRWMutex.RLock()
	configured := common.ReferralApplicationRecipient
	common.OptionMapRWMutex.RUnlock()
	recipients := parseNotificationEmails(configured)
	if len(recipients) == 0 {
		return []string{common.DefaultReferralApplicationRecipient}
	}
	return recipients
}

// ValidateReferralApplicationRecipients validates the administrator option:
// one or more addresses separated by commas, semicolons or whitespace.
func ValidateReferralApplicationRecipients(value string) error {
	recipients := parseNotificationEmails(value)
	if len(recipients) == 0 {
		return errors.New("at least one email address is required")
	}
	for _, recipient := range recipients {
		if !isReferralApplicationEmail(recipient) {
			return fmt.Errorf("invalid email address: %s", recipient)
		}
	}
	return nil
}

// RenderReferralApplicationEmail builds the administrator notification. All
// applicant-provided values are HTML-escaped. The subject carries only the
// reference so no personal data remains once the outbox clears the body.
func RenderReferralApplicationEmail(application *ReferralApplication) (string, string) {
	beijing := time.FixedZone("UTC+8", 8*60*60)
	rows := make([]common.EmailTemplateRow, 0, len(application.Entries)+5)
	for _, entry := range application.Entries {
		rows = append(rows, common.EmailTemplateRow{Label: entry.Label, Value: common.EscapeAndBreak(entry.Value)})
	}
	sourcePage := application.SourcePage
	if sourcePage == "" {
		sourcePage = "-"
	}
	locale := application.Locale
	if locale == "" {
		locale = "-"
	}
	clientIP := application.ClientIP
	if clientIP == "" {
		clientIP = "-"
	}
	meta := []common.EmailTemplateRow{
		{Label: "申请编号", Value: html.EscapeString(application.Reference)},
		{Label: "提交时间", Value: html.EscapeString(application.SubmittedAt.In(beijing).Format("2006-01-02 15:04:05") + "（北京时间）")},
		{Label: "来源页面", Value: html.EscapeString(sourcePage)},
		{Label: "页面语言", Value: html.EscapeString(locale)},
		{Label: "IP", Value: html.EscapeString(clientIP)},
	}
	systemName := common.SystemNameOrDefault()
	subject := fmt.Sprintf("[%s] 推广返佣计划申请 %s", systemName, application.Reference)
	var body strings.Builder
	body.WriteString(`<div style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;max-width:640px;margin:0 auto;color:#1d1d1f;">`)
	fmt.Fprintf(&body, `<h2 style="font-size:18px;margin:0 0 8px;">%s</h2>`, html.EscapeString("推广返佣计划申请"))
	body.WriteString(`<p style="margin:0 0 16px;color:#6e6e73;font-size:14px;">官网推广返佣页的申请表收到一份新申请。账号与联系方式由申请人自行填写，联系前请先核实。</p>`)
	body.WriteString(common.RenderInfoTableHTML(rows))
	body.WriteString(`<p style="margin:20px 0 8px;color:#8e8e93;font-size:13px;">提交信息</p>`)
	body.WriteString(common.RenderInfoTableHTML(meta))
	body.WriteString(`</div>`)
	return subject, body.String()
}

// ErrReferralApplicationEmailNotConfigured means the notification SMTP
// profile has no server, so the application cannot be delivered.
var ErrReferralApplicationEmailNotConfigured = errors.New("referral application email is not configured")

// QueueReferralApplicationEmail stores the notification in the email outbox
// for each configured recipient. The outbox clears subject and body after
// delivery, so only delivery metadata stays in the database. It fails only
// when no recipient could be queued.
func QueueReferralApplicationEmail(application *ReferralApplication) error {
	common.OptionMapRWMutex.RLock()
	smtpServer := strings.TrimSpace(common.SMTPServer)
	common.OptionMapRWMutex.RUnlock()
	if smtpServer == "" {
		return ErrReferralApplicationEmailNotConfigured
	}
	subject, body := RenderReferralApplicationEmail(application)
	expires := application.SubmittedAt.Add(7 * 24 * time.Hour).Unix()
	var queueErr error
	queued := 0
	for _, recipient := range ReferralApplicationRecipients() {
		deliveryKey := fmt.Sprintf("referral-application:%s:%s", application.Reference, strings.ToLower(recipient))
		if _, err := QueueSystemEmail(deliveryKey, ReferralApplicationEmailCategory, 0, 0, recipient, subject, body, expires); err != nil {
			queueErr = errors.Join(queueErr, fmt.Errorf("%s: %w", recipient, err))
			continue
		}
		queued++
	}
	if queued == 0 {
		return queueErr
	}
	if queueErr != nil {
		common.SysError("referral application " + application.Reference + " was not queued for every recipient: " + queueErr.Error())
	}
	return nil
}

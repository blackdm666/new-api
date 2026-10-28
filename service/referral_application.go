package service

import (
	"errors"
	"fmt"
	"html"
	"maps"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
)

// ReferralApplicationEmailCategory is the email outbox category of the
// official site's referral program application notification.
const ReferralApplicationEmailCategory = "referral_application_admin"

// ReferralApplicationReplyEmailCategory is the email outbox category of the
// confirmation sent to the applicant.
const ReferralApplicationReplyEmailCategory = "referral_application_user"

// ReferralPromoKitPath is the fixed download address of the promotion kit on
// the official site (88api-home-live site/downloads/). Updating the kit only
// replaces the file, so the address in confirmation emails never changes.
const ReferralPromoKitPath = "/downloads/88api-promo-kit.zip"

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
	// referralFieldUID accepts only a numeric user id.
	referralFieldUID
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
	// RequiredWhen makes an optional field required when another field holds
	// one of the listed option values.
	RequiredWhen *applicationFieldCondition
}

type applicationFieldCondition struct {
	Key    string
	Values []string
}

// applicationForm describes one of the official site's public application
// forms: the field table that drives validation and the notification email,
// the contact fields of which at least one must be filled, and how the
// notification is titled and queued.
type applicationForm struct {
	Fields          []referralApplicationField
	ContactKeys     []string
	ReferencePrefix string
	Title           string
	Intro           string
	EmailCategory   string
	DeliveryPrefix  string
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

var referralApplicationForm = applicationForm{
	Fields:          referralApplicationFields,
	ContactKeys:     referralApplicationContactKeys,
	ReferencePrefix: "RA-",
	Title:           "推广返佣计划申请",
	Intro:           "官网推广返佣页的申请表收到一份新申请。账号与联系方式由申请人自行填写，联系前请先核实。",
	EmailCategory:   ReferralApplicationEmailCategory,
	DeliveryPrefix:  "referral-application",
}

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
// site can show its own localized message next to that field. Every official
// site application form reports field errors with it.
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

// applicationSubmission is a validated application form submission ready to
// be emailed.
type applicationSubmission struct {
	Reference string
	Entries   []referralApplicationEntry
	// Values holds the normalized value of every field by key: the option
	// value of a single choice, comma-joined option values of a multiple
	// choice, otherwise the text shown in the email; empty when not filled.
	Values      map[string]string
	SourcePage  string
	Locale      string
	ClientIP    string
	SubmittedAt time.Time
}

// ReferralApplication is a validated submission ready to be emailed.
type ReferralApplication struct {
	applicationSubmission
	// Account is the normalized account value used to pick the confirmation
	// recipient.
	Account string
}

// ReferralApplicationContext carries request facts that are not form fields.
// Every official site application form uses it.
type ReferralApplicationContext struct {
	SourcePage string
	Referer    string
	Locale     string
	ClientIP   string
	Now        time.Time
}

// IsReferralApplicationSpam reports whether the hidden honeypot field was
// filled. Such submissions are accepted silently and never emailed. Every
// official site application form uses the same honeypot field.
func IsReferralApplicationSpam(payload map[string]any) bool {
	value, _ := payload["website"].(string)
	return strings.TrimSpace(value) != ""
}

// ValidateReferralApplication checks a decoded form payload against
// referralApplicationFields and returns the normalized submission.
func ValidateReferralApplication(payload map[string]any, request ReferralApplicationContext) (*ReferralApplication, error) {
	submission, err := validateApplicationForm(referralApplicationForm, payload, request)
	if err != nil {
		return nil, err
	}
	return &ReferralApplication{
		applicationSubmission: *submission,
		Account:               submission.Values["account"],
	}, nil
}

// validateApplicationForm checks a decoded payload against the form's field
// table: the format of every field first, then conditionally required fields,
// then the contact fields. It reports the first invalid field.
func validateApplicationForm(form applicationForm, payload map[string]any, request ReferralApplicationContext) (*applicationSubmission, error) {
	entries := make([]referralApplicationEntry, 0, len(form.Fields))
	values := make(map[string]string, len(form.Fields))
	for _, field := range form.Fields {
		value, display, err := normalizeReferralApplicationField(field, payload[field.Key])
		if err != nil {
			return nil, err
		}
		values[field.Key] = value
		if display == "" {
			display = "-"
		}
		entries = append(entries, referralApplicationEntry{Label: field.Label, Value: display})
	}
	for _, field := range form.Fields {
		condition := field.RequiredWhen
		if condition != nil && values[field.Key] == "" && slices.Contains(condition.Values, values[condition.Key]) {
			return nil, &ReferralApplicationFieldError{Field: field.Key, Code: ReferralApplicationErrorRequired}
		}
	}
	contactFilled := false
	for _, key := range form.ContactKeys {
		contactFilled = contactFilled || values[key] != ""
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
	return &applicationSubmission{
		Reference:   form.ReferencePrefix + strings.ToUpper(common.GetRandomString(8)),
		Entries:     entries,
		Values:      values,
		SourcePage:  sourcePage,
		Locale:      locale,
		ClientIP:    strings.TrimSpace(request.ClientIP),
		SubmittedAt: submittedAt,
	}, nil
}

// normalizeReferralApplicationField returns the normalized value of one field
// (see applicationSubmission.Values) and the text shown in the email.
func normalizeReferralApplicationField(field referralApplicationField, raw any) (string, string, error) {
	fieldError := func(code string) error {
		return &ReferralApplicationFieldError{Field: field.Key, Code: code}
	}
	switch field.Kind {
	case referralFieldConsent:
		accepted, _ := raw.(bool)
		if field.Required && !accepted {
			return "", "", fieldError(ReferralApplicationErrorRequired)
		}
		if accepted {
			return "是", "是", nil
		}
		return "", "", nil
	case referralFieldMultiChoice:
		values, ok := raw.([]any)
		if raw != nil && !ok {
			return "", "", fieldError(ReferralApplicationErrorInvalid)
		}
		selected := make([]string, 0, len(values))
		labels := make([]string, 0, len(values))
		for _, item := range values {
			value, _ := item.(string)
			label, valid := referralApplicationOptionLabel(field, value)
			if !valid {
				return "", "", fieldError(ReferralApplicationErrorInvalid)
			}
			if slices.Contains(selected, value) {
				continue
			}
			selected = append(selected, value)
			labels = append(labels, label)
		}
		if field.Required && len(labels) == 0 {
			return "", "", fieldError(ReferralApplicationErrorRequired)
		}
		return strings.Join(selected, ","), strings.Join(labels, "、"), nil
	}

	if raw != nil {
		if _, ok := raw.(string); !ok {
			return "", "", fieldError(ReferralApplicationErrorInvalid)
		}
	}
	value, _ := raw.(string)
	value = strings.TrimSpace(value)
	if value == "" {
		if field.Required {
			return "", "", fieldError(ReferralApplicationErrorRequired)
		}
		return "", "", nil
	}
	if field.MaxLength > 0 && utf8.RuneCountInString(value) > field.MaxLength {
		return "", "", fieldError(ReferralApplicationErrorTooLong)
	}
	if strings.ContainsFunc(value, func(r rune) bool {
		return unicode.IsControl(r) && !(field.Kind == referralFieldMultiline && (r == '\n' || r == '\r' || r == '\t'))
	}) {
		return "", "", fieldError(ReferralApplicationErrorInvalid)
	}
	if field.MaxLinks > 0 && len(referralApplicationLinkPattern.FindAllStringIndex(value, -1)) > field.MaxLinks {
		return "", "", fieldError(ReferralApplicationErrorTooManyLinks)
	}
	switch field.Kind {
	case referralFieldAccount:
		if !referralApplicationUIDPattern.MatchString(value) && !isReferralApplicationEmail(value) {
			return "", "", fieldError(ReferralApplicationErrorInvalid)
		}
	case referralFieldUID:
		if !referralApplicationUIDPattern.MatchString(value) {
			return "", "", fieldError(ReferralApplicationErrorInvalid)
		}
	case referralFieldEmail:
		if !isReferralApplicationEmail(value) {
			return "", "", fieldError(ReferralApplicationErrorInvalid)
		}
	case referralFieldTelegram:
		normalized, ok := normalizeReferralApplicationTelegram(value)
		if !ok {
			return "", "", fieldError(ReferralApplicationErrorInvalid)
		}
		value = normalized
	case referralFieldChoice:
		label, valid := referralApplicationOptionLabel(field, value)
		if !valid {
			return "", "", fieldError(ReferralApplicationErrorInvalid)
		}
		return value, label, nil
	}
	return value, value, nil
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
// replyNote tells the administrator where the applicant's confirmation went.
func RenderReferralApplicationEmail(application *ReferralApplication, replyNote string) (string, string) {
	return renderApplicationEmail(referralApplicationForm, &application.applicationSubmission, "", nil,
		common.EmailTemplateRow{Label: "自动回信", Value: html.EscapeString(replyNote)})
}

// applicationEmailSection is an extra table of a notification email, shown
// between the form answers and the submission facts.
type applicationEmailSection struct {
	Title string
	Rows  []common.EmailTemplateRow
}

var applicationEmailLocation = time.FixedZone("UTC+8", 8*60*60)

// renderApplicationEmail builds an application form's administrator
// notification: the form answers, any extra sections, then the submission
// facts followed by extraMeta. Section and extraMeta values must already be
// HTML-safe. The subject carries only subjectTag and the reference.
func renderApplicationEmail(form applicationForm, submission *applicationSubmission, subjectTag string, sections []applicationEmailSection, extraMeta ...common.EmailTemplateRow) (string, string) {
	rows := make([]common.EmailTemplateRow, 0, len(submission.Entries))
	for _, entry := range submission.Entries {
		rows = append(rows, common.EmailTemplateRow{Label: entry.Label, Value: common.EscapeAndBreak(entry.Value)})
	}
	sourcePage := submission.SourcePage
	if sourcePage == "" {
		sourcePage = "-"
	}
	locale := submission.Locale
	if locale == "" {
		locale = "-"
	}
	clientIP := submission.ClientIP
	if clientIP == "" {
		clientIP = "-"
	}
	meta := []common.EmailTemplateRow{
		{Label: "申请编号", Value: html.EscapeString(submission.Reference)},
		{Label: "提交时间", Value: html.EscapeString(submission.SubmittedAt.In(applicationEmailLocation).Format("2006-01-02 15:04:05") + "（北京时间）")},
		{Label: "来源页面", Value: html.EscapeString(sourcePage)},
		{Label: "页面语言", Value: html.EscapeString(locale)},
		{Label: "IP", Value: html.EscapeString(clientIP)},
	}
	meta = append(meta, extraMeta...)
	subjectReference := submission.Reference
	if subjectTag != "" {
		subjectReference = subjectTag + " " + subjectReference
	}
	subject := fmt.Sprintf("[%s] %s %s", common.SystemNameOrDefault(), form.Title, subjectReference)
	var body strings.Builder
	body.WriteString(`<div style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;max-width:640px;margin:0 auto;color:#1d1d1f;">`)
	fmt.Fprintf(&body, `<h2 style="font-size:18px;margin:0 0 8px;">%s</h2>`, html.EscapeString(form.Title))
	fmt.Fprintf(&body, `<p style="margin:0 0 16px;color:#6e6e73;font-size:14px;">%s</p>`, html.EscapeString(form.Intro))
	body.WriteString(common.RenderInfoTableHTML(rows))
	for _, section := range sections {
		fmt.Fprintf(&body, `<p style="margin:20px 0 8px;color:#8e8e93;font-size:13px;">%s</p>`, html.EscapeString(section.Title))
		body.WriteString(common.RenderInfoTableHTML(section.Rows))
	}
	body.WriteString(`<p style="margin:20px 0 8px;color:#8e8e93;font-size:13px;">提交信息</p>`)
	body.WriteString(common.RenderInfoTableHTML(meta))
	body.WriteString(`</div>`)
	return subject, body.String()
}

// ErrReferralApplicationEmailNotConfigured means the notification SMTP
// profile has no server, so the application cannot be delivered.
var ErrReferralApplicationEmailNotConfigured = errors.New("referral application email is not configured")

// ReferralApplicationReplyInterval is how long an account waits before it
// can receive another confirmation, so repeated submissions naming someone
// else's UID cannot flood that user's mailbox.
const ReferralApplicationReplyInterval = 30 * 24 * time.Hour

// QueueReferralApplicationEmail stores the notification in the email outbox
// for each configured recipient, then queues the applicant's confirmation
// (see referralApplicationReplyRecipient). The outbox clears subject and body
// after delivery, so only delivery metadata stays in the database. It fails
// only when no administrator recipient could be queued; a confirmation that
// cannot be queued is only logged. Past ApplicationDailyLimit nothing is
// queued.
func QueueReferralApplicationEmail(application *ReferralApplication) error {
	if !applicationEmailConfigured() {
		return ErrReferralApplicationEmailNotConfigured
	}
	recipients := ReferralApplicationRecipients()
	if applicationDailyLimitReached(referralApplicationForm, recipients, &application.applicationSubmission) {
		return nil
	}
	replyTo, replyUserId := "", 0
	replyNote := "未发送：推广返佣未开启"
	if enabled, _, _, _ := model.GetAffiliatePolicyState(); enabled {
		replyTo, replyUserId = referralApplicationReplyRecipient(application)
		replyNote = "未发送：只回信到所填账号的注册邮箱，这个账号查不到"
		if replyTo != "" {
			replyNote = "已发送「申请已通过」邮件至 " + replyTo
			since := application.SubmittedAt.Add(-ReferralApplicationReplyInterval).Unix()
			replied, err := model.CountRecentEmailDeliveries(ReferralApplicationReplyEmailCategory, replyUserId, since)
			switch {
			case err != nil:
				common.SysError("failed to count referral application confirmations: " + err.Error())
				replyTo, replyNote = "", "未发送：查询回信记录失败"
			case replied > 0:
				replyTo, replyNote = "", "未发送：这个账号 30 天内已收到过回信"
			}
		}
	}
	subject, body := RenderReferralApplicationEmail(application, replyNote)
	expires := application.SubmittedAt.Add(7 * 24 * time.Hour).Unix()
	if err := queueApplicationNotification(referralApplicationForm, recipients, application.Reference, subject, body, expires); err != nil {
		return err
	}
	if replyTo == "" {
		return nil
	}
	lang := appI18n.ParseAcceptLanguage(application.Locale)
	if lang == "" {
		lang = appI18n.LangZhCN
	}
	replySubject, replyBody := RenderEmailByKeyForLang(constant.EmailTemplateKeyReferralApplicationUser, lang, referralApplicationReplyVars(application.Reference, lang))
	if _, err := QueueSystemEmail("referral-application-reply:"+application.Reference, ReferralApplicationReplyEmailCategory, 0, replyUserId, replyTo, replySubject, replyBody, expires); err != nil {
		common.SysError("failed to queue referral application confirmation " + application.Reference + ": " + err.Error())
	}
	return nil
}

// applicationEmailConfigured reports whether the notification SMTP profile has
// a server, without which no application can be delivered.
func applicationEmailConfigured() bool {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return strings.TrimSpace(common.SMTPServer) != ""
}

// ApplicationDailyLimit bounds the applications of each official site form
// emailed in any 24 hours. The forms are public and scripted submissions
// rotate IPs past the per-IP limit, so later submissions are accepted but
// not emailed.
const ApplicationDailyLimit = 20

// applicationDailyLimitReached reports whether the form already emailed
// ApplicationDailyLimit applications in the 24 hours before the submission,
// counting the administrator notifications in the outbox, and logs the
// submission that is dropped. A failed count does not hold the application
// back.
func applicationDailyLimitReached(form applicationForm, recipients []string, submission *applicationSubmission) bool {
	since := submission.SubmittedAt.Add(-24 * time.Hour).Unix()
	notified, err := model.CountRecentEmailDeliveries(form.EmailCategory, 0, since)
	if err != nil {
		common.SysError("failed to count recent " + form.DeliveryPrefix + " emails: " + err.Error())
		return false
	}
	if notified < int64(ApplicationDailyLimit*len(recipients)) {
		return false
	}
	common.SysError(form.DeliveryPrefix + " " + submission.Reference + " from " + submission.ClientIP + " was not emailed: daily limit reached")
	return true
}

// queueApplicationNotification stores an application form's administrator
// notification in the email outbox for each configured recipient. The outbox
// clears subject and body after delivery, so only delivery metadata stays in
// the database. It fails only when no recipient could be queued.
func queueApplicationNotification(form applicationForm, recipients []string, reference string, subject string, body string, expires int64) error {
	var queueErr error
	queued := 0
	for _, recipient := range recipients {
		deliveryKey := fmt.Sprintf("%s:%s:%s", form.DeliveryPrefix, reference, strings.ToLower(recipient))
		if _, err := QueueSystemEmail(deliveryKey, form.EmailCategory, 0, 0, recipient, subject, body, expires); err != nil {
			queueErr = errors.Join(queueErr, fmt.Errorf("%s: %w", recipient, err))
			continue
		}
		queued++
	}
	if queued == 0 {
		return queueErr
	}
	if queueErr != nil {
		common.SysError(form.DeliveryPrefix + " " + reference + " was not queued for every recipient: " + queueErr.Error())
	}
	return nil
}

// referralApplicationReplyRecipient picks the address of the applicant's
// confirmation: only the registered email of the enabled user whose UID or
// sign-up email was entered as the account (returned with that user's id).
// The contact email is never used, so the public form cannot make the site
// email arbitrary addresses. The submitter never learns which address was
// used, and the confirmation carries none of the submitted text.
func referralApplicationReplyRecipient(application *ReferralApplication) (string, int) {
	var user *model.User
	var err error
	if isReferralApplicationEmail(application.Account) {
		user, err = model.GetUniqueUserByEmail(application.Account)
	} else {
		userId, _ := strconv.Atoi(application.Account)
		if userId <= 0 {
			return "", 0
		}
		user, err = model.GetUserById(userId, false)
	}
	if err != nil || user.Status != common.UserStatusEnabled {
		return "", 0
	}
	email := strings.TrimSpace(user.Email)
	if !isReferralApplicationEmail(email) {
		return "", 0
	}
	return email, user.Id
}

// referralApplicationReplyVars fills the confirmation template from the live
// referral policy, so its rates and thresholds match the console. The
// template preview uses it too.
func referralApplicationReplyVars(reference string, lang string) map[string]string {
	systemName := common.SystemNameOrDefault()
	baseURL := systemEmailSiteURL()
	text := func(key string, data ...map[string]any) string {
		return affiliateEmailText(lang, key, data...)
	}
	startRate := formatAffiliateRate(model.AffiliateRateBasisPointsForGroup(model.AffiliatePromoterGroupDefault))
	rows := []common.EmailTemplateRow{
		{Label: text(appI18n.MsgReferralApplicationEmailLabelReference), Value: html.EscapeString(reference)},
		{Label: text(appI18n.MsgReferralApplicationEmailLabelStartRate), Value: html.EscapeString(text(appI18n.MsgReferralApplicationEmailValueStartRate, map[string]any{
			"Rate": startRate, "Tier": affiliateTierLabel(model.AffiliatePromoterGroupDefault, lang),
		}))},
	}
	vars := map[string]string{"next_group": "", "next_rate": "", "upgrade_invitees": "", "upgrade_amount": ""}
	if target, ok := model.GetAffiliateUpgradeTarget(model.AffiliatePromoterGroupDefault); ok {
		nextGroup := affiliateTierLabel(target.Group, lang)
		nextRate := formatAffiliateRate(target.RateBasisPoints)
		upgradeAmount := formatAffiliateMoney(target.TopUpAmountThresholdCents)
		rows = append(rows,
			common.EmailTemplateRow{Label: text(appI18n.MsgReferralApplicationEmailLabelNextGroup), Value: html.EscapeString(text(appI18n.MsgReferralApplicationEmailValueNextGroup, map[string]any{
				"Tier": nextGroup, "Rate": nextRate,
			}))},
			common.EmailTemplateRow{Label: text(appI18n.MsgReferralApplicationEmailLabelUpgrade), Value: html.EscapeString(text(appI18n.MsgReferralApplicationEmailValueUpgrade, map[string]any{
				"Invitees": target.Threshold, "Amount": upgradeAmount,
			}))},
		)
		vars["next_group"] = html.EscapeString(nextGroup)
		vars["next_rate"] = html.EscapeString(nextRate)
		vars["upgrade_invitees"] = strconv.Itoa(target.Threshold)
		vars["upgrade_amount"] = html.EscapeString(upgradeAmount)
	}
	payoutMinimum := formatAffiliateMoney(model.AffiliatePayoutMinimumCents)
	rows = append(rows, common.EmailTemplateRow{Label: text(appI18n.MsgReferralApplicationEmailLabelPayout), Value: html.EscapeString(text(appI18n.MsgReferralApplicationEmailValuePayout, map[string]any{
		"Amount": payoutMinimum, "Day": model.AffiliatePayoutSettlementDay,
	}))})

	var steps strings.Builder
	steps.WriteString(`<ol style="margin:0;padding-left:20px;color:#27324d;font-size:14px;line-height:1.7;">`)
	promoKitURL := baseURL + ReferralPromoKitPath
	for _, key := range []string{appI18n.MsgReferralApplicationEmailStepLink, appI18n.MsgReferralApplicationEmailStepKit, appI18n.MsgReferralApplicationEmailStepShare, appI18n.MsgReferralApplicationEmailStepCommission} {
		item := html.EscapeString(text(key, map[string]any{"SystemName": systemName}))
		if key == appI18n.MsgReferralApplicationEmailStepKit {
			item += fmt.Sprintf(` <a href="%s" style="color:#4f46e5;font-weight:650;">%s</a>`, html.EscapeString(promoKitURL), html.EscapeString(text(appI18n.MsgReferralApplicationEmailStepKitLink)))
		}
		fmt.Fprintf(&steps, `<li style="margin:0 0 6px;">%s</li>`, item)
	}
	steps.WriteString(`</ol>`)

	maps.Copy(vars, map[string]string{
		"email_subject":  html.EscapeString(text(appI18n.MsgReferralApplicationEmailSubject)),
		"system_name":    html.EscapeString(systemName),
		"server_address": html.EscapeString(baseURL),
		"heading":        html.EscapeString(text(appI18n.MsgReferralApplicationEmailHeading)),
		"intro":          html.EscapeString(text(appI18n.MsgReferralApplicationEmailIntro, map[string]any{"SystemName": systemName})),
		"reference":      html.EscapeString(reference),
		"start_rate":     html.EscapeString(startRate),
		"payout_minimum": html.EscapeString(payoutMinimum),
		"settlement_day": strconv.Itoa(model.AffiliatePayoutSettlementDay),
		"info_table":     common.RenderInfoTableHTML(rows),
		"steps_title":    html.EscapeString(text(appI18n.MsgReferralApplicationEmailStepsTitle)),
		"next_steps":     steps.String(),
		"action_url":     html.EscapeString(baseURL + "/referral"),
		"promo_kit_url":  html.EscapeString(promoKitURL),
		"action_label":   html.EscapeString(text(appI18n.MsgReferralApplicationEmailAction)),
		"support_note":   html.EscapeString(text(appI18n.MsgReferralApplicationEmailSupportNote)),
		"rules_note":     html.EscapeString(text(appI18n.MsgReferralApplicationEmailRulesNote)),
		"security_note":  html.EscapeString(text(appI18n.MsgReferralApplicationEmailSecurityNote)),
	})
	return vars
}

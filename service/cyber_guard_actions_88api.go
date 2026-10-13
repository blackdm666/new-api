package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"html"
	"net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// 88api cyberguard: ban side effects (cache, email, root notice, audit) and
// the operations behind the CyberGuard admin API.

const (
	CyberGuardBanEmailCategory = "cyber_guard_ban"
	cyberGuardMaxManualHours   = operation_setting.CyberGuardMaxTierBanHours
	cyberGuardAccountCacheTTL  = 60
	cyberGuardAccountTimeout   = 5 * time.Second
	cyberGuardMaxAccountRows   = 100
	cyberGuardAdoptableLimit   = 500
	cyberGuardUserSearchLimit  = 200
)

var (
	ErrCyberGuardInvalidInput = errors.New("invalid CyberGuard request")

	cyberGuardBeijing        = time.FixedZone("UTC+8", 8*3600)
	cyberGuardResellerGroups = []string{"代理商", "特殊代理商"}
	// Manual bans before CyberGuard wrote "原分组:<group>" into users.remark.
	cyberGuardRemarkGroupPattern = regexp.MustCompile(`原分组\s*[:：]\s*([^\s\]】,，;；)）]+)`)
)

func refreshCyberGuardGroupCache(userId int) {
	if err := model.RefreshUserGroupCache(userId); err != nil {
		common.SysError(fmt.Sprintf("cyberguard: failed to refresh group cache of user %d: %v", userId, err))
	}
}

func notifyCyberGuardRoot(subject string, content string) {
	if root := model.GetRootUser(); root == nil || root.Id == 0 {
		return
	}
	NotifyRootUser("cyber_guard", subject, content)
}

// recordCyberGuardSystemAudit writes a poller-originated audit entry owned by
// the root user (system actor, root-visible only).
func recordCyberGuardSystemAudit(action string, targetUserId int, params map[string]any, content string) {
	ownerId := 0
	if root := model.GetRootUser(); root != nil {
		ownerId = root.Id
	}
	if params == nil {
		params = map[string]any{}
	}
	params["target_user_id"] = targetUserId
	params["actor"] = "system"
	model.RecordOperationAuditLog(ownerId, common.RoleRootUser, content, "", action, params,
		&model.AuditAdminInfo{AdminUsername: "system:cyberguard", AdminRole: common.RoleRootUser, AuthMethod: "system"}, nil)
}

func CyberGuardBanAuditParams(ban *model.CyberGuardBan) map[string]any {
	return map[string]any{
		"case_id":       ban.CaseId,
		"source":        ban.Source,
		"from_group":    ban.OriginalGroup,
		"to_group":      ban.BanGroup,
		"ban_hours":     ban.BanHours,
		"expires_at":    ban.ExpiresAt,
		"units_summary": ban.UnitsSummary,
		"reason":        ban.Reason,
		"email_state":   ban.EmailState,
	}
}

func cyberGuardEndAuditParams(outcome *model.CyberGuardEndResult) map[string]any {
	return map[string]any{
		"case_id":       outcome.Ban.CaseId,
		"end_reason":    outcome.Ban.EndReason,
		"from_group":    outcome.FromGroup,
		"to_group":      outcome.ToGroup,
		"group_changed": outcome.GroupChanged,
		"note":          outcome.Ban.EndNote,
	}
}

// applyCyberGuardBan performs the ban transaction and its post-commit effects.
// Root is notified only for automatic bans.
func applyCyberGuardBan(s operation_setting.CyberGuardSetting, input model.CyberGuardBanInput, notifyRoot bool) (*model.CyberGuardBan, *model.CyberGuardUser, error) {
	input.EmailState = model.CyberGuardEmailStateNone
	if !s.EmailEnabled {
		input.EmailState = model.CyberGuardEmailStateDisabled
	}
	ban, user, err := model.ApplyCyberGuardBan(input)
	if err != nil {
		return nil, nil, err
	}
	refreshCyberGuardGroupCache(user.Id)
	if s.EmailEnabled {
		queueCyberGuardBanEmail(ban, user, fmt.Sprintf("cyberguard-ban:%d", ban.Id))
	}
	if notifyRoot && s.NotifyRoot {
		notifyCyberGuardRoot(fmt.Sprintf("风控中心已自动封禁 %s", ban.CaseId),
			fmt.Sprintf("用户 %s（ID %d）已从分组「%s」移入「%s」。触发：%s；%s。恢复：%s。",
				ban.Username, ban.UserId, ban.OriginalGroup, ban.BanGroup, ban.UnitsSummary, ban.Reason,
				cyberGuardExpiryText(ban.ExpiresAt, appI18n.LangZhCN)))
	}
	return ban, user, nil
}

func endCyberGuardBan(banId int, actor int, reason string, note string, now int64, onlyIfDrifted bool) (*model.CyberGuardEndResult, error) {
	outcome, err := model.EndCyberGuardBan(banId, actor, reason, note, now, onlyIfDrifted)
	if err != nil || outcome == nil {
		return outcome, err
	}
	if outcome.GroupChanged {
		refreshCyberGuardGroupCache(outcome.Ban.UserId)
	}
	return outcome, nil
}

type cyberGuardBanTexts struct {
	Subject       string
	Greeting      string
	Intro         string
	CaseLabel     string
	TimeLabel     string
	RestoreLabel  string
	TimedRestore  string
	ManualRestore string
	ReviewNote    string
}

var cyberGuardBanTextsByLang = map[string]cyberGuardBanTexts{
	appI18n.LangZhCN: {
		Subject:       "账号 GPT 分组使用权限已暂停",
		Greeting:      "你好 %s：",
		Intro:         "我们检测到你的账号在使用 GPT 分组时多次触发上游（OpenAI）的网络安全内容策略。根据平台使用规范，你的账号已被暂停 GPT 分组（GPT-Plus、GPT-Pro 等）的使用权限，其他模型仍可正常使用。",
		CaseLabel:     "案例号",
		TimeLabel:     "暂停时间",
		RestoreLabel:  "恢复方式",
		TimedRestore:  "%s（北京时间）自动恢复",
		ManualRestore: "需联系客服复核后恢复",
		ReviewNote:    "如你认为判断有误，请通过网站客服提交说明并附上案例号，我们会进一步复核。",
	},
	appI18n.LangZhTW: {
		Subject:       "帳號 GPT 分組使用權限已暫停",
		Greeting:      "你好 %s：",
		Intro:         "我們偵測到你的帳號在使用 GPT 分組時多次觸發上游（OpenAI）的網路安全內容政策。根據平台使用規範，你的帳號已被暫停 GPT 分組（GPT-Plus、GPT-Pro 等）的使用權限，其他模型仍可正常使用。",
		CaseLabel:     "案例編號",
		TimeLabel:     "暫停時間",
		RestoreLabel:  "恢復方式",
		TimedRestore:  "%s（北京時間）自動恢復",
		ManualRestore: "需聯絡客服複核後恢復",
		ReviewNote:    "如你認為判斷有誤，請透過網站客服提交說明並附上案例編號，我們會進一步複核。",
	},
	appI18n.LangEn: {
		Subject:       "GPT group access on your account has been suspended",
		Greeting:      "Hello %s,",
		Intro:         "We detected that requests from your account in the GPT groups repeatedly triggered the upstream (OpenAI) cybersecurity content policy. Under the platform usage policy, access to the GPT groups (GPT-Plus, GPT-Pro, etc.) has been suspended for your account. Other models remain available.",
		CaseLabel:     "Case ID",
		TimeLabel:     "Suspended at (UTC+8)",
		RestoreLabel:  "Restoration",
		TimedRestore:  "automatically restored at %s (UTC+8)",
		ManualRestore: "restored after review — please contact support",
		ReviewNote:    "If you believe this is a mistake, please contact support through the website and include the case ID so we can review it.",
	},
}

// normalizeCyberGuardLang maps a user language setting to an email template
// language; Chinese is the default, other non-Chinese languages use English.
func normalizeCyberGuardLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	switch {
	case lang == "":
		return appI18n.LangZhCN
	case lang == "zhtw", strings.HasPrefix(lang, "zh-tw"), strings.HasPrefix(lang, "zh-hant"), strings.HasPrefix(lang, "zh-hk"):
		return appI18n.LangZhTW
	case strings.HasPrefix(lang, "zh"):
		return appI18n.LangZhCN
	case strings.HasPrefix(lang, "fr"):
		return appI18n.LangFr
	case strings.HasPrefix(lang, "ja"):
		return appI18n.LangJa
	case strings.HasPrefix(lang, "ru"):
		return appI18n.LangRu
	case strings.HasPrefix(lang, "vi"):
		return appI18n.LangVi
	default:
		return appI18n.LangEn
	}
}

func cyberGuardTextsFor(lang string) cyberGuardBanTexts {
	if texts, ok := cyberGuardBanTextsByLang[lang]; ok {
		return texts
	}
	return cyberGuardBanTextsByLang[appI18n.LangEn]
}

func cyberGuardFormatTime(ts int64) string {
	return time.Unix(ts, 0).In(cyberGuardBeijing).Format("2006-01-02 15:04")
}

func cyberGuardExpiryText(expiresAt int64, lang string) string {
	texts := cyberGuardTextsFor(lang)
	if expiresAt <= 0 {
		return texts.ManualRestore
	}
	return fmt.Sprintf(texts.TimedRestore, cyberGuardFormatTime(expiresAt))
}

// cyberGuardBanEmailVars builds escaped template variables (RenderPlaceholders
// does not escape).
func cyberGuardBanEmailVars(ban *model.CyberGuardBan, username string, lang string) map[string]string {
	texts := cyberGuardTextsFor(lang)
	caseId := model.CyberGuardCaseId(ban.Id)
	banTime := cyberGuardFormatTime(ban.StartedAt)
	expiresText := cyberGuardExpiryText(ban.ExpiresAt, lang)
	return map[string]string{
		"system_name":    html.EscapeString(common.SystemNameOrDefault()),
		"server_address": html.EscapeString(systemEmailSiteURL()),
		"email_subject":  html.EscapeString(texts.Subject),
		"heading":        html.EscapeString(texts.Subject),
		"greeting":       html.EscapeString(fmt.Sprintf(texts.Greeting, username)),
		"intro":          html.EscapeString(texts.Intro),
		"review_note":    html.EscapeString(texts.ReviewNote),
		"username":       html.EscapeString(username),
		"case_id":        html.EscapeString(caseId),
		"ban_time":       html.EscapeString(banTime),
		"expires_text":   html.EscapeString(expiresText),
		"reason":         html.EscapeString(ban.Reason),
		"info_table": common.RenderInfoTableHTML([]common.EmailTemplateRow{
			{Label: texts.CaseLabel, Value: html.EscapeString(caseId)},
			{Label: texts.TimeLabel, Value: html.EscapeString(banTime)},
			{Label: texts.RestoreLabel, Value: html.EscapeString(expiresText)},
		}),
	}
}

func applyCyberGuardTemplatePreviewSamples(key string, lang string, vars map[string]string) {
	if key != constant.EmailTemplateKeyCyberGuardBan {
		return
	}
	started := time.Date(2026, 10, 13, 21, 30, 0, 0, cyberGuardBeijing).Unix()
	sample := &model.CyberGuardBan{Id: 12, StartedAt: started, ExpiresAt: started + 72*3600, Reason: "自动：第 1 档（7 天内 ≥2 个被标记会话）"}
	for name, value := range cyberGuardBanEmailVars(sample, "alice", normalizeCyberGuardLang(lang)) {
		vars[name] = value
	}
	vars["server_address"] = html.EscapeString(emailTemplatePreviewSiteURL())
}

// queueCyberGuardBanEmail renders the editable ban template in the user's
// language and queues it in the outbox; the outcome is stored on the ban row.
func queueCyberGuardBanEmail(ban *model.CyberGuardBan, user *model.CyberGuardUser, deliveryKey string) {
	state, subject, emailError := model.CyberGuardEmailStateQueued, "", ""
	email := strings.TrimSpace(user.Email)
	parsed, parseErr := mail.ParseAddress(email)
	switch {
	case email == "" || parseErr != nil || parsed.Address != email:
		state = model.CyberGuardEmailStateSkippedNoEmail
	default:
		lang := normalizeCyberGuardLang(user.Language())
		var body string
		subject, body = RenderEmailByKeyForLang(constant.EmailTemplateKeyCyberGuardBan, lang, cyberGuardBanEmailVars(ban, user.Username, lang))
		if subject == "" || body == "" {
			state, emailError = model.CyberGuardEmailStateFailed, "email template rendered empty"
			break
		}
		if _, err := QueueSystemEmail(deliveryKey, CyberGuardBanEmailCategory, ban.Id, user.Id, email, subject, body, 0); err != nil {
			state, emailError = model.CyberGuardEmailStateFailed, err.Error()
		}
	}
	if err := model.UpdateCyberGuardBanEmail(ban.Id, state, deliveryKey, subject, emailError); err != nil {
		common.SysError(fmt.Sprintf("cyberguard: failed to store email state of %s: %v", ban.CaseId, err))
	}
	ban.EmailState, ban.EmailDeliveryKey = state, model.TruncateCyberGuardText(deliveryKey, 64)
	ban.EmailSubject, ban.EmailError = model.TruncateCyberGuardText(subject, 255), model.TruncateCyberGuardText(emailError, 255)
}

func validateCyberGuardHours(hours int) error {
	if hours < 0 || hours > cyberGuardMaxManualHours {
		return fmt.Errorf("%w: hours must be between 0 and %d", ErrCyberGuardInvalidInput, cyberGuardMaxManualHours)
	}
	return nil
}

// CyberGuardManualBan bans a user by hand; it works in every mode.
func CyberGuardManualBan(actorId int, userId int, reason string, hours int) (*model.CyberGuardBan, error) {
	if err := validateCyberGuardHours(hours); err != nil {
		return nil, err
	}
	s := operation_setting.GetCyberGuardSetting()
	now := common.GetTimestamp()
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "手动封禁"
	}
	_, summary, err := matchCyberGuardTier(s, userId, now)
	if err != nil {
		return nil, err
	}
	ban, _, err := applyCyberGuardBan(s, model.CyberGuardBanInput{
		UserId: userId, BanGroup: s.BanGroup, Source: model.CyberGuardBanSourceManual, TierIndex: -1, BanHours: hours,
		UnitsSummary: summary, Reason: reason, CreatedBy: actorId,
		ExemptUserIds: s.ExemptUserIds, Now: now,
	}, false)
	return ban, err
}

// CyberGuardPromote turns an observed proposal into a real ban, whatever the mode.
func CyberGuardPromote(actorId int, banId int) (*model.CyberGuardBan, error) {
	proposal, err := model.GetCyberGuardBan(banId)
	if err != nil {
		return nil, err
	}
	if proposal.Status != model.CyberGuardBanStatusObserved {
		return nil, model.ErrCyberGuardBanStateChanged
	}
	s := operation_setting.GetCyberGuardSetting()
	ban, _, err := applyCyberGuardBan(s, model.CyberGuardBanInput{
		UserId: proposal.UserId, BanGroup: s.BanGroup, Source: model.CyberGuardBanSourceAuto, TierIndex: proposal.TierIndex,
		BanHours: proposal.BanHours, UnitsSummary: proposal.UnitsSummary, Reason: proposal.Reason, CreatedBy: actorId,
		ExemptUserIds: s.ExemptUserIds, Now: common.GetTimestamp(), PromoteFromId: proposal.Id,
	}, false)
	return ban, err
}

func CyberGuardUnban(actorId int, banId int, note string) (*model.CyberGuardEndResult, error) {
	return endCyberGuardBan(banId, actorId, model.CyberGuardEndReasonManualUnban, note, common.GetTimestamp(), false)
}

func CyberGuardDismiss(actorId int, banId int, note string) (*model.CyberGuardBan, error) {
	return model.DismissCyberGuardProposal(banId, actorId, note, common.GetTimestamp())
}

func CyberGuardAdopt(actorId int, userId int, originalGroup string, reason string, hours int) (*model.CyberGuardBan, error) {
	if err := validateCyberGuardHours(hours); err != nil {
		return nil, err
	}
	s := operation_setting.GetCyberGuardSetting()
	originalGroup = strings.TrimSpace(originalGroup)
	if originalGroup == "" || len(originalGroup) > 64 || originalGroup == s.BanGroup {
		return nil, fmt.Errorf("%w: original_group must be a group other than the ban group", ErrCyberGuardInvalidInput)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "导入已有封禁"
	}
	return model.AdoptCyberGuardBan(model.CyberGuardAdoptInput{
		UserId: userId, OriginalGroup: originalGroup, Reason: reason, Hours: hours, BanGroup: s.BanGroup,
		ExemptUserIds: s.ExemptUserIds, Actor: actorId, Now: common.GetTimestamp(),
	})
}

func CyberGuardVoidEvent(actorId int, eventId int, reason string) (*model.CyberGuardEvent, error) {
	return model.VoidCyberGuardEvent(eventId, actorId, reason, common.GetTimestamp())
}

// CyberGuardResendEmail queues the ban email of an active ban again under a
// new delivery key (cyberguard-ban:<id>:r<n>), regardless of email_enabled.
func CyberGuardResendEmail(banId int) (*model.CyberGuardBan, error) {
	ban, err := model.GetCyberGuardBan(banId)
	if err != nil {
		return nil, err
	}
	if ban.Status != model.CyberGuardBanStatusActive {
		return nil, model.ErrCyberGuardBanStateChanged
	}
	user, err := model.GetCyberGuardUser(ban.UserId)
	if err != nil {
		return nil, err
	}
	next := 1
	if _, suffix, ok := strings.Cut(strings.TrimPrefix(ban.EmailDeliveryKey, fmt.Sprintf("cyberguard-ban:%d", ban.Id)), ":r"); ok {
		if n, err := strconv.Atoi(suffix); err == nil {
			next = n + 1
		}
	}
	queueCyberGuardBanEmail(ban, user, fmt.Sprintf("cyberguard-ban:%d:r%d", ban.Id, next))
	return ban, nil
}

// CyberGuardOnAdminGroupChange is called by the admin user editor after it
// changed a user's group. An active ban whose user left the ban group this
// way ends as admin_override (the admin's choice is kept).
func CyberGuardOnAdminGroupChange(userId int, adminId int, adminRole int) {
	active, err := model.GetActiveCyberGuardBanByUser(userId)
	if err != nil || active == nil {
		return
	}
	outcome, err := endCyberGuardBan(active.Id, adminId, model.CyberGuardEndReasonAdminOverride,
		"group changed in the user editor", common.GetTimestamp(), true)
	if err != nil {
		if !errors.Is(err, model.ErrCyberGuardBanStateChanged) {
			common.SysError(fmt.Sprintf("cyberguard: admin override of %s failed: %v", active.CaseId, err))
		}
		return
	}
	if outcome == nil {
		return
	}
	params := cyberGuardEndAuditParams(outcome)
	params["target_user_id"] = userId
	model.RecordOperationAuditLog(adminId, adminRole, fmt.Sprintf("CyberGuard %s ended: group changed by an administrator", active.CaseId),
		"", "cyber_guard.admin_override", params, &model.AuditAdminInfo{AdminID: adminId, AdminRole: adminRole, AuthMethod: "session"}, nil)
}

type CyberGuardAdoptableRow struct {
	UserId                 int    `json:"user_id"`
	Username               string `json:"username"`
	Remark                 string `json:"remark"`
	SuggestedOriginalGroup string `json:"suggested_original_group"`
}

func ListCyberGuardAdoptable() ([]CyberGuardAdoptableRow, error) {
	s := operation_setting.GetCyberGuardSetting()
	users, err := model.ListCyberGuardAdoptable(s.BanGroup, cyberGuardAdoptableLimit)
	if err != nil {
		return nil, err
	}
	rows := make([]CyberGuardAdoptableRow, 0, len(users))
	for _, user := range users {
		suggested := "default"
		if match := cyberGuardRemarkGroupPattern.FindStringSubmatch(user.Remark); match != nil {
			suggested = match[1]
		}
		rows = append(rows, CyberGuardAdoptableRow{UserId: user.Id, Username: user.Username, Remark: user.Remark, SuggestedOriginalGroup: suggested})
	}
	return rows, nil
}

type CyberGuardUserBanBrief struct {
	Id        int    `json:"id"`
	CaseId    string `json:"case_id"`
	Status    string `json:"status"`
	Source    string `json:"source"`
	ExpiresAt int64  `json:"expires_at"`
}

type CyberGuardUserRow struct {
	UserId       int                     `json:"user_id"`
	Username     string                  `json:"username"`
	Email        string                  `json:"email"`
	CurrentGroup string                  `json:"current_group"`
	Role         int                     `json:"role"`
	Units7d      int                     `json:"units_7d"`
	Units30d     int                     `json:"units_30d"`
	Hits30d      int64                   `json:"hits_30d"`
	LastHitAt    int64                   `json:"last_hit_at"`
	Ban          *CyberGuardUserBanBrief `json:"ban"`
	IsReseller   bool                    `json:"is_reseller"`
}

// ListCyberGuardUsers lists users with CyberGuard activity (or, with a
// keyword, any matching user so that an admin can ban by hand). Units are
// counted like the tier evaluation (distinct units after the counting floor).
func ListCyberGuardUsers(keyword string, status string, page int, pageSize int) ([]CyberGuardUserRow, int, error) {
	activity, err := model.ListCyberGuardUserActivity()
	if err != nil {
		return nil, 0, err
	}
	lastHit := make(map[int]int64, len(activity))
	for _, row := range activity {
		lastHit[row.UserId] = row.LastHitAt
	}
	candidates := make([]int, 0, len(activity))
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		if candidates, err = model.SearchCyberGuardUserIds(keyword, cyberGuardUserSearchLimit); err != nil {
			return nil, 0, err
		}
	} else {
		for _, row := range activity {
			candidates = append(candidates, row.UserId)
		}
	}
	slices.SortStableFunc(candidates, func(a, b int) int {
		if lastHit[a] != lastHit[b] {
			return cmp.Compare(lastHit[b], lastHit[a])
		}
		return cmp.Compare(b, a)
	})
	open, err := model.CyberGuardOpenBansByUser(candidates)
	if err != nil {
		return nil, 0, err
	}
	filtered := make([]int, 0, len(candidates))
	for _, userId := range candidates {
		ban := open[userId]
		switch status {
		case "active":
			if ban == nil || ban.Status != model.CyberGuardBanStatusActive {
				continue
			}
		case "observed":
			if ban == nil || ban.Status != model.CyberGuardBanStatusObserved {
				continue
			}
		case "clean":
			if ban != nil {
				continue
			}
		}
		filtered = append(filtered, userId)
	}
	total := len(filtered)
	start := min(max((page-1)*pageSize, 0), total)
	pageIds := filtered[start:min(start+pageSize, total)]

	now := common.GetTimestamp()
	users, err := model.GetCyberGuardUsers(pageIds)
	if err != nil {
		return nil, 0, err
	}
	floors, err := model.CyberGuardCountingFloors(pageIds)
	if err != nil {
		return nil, 0, err
	}
	units, err := model.ListCyberGuardCountedUnits(pageIds, now-30*86400)
	if err != nil {
		return nil, 0, err
	}
	hits, err := model.CyberGuardHitsByUser(pageIds, now-30*86400)
	if err != nil {
		return nil, 0, err
	}
	units7d, units30d := map[int]map[string]bool{}, map[int]map[string]bool{}
	for _, unit := range units {
		if unit.OccurredAt <= floors[unit.UserId] {
			continue
		}
		if units30d[unit.UserId] == nil {
			units30d[unit.UserId], units7d[unit.UserId] = map[string]bool{}, map[string]bool{}
		}
		units30d[unit.UserId][unit.UnitKey] = true
		if unit.OccurredAt > now-7*86400 {
			units7d[unit.UserId][unit.UnitKey] = true
		}
	}
	rows := make([]CyberGuardUserRow, 0, len(pageIds))
	for _, userId := range pageIds {
		row := CyberGuardUserRow{UserId: userId, Units7d: len(units7d[userId]), Units30d: len(units30d[userId]),
			Hits30d: hits[userId].Hits, LastHitAt: lastHit[userId]}
		if user := users[userId]; user != nil {
			row.Username, row.Email, row.CurrentGroup, row.Role = user.Username, user.MaskedEmail(), user.Group, user.Role
		}
		if ban := open[userId]; ban != nil {
			row.Ban = &CyberGuardUserBanBrief{Id: ban.Id, CaseId: ban.CaseId, Status: ban.Status, Source: ban.Source, ExpiresAt: ban.ExpiresAt}
			row.IsReseller = slices.Contains(cyberGuardResellerGroups, ban.OriginalGroup)
		}
		row.IsReseller = row.IsReseller || slices.Contains(cyberGuardResellerGroups, row.CurrentGroup)
		rows = append(rows, row)
	}
	return rows, total, nil
}

type CyberGuardBanRow struct {
	*model.CyberGuardBan
	CurrentGroup       string `json:"current_group"`
	EmailDeliveryState string `json:"email_delivery_state"`
}

func ListCyberGuardBanRows(status string, userId int, offset int, limit int) ([]CyberGuardBanRow, int64, error) {
	bans, total, err := model.ListCyberGuardBans(status, userId, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	userIds := make([]int, 0, len(bans))
	keys := make([]string, 0, len(bans))
	for _, ban := range bans {
		userIds = append(userIds, ban.UserId)
		if ban.EmailDeliveryKey != "" {
			keys = append(keys, ban.EmailDeliveryKey)
		}
	}
	groups, err := model.GetCyberGuardUserGroups(userIds)
	if err != nil {
		return nil, 0, err
	}
	states, err := model.CyberGuardEmailDeliveryStates(keys)
	if err != nil {
		return nil, 0, err
	}
	rows := make([]CyberGuardBanRow, 0, len(bans))
	for _, ban := range bans {
		rows = append(rows, CyberGuardBanRow{CyberGuardBan: ban, CurrentGroup: groups[ban.UserId], EmailDeliveryState: states[ban.EmailDeliveryKey]})
	}
	return rows, total, nil
}

type CyberGuardLastPoll struct {
	At        int64  `json:"at"`
	Ok        bool   `json:"ok"`
	Status    string `json:"status"`
	Error     string `json:"error"`
	Fetched   int    `json:"fetched"`
	NewEvents int    `json:"new_events"`
	CursorEnd int64  `json:"cursor_end"`
}

type CyberGuardOverview struct {
	Mode              string                         `json:"mode"`
	IsMasterNode      bool                           `json:"is_master_node"`
	LastPoll          CyberGuardLastPoll             `json:"last_poll"`
	Counts            model.CyberGuardOverviewCounts `json:"counts"`
	Reconciliation24h model.CyberGuardReconciliation `json:"reconciliation_24h"`
}

func GetCyberGuardOverview() (*CyberGuardOverview, error) {
	s := operation_setting.GetCyberGuardSetting()
	state, err := model.GetCyberGuardPollState()
	if err != nil {
		return nil, err
	}
	now := time.Now().In(cyberGuardBeijing)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cyberGuardBeijing).Unix()
	counts, recon, err := model.GetCyberGuardOverviewCounts(now.Unix(), todayStart)
	if err != nil {
		return nil, err
	}
	return &CyberGuardOverview{
		Mode:         s.Mode,
		IsMasterNode: common.IsMasterNode,
		LastPoll: CyberGuardLastPoll{
			At: state.LastRunAt, Ok: state.LastStatus == CyberGuardPollStatusOK, Status: state.LastStatus, Error: state.LastError,
			Fetched: state.LastFetched, NewEvents: state.LastNewEvents, CursorEnd: state.CursorEnd,
		},
		Counts:            counts,
		Reconciliation24h: recon,
	}, nil
}

type CyberGuardAccountRow struct {
	model.CyberGuardAccountStat
	Status       string `json:"status"`
	Schedulable  *bool  `json:"schedulable"`
	ErrorMessage string `json:"error_message"`
}

type cyberGuardAccountInfo struct {
	Status       string `json:"status"`
	Schedulable  *bool  `json:"schedulable"`
	ErrorMessage string `json:"error_message"`
	fetchedAt    int64
}

var (
	cyberGuardAccountCacheMu sync.Mutex
	cyberGuardAccountCache   = map[int64]cyberGuardAccountInfo{}
)

// ListCyberGuardAccounts aggregates hits per sub2api account and adds the
// account status from sub2api (GET /api/v1/admin/accounts/:id, cached 60 s).
// After the first failed lookup the remaining accounts show "unknown".
func ListCyberGuardAccounts(ctx context.Context, days int) ([]CyberGuardAccountRow, error) {
	stats, err := model.CyberGuardAccountStats(common.GetTimestamp()-int64(days)*86400, cyberGuardMaxAccountRows)
	if err != nil {
		return nil, err
	}
	s := operation_setting.GetCyberGuardSetting()
	reachable := s.Sub2apiAdminApiKey != "" && s.Sub2apiBaseUrl != ""
	now := common.GetTimestamp()
	rows := make([]CyberGuardAccountRow, 0, len(stats))
	for _, stat := range stats {
		cyberGuardAccountCacheMu.Lock()
		info, cached := cyberGuardAccountCache[stat.AccountId]
		cyberGuardAccountCacheMu.Unlock()
		if !cached || now-info.fetchedAt >= cyberGuardAccountCacheTTL {
			info = cyberGuardAccountInfo{Status: "unknown"}
			if reachable {
				lookupCtx, cancel := context.WithTimeout(ctx, cyberGuardAccountTimeout)
				var fetched cyberGuardAccountInfo
				err := cyberGuardSub2apiGet(lookupCtx, s, "/api/v1/admin/accounts/"+strconv.FormatInt(stat.AccountId, 10), nil, &fetched)
				cancel()
				if err == nil {
					info = fetched
				} else {
					reachable = false
				}
			}
			info.fetchedAt = now
			cyberGuardAccountCacheMu.Lock()
			cyberGuardAccountCache[stat.AccountId] = info
			cyberGuardAccountCacheMu.Unlock()
		}
		rows = append(rows, CyberGuardAccountRow{CyberGuardAccountStat: stat, Status: info.Status,
			Schedulable: info.Schedulable, ErrorMessage: model.TruncateCyberGuardText(info.ErrorMessage, 300)})
	}
	return rows, nil
}

// CyberGuardSettingView never carries the sub2api admin API key; the shadowing
// field keeps the embedded secret out of the JSON.
type CyberGuardSettingView struct {
	operation_setting.CyberGuardSetting
	Sub2apiAdminApiKey    string `json:"sub2api_admin_api_key,omitempty"`
	HasSub2apiAdminApiKey bool   `json:"has_sub2api_admin_api_key"`
}

func GetCyberGuardSettingView() CyberGuardSettingView {
	s := operation_setting.GetCyberGuardSetting()
	hasKey := s.Sub2apiAdminApiKey != ""
	s.Sub2apiAdminApiKey = ""
	return CyberGuardSettingView{CyberGuardSetting: s, HasSub2apiAdminApiKey: hasKey}
}

// MergeCyberGuardSettingUpdate decodes a PUT /settings body on top of the
// current setting, so omitted fields keep their values. The secret starts
// empty (empty = keep the stored key) and is never echoed back.
func MergeCyberGuardSettingUpdate(body []byte) (operation_setting.CyberGuardSetting, error) {
	current := operation_setting.GetCyberGuardSetting()
	input := current
	input.Sub2apiAdminApiKey = ""
	input.ProtectedChannelIds, input.Sub2apiApiKeyIds, input.CountCategories, input.Tiers, input.ExemptUserIds = nil, nil, nil, nil, nil
	if err := common.Unmarshal(body, &input); err != nil {
		return input, fmt.Errorf("%w: %v", ErrCyberGuardInvalidInput, err)
	}
	if input.ProtectedChannelIds == nil {
		input.ProtectedChannelIds = current.ProtectedChannelIds
	}
	if input.Sub2apiApiKeyIds == nil {
		input.Sub2apiApiKeyIds = current.Sub2apiApiKeyIds
	}
	if input.CountCategories == nil {
		input.CountCategories = current.CountCategories
	}
	if input.Tiers == nil {
		input.Tiers = current.Tiers
	}
	if input.ExemptUserIds == nil {
		input.ExemptUserIds = current.ExemptUserIds
	}
	return input, nil
}

// UpdateCyberGuardSettings validates and persists the whole setting (empty
// secret keeps the stored key). It returns the option fields that changed;
// turning the mode on from off resets the poll cursor so nothing is backfilled.
func UpdateCyberGuardSettings(input operation_setting.CyberGuardSetting) ([]string, error) {
	current := operation_setting.GetCyberGuardSetting()
	input.Mode = strings.TrimSpace(input.Mode)
	input.BanGroup = strings.TrimSpace(input.BanGroup)
	input.Sub2apiBaseUrl = strings.TrimRight(strings.TrimSpace(input.Sub2apiBaseUrl), "/")
	input.Sub2apiAdminApiKey = strings.TrimSpace(input.Sub2apiAdminApiKey)
	if input.ProtectedChannelIds == nil {
		input.ProtectedChannelIds = []int{}
	}
	if input.CountCategories == nil {
		input.CountCategories = []string{}
	}
	if input.ExemptUserIds == nil {
		input.ExemptUserIds = []int{}
	}
	if err := operation_setting.ValidateCyberGuardSetting(input); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCyberGuardInvalidInput, err)
	}
	_, hasSpecialRules := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(input.BanGroup)
	if !ratio_setting.ContainsGroupRatio(input.BanGroup) && !hasSpecialRules {
		return nil, fmt.Errorf("%w: ban_group %q does not exist", ErrCyberGuardInvalidInput, input.BanGroup)
	}
	if input.Sub2apiAdminApiKey == "" {
		input.Sub2apiAdminApiKey = current.Sub2apiAdminApiKey
	} else if len(input.Sub2apiAdminApiKey) > 512 || strings.ContainsAny(input.Sub2apiAdminApiKey, " \r\n\t") {
		return nil, fmt.Errorf("%w: sub2api_admin_api_key is invalid", ErrCyberGuardInvalidInput)
	}
	next, err := config.ConfigToMap(&input)
	if err != nil {
		return nil, err
	}
	previous, err := config.ConfigToMap(&current)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string, len(next))
	changed := make([]string, 0, len(next))
	for field, value := range next {
		if previous[field] == value {
			continue
		}
		values[operation_setting.CyberGuardSettingName+"."+field] = value
		changed = append(changed, field)
	}
	if len(values) == 0 {
		return changed, nil
	}
	if err := model.UpdateOptionsBulk(values); err != nil {
		return nil, err
	}
	if current.Mode == operation_setting.CyberGuardModeOff && input.Mode != operation_setting.CyberGuardModeOff {
		if err := model.ResetCyberGuardPollCursor(); err != nil {
			return changed, err
		}
	}
	slices.Sort(changed)
	return changed, nil
}

package model

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 88api cyberguard: ledger of sub2api upstream content-policy refusals and the
// ban records CyberGuard creates when a user reaches a configured threshold.

const (
	CyberGuardEventStatePending      = "pending"
	CyberGuardEventStateCounted      = "counted"
	CyberGuardEventStateRecordOnly   = "record_only"
	CyberGuardEventStateUnattributed = "unattributed"
	CyberGuardEventStateExempt       = "exempt"
	CyberGuardEventStateVoid         = "void"

	CyberGuardBanSourceAuto    = "auto"
	CyberGuardBanSourceManual  = "manual"
	CyberGuardBanSourceAdopted = "adopted"

	CyberGuardBanStatusObserved  = "observed"
	CyberGuardBanStatusActive    = "active"
	CyberGuardBanStatusEnded     = "ended"
	CyberGuardBanStatusDismissed = "dismissed"

	CyberGuardEndReasonExpired       = "expired"
	CyberGuardEndReasonManualUnban   = "manual_unban"
	CyberGuardEndReasonAdminOverride = "admin_override"
	CyberGuardEndReasonPromoted      = "promoted"
	CyberGuardEndReasonDismissed     = "dismissed"
	CyberGuardEndReasonUserMissing   = "user_missing"

	CyberGuardEmailStateNone           = "none"
	CyberGuardEmailStateDisabled       = "disabled"
	CyberGuardEmailStateQueued         = "queued"
	CyberGuardEmailStateSkippedNoEmail = "skipped_no_email"
	CyberGuardEmailStateFailed         = "failed"

	cyberGuardPollStateId = 1
)

var (
	ErrCyberGuardUserNotFound    = errors.New("user not found")
	ErrCyberGuardProtectedUser   = errors.New("administrators and exempt users cannot be banned")
	ErrCyberGuardAlreadyBanned   = errors.New("user already has an active CyberGuard ban")
	ErrCyberGuardInBanGroup      = errors.New("user is already in the ban group; use adopt instead")
	ErrCyberGuardNotInBanGroup   = errors.New("user is not in the ban group")
	ErrCyberGuardOpenRecord      = errors.New("user already has an active or observed CyberGuard record")
	ErrCyberGuardBanNotFound     = errors.New("CyberGuard record not found")
	ErrCyberGuardBanStateChanged = errors.New("CyberGuard record is not in the expected status")
	ErrCyberGuardEventNotFound   = errors.New("CyberGuard event not found or already void")
)

// CyberGuardEvent is one sub2api ops_error_logs row that CyberGuard classified.
type CyberGuardEvent struct {
	Id               int    `json:"id"`
	OpsId            int64  `json:"ops_id" gorm:"uniqueIndex:uk_cg_ev_ops_id;not null"`
	OccurredAt       int64  `json:"occurred_at" gorm:"index:idx_cg_ev_user_time,priority:2;index:idx_cg_ev_state_time,priority:2"`
	Sub2apiRequestId string `json:"sub2api_request_id" gorm:"type:varchar(128);index:idx_cg_ev_request_id"`
	ApiKeyId         int64  `json:"api_key_id"`
	AccountId        int64  `json:"account_id"`
	AccountName      string `json:"account_name" gorm:"type:varchar(128)"`
	Category         string `json:"category" gorm:"type:varchar(16)"`
	ErrorType        string `json:"error_type" gorm:"type:varchar(64)"`
	StatusCode       int    `json:"status_code"`
	Endpoint         string `json:"endpoint" gorm:"type:varchar(128)"`
	Model            string `json:"model" gorm:"type:varchar(128)"`
	Message          string `json:"message" gorm:"type:varchar(300)"`

	UserId     int    `json:"user_id" gorm:"index:idx_cg_ev_user_time,priority:1"`
	Username   string `json:"username" gorm:"type:varchar(64)"`
	TokenId    int    `json:"token_id"`
	TokenName  string `json:"token_name" gorm:"type:varchar(64)"`
	TokenGroup string `json:"token_group" gorm:"type:varchar(64)"`
	ChannelId  int    `json:"channel_id"`
	LogId      int    `json:"log_id"`
	SessionFp  string `json:"session_fp" gorm:"type:varchar(16)"`
	UnitKey    string `json:"unit_key" gorm:"type:varchar(96);index:idx_cg_ev_unit_key"`
	IsNewUnit  bool   `json:"is_new_unit"`

	State      string `json:"state" gorm:"type:varchar(16);index:idx_cg_ev_state_time,priority:1"`
	VoidReason string `json:"void_reason" gorm:"type:varchar(255)"`
	VoidedBy   int    `json:"voided_by"`
	VoidedAt   int64  `json:"voided_at"`
	CreatedAt  int64  `json:"created_at"`
}

// CyberGuardBan is an observed proposal or an actual ban (case id CG-<id>).
type CyberGuardBan struct {
	Id               int    `json:"id"`
	CaseId           string `json:"case_id" gorm:"-"`
	UserId           int    `json:"user_id" gorm:"index:idx_cg_ban_user"`
	Username         string `json:"username" gorm:"type:varchar(64)"`
	OriginalGroup    string `json:"original_group" gorm:"type:varchar(64)"`
	RestoreGroup     string `json:"restore_group" gorm:"type:varchar(64)"`
	BanGroup         string `json:"ban_group" gorm:"type:varchar(64)"`
	Source           string `json:"source" gorm:"type:varchar(16)"`
	Status           string `json:"status" gorm:"type:varchar(16);index:idx_cg_ban_status"`
	TierIndex        int    `json:"tier_index"`
	BanHours         int    `json:"ban_hours"`
	UnitsSummary     string `json:"units_summary" gorm:"type:varchar(255)"`
	Reason           string `json:"reason" gorm:"type:varchar(500)"`
	StartedAt        int64  `json:"started_at"`
	ExpiresAt        int64  `json:"expires_at"`
	EndedAt          int64  `json:"ended_at"`
	EndReason        string `json:"end_reason" gorm:"type:varchar(32)"`
	EndNote          string `json:"end_note" gorm:"type:varchar(255)"`
	EndedBy          int    `json:"ended_by"`
	CreatedBy        int    `json:"created_by"`
	EmailState       string `json:"email_state" gorm:"type:varchar(24)"`
	EmailDeliveryKey string `json:"email_delivery_key" gorm:"type:varchar(64)"`
	EmailSubject     string `json:"email_subject" gorm:"type:varchar(255)"`
	EmailError       string `json:"email_error" gorm:"type:varchar(255)"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
}

// CyberGuardPollState is a single row (id 1) holding the poller cursor and the
// outcome of the latest run.
type CyberGuardPollState struct {
	Id            int    `json:"-" gorm:"primaryKey;autoIncrement:false"`
	CursorEnd     int64  `json:"cursor_end"`
	LastRunAt     int64  `json:"last_run_at"`
	LastStatus    string `json:"last_status" gorm:"type:varchar(16)"`
	LastError     string `json:"last_error" gorm:"type:varchar(500)"`
	LastFetched   int    `json:"last_fetched"`
	LastNewEvents int    `json:"last_new_events"`
	UpdatedAt     int64  `json:"updated_at"`
}

func CyberGuardCaseId(id int) string {
	return fmt.Sprintf("CG-%d", id)
}

func (ban *CyberGuardBan) AfterFind(_ *gorm.DB) error {
	ban.CaseId = CyberGuardCaseId(ban.Id)
	return nil
}

func (ban *CyberGuardBan) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if ban.CreatedAt == 0 {
		ban.CreatedAt = now
	}
	ban.UpdatedAt = ban.CreatedAt
	return nil
}

func (ban *CyberGuardBan) AfterCreate(_ *gorm.DB) error {
	ban.CaseId = CyberGuardCaseId(ban.Id)
	return nil
}

func (event *CyberGuardEvent) BeforeCreate(_ *gorm.DB) error {
	if event.CreatedAt == 0 {
		event.CreatedAt = common.GetTimestamp()
	}
	return nil
}

func TruncateCyberGuardText(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

// CyberGuardUser is the subset of a user row CyberGuard reads.
type CyberGuardUser struct {
	Id       int    `json:"id"`
	Username string `json:"username"`
	Role     int    `json:"role"`
	Status   int    `json:"status"`
	Group    string `json:"group"`
	Email    string `json:"email"`
	Remark   string `json:"remark"`
	Setting  string `json:"-"`
}

func (u *CyberGuardUser) Language() string {
	return (&User{Setting: u.Setting}).GetSetting().Language
}

// MaskedEmail hides the local part the same way user-notice recipients are shown.
func (u *CyberGuardUser) MaskedEmail() string {
	if strings.TrimSpace(u.Email) == "" {
		return ""
	}
	return maskEmailAddress(u.Email)
}

// selectCyberGuardUser selects the CyberGuardUser columns. Field names (not a
// raw column list) let GORM quote the reserved "group" column per dialect.
func selectCyberGuardUser(db *gorm.DB) *gorm.DB {
	return db.Model(&User{}).Select("id", "username", "role", "status", "group", "email", "remark", "setting")
}

// lockCyberGuardUser row-locks the user (no-op on SQLite) and reports whether
// a non-deleted user exists.
func lockCyberGuardUser(tx *gorm.DB, userId int) (*CyberGuardUser, bool, error) {
	var user User
	result := lockForUpdate(selectCyberGuardUser(tx)).Where("id = ?", userId).Limit(1).Find(&user)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}
	return cyberGuardUserFrom(&user), true, nil
}

func cyberGuardUserFrom(user *User) *CyberGuardUser {
	return &CyberGuardUser{Id: user.Id, Username: user.Username, Role: user.Role, Status: user.Status,
		Group: user.Group, Email: user.Email, Remark: user.Remark, Setting: user.Setting}
}

func GetCyberGuardUser(userId int) (*CyberGuardUser, error) {
	var user User
	result := selectCyberGuardUser(DB).Where("id = ?", userId).Limit(1).Find(&user)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrCyberGuardUserNotFound
	}
	return cyberGuardUserFrom(&user), nil
}

func lockCyberGuardBan(tx *gorm.DB, banId int) (*CyberGuardBan, error) {
	var ban CyberGuardBan
	result := lockForUpdate(tx).Where("id = ?", banId).Limit(1).Find(&ban)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrCyberGuardBanNotFound
	}
	return &ban, nil
}

func GetCyberGuardBan(banId int) (*CyberGuardBan, error) {
	var ban CyberGuardBan
	result := DB.Where("id = ?", banId).Limit(1).Find(&ban)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrCyberGuardBanNotFound
	}
	return &ban, nil
}

func findOpenCyberGuardBan(tx *gorm.DB, userId int, status string) (*CyberGuardBan, error) {
	var ban CyberGuardBan
	result := tx.Where("user_id = ? AND status = ?", userId, status).Order("id desc").Limit(1).Find(&ban)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &ban, nil
}

func GetActiveCyberGuardBanByUser(userId int) (*CyberGuardBan, error) {
	return findOpenCyberGuardBan(DB, userId, CyberGuardBanStatusActive)
}

func cyberGuardRestoreTarget(ban *CyberGuardBan) string {
	if group := strings.TrimSpace(ban.RestoreGroup); group != "" {
		return group
	}
	if group := strings.TrimSpace(ban.OriginalGroup); group != "" {
		return group
	}
	return "default"
}

func cyberGuardExpiresAt(now int64, hours int) int64 {
	if hours <= 0 {
		return 0
	}
	return now + int64(hours)*3600
}

type CyberGuardBanInput struct {
	UserId        int
	BanGroup      string
	Source        string
	TierIndex     int
	BanHours      int
	UnitsSummary  string
	Reason        string
	CreatedBy     int
	ExemptUserIds []int
	EmailState    string
	Now           int64
	// PromoteFromId, when set, must still be an observed proposal of the user.
	PromoteFromId int
}

// ApplyCyberGuardBan moves the user into the ban group and records an active
// ban in one transaction. Any open observed proposal of the user is closed as
// promoted. The caller refreshes caches, audits and emails after commit.
func ApplyCyberGuardBan(input CyberGuardBanInput) (*CyberGuardBan, *CyberGuardUser, error) {
	banGroup := strings.TrimSpace(input.BanGroup)
	if banGroup == "" {
		return nil, nil, errors.New("ban group is not configured")
	}
	var ban *CyberGuardBan
	var user *CyberGuardUser
	err := DB.Transaction(func(tx *gorm.DB) error {
		locked, ok, err := lockCyberGuardUser(tx, input.UserId)
		if err != nil {
			return err
		}
		if !ok {
			return ErrCyberGuardUserNotFound
		}
		if locked.Role >= common.RoleAdminUser || slices.Contains(input.ExemptUserIds, locked.Id) {
			return ErrCyberGuardProtectedUser
		}
		active, err := findOpenCyberGuardBan(tx, locked.Id, CyberGuardBanStatusActive)
		if err != nil {
			return err
		}
		if active != nil {
			return ErrCyberGuardAlreadyBanned
		}
		if locked.Group == banGroup {
			return ErrCyberGuardInBanGroup
		}
		if input.PromoteFromId > 0 {
			proposal, err := lockCyberGuardBan(tx, input.PromoteFromId)
			if err != nil {
				return err
			}
			if proposal.UserId != locked.Id || proposal.Status != CyberGuardBanStatusObserved {
				return ErrCyberGuardBanStateChanged
			}
		}
		if err := tx.Model(&CyberGuardBan{}).
			Where("user_id = ? AND status = ?", locked.Id, CyberGuardBanStatusObserved).
			Updates(map[string]any{
				"status":     CyberGuardBanStatusEnded,
				"ended_at":   input.Now,
				"ended_by":   input.CreatedBy,
				"end_reason": CyberGuardEndReasonPromoted,
				"updated_at": input.Now,
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&User{}).Where("id = ?", locked.Id).Update("group", banGroup).Error; err != nil {
			return err
		}
		ban = &CyberGuardBan{
			UserId:        locked.Id,
			Username:      TruncateCyberGuardText(locked.Username, 64),
			OriginalGroup: locked.Group,
			RestoreGroup:  locked.Group,
			BanGroup:      banGroup,
			Source:        input.Source,
			Status:        CyberGuardBanStatusActive,
			TierIndex:     input.TierIndex,
			BanHours:      max(input.BanHours, 0),
			UnitsSummary:  TruncateCyberGuardText(input.UnitsSummary, 255),
			Reason:        TruncateCyberGuardText(input.Reason, 500),
			StartedAt:     input.Now,
			ExpiresAt:     cyberGuardExpiresAt(input.Now, input.BanHours),
			CreatedBy:     input.CreatedBy,
			EmailState:    input.EmailState,
			CreatedAt:     input.Now,
		}
		if err := tx.Create(ban).Error; err != nil {
			return err
		}
		user = locked
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return ban, user, nil
}

// CyberGuardEndResult describes the group transition an unban performed.
type CyberGuardEndResult struct {
	Ban          *CyberGuardBan
	FromGroup    string
	ToGroup      string
	GroupChanged bool
}

// EndCyberGuardBan ends an active ban. The group is restored only while the
// user is still in the ban group; otherwise the ban ends as admin_override and
// the group is left alone. With onlyIfDrifted the call is a no-op (nil result)
// unless the user already left the ban group.
func EndCyberGuardBan(banId int, actor int, endReason string, note string, now int64, onlyIfDrifted bool) (*CyberGuardEndResult, error) {
	existing, err := GetCyberGuardBan(banId)
	if err != nil {
		return nil, err
	}
	var result *CyberGuardEndResult
	err = DB.Transaction(func(tx *gorm.DB) error {
		user, userExists, err := lockCyberGuardUser(tx, existing.UserId)
		if err != nil {
			return err
		}
		ban, err := lockCyberGuardBan(tx, banId)
		if err != nil {
			return err
		}
		if ban.Status != CyberGuardBanStatusActive {
			return ErrCyberGuardBanStateChanged
		}
		reason := endReason
		res := &CyberGuardEndResult{Ban: ban}
		switch {
		case !userExists:
			reason = CyberGuardEndReasonUserMissing
		case user.Group == ban.BanGroup:
			if onlyIfDrifted {
				return nil
			}
			res.FromGroup = user.Group
			res.ToGroup = cyberGuardRestoreTarget(ban)
			if err := tx.Model(&User{}).Where("id = ?", user.Id).Update("group", res.ToGroup).Error; err != nil {
				return err
			}
			res.GroupChanged = true
		default:
			reason = CyberGuardEndReasonAdminOverride
			res.FromGroup = user.Group
			res.ToGroup = user.Group
		}
		updates := map[string]any{
			"status":     CyberGuardBanStatusEnded,
			"ended_at":   now,
			"ended_by":   actor,
			"end_reason": reason,
			"end_note":   TruncateCyberGuardText(note, 255),
			"updated_at": now,
		}
		if err := tx.Model(&CyberGuardBan{}).Where("id = ?", ban.Id).Updates(updates).Error; err != nil {
			return err
		}
		ban.Status, ban.EndedAt, ban.EndedBy, ban.EndReason = CyberGuardBanStatusEnded, now, actor, reason
		ban.EndNote, ban.UpdatedAt = TruncateCyberGuardText(note, 255), now
		result = res
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ReapplyCyberGuardBanDrift puts a user whose group changed during an active
// ban (subscription purchase/expiry, affiliate upgrade, ...) back into the ban
// group and remembers the new group as the restore target.
func ReapplyCyberGuardBanDrift(banId int, now int64) (*CyberGuardEndResult, error) {
	existing, err := GetCyberGuardBan(banId)
	if err != nil {
		return nil, err
	}
	var result *CyberGuardEndResult
	err = DB.Transaction(func(tx *gorm.DB) error {
		user, userExists, err := lockCyberGuardUser(tx, existing.UserId)
		if err != nil {
			return err
		}
		ban, err := lockCyberGuardBan(tx, banId)
		if err != nil {
			return err
		}
		if ban.Status != CyberGuardBanStatusActive || !userExists || user.Group == ban.BanGroup {
			return nil
		}
		reason := TruncateCyberGuardText(ban.Reason+" | drift reapplied ("+user.Group+")", 500)
		if err := tx.Model(&User{}).Where("id = ?", user.Id).Update("group", ban.BanGroup).Error; err != nil {
			return err
		}
		if err := tx.Model(&CyberGuardBan{}).Where("id = ?", ban.Id).Updates(map[string]any{
			"restore_group": user.Group,
			"reason":        reason,
			"updated_at":    now,
		}).Error; err != nil {
			return err
		}
		ban.RestoreGroup, ban.Reason, ban.UpdatedAt = user.Group, reason, now
		result = &CyberGuardEndResult{Ban: ban, FromGroup: user.Group, ToGroup: ban.BanGroup, GroupChanged: true}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// EscalateCyberGuardBan applies a stricter tier to an active ban: indefinite
// beats timed, and a longer duration (from the original start) extends it.
func EscalateCyberGuardBan(banId int, tierIndex int, banHours int, unitsSummary string, now int64) (*CyberGuardBan, bool, error) {
	var ban *CyberGuardBan
	changed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		locked, err := lockCyberGuardBan(tx, banId)
		if err != nil {
			return err
		}
		if locked.Status != CyberGuardBanStatusActive || locked.ExpiresAt == 0 {
			ban = locked
			return nil
		}
		newExpiry := int64(0)
		if banHours > 0 {
			newExpiry = locked.StartedAt + int64(banHours)*3600
			if newExpiry <= locked.ExpiresAt {
				ban = locked
				return nil
			}
		}
		updates := map[string]any{
			"expires_at":    newExpiry,
			"tier_index":    tierIndex,
			"ban_hours":     max(banHours, 0),
			"units_summary": TruncateCyberGuardText(unitsSummary, 255),
			"updated_at":    now,
		}
		if err := tx.Model(&CyberGuardBan{}).Where("id = ?", locked.Id).Updates(updates).Error; err != nil {
			return err
		}
		locked.ExpiresAt, locked.TierIndex, locked.BanHours = newExpiry, tierIndex, max(banHours, 0)
		locked.UnitsSummary, locked.UpdatedAt = TruncateCyberGuardText(unitsSummary, 255), now
		ban, changed = locked, true
		return nil
	})
	return ban, changed, err
}

// UpsertCyberGuardProposal creates (or refreshes) the observe-mode proposal of
// a user. It never changes the user's group.
func UpsertCyberGuardProposal(input CyberGuardBanInput) (*CyberGuardBan, bool, error) {
	var ban *CyberGuardBan
	created := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		user, ok, err := lockCyberGuardUser(tx, input.UserId)
		if err != nil {
			return err
		}
		if !ok {
			return ErrCyberGuardUserNotFound
		}
		if user.Role >= common.RoleAdminUser || slices.Contains(input.ExemptUserIds, user.Id) {
			return ErrCyberGuardProtectedUser
		}
		if user.Group == strings.TrimSpace(input.BanGroup) {
			return ErrCyberGuardInBanGroup
		}
		active, err := findOpenCyberGuardBan(tx, user.Id, CyberGuardBanStatusActive)
		if err != nil {
			return err
		}
		if active != nil {
			return ErrCyberGuardAlreadyBanned
		}
		open, err := findOpenCyberGuardBan(tx, user.Id, CyberGuardBanStatusObserved)
		if err != nil {
			return err
		}
		if open != nil {
			updates := map[string]any{
				"tier_index":    input.TierIndex,
				"ban_hours":     max(input.BanHours, 0),
				"units_summary": TruncateCyberGuardText(input.UnitsSummary, 255),
				"reason":        TruncateCyberGuardText(input.Reason, 500),
				"updated_at":    input.Now,
			}
			if err := tx.Model(&CyberGuardBan{}).Where("id = ?", open.Id).Updates(updates).Error; err != nil {
				return err
			}
			open.TierIndex, open.BanHours = input.TierIndex, max(input.BanHours, 0)
			open.UnitsSummary, open.Reason = TruncateCyberGuardText(input.UnitsSummary, 255), TruncateCyberGuardText(input.Reason, 500)
			ban = open
			return nil
		}
		ban = &CyberGuardBan{
			UserId:        user.Id,
			Username:      TruncateCyberGuardText(user.Username, 64),
			OriginalGroup: user.Group,
			RestoreGroup:  user.Group,
			BanGroup:      strings.TrimSpace(input.BanGroup),
			Source:        CyberGuardBanSourceAuto,
			Status:        CyberGuardBanStatusObserved,
			TierIndex:     input.TierIndex,
			BanHours:      max(input.BanHours, 0),
			UnitsSummary:  TruncateCyberGuardText(input.UnitsSummary, 255),
			Reason:        TruncateCyberGuardText(input.Reason, 500),
			StartedAt:     input.Now,
			EmailState:    CyberGuardEmailStateNone,
			CreatedAt:     input.Now,
		}
		created = true
		return tx.Create(ban).Error
	})
	if err != nil {
		return nil, false, err
	}
	return ban, created, nil
}

type CyberGuardAdoptInput struct {
	UserId        int
	OriginalGroup string
	Reason        string
	Hours         int
	BanGroup      string
	ExemptUserIds []int
	Actor         int
	Now           int64
}

// AdoptCyberGuardBan records an existing manual ban (user already in the ban
// group) so that unban can restore the given original group. No group change.
func AdoptCyberGuardBan(input CyberGuardAdoptInput) (*CyberGuardBan, error) {
	var ban *CyberGuardBan
	err := DB.Transaction(func(tx *gorm.DB) error {
		user, ok, err := lockCyberGuardUser(tx, input.UserId)
		if err != nil {
			return err
		}
		if !ok {
			return ErrCyberGuardUserNotFound
		}
		if user.Role >= common.RoleAdminUser || slices.Contains(input.ExemptUserIds, user.Id) {
			return ErrCyberGuardProtectedUser
		}
		if user.Group != strings.TrimSpace(input.BanGroup) {
			return ErrCyberGuardNotInBanGroup
		}
		var open int64
		if err := tx.Model(&CyberGuardBan{}).
			Where("user_id = ? AND status IN ?", user.Id, []string{CyberGuardBanStatusActive, CyberGuardBanStatusObserved}).
			Count(&open).Error; err != nil {
			return err
		}
		if open > 0 {
			return ErrCyberGuardOpenRecord
		}
		ban = &CyberGuardBan{
			UserId:        user.Id,
			Username:      TruncateCyberGuardText(user.Username, 64),
			OriginalGroup: input.OriginalGroup,
			RestoreGroup:  input.OriginalGroup,
			BanGroup:      user.Group,
			Source:        CyberGuardBanSourceAdopted,
			Status:        CyberGuardBanStatusActive,
			TierIndex:     -1,
			BanHours:      max(input.Hours, 0),
			Reason:        TruncateCyberGuardText(input.Reason, 500),
			StartedAt:     input.Now,
			ExpiresAt:     cyberGuardExpiresAt(input.Now, input.Hours),
			CreatedBy:     input.Actor,
			EmailState:    CyberGuardEmailStateNone,
			CreatedAt:     input.Now,
		}
		return tx.Create(ban).Error
	})
	if err != nil {
		return nil, err
	}
	return ban, nil
}

// DismissCyberGuardProposal closes an observed proposal; its ended_at acts as
// the counting floor for the user.
func DismissCyberGuardProposal(banId int, actor int, note string, now int64) (*CyberGuardBan, error) {
	var ban *CyberGuardBan
	err := DB.Transaction(func(tx *gorm.DB) error {
		locked, err := lockCyberGuardBan(tx, banId)
		if err != nil {
			return err
		}
		if locked.Status != CyberGuardBanStatusObserved {
			return ErrCyberGuardBanStateChanged
		}
		note = TruncateCyberGuardText(note, 255)
		if err := tx.Model(&CyberGuardBan{}).Where("id = ?", locked.Id).Updates(map[string]any{
			"status":     CyberGuardBanStatusDismissed,
			"ended_at":   now,
			"ended_by":   actor,
			"end_reason": CyberGuardEndReasonDismissed,
			"end_note":   note,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		locked.Status, locked.EndedAt, locked.EndedBy = CyberGuardBanStatusDismissed, now, actor
		locked.EndReason, locked.EndNote, locked.UpdatedAt = CyberGuardEndReasonDismissed, note, now
		ban = locked
		return nil
	})
	return ban, err
}

func UpdateCyberGuardBanEmail(banId int, state string, deliveryKey string, subject string, emailError string) error {
	return DB.Model(&CyberGuardBan{}).Where("id = ?", banId).Updates(map[string]any{
		"email_state":        state,
		"email_delivery_key": TruncateCyberGuardText(deliveryKey, 64),
		"email_subject":      TruncateCyberGuardText(subject, 255),
		"email_error":        TruncateCyberGuardText(emailError, 255),
		"updated_at":         common.GetTimestamp(),
	}).Error
}

// InsertCyberGuardEvent stores a classified ops row once (ops_id is unique).
func InsertCyberGuardEvent(event *CyberGuardEvent) (bool, error) {
	result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(event)
	return result.RowsAffected == 1, result.Error
}

func ListPendingCyberGuardEvents(limit int) ([]*CyberGuardEvent, error) {
	var events []*CyberGuardEvent
	err := DB.Where("state = ?", CyberGuardEventStatePending).Order("occurred_at asc, id asc").Limit(limit).Find(&events).Error
	return events, err
}

// FindCyberGuardAttributionLogs returns new-api log rows (from the log
// database) whose upstream request id matches and that were relayed through a
// protected channel within the given time range.
func FindCyberGuardAttributionLogs(requestIds []string, channelIds []int, from int64, to int64) ([]*Log, error) {
	var logs []*Log
	if len(requestIds) == 0 || len(channelIds) == 0 {
		return logs, nil
	}
	err := LOG_DB.Model(&Log{}).
		Select("id", "user_id", "username", "token_id", "token_name", "group", "channel_id", "created_at", "upstream_request_id", "other").
		Where("upstream_request_id IN ? AND channel_id IN ? AND created_at >= ? AND created_at <= ?", requestIds, channelIds, from, to).
		Find(&logs).Error
	return logs, err
}

// SaveCyberGuardEventResolution stores attribution/unit fields while the event
// is still pending, so a concurrent admin void is never overwritten.
func SaveCyberGuardEventResolution(event *CyberGuardEvent) (bool, error) {
	result := DB.Model(&CyberGuardEvent{}).
		Where("id = ? AND state = ?", event.Id, CyberGuardEventStatePending).
		Updates(map[string]any{
			"user_id":     event.UserId,
			"username":    TruncateCyberGuardText(event.Username, 64),
			"token_id":    event.TokenId,
			"token_name":  TruncateCyberGuardText(event.TokenName, 64),
			"token_group": TruncateCyberGuardText(event.TokenGroup, 64),
			"channel_id":  event.ChannelId,
			"log_id":      event.LogId,
			"session_fp":  event.SessionFp,
			"unit_key":    event.UnitKey,
			"is_new_unit": event.IsNewUnit,
			"state":       event.State,
		})
	return result.RowsAffected == 1, result.Error
}

// FindPreviousCyberGuardNoFpEvent returns the user's latest earlier counted (or
// exempt) event without a session fingerprint for the same token and model.
func FindPreviousCyberGuardNoFpEvent(userId int, tokenId int, modelName string, occurredAt int64, excludeId int) (*CyberGuardEvent, error) {
	var event CyberGuardEvent
	result := DB.Where("user_id = ? AND token_id = ? AND model = ? AND session_fp = '' AND unit_key <> '' AND state IN ? AND occurred_at <= ? AND id <> ?",
		userId, tokenId, modelName, []string{CyberGuardEventStateCounted, CyberGuardEventStateExempt}, occurredAt, excludeId).
		Order("occurred_at desc, id desc").Limit(1).Find(&event)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &event, nil
}

func CyberGuardUnitAlreadyCounted(userId int, unitKey string, excludeId int) (bool, error) {
	var count int64
	err := DB.Model(&CyberGuardEvent{}).
		Where("user_id = ? AND unit_key = ? AND state = ? AND id <> ?", userId, unitKey, CyberGuardEventStateCounted, excludeId).
		Count(&count).Error
	return count > 0, err
}

// CyberGuardCountingFloor is the end time of the user's most recent ended or
// dismissed record (promoted proposals excluded). Units at or before it never
// count again, so an unban does not lead to an instant re-ban.
func CyberGuardCountingFloor(userId int) (int64, error) {
	var floor int64
	err := DB.Model(&CyberGuardBan{}).
		Select("COALESCE(MAX(ended_at), 0)").
		Where("user_id = ? AND status IN ? AND end_reason <> ?", userId,
			[]string{CyberGuardBanStatusEnded, CyberGuardBanStatusDismissed}, CyberGuardEndReasonPromoted).
		Scan(&floor).Error
	return floor, err
}

// CountCyberGuardUnits counts distinct units of counted events after since.
func CountCyberGuardUnits(userId int, since int64) (int64, error) {
	var count int64
	err := DB.Model(&CyberGuardEvent{}).
		Where("user_id = ? AND state = ? AND occurred_at > ?", userId, CyberGuardEventStateCounted, since).
		Distinct("unit_key").Count(&count).Error
	return count, err
}

func VoidCyberGuardEvent(eventId int, actor int, reason string, now int64) (*CyberGuardEvent, error) {
	result := DB.Model(&CyberGuardEvent{}).
		Where("id = ? AND state <> ?", eventId, CyberGuardEventStateVoid).
		Updates(map[string]any{
			"state":       CyberGuardEventStateVoid,
			"void_reason": TruncateCyberGuardText(reason, 255),
			"voided_by":   actor,
			"voided_at":   now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrCyberGuardEventNotFound
	}
	var event CyberGuardEvent
	if err := DB.Where("id = ?", eventId).First(&event).Error; err != nil {
		return nil, err
	}
	return &event, nil
}

func ListDueCyberGuardBans(now int64) ([]*CyberGuardBan, error) {
	var bans []*CyberGuardBan
	err := DB.Where("status = ? AND expires_at > 0 AND expires_at <= ?", CyberGuardBanStatusActive, now).
		Order("expires_at asc, id asc").Find(&bans).Error
	return bans, err
}

func ListActiveCyberGuardBans() ([]*CyberGuardBan, error) {
	var bans []*CyberGuardBan
	err := DB.Where("status = ?", CyberGuardBanStatusActive).Order("id asc").Find(&bans).Error
	return bans, err
}

func HasTimedActiveCyberGuardBans() bool {
	var count int64
	if err := DB.Model(&CyberGuardBan{}).Where("status = ? AND expires_at > 0", CyberGuardBanStatusActive).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

// GetCyberGuardUserGroups maps existing (non-deleted) user ids to their group.
func GetCyberGuardUserGroups(userIds []int) (map[int]string, error) {
	groups := make(map[int]string, len(userIds))
	if len(userIds) == 0 {
		return groups, nil
	}
	var users []User
	if err := DB.Model(&User{}).Select("id", "group").Where("id IN ?", userIds).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		groups[user.Id] = user.Group
	}
	return groups, nil
}

func GetCyberGuardPollState() (CyberGuardPollState, error) {
	var state CyberGuardPollState
	err := DB.Where("id = ?", cyberGuardPollStateId).Limit(1).Find(&state).Error
	state.Id = cyberGuardPollStateId
	return state, err
}

func SaveCyberGuardPollState(state CyberGuardPollState) error {
	state.Id = cyberGuardPollStateId
	state.LastError = TruncateCyberGuardText(state.LastError, 500)
	state.UpdatedAt = common.GetTimestamp()
	return DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&state).Error
}

func ResetCyberGuardPollCursor() error {
	state, err := GetCyberGuardPollState()
	if err != nil {
		return err
	}
	state.CursorEnd = 0
	return SaveCyberGuardPollState(state)
}

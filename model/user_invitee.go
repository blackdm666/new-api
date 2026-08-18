package model

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// UserInvitee is the minimal invitee view exposed to an inviter.
type UserInvitee struct {
	Id          int    `json:"-"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	CreatedAt   int64  `json:"created_at"`
	IsNew       bool   `json:"is_new" gorm:"-"`
}

// AdminAffiliateInviteRecord is the global invitation relationship view used
// by administrators. It includes users who have never generated commission so
// the invitation list is not limited to the commission ledger.
type AdminAffiliateInviteRecord struct {
	InviterId          int    `json:"inviter_id"`
	InviterUsername    string `json:"inviter_username"`
	InviterDisplayName string `json:"inviter_display_name"`
	InviteeId          int    `json:"invitee_id"`
	InviteeUsername    string `json:"invitee_username"`
	InviteeDisplayName string `json:"invitee_display_name"`
	CreatedAt          int64  `json:"created_at"`
	IsNew              bool   `json:"is_new" gorm:"-"`
	TopUpCount         int64  `json:"topup_count"`
	TopUpAmountCents   int64  `json:"topup_amount_cents"`
	CommissionCents    int64  `json:"commission_cents"`
	LastTopUpTime      int64  `json:"last_topup_time"`
}

// CountUserInvitees returns the authoritative number of registrations linked
// to an inviter. It intentionally includes soft-deleted users so the count
// remains historical and matches the invitee list semantics.
func CountUserInvitees(inviterId int) (int64, error) {
	if inviterId <= 0 {
		return 0, errors.New("invalid inviter id")
	}

	var total int64
	err := DB.Unscoped().Model(&User{}).Where("inviter_id = ?", inviterId).Count(&total).Error
	return total, err
}

// GetUserInvitees returns display names and registration times. Deleted
// users remain in the inviter's historical count, matching aff_count semantics.
func GetUserInvitees(inviterId int, pageInfo *common.PageInfo) (invitees []*UserInvitee, total int64, err error) {
	if inviterId <= 0 {
		return nil, 0, errors.New("invalid inviter id")
	}

	if total, err = CountUserInvitees(inviterId); err != nil {
		return nil, 0, err
	}

	query := DB.Unscoped().Model(&User{}).Where("inviter_id = ?", inviterId)
	err = query.
		Select("id", "username", "display_name", "created_at").
		Order("created_at DESC").
		Order("id DESC").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&invitees).Error
	if err != nil {
		return nil, 0, err
	}

	now := time.Now().In(time.Local)
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	startOfTomorrow := startOfToday.AddDate(0, 0, 1)
	for _, invitee := range invitees {
		invitee.IsNew = invitee.CreatedAt >= startOfToday.Unix() && invitee.CreatedAt < startOfTomorrow.Unix()
	}
	return invitees, total, nil
}

// ListAdminAffiliateInviteRecords returns every persisted invitation
// relationship across the site, including soft-deleted invitees and users who
// have not completed a commission-eligible top-up.
func ListAdminAffiliateInviteRecords(keyword string, pageInfo *common.PageInfo) ([]*AdminAffiliateInviteRecord, int64, error) {
	query := DB.Unscoped().Table("users AS invitee").
		Joins("LEFT JOIN users AS inviter ON inviter.id = invitee.inviter_id").
		Where("invitee.inviter_id > 0")

	if keyword = strings.TrimSpace(keyword); keyword != "" {
		pattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		nameCondition := "inviter.username LIKE ? ESCAPE '!' OR inviter.display_name LIKE ? ESCAPE '!' OR inviter.email LIKE ? ESCAPE '!' OR invitee.username LIKE ? ESCAPE '!' OR invitee.display_name LIKE ? ESCAPE '!' OR invitee.email LIKE ? ESCAPE '!'"
		args := []any{pattern, pattern, pattern, pattern, pattern, pattern}
		if userId, err := strconv.Atoi(keyword); err == nil {
			query = query.Where("(invitee.id = ? OR invitee.inviter_id = ? OR "+nameCondition+")", append([]any{userId, userId}, args...)...)
		} else {
			query = query.Where("("+nameCondition+")", args...)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	rows := []*AdminAffiliateInviteRecord{}
	err := query.
		Select("invitee.inviter_id, COALESCE(inviter.username, '') AS inviter_username, COALESCE(inviter.display_name, '') AS inviter_display_name, invitee.id AS invitee_id, invitee.username AS invitee_username, invitee.display_name AS invitee_display_name, invitee.created_at, COUNT(affiliate_commissions.id) AS top_up_count, COALESCE(SUM(affiliate_commissions.top_up_amount_cents), 0) AS top_up_amount_cents, COALESCE(SUM(affiliate_commissions.commission_cents), 0) AS commission_cents, COALESCE(MAX(affiliate_commissions.created_time), 0) AS last_top_up_time").
		Joins("LEFT JOIN affiliate_commissions ON affiliate_commissions.invitee_id = invitee.id AND affiliate_commissions.inviter_id = invitee.inviter_id AND affiliate_commissions.status IN (?, ?)", AffiliateCommissionStatusPending, AffiliateCommissionStatusApproved).
		Group("invitee.inviter_id, inviter.username, inviter.display_name, invitee.id, invitee.username, invitee.display_name, invitee.created_at").
		Order("invitee.created_at DESC, invitee.id DESC").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	now := time.Now().In(time.Local)
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	startOfTomorrow := startOfToday.AddDate(0, 0, 1)
	for _, row := range rows {
		row.IsNew = row.CreatedAt >= startOfToday.Unix() && row.CreatedAt < startOfTomorrow.Unix()
	}
	return rows, total, nil
}

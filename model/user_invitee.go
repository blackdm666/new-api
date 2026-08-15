package model

import (
	"errors"
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

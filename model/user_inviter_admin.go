package model

import (
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// UserInviterOption is the minimal administrator-facing user record needed
// to select an inviter without exposing account credentials or billing data.
type UserInviterOption struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Status      int    `json:"status"`
}

// SearchUserInviterOptions returns non-deleted inviter candidates. The
// currently selected inviter is kept at the top even when it does not match
// the current search text so an edit form can always render its saved value.
func SearchUserInviterOptions(keyword string, excludeUserId int, selectedUserId int, limit int) ([]UserInviterOption, error) {
	if excludeUserId <= 0 {
		return nil, ErrUserInviterInvalid
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	users := make([]UserInviterOption, 0, limit)
	if selectedUserId > 0 && selectedUserId != excludeUserId {
		var selected UserInviterOption
		err := DB.Model(&User{}).
			Select("id", "username", "display_name", "status").
			First(&selected, selectedUserId).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		if err == nil {
			users = append(users, selected)
		}
	}

	query := DB.Model(&User{}).
		Select("id", "username", "display_name", "status").
		Where("id <> ?", excludeUserId)
	if selectedUserId > 0 {
		query = query.Where("id <> ?", selectedUserId)
	}

	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		like := "%" + keyword + "%"
		condition := "username LIKE ? OR email LIKE ? OR display_name LIKE ?"
		args := []any{like, like, like}
		if id, err := strconv.Atoi(keyword); err == nil {
			condition = "id = ? OR " + condition
			args = append([]any{id}, args...)
		}
		query = query.Where("("+condition+")", args...)
	}

	var matches []UserInviterOption
	err := query.Order("id DESC").Limit(limit - len(users)).Find(&matches).Error
	users = append(users, matches...)
	return users, err
}

// UpdateUserInviterWithTx changes an invitation relationship without granting
// registration rewards or rewriting historical commission records. It keeps
// aff_count aligned with the authoritative linked-user count.
func UpdateUserInviterWithTx(tx *gorm.DB, userId int, inviterId int) (int, bool, error) {
	if tx == nil || userId <= 0 || inviterId < 0 {
		return 0, false, ErrUserInviterInvalid
	}

	var user User
	if err := lockForUpdate(tx).Select("id", "inviter_id").First(&user, userId).Error; err != nil {
		return 0, false, err
	}
	previousInviterId := user.InviterId
	if previousInviterId == inviterId {
		return previousInviterId, false, nil
	}
	if inviterId == userId {
		return previousInviterId, false, ErrUserInviterSelf
	}

	if inviterId > 0 {
		var inviter User
		if err := lockForUpdate(tx).Select("id", "inviter_id").First(&inviter, inviterId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return previousInviterId, false, ErrUserInviterNotFound
			}
			return previousInviterId, false, err
		}

		seen := map[int]struct{}{userId: {}, inviter.Id: {}}
		ancestorId := inviter.InviterId
		for ancestorId > 0 {
			if _, exists := seen[ancestorId]; exists {
				return previousInviterId, false, ErrUserInviterCycle
			}
			seen[ancestorId] = struct{}{}

			var ancestor User
			err := lockForUpdate(tx).Unscoped().Select("id", "inviter_id").First(&ancestor, ancestorId).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				break
			}
			if err != nil {
				return previousInviterId, false, err
			}
			ancestorId = ancestor.InviterId
		}
	}

	result := tx.Model(&User{}).Where("id = ?", userId).Update("inviter_id", inviterId)
	if result.Error != nil {
		return previousInviterId, false, result.Error
	}
	if result.RowsAffected != 1 {
		return previousInviterId, false, gorm.ErrRecordNotFound
	}

	for _, affectedInviterId := range distinctPositiveIds(previousInviterId, inviterId) {
		if err := refreshUserAffCountWithTx(tx, affectedInviterId); err != nil {
			return previousInviterId, false, err
		}
	}
	return previousInviterId, true, nil
}

func distinctPositiveIds(ids ...int) []int {
	seen := make(map[int]struct{}, len(ids))
	result := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func refreshUserAffCountWithTx(tx *gorm.DB, inviterId int) error {
	var count int64
	if err := tx.Unscoped().Model(&User{}).Where("inviter_id = ?", inviterId).Count(&count).Error; err != nil {
		return err
	}
	return tx.Unscoped().Model(&User{}).Where("id = ?", inviterId).Update("aff_count", count).Error
}

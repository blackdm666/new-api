package model

import "gorm.io/gorm"

// populateUserInviterRemarksTx enriches an admin user-list page with one
// batched lookup. Remarks are never added to the shared user cache or self DTO.
func populateUserInviterRemarksTx(tx *gorm.DB, users []*User) error {
	ids := make([]int, 0, len(users))
	seen := make(map[int]struct{}, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		user.InviterRemark = ""
		if user.InviterId <= 0 {
			continue
		}
		if _, ok := seen[user.InviterId]; !ok {
			seen[user.InviterId] = struct{}{}
			ids = append(ids, user.InviterId)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var inviters []User
	if err := tx.Unscoped().Select("id", "remark").Where("id IN ?", ids).Find(&inviters).Error; err != nil {
		return err
	}
	remarks := make(map[int]string, len(inviters))
	for _, inviter := range inviters {
		remarks[inviter.Id] = inviter.Remark
	}
	for _, user := range users {
		if user != nil {
			user.InviterRemark = remarks[user.InviterId]
		}
	}
	return nil
}

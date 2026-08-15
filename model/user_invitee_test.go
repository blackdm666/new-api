package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserInviteesScopesIncludesDeletedAndPaginates(t *testing.T) {
	truncateTables(t)
	now := time.Now().In(time.Local)
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	inviter := User{Username: "invite-list-owner", AffCode: "invite-list-owner-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(&inviter).Error)
	otherInviter := User{Username: "invite-list-other-owner", AffCode: "invite-list-other-owner-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(&otherInviter).Error)

	invitees := []*User{
		{Username: "invite-list-enabled", DisplayName: "Enabled Invitee", AffCode: "invite-list-enabled-code", Status: common.UserStatusEnabled, UsedQuota: 100, InviterId: inviter.Id, CreatedAt: startOfToday.Unix() + 1},
		{Username: "invite-list-disabled", DisplayName: "Disabled Invitee", AffCode: "invite-list-disabled-code", Status: common.UserStatusDisabled, UsedQuota: 200, InviterId: inviter.Id, CreatedAt: startOfToday.Unix() - 1},
		{Username: "invite-list-deleted", DisplayName: "Deleted Invitee", AffCode: "invite-list-deleted-code", Status: common.UserStatusEnabled, UsedQuota: 300, InviterId: inviter.Id, CreatedAt: startOfToday.Unix() + 2},
		{Username: "invite-list-unrelated", DisplayName: "Unrelated Invitee", AffCode: "invite-list-unrelated-code", Status: common.UserStatusEnabled, UsedQuota: 400, InviterId: otherInviter.Id, CreatedAt: startOfToday.Unix() + 3},
	}
	require.NoError(t, DB.Create(invitees).Error)
	require.NoError(t, DB.Delete(invitees[2]).Error)

	pageOne, total, err := GetUserInvitees(inviter.Id, &common.PageInfo{Page: 1, PageSize: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, pageOne, 2)
	assert.Equal(t, invitees[2].Id, pageOne[0].Id)
	assert.Equal(t, "Deleted Invitee", pageOne[0].DisplayName)
	assert.True(t, pageOne[0].IsNew)
	assert.Equal(t, invitees[0].Id, pageOne[1].Id)
	assert.Equal(t, "Enabled Invitee", pageOne[1].DisplayName)
	assert.True(t, pageOne[1].IsNew)
	payload, err := common.Marshal(pageOne[0])
	require.NoError(t, err)
	assert.NotContains(t, string(payload), `"id"`)
	assert.Contains(t, string(payload), `"username":"invite-list-deleted"`)
	assert.NotContains(t, string(payload), "used_quota")
	assert.NotContains(t, string(payload), "status")

	pageTwo, total, err := GetUserInvitees(inviter.Id, &common.PageInfo{Page: 2, PageSize: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, pageTwo, 1)
	assert.Equal(t, invitees[1].Id, pageTwo[0].Id)
	assert.False(t, pageTwo[0].IsNew)
}

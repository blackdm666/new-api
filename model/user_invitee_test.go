package model

import (
	"strconv"
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

func TestListAdminAffiliateInviteRecordsIncludesUsersWithoutCommission(t *testing.T) {
	truncateTables(t)

	inviter := User{Username: "global-invite-owner", DisplayName: "Global Owner", Email: "global-owner@example.com", AffCode: "global-invite-owner-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(&inviter).Error)
	otherInviter := User{Username: "other-invite-owner", DisplayName: "Other Owner", Email: "other-owner@example.com", AffCode: "other-invite-owner-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(&otherInviter).Error)

	withCommission := User{Username: "global-invite-paid", DisplayName: "Paid Invitee", Email: "paid-invitee@example.com", AffCode: "global-invite-paid-code", Status: common.UserStatusEnabled, InviterId: inviter.Id, CreatedAt: 1_786_700_100}
	withoutCommission := User{Username: "global-invite-free", DisplayName: "Free Invitee", Email: "free-invitee@example.com", AffCode: "global-invite-free-code", Status: common.UserStatusEnabled, InviterId: inviter.Id, CreatedAt: 1_786_700_200}
	otherInvitee := User{Username: "other-invite-user", DisplayName: "Other Invitee", Email: "other-invitee@example.com", AffCode: "other-invite-user-code", Status: common.UserStatusEnabled, InviterId: otherInviter.Id, CreatedAt: 1_786_700_300}
	require.NoError(t, DB.Create(&withCommission).Error)
	require.NoError(t, DB.Create(&withoutCommission).Error)
	require.NoError(t, DB.Create(&otherInvitee).Error)
	require.NoError(t, DB.Delete(&withoutCommission).Error)

	require.NoError(t, DB.Create(&AffiliateCommission{
		InviterId:        inviter.Id,
		InviteeId:        withCommission.Id,
		TopUpId:          101,
		TradeNo:          "GLOBAL-INVITE-PAID-1",
		TopUpAmountCents: 10_000,
		RateBasisPoints:  500,
		CommissionCents:  500,
		CommissionQuota:  500_000,
		Status:           AffiliateCommissionStatusApproved,
		CreatedTime:      1_786_700_400,
	}).Error)
	require.NoError(t, DB.Create(&AffiliateCommission{
		InviterId:        inviter.Id,
		InviteeId:        withCommission.Id,
		TopUpId:          102,
		TradeNo:          "GLOBAL-INVITE-PAID-2",
		TopUpAmountCents: 20_000,
		RateBasisPoints:  500,
		CommissionCents:  1_000,
		CommissionQuota:  1_000_000,
		Status:           AffiliateCommissionStatusPending,
		CreatedTime:      1_786_700_500,
	}).Error)

	allRows, total, err := ListAdminAffiliateInviteRecords("", &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, allRows, 3)
	adminSummary, err := GetAffiliateAdminSummary()
	require.NoError(t, err)
	assert.Equal(t, int64(3), adminSummary.TotalInviteeCount)

	filteredRows, total, err := ListAdminAffiliateInviteRecords("global-invite-owner", &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, filteredRows, 2)
	assert.Equal(t, withoutCommission.Id, filteredRows[0].InviteeId)
	assert.Zero(t, filteredRows[0].TopUpCount)
	assert.Zero(t, filteredRows[0].TopUpAmountCents)
	assert.Equal(t, withCommission.Id, filteredRows[1].InviteeId)
	assert.Equal(t, int64(2), filteredRows[1].TopUpCount)
	assert.Equal(t, int64(30_000), filteredRows[1].TopUpAmountCents)
	assert.Equal(t, int64(1_500), filteredRows[1].CommissionCents)
	assert.Equal(t, int64(1_786_700_500), filteredRows[1].LastTopUpTime)

	byUid, total, err := ListAdminAffiliateInviteRecords(strconv.Itoa(withoutCommission.Id), &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, byUid, 1)
	assert.Equal(t, withoutCommission.Id, byUid[0].InviteeId)
}

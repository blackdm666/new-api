package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createInviterAdminTestUser(t *testing.T, username string, inviterId int) *User {
	t.Helper()
	user := &User{
		Username:  username,
		Password:  "unused-password-hash",
		Status:    common.UserStatusEnabled,
		Role:      common.RoleCommonUser,
		Group:     "default",
		AffCode:   username + "-aff",
		InviterId: inviterId,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func TestUpdateUserInviterAssignsRelationshipAndRefreshesCount(t *testing.T) {
	setupUserUpdateTestState(t)

	inviter := createInviterAdminTestUser(t, "manual-inviter", 0)
	createInviterAdminTestUser(t, "existing-invitee", inviter.Id)
	target := createInviterAdminTestUser(t, "manual-invitee", 0)
	require.NoError(t, DB.Model(inviter).Update("aff_count", 99).Error)

	var previousInviterId int
	var changed bool
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		previousInviterId, changed, err = UpdateUserInviterWithTx(tx, target.Id, inviter.Id)
		return err
	}))

	assert.Zero(t, previousInviterId)
	assert.True(t, changed)
	require.NoError(t, DB.First(target, target.Id).Error)
	require.NoError(t, DB.First(inviter, inviter.Id).Error)
	assert.Equal(t, inviter.Id, target.InviterId)
	assert.Equal(t, 2, inviter.AffCount)
}

func TestUpdateUserInviterReassignsAndClearsRelationshipCounts(t *testing.T) {
	setupUserUpdateTestState(t)

	oldInviter := createInviterAdminTestUser(t, "old-manual-inviter", 0)
	newInviter := createInviterAdminTestUser(t, "new-manual-inviter", 0)
	target := createInviterAdminTestUser(t, "reassigned-invitee", oldInviter.Id)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		_, _, err := UpdateUserInviterWithTx(tx, target.Id, newInviter.Id)
		return err
	}))
	require.NoError(t, DB.First(oldInviter, oldInviter.Id).Error)
	require.NoError(t, DB.First(newInviter, newInviter.Id).Error)
	assert.Zero(t, oldInviter.AffCount)
	assert.Equal(t, 1, newInviter.AffCount)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		_, _, err := UpdateUserInviterWithTx(tx, target.Id, 0)
		return err
	}))
	require.NoError(t, DB.First(target, target.Id).Error)
	require.NoError(t, DB.First(newInviter, newInviter.Id).Error)
	assert.Zero(t, target.InviterId)
	assert.Zero(t, newInviter.AffCount)
}

func TestUpdateUserInviterRejectsSelfAndInvitationCycles(t *testing.T) {
	setupUserUpdateTestState(t)

	parent := createInviterAdminTestUser(t, "cycle-parent", 0)
	child := createInviterAdminTestUser(t, "cycle-child", parent.Id)

	selfErr := DB.Transaction(func(tx *gorm.DB) error {
		_, _, err := UpdateUserInviterWithTx(tx, parent.Id, parent.Id)
		return err
	})
	require.ErrorIs(t, selfErr, ErrUserInviterSelf)

	cycleErr := DB.Transaction(func(tx *gorm.DB) error {
		_, _, err := UpdateUserInviterWithTx(tx, parent.Id, child.Id)
		return err
	})
	require.ErrorIs(t, cycleErr, ErrUserInviterCycle)

	require.NoError(t, DB.First(parent, parent.Id).Error)
	assert.Zero(t, parent.InviterId)
}

func TestSearchUserInviterOptionsExcludesTargetAndKeepsSelection(t *testing.T) {
	setupUserUpdateTestState(t)

	target := createInviterAdminTestUser(t, "option-target", 0)
	selected := createInviterAdminTestUser(t, "saved-inviter", 0)
	match := createInviterAdminTestUser(t, "matching-inviter", 0)

	options, err := SearchUserInviterOptions("matching", target.Id, selected.Id, 20)
	require.NoError(t, err)
	require.Len(t, options, 2)
	assert.Equal(t, selected.Id, options[0].Id)
	assert.Equal(t, match.Id, options[1].Id)
	for _, option := range options {
		assert.NotEqual(t, target.Id, option.Id)
	}
}

package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInsertWithTxRecordsInviter(t *testing.T) {
	truncateTables(t)

	inviter := User{Username: "oauth-inviter-owner", AffCode: "oauth-inviter-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(&inviter).Error)

	invitee := User{Username: "oauth-inviter-invitee", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return invitee.InsertWithTx(tx, inviter.Id)
	}))

	var stored User
	require.NoError(t, DB.First(&stored, invitee.Id).Error)
	assert.Equal(t, inviter.Id, stored.InviterId)

	plain := User{Username: "oauth-inviter-none", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return plain.InsertWithTx(tx, 0)
	}))
	var storedPlain User
	require.NoError(t, DB.First(&storedPlain, plain.Id).Error)
	assert.Zero(t, storedPlain.InviterId)
}

func TestInsertRecordsInviter(t *testing.T) {
	truncateTables(t)

	inviter := User{Username: "insert-inviter-owner", AffCode: "insert-inviter-code", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(&inviter).Error)

	invitee := User{Username: "insert-inviter-invitee", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, invitee.Insert(inviter.Id))

	var stored User
	require.NoError(t, DB.First(&stored, invitee.Id).Error)
	assert.Equal(t, inviter.Id, stored.InviterId)
}

package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendSMTPTestEmailUsesCurrentAdministratorEmailWhenRecipientIsBlank(t *testing.T) {
	user := &model.User{
		Username: "smtp-admin",
		Password: "unused-password",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Email:    "admin@example.com",
	}
	require.NoError(t, model.DB.Create(user).Error)
	t.Cleanup(func() { model.DB.Delete(user) })

	originalServer := common.SMTPServer
	originalAccount := common.SMTPAccount
	originalFrom := common.SMTPFrom
	originalBackupEnabled := common.SMTPBackupEnabled
	common.SMTPServer = ""
	common.SMTPAccount = ""
	common.SMTPFrom = ""
	common.SMTPBackupEnabled = false
	t.Cleanup(func() {
		common.SMTPServer = originalServer
		common.SMTPAccount = originalAccount
		common.SMTPFrom = originalFrom
		common.SMTPBackupEnabled = originalBackupEnabled
	})

	_, _, err := SendSMTPTestEmail(user.Id, "", common.SMTPChannelPrimary)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrSMTPTestRecipientRequired)
	assert.NotErrorIs(t, err, ErrSMTPTestRecipientInvalid)
	assert.Contains(t, err.Error(), "SMTP server is not configured")
}

func TestSendSMTPTestEmailRejectsInvalidManualRecipient(t *testing.T) {
	_, _, err := SendSMTPTestEmail(0, "first@example.com;second@example.com", common.SMTPChannelPrimary)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSMTPTestRecipientInvalid)
}

func TestSendSMTPTestEmailRejectsInvalidChannel(t *testing.T) {
	_, _, err := SendSMTPTestEmail(0, "admin@example.com", "unknown")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSMTPTestChannelInvalid)
}

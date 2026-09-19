package model

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailSenderAccountEncryptsCredentialsAndRequiresReceipt(t *testing.T) {
	truncateTables(t)
	account := newTestEmailSenderAccount(t, "marketing-secret")

	stored, err := GetEmailSenderAccount(account.Id)
	require.NoError(t, err)
	assert.True(t, stored.CredentialConfigured)
	assert.NotEqual(t, "marketing-secret", stored.TokenEncrypted)
	plain, err := stored.Token()
	require.NoError(t, err)
	assert.Equal(t, "marketing-secret", plain)

	encoded, err := json.Marshal(stored)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "marketing-secret")
	assert.False(t, stored.Enabled)
	assert.Zero(t, stored.ReceiptVerifiedTime)
}

func TestMarketingEmailSenderRejectsGenericUntrackedSMTP(t *testing.T) {
	account := &EmailSenderAccount{
		Name: "Generic SMTP", Profile: EmailSenderProfileMarketing,
		Provider: EmailSenderProviderAliyunEventBridge,
		Server:   "smtp.qq.com", Port: 465,
		Account: "sender@qq.com", From: "sender@qq.com",
		SSLEnabled: true, Weight: 1, RateLimitPerMinute: 20,
	}
	assert.ErrorIs(t, account.Normalize(), ErrEmailSenderAccountInvalid)
}

func TestVerifiedEmailSenderAccountCanBePausedAndResumed(t *testing.T) {
	truncateTables(t)
	account := newTestEmailSenderAccount(t, "marketing-secret")
	now := common.GetTimestamp()
	require.NoError(t, DB.Model(account).Updates(map[string]any{
		"tested_time": now, "receipt_verified_time": now,
		"enabled": true, "health_status": EmailSenderHealthHealthy,
	}).Error)

	require.NoError(t, SetEmailSenderAccountEnabled(account.Id, false))
	require.NoError(t, DB.First(account, account.Id).Error)
	assert.False(t, account.Enabled)
	assert.Equal(t, EmailSenderHealthDisabled, account.HealthStatus)
	require.NoError(t, SetEmailSenderAccountEnabled(account.Id, true))
	require.NoError(t, DB.First(account, account.Id).Error)
	assert.True(t, account.Enabled)
	assert.Equal(t, EmailSenderHealthHealthy, account.HealthStatus)
}

func TestEmailReceiptEndpointTokenRotationDisablesOldConfiguration(t *testing.T) {
	truncateTables(t)
	first, err := RotateEmailReceiptEndpointToken()
	require.NoError(t, err)
	require.NoError(t, UpdateEmailReceiptEndpointEnabled(true))
	assert.True(t, VerifyEmailReceiptEndpointToken(first))

	second, err := RotateEmailReceiptEndpointToken()
	require.NoError(t, err)
	assert.False(t, VerifyEmailReceiptEndpointToken(first))
	assert.False(t, VerifyEmailReceiptEndpointToken(second))
	require.NoError(t, UpdateEmailReceiptEndpointEnabled(true))
	assert.True(t, VerifyEmailReceiptEndpointToken(second))
}

func TestAwaitingReceiptIsNotRequeuedAndTimeoutScrubsContent(t *testing.T) {
	truncateTables(t)
	account := newTestEmailSenderAccount(t, "marketing-secret")
	delivery, _, err := EnqueueEmailDelivery(&EmailDelivery{
		DeliveryKey: "receipt-timeout", Category: "marketing_custom",
		Recipient: "user@qq.com", Subject: "sensitive subject", Body: "sensitive body",
		Priority: EmailPriorityMarketing,
	})
	require.NoError(t, err)
	attempt, err := CreateEmailDeliveryAttempt(delivery.Id, account, EmailAttemptPurposeDelivery, delivery.Recipient, "<receipt-timeout@example.com>", "notify-timeout@example.com")
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, MarkEmailDeliveryAttemptAccepted(attempt.Id, now-60))
	require.NoError(t, MarkEmailDeliveryAwaitingReceipt(delivery.Id, account.Id, attempt.Id, attempt.MessageId, now-60, now-1))

	due, err := ListDueEmailDeliveries(10, now)
	require.NoError(t, err)
	assert.Empty(t, due)
	rows, total, err := ListEmailDeliveries(
		EmailDeliveryQueryOptions{Status: EmailDeliveryStatusAwaitingReceipt},
		&common.PageInfo{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	assert.Equal(t, account.Name, rows[0].SenderAccountName)
	require.NoError(t, ExpireAwaitingEmailReceipts(now))

	require.NoError(t, DB.First(delivery, delivery.Id).Error)
	require.NoError(t, DB.First(attempt, attempt.Id).Error)
	assert.Equal(t, EmailDeliveryStatusFailed, delivery.State)
	assert.Equal(t, "receipt_timeout", delivery.FailureType)
	assert.Empty(t, delivery.Subject)
	assert.Empty(t, delivery.Body)
	assert.Equal(t, EmailAttemptStatusFailed, attempt.Status)
}

func newTestEmailSenderAccount(t *testing.T, token string) *EmailSenderAccount {
	t.Helper()
	account := &EmailSenderAccount{
		Name: "Alibaba marketing", Profile: EmailSenderProfileMarketing,
		Provider: EmailSenderProviderAliyunEventBridge,
		Server:   "smtpdm.aliyun.com", Port: 465,
		Account: "marketing@example.com", From: "marketing@example.com",
		SSLEnabled: true, Weight: 1, RateLimitPerMinute: 20,
	}
	require.NoError(t, account.SetToken(token))
	require.NoError(t, CreateEmailSenderAccount(account))
	return account
}

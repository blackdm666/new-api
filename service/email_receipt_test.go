package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAliyunEventBridgeSuccessEnablesAccountAndIsIdempotent(t *testing.T) {
	truncate(t)
	account := newReceiptTestAccount(t)
	attempt, err := model.CreateEmailDeliveryAttempt(0, account, model.EmailAttemptPurposeAccountTest, "recipient@qq.com", "<success@example.com>", "notify-success@example.com")
	require.NoError(t, err)
	require.NoError(t, model.MarkEmailDeliveryAttemptAccepted(attempt.Id, common.GetTimestamp()))
	payload := []byte(`{
		"id":"event-success-1","source":"acs:dm","type":"dm:Deliver:Succeed",
		"data":{"header":{"X-Notify-Message-ID":"notify-success@example.com"},"from":"marketing@example.com","rcpt":"recipient@qq.com","msg_id":"success@example.com","status":"0","event":"dm:Deliver:Succeed","err_code":"250","err_msg":"250 Send Mail OK","failed_type":"SendOk"}
	}`)

	require.NoError(t, ProcessAliyunEventBridgeReceipt(payload))
	require.NoError(t, ProcessAliyunEventBridgeReceipt(payload))
	require.NoError(t, model.DB.First(account, account.Id).Error)
	require.NoError(t, model.DB.First(attempt, attempt.Id).Error)
	assert.True(t, account.Enabled)
	assert.Positive(t, account.ReceiptVerifiedTime)
	assert.Equal(t, model.EmailAttemptStatusDelivered, attempt.Status)
	var events int64
	require.NoError(t, model.DB.Model(&model.EmailReceiptEvent{}).Count(&events).Error)
	assert.EqualValues(t, 1, events)
}

func TestMarketingDeliveryRequiresEnabledReceiptEndpoint(t *testing.T) {
	truncate(t)
	err := SendMarketingEmailDelivery(&model.EmailDelivery{Id: 1})
	assert.ErrorIs(t, err, ErrEmailReceiptEndpointDisabled)
	_, err = model.RotateEmailReceiptEndpointToken()
	require.NoError(t, err)
	require.NoError(t, model.UpdateEmailReceiptEndpointEnabled(true))
	err = SendMarketingEmailDelivery(&model.EmailDelivery{Id: 1})
	assert.ErrorIs(t, err, ErrEmailReceiptEndpointDisabled)
}

func TestMarketingDeliveryDefersAtAccountMinuteLimit(t *testing.T) {
	truncate(t)
	_, err := model.RotateEmailReceiptEndpointToken()
	require.NoError(t, err)
	require.NoError(t, model.UpdateEmailReceiptEndpointEnabled(true))
	require.NoError(t, model.UpdateEmailReceiptEndpointActivity(true, ""))
	account := newReceiptTestAccount(t)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Model(account).Updates(map[string]any{
		"enabled": true, "tested_time": now, "receipt_verified_time": now,
		"health_status": model.EmailSenderHealthHealthy, "rate_limit_per_minute": 1,
	}).Error)
	delivery, _, err := model.EnqueueEmailDelivery(&model.EmailDelivery{
		DeliveryKey: "account-minute-limit", Category: "marketing_custom",
		Recipient: "recipient@qq.com", Subject: "subject", Body: "body",
		Priority: model.EmailPriorityMarketing,
	})
	require.NoError(t, err)
	reserved, err := model.ReserveEmailDeliveryMinuteQuota(
		fmt.Sprintf("marketing-account-%d", account.Id),
		now/60*60,
		1,
	)
	require.NoError(t, err)
	assert.True(t, reserved)

	require.NoError(t, SendMarketingEmailDelivery(delivery))
	require.NoError(t, model.DB.First(delivery, delivery.Id).Error)
	assert.Equal(t, model.EmailDeliveryStatusQueued, delivery.State)
	assert.Greater(t, delivery.NextAttemptTime, now)
}

func TestAliyunUnmatchedReceiptIsReprocessedAfterAttemptAppears(t *testing.T) {
	truncate(t)
	account := newReceiptTestAccount(t)
	payload := []byte(`{
		"id":"event-late-attempt","source":"acs:dm","type":"dm:Deliver:Succeed",
		"data":{"header":{"X-Notify-Message-ID":"notify-late@example.com"},"from":"marketing@example.com","rcpt":"recipient@qq.com","msg_id":"late@example.com","status":"0","event":"dm:Deliver:Succeed","err_code":"250","err_msg":"250 Send Mail OK","failed_type":"SendOk"}
	}`)
	require.NoError(t, ProcessAliyunEventBridgeReceipt(payload))

	attempt, err := model.CreateEmailDeliveryAttempt(0, account, model.EmailAttemptPurposeAccountTest, "recipient@qq.com", "<late@example.com>", "notify-late@example.com")
	require.NoError(t, err)
	require.NoError(t, model.MarkEmailDeliveryAttemptAccepted(attempt.Id, common.GetTimestamp()))
	require.NoError(t, ProcessAliyunEventBridgeReceipt(payload))
	require.NoError(t, model.DB.First(account, account.Id).Error)
	assert.True(t, account.Enabled)
}

func TestAliyunDomainThrottleRequeuesForAnotherSenderDomain(t *testing.T) {
	truncate(t)
	account := newReceiptTestAccount(t)
	delivery, _, err := model.EnqueueEmailDelivery(&model.EmailDelivery{
		DeliveryKey: "domain-throttle", Category: "marketing_custom",
		Recipient: "recipient@qq.com", Subject: "subject", Body: "body",
		Priority: model.EmailPriorityMarketing, MarketingQuotaTime: common.GetTimestamp(),
	})
	require.NoError(t, err)
	attempt, err := model.CreateEmailDeliveryAttempt(delivery.Id, account, model.EmailAttemptPurposeDelivery, delivery.Recipient, "<throttle@example.com>", "notify-throttle@example.com")
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.MarkEmailDeliveryAttemptAccepted(attempt.Id, now))
	require.NoError(t, model.MarkEmailDeliveryAwaitingReceipt(delivery.Id, account.Id, attempt.Id, attempt.MessageId, now, now+3600))
	payload := []byte(`{
		"id":"event-throttle-1","source":"acs:dm","type":"dm:Deliver:Fail",
		"data":{"header":{"X-Notify-Message-ID":"notify-throttle@example.com"},"from":"marketing@example.com","rcpt":"recipient@qq.com","msg_id":"throttle@example.com","status":"4","event":"dm:Deliver:Fail","err_code":"550","err_msg":"550 Domain frequency limited","failed_type":"SmtpMfdFreq"}
	}`)

	require.NoError(t, ProcessAliyunEventBridgeReceipt(payload))
	require.NoError(t, model.DB.First(delivery, delivery.Id).Error)
	assert.Equal(t, model.EmailDeliveryStatusRetrying, delivery.State)
	assert.Zero(t, delivery.MarketingQuotaTime)
	assert.Greater(t, delivery.NextAttemptTime, now)
	assert.LessOrEqual(t, delivery.NextAttemptTime, now+int64(setting.GetEmailDeliveryRules().EmailRetryInitialSeconds)+1)
	throttle, err := model.ActiveEmailDeliveryThrottle("domain:example.com:qq.com", now)
	require.NoError(t, err)
	assert.Equal(t, "domain", throttle.ScopeType)
	assert.Equal(t, 10, throttle.EffectiveRPM)
	alternate := &model.EmailSenderAccount{
		Name: "Alternate sender", Profile: model.EmailSenderProfileMarketing,
		Provider: model.EmailSenderProviderAliyunEventBridge,
		Server:   "smtpdm.aliyun.com", Port: 465,
		Account: "marketing@alternate.com", From: "marketing@alternate.com",
		SSLEnabled: true, Weight: 1, RateLimitPerMinute: 20,
	}
	require.NoError(t, alternate.SetToken("secret"))
	require.NoError(t, model.CreateEmailSenderAccount(alternate))
	eligible, err := eligibleMarketingAccounts(
		[]*model.EmailSenderAccount{account, alternate},
		delivery.Recipient,
	)
	require.NoError(t, err)
	require.Len(t, eligible, 1)
	assert.Equal(t, alternate.Id, eligible[0].Id)

	require.NoError(t, recoverAliyunEmailThrottles(attempt, now+1))
	_, err = model.GetEmailDeliveryThrottle("domain:example.com:qq.com")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestAliyunFeedbackSupportsDotSourceAndLegacyRecipientField(t *testing.T) {
	truncate(t)
	unsubscribe := []byte(`{
		"id":"event-unsubscribe-1","source":"acs.dm","type":"dm:Feedback:UnSubscribe",
		"data":{"from":"marketing@example.com","rcpt":"recipient@qq.com","envid":"60001"}
	}`)
	require.NoError(t, ProcessAliyunEventBridgeReceipt(unsubscribe))
	assert.True(t, model.IsMarketingSuppressed(0, "recipient@qq.com"))

	subscribe := []byte(`{
		"id":"event-subscribe-1","source":"acs.dm","type":"dm:Feedback:Subscribe",
		"data":{"from":"marketing@example.com","rcpt":"recipient@qq.com","envid":"60001"}
	}`)
	require.NoError(t, ProcessAliyunEventBridgeReceipt(subscribe))
	assert.False(t, model.IsMarketingSuppressed(0, "recipient@qq.com"))

	fbl := []byte(fmt.Sprintf(`{
		"id":"event-fbl-1","source":"acs.dm","type":"dm:Feedback:FblReport",
		"data":{"send_email":"marketing@example.com","block_email":"%s","message_id":"<fbl@example.com>"}
	}`, "complaint@qq.com"))
	require.NoError(t, ProcessAliyunEventBridgeReceipt(fbl))
	assert.True(t, model.IsMarketingSuppressed(0, "complaint@qq.com"))
}

func TestAliyunTransientReceiptStopsAtConfiguredAttemptLimit(t *testing.T) {
	truncate(t)
	originalRules := setting.GetEmailDeliveryRules()
	t.Cleanup(func() {
		raw, err := common.Marshal(originalRules)
		require.NoError(t, err)
		require.NoError(t, setting.UpdateEmailDeliveryRulesByJSONString(string(raw)))
	})
	rules := originalRules
	rules.EmailMaxAttempts = 1
	raw, err := common.Marshal(rules)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateEmailDeliveryRulesByJSONString(string(raw)))

	account := newReceiptTestAccount(t)
	delivery, _, err := model.EnqueueEmailDelivery(&model.EmailDelivery{
		DeliveryKey: "attempt-limit", Category: "marketing_custom",
		Recipient: "recipient@qq.com", Subject: "subject", Body: "body",
		Priority: model.EmailPriorityMarketing,
	})
	require.NoError(t, err)
	attempt, err := model.CreateEmailDeliveryAttempt(delivery.Id, account, model.EmailAttemptPurposeDelivery, delivery.Recipient, "<attempt-limit@example.com>", "notify-attempt-limit@example.com")
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.MarkEmailDeliveryAttemptAccepted(attempt.Id, now))
	require.NoError(t, model.MarkEmailDeliveryAwaitingReceipt(delivery.Id, account.Id, attempt.Id, attempt.MessageId, now, now+3600))
	payload := []byte(`{
		"id":"event-attempt-limit","source":"acs:dm","type":"dm:Deliver:Fail",
		"data":{"header":{"X-Notify-Message-ID":"notify-attempt-limit@example.com"},"from":"marketing@example.com","rcpt":"recipient@qq.com","msg_id":"attempt-limit@example.com","status":"4","event":"dm:Deliver:Fail","err_code":"451","err_msg":"temporary error","failed_type":"UnkSmtpError"}
	}`)

	require.NoError(t, ProcessAliyunEventBridgeReceipt(payload))
	require.NoError(t, model.DB.First(delivery, delivery.Id).Error)
	assert.Equal(t, model.EmailDeliveryStatusFailed, delivery.State)
	assert.Positive(t, delivery.DeadLetterTime)
}

func TestAliyunAuthenticationFailureDisablesVerifiedAccount(t *testing.T) {
	truncate(t)
	account := newReceiptTestAccount(t)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Model(account).Updates(map[string]any{
		"enabled": true, "tested_time": now, "receipt_verified_time": now,
		"health_status": model.EmailSenderHealthHealthy,
	}).Error)
	attempt, err := model.CreateEmailDeliveryAttempt(0, account, model.EmailAttemptPurposeDelivery, "recipient@qq.com", "<auth-fail@example.com>", "notify-auth-fail@example.com")
	require.NoError(t, err)
	require.NoError(t, model.MarkEmailDeliveryAttemptAccepted(attempt.Id, now))
	payload := []byte(`{
		"id":"event-auth-fail","source":"acs:dm","type":"dm:Deliver:Fail",
		"data":{"header":{"X-Notify-Message-ID":"notify-auth-fail@example.com"},"from":"marketing@example.com","rcpt":"recipient@qq.com","msg_id":"auth-fail@example.com","status":"4","event":"dm:Deliver:Fail","err_code":"550","err_msg":"authentication failed","failed_type":"SmtpAuthFail"}
	}`)

	require.NoError(t, ProcessAliyunEventBridgeReceipt(payload))
	require.NoError(t, model.DB.First(account, account.Id).Error)
	assert.False(t, account.Enabled)
	assert.Equal(t, model.EmailSenderHealthDegraded, account.HealthStatus)
	assert.Error(t, model.SetEmailSenderAccountEnabled(account.Id, true))
}

func newReceiptTestAccount(t *testing.T) *model.EmailSenderAccount {
	t.Helper()
	account := &model.EmailSenderAccount{
		Name: "Alibaba marketing", Profile: model.EmailSenderProfileMarketing,
		Provider: model.EmailSenderProviderAliyunEventBridge,
		Server:   "smtpdm.aliyun.com", Port: 465,
		Account: "marketing@example.com", From: "marketing@example.com",
		SSLEnabled: true, Weight: 1, RateLimitPerMinute: 20,
	}
	require.NoError(t, account.SetToken("secret"))
	require.NoError(t, model.CreateEmailSenderAccount(account))
	return account
}

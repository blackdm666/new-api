package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
)

type AliyunDirectMailEvent struct {
	Data   AliyunDirectMailEventData `json:"data"`
	Id     string                    `json:"id"`
	Source string                    `json:"source"`
	Type   string                    `json:"type"`
	Time   string                    `json:"time"`
}

type AliyunDirectMailEventData struct {
	Header struct {
		NotifyMessageId string `json:"X-Notify-Message-ID"`
	} `json:"header"`
	EnvId        string `json:"env_id"`
	LegacyEnvId  string `json:"envid"`
	Account      string `json:"account"`
	From         string `json:"from"`
	SendEmail    string `json:"send_email"`
	Recipient    string `json:"rcpt"`
	BlockedEmail string `json:"block_email"`
	MessageId    string `json:"msg_id"`
	LegacyMsgId  string `json:"message_id"`
	OutboundIP   string `json:"outbound_ip"`
	Status       string `json:"status"`
	Event        string `json:"event"`
	ErrorCode    string `json:"err_code"`
	ErrorMessage string `json:"err_msg"`
	FailureType  string `json:"failed_type"`
	ESP          string `json:"esp"`
	DeliverTime  string `json:"deliver_time"`
}

func ProcessAliyunEventBridgeReceipt(body []byte) error {
	eventPayload := AliyunDirectMailEvent{}
	if err := common.Unmarshal(body, &eventPayload); err != nil {
		return fmt.Errorf("decode EventBridge email receipt: %w", err)
	}
	if !validAliyunDirectMailSource(eventPayload.Source) || eventPayload.Id == "" || eventPayload.Type == "" {
		return errors.New("invalid EventBridge email receipt")
	}
	if eventPayload.Data.Event != "" && eventPayload.Data.Event != eventPayload.Type {
		return errors.New("EventBridge email receipt type does not match its data")
	}
	senderAddress := firstNonEmpty(eventPayload.Data.From, eventPayload.Data.SendEmail, eventPayload.Data.Account)
	recipientAddress := firstNonEmpty(eventPayload.Data.Recipient, eventPayload.Data.BlockedEmail)
	messageID := firstNonEmpty(eventPayload.Data.MessageId, eventPayload.Data.LegacyMsgId)
	environmentID := firstNonEmpty(eventPayload.Data.EnvId, eventPayload.Data.LegacyEnvId)
	errorMessage := receiptErrorMessage(eventPayload.Data)
	hash := sha256.Sum256(body)
	receipt := &model.EmailReceiptEvent{
		Provider: model.EmailReceiptProviderAliyunEventBridge, EventId: eventPayload.Id,
		EventType: eventPayload.Type, MessageId: messageID,
		NotifyMessageId: eventPayload.Data.Header.NotifyMessageId,
		SenderAddress:   senderAddress,
		RecipientMasked: maskReceiptAddress(recipientAddress),
		RecipientDomain: receiptAddressDomain(recipientAddress),
		Status:          eventPayload.Data.Status, ErrorCode: eventPayload.Data.ErrorCode,
		ErrorMessage: errorMessage,
		FailureType:  eventPayload.Data.FailureType, ProviderEnvId: environmentID,
		ESP: eventPayload.Data.ESP, OutboundIP: eventPayload.Data.OutboundIP,
		PayloadHash: hex.EncodeToString(hash[:]),
	}
	created, err := model.StoreEmailReceiptEvent(receipt)
	if err != nil {
		return err
	}
	if !created && receipt.ProcessedTime > 0 {
		return nil
	}
	if eventPayload.Type == "dm:Feedback:FblReport" || eventPayload.Type == "dm:Feedback:UnSubscribe" {
		if recipientAddress != "" {
			if err := model.CreateMarketingSuppression(0, recipientAddress, eventPayload.Type, 0); err != nil {
				_ = model.MarkEmailReceiptEventUnmatched(receipt.Id, err.Error())
				return err
			}
		}
		return markEmailReceiptProcessed(receipt)
	}
	if eventPayload.Type == "dm:Feedback:Subscribe" {
		if recipientAddress != "" {
			if err := model.DeleteMarketingSuppressionByEmailAndReasons(recipientAddress, []string{"dm:Feedback:UnSubscribe", "SysOutRecipientUnsubscribed"}); err != nil {
				_ = model.MarkEmailReceiptEventUnmatched(receipt.Id, err.Error())
				return err
			}
		}
		return markEmailReceiptProcessed(receipt)
	}
	if eventPayload.Type != "dm:Deliver:Succeed" && eventPayload.Type != "dm:Deliver:Fail" {
		return markEmailReceiptProcessed(receipt)
	}

	attempt, err := model.FindEmailDeliveryAttempt(messageID, eventPayload.Data.Header.NotifyMessageId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = model.MarkEmailReceiptEventUnmatched(receipt.Id, "matching delivery attempt is not available yet")
			return nil
		}
		return err
	}
	if !strings.EqualFold(senderAddress, strings.TrimSpace(attempt.SenderAddress)) {
		_ = model.MarkEmailReceiptEventUnmatched(receipt.Id, "sender address does not match delivery attempt")
		return nil
	}
	receipt.AttemptId = attempt.Id
	if attempt.FinalizedTime > 0 {
		return markEmailReceiptProcessed(receipt)
	}

	now := common.GetTimestamp()
	if eventPayload.Type == "dm:Deliver:Succeed" && (eventPayload.Data.ErrorCode == "250" || eventPayload.Data.Status == "0") {
		if err := recoverAliyunEmailThrottles(attempt, now); err != nil {
			common.SysError("failed to recover email delivery throttle: " + err.Error())
		}
		return model.ApplySuccessfulEmailReceipt(receipt, attempt, now)
	}

	permanent := permanentAliyunEmailFailure(eventPayload.Data.FailureType)
	terminal := permanent || attempt.Sequence >= setting.GetEmailDeliveryRules().EmailMaxAttempts
	nextAttempt := emailReceiptRetryTime(now, eventPayload.Data.FailureType, attempt.Sequence)
	throttleUntil := emailReceiptThrottleUntil(now, eventPayload.Data.FailureType, attempt.Sequence)
	if err := recordAliyunEmailThrottle(attempt, eventPayload.Data.FailureType, throttleUntil); err != nil {
		return err
	}
	if accountFatalAliyunEmailFailure(eventPayload.Data.FailureType) {
		if err := model.DisableEmailSenderAccountForFailure(attempt.SenderAccountId, errorMessage); err != nil {
			return err
		}
	}
	if err := model.ApplyFailedEmailReceipt(receipt, attempt, now, nextAttempt, terminal); err != nil {
		return err
	}
	if attempt.DeliveryId > 0 {
		delivery, getErr := model.GetEmailDeliveryById(attempt.DeliveryId)
		if getErr == nil {
			if permanent && suppressAliyunRecipient(eventPayload.Data.FailureType) && delivery.Recipient != "" {
				_ = model.CreateMarketingSuppression(delivery.UserId, delivery.Recipient, eventPayload.Data.FailureType, 0)
			}
			if !terminal && delivery.ExpiresTime > 0 && nextAttempt >= delivery.ExpiresTime {
				_ = model.ExpireEmailDelivery(delivery.Id, "retry window exceeds delivery expiry")
			}
		}
	}
	return nil
}

func validAliyunDirectMailSource(source string) bool {
	source = strings.TrimSpace(source)
	return source == "acs:dm" || source == "acs.dm"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func receiptErrorMessage(data AliyunDirectMailEventData) string {
	if message := truncateReceiptText(data.ErrorMessage, 2000); message != "" {
		return message
	}
	parts := make([]string, 0, 2)
	if data.ErrorCode != "" {
		parts = append(parts, strings.TrimSpace(data.ErrorCode))
	}
	if data.FailureType != "" {
		parts = append(parts, strings.TrimSpace(data.FailureType))
	}
	if len(parts) == 0 {
		return "Alibaba Cloud reported a delivery failure"
	}
	return strings.Join(parts, " ")
}

func markEmailReceiptProcessed(receipt *model.EmailReceiptEvent) error {
	updates := map[string]any{
		"processed_time": common.GetTimestamp(), "processing_error": "",
	}
	if receipt.AttemptId > 0 {
		updates["attempt_id"] = receipt.AttemptId
	}
	return model.DB.Model(&model.EmailReceiptEvent{}).Where("id = ?", receipt.Id).Updates(updates).Error
}

func permanentAliyunEmailFailure(failureType string) bool {
	switch strings.TrimSpace(failureType) {
	case "SysOutDnsResolveFail", "SmtpContSpam", "SmtpAuthFail", "SmtpNxBox", "SysOutInvRcpt",
		"SmtpZPermErr", "SmtpMiscSpam", "SmtpMfBad", "SmtpSpfFail", "SmtpDmaFail",
		"SysOutRecipientReportedSpam", "SysOutHoneypot", "SysOutRecipientUnsubscribed",
		"SysIncomingInvRcpt", "SysOutRcptOnAccountLevelBounceList", "SmtpDbl":
		return true
	default:
		return false
	}
}

func suppressAliyunRecipient(failureType string) bool {
	switch strings.TrimSpace(failureType) {
	case "SmtpNxBox", "SysOutInvRcpt", "SysOutRecipientReportedSpam", "SysOutHoneypot",
		"SysOutRecipientUnsubscribed", "SysIncomingInvRcpt", "SysOutRcptOnAccountLevelBounceList":
		return true
	default:
		return false
	}
}

func accountFatalAliyunEmailFailure(failureType string) bool {
	switch strings.TrimSpace(failureType) {
	case "SmtpAuthFail", "SmtpSpfFail", "SmtpDmaFail", "SmtpMfBad", "SmtpDbl":
		return true
	default:
		return false
	}
}

func emailReceiptRetryTime(now int64, failureType string, sequence int) int64 {
	if switchableAliyunEmailThrottle(failureType) {
		return now + int64(setting.GetEmailDeliveryRules().EmailRetryInitialSeconds)
	}
	if failureType == "SmtpRcptFreq" {
		return now + 24*3600
	}
	rules := setting.GetEmailDeliveryRules()
	delay := int64(rules.EmailRetryInitialSeconds)
	for attempt := 1; attempt < sequence && delay < int64(rules.EmailRetryMaxSeconds); attempt++ {
		delay *= 2
	}
	if delay > int64(rules.EmailRetryMaxSeconds) {
		delay = int64(rules.EmailRetryMaxSeconds)
	}
	return now + delay
}

func emailReceiptThrottleUntil(now int64, failureType string, sequence int) int64 {
	if failureType == "SmtpMfLimit" {
		location, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			location = time.FixedZone("Asia/Shanghai", 8*60*60)
		}
		local := time.Unix(now, 0).In(location).Add(24 * time.Hour)
		return time.Date(local.Year(), local.Month(), local.Day(), 9, 0, 0, 0, location).Unix()
	}
	if sequence <= 1 {
		return now + 30*60
	}
	if sequence == 2 {
		return now + 2*3600
	}
	return now + 24*3600
}

func switchableAliyunEmailThrottle(failureType string) bool {
	switch failureType {
	case "SmtpMfFreq", "SmtpMfdFreq", "SmtpMfLimit", "SmtpIPFreq",
		"SysOutSocksConnError", "SysOutSocksError", "SysOutConnError", "SysOutConnTooMuch":
		return true
	default:
		return false
	}
}

func recordAliyunEmailThrottle(attempt *model.EmailDeliveryAttempt, failureType string, disabledUntil int64) error {
	account, err := model.GetEmailSenderAccount(attempt.SenderAccountId)
	if err != nil {
		return err
	}
	scopeType := ""
	scopeKey := ""
	switch failureType {
	case "SmtpMfFreq":
		scopeType = "account"
		scopeKey = fmt.Sprintf("account:%d:%s", attempt.SenderAccountId, attempt.RecipientDomain)
	case "SmtpMfdFreq", "SmtpMfLimit":
		scopeType = "domain"
		scopeKey = fmt.Sprintf("domain:%s:%s", attempt.SenderDomain, attempt.RecipientDomain)
	case "SmtpIPFreq", "SysOutSocksConnError", "SysOutSocksError", "SysOutConnError", "SysOutConnTooMuch":
		scopeType = "provider"
		scopeKey = fmt.Sprintf("provider:%s:%s", strings.ToLower(account.Server), attempt.RecipientDomain)
	default:
		return nil
	}
	existing, err := model.GetEmailDeliveryThrottle(scopeKey)
	strikes := 1
	baseRPM := account.RateLimitPerMinute
	if baseRPM < 1 {
		baseRPM = 1
	}
	if err == nil {
		strikes = existing.StrikeCount + 1
		if existing.EffectiveRPM > 0 {
			baseRPM = existing.EffectiveRPM
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	effectiveRPM := baseRPM
	if effectiveRPM > 1 {
		effectiveRPM /= 2
	}
	throttle := &model.EmailDeliveryThrottle{
		ScopeKey: scopeKey, ScopeType: scopeType, SenderAccountId: attempt.SenderAccountId,
		SenderDomain: attempt.SenderDomain, RecipientDomain: attempt.RecipientDomain,
		EffectiveRPM: effectiveRPM, StrikeCount: strikes, DisabledUntil: disabledUntil,
		LastFailureTime: common.GetTimestamp(), LastFailureType: failureType,
	}
	if err := model.UpsertEmailDeliveryThrottle(throttle); err != nil {
		return err
	}
	if scopeType == "account" || scopeType == "provider" {
		return model.DegradeEmailSenderAccount(attempt.SenderAccountId, disabledUntil, failureType)
	}
	return nil
}

func recoverAliyunEmailThrottles(attempt *model.EmailDeliveryAttempt, recoveredTime int64) error {
	account, err := model.GetEmailSenderAccount(attempt.SenderAccountId)
	if err != nil {
		return err
	}
	keys := []string{
		fmt.Sprintf("account:%d:%s", account.Id, attempt.RecipientDomain),
		fmt.Sprintf("domain:%s:%s", attempt.SenderDomain, attempt.RecipientDomain),
		fmt.Sprintf("provider:%s:%s", strings.ToLower(account.Server), attempt.RecipientDomain),
	}
	for _, key := range keys {
		if err := model.RecoverEmailDeliveryThrottle(key, account.RateLimitPerMinute, recoveredTime); err != nil {
			return err
		}
	}
	return nil
}

func maskReceiptAddress(address string) string {
	address = strings.TrimSpace(address)
	at := strings.LastIndex(address, "@")
	if at <= 0 || at == len(address)-1 {
		return "***"
	}
	local := []rune(address[:at])
	visible := string(local[:1])
	if len(local) > 2 {
		visible += string(local[len(local)-1:])
	}
	return visible + "***@" + address[at+1:]
}

func receiptAddressDomain(address string) string {
	address = strings.TrimSpace(address)
	at := strings.LastIndex(address, "@")
	if at < 0 || at == len(address)-1 {
		return ""
	}
	return strings.ToLower(address[at+1:])
}

func truncateReceiptText(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) > max {
		return value[:max]
	}
	return value
}

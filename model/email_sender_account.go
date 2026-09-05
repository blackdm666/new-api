package model

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/mail"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	EmailSenderProfileMarketing          = "marketing"
	EmailSenderProviderAliyunEventBridge = "aliyun_eventbridge"

	EmailSenderHealthPending  = "pending"
	EmailSenderHealthHealthy  = "healthy"
	EmailSenderHealthDegraded = "degraded"
	EmailSenderHealthDisabled = "disabled"

	EmailAttemptPurposeDelivery    = "delivery"
	EmailAttemptPurposeAccountTest = "account_test"

	EmailAttemptStatusSubmitting      = "submitting"
	EmailAttemptStatusAwaitingReceipt = "awaiting_receipt"
	EmailAttemptStatusDelivered       = "delivered"
	EmailAttemptStatusFailed          = "failed"

	EmailReceiptProviderAliyunEventBridge = "aliyun_eventbridge"
)

var (
	ErrEmailSenderAccountInvalid = errors.New("email sender account is invalid")
	ErrEmailSenderAccountMissing = errors.New("email sender account was not found")
)

const emailSenderAccountTokenPurpose = "email-sender-account-token"

type EmailSenderAccount struct {
	Id                   int    `json:"id"`
	Name                 string `json:"name" gorm:"type:varchar(100);not null"`
	Profile              string `json:"profile" gorm:"type:varchar(24);not null;index"`
	Provider             string `json:"provider" gorm:"type:varchar(32);not null;index"`
	Server               string `json:"server" gorm:"type:varchar(255);not null"`
	Port                 int    `json:"port" gorm:"not null"`
	Account              string `json:"account" gorm:"type:varchar(320);not null"`
	From                 string `json:"from" gorm:"type:varchar(320);not null;index"`
	TokenEncrypted       string `json:"-" gorm:"type:text"`
	SSLEnabled           bool   `json:"ssl_enabled" gorm:"not null"`
	StartTLSEnabled      bool   `json:"starttls_enabled" gorm:"not null"`
	InsecureSkipVerify   bool   `json:"insecure_skip_verify" gorm:"not null"`
	ForceAuthLogin       bool   `json:"force_auth_login" gorm:"not null"`
	Weight               int    `json:"weight" gorm:"not null"`
	RateLimitPerMinute   int    `json:"rate_limit_per_minute" gorm:"not null"`
	Enabled              bool   `json:"enabled" gorm:"not null;index"`
	ConfigHash           string `json:"config_hash" gorm:"type:char(64);not null"`
	TestedTime           int64  `json:"tested_time" gorm:"bigint;not null;default:0"`
	ReceiptVerifiedTime  int64  `json:"receipt_verified_time" gorm:"bigint;not null;default:0"`
	DisabledUntil        int64  `json:"disabled_until" gorm:"bigint;not null;default:0;index"`
	HealthStatus         string `json:"health_status" gorm:"type:varchar(24);not null;index"`
	ConsecutiveFailures  int    `json:"consecutive_failures" gorm:"not null;default:0"`
	LastSuccessTime      int64  `json:"last_success_time" gorm:"bigint;not null;default:0"`
	LastFailureTime      int64  `json:"last_failure_time" gorm:"bigint;not null;default:0"`
	LastError            string `json:"last_error" gorm:"type:varchar(1000)"`
	CreatedTime          int64  `json:"created_time" gorm:"bigint;autoCreateTime;index"`
	UpdatedTime          int64  `json:"updated_time" gorm:"bigint;autoUpdateTime"`
	CredentialConfigured bool   `json:"credential_configured" gorm:"-"`
}

type EmailDeliveryAttempt struct {
	Id              int    `json:"id"`
	DeliveryId      int    `json:"delivery_id" gorm:"index;index:idx_email_delivery_attempt,priority:1"`
	SenderAccountId int    `json:"sender_account_id" gorm:"index"`
	Purpose         string `json:"purpose" gorm:"type:varchar(24);not null;index"`
	Sequence        int    `json:"sequence" gorm:"not null;index:idx_email_delivery_attempt,priority:2"`
	MessageId       string `json:"message_id" gorm:"column:message_id;type:varchar(191);not null;uniqueIndex"`
	NotifyMessageId string `json:"notify_message_id" gorm:"column:notify_message_id;type:varchar(191);not null;uniqueIndex"`
	SenderAddress   string `json:"sender_address" gorm:"type:varchar(320);not null"`
	SenderDomain    string `json:"sender_domain" gorm:"type:varchar(255);not null;index"`
	RecipientDomain string `json:"recipient_domain" gorm:"type:varchar(255);not null;index"`
	Status          string `json:"status" gorm:"type:varchar(32);not null;index"`
	AcceptedTime    int64  `json:"accepted_time" gorm:"bigint;not null;default:0;index"`
	FinalizedTime   int64  `json:"finalized_time" gorm:"bigint;not null;default:0;index"`
	ProviderEventId string `json:"provider_event_id" gorm:"type:varchar(191);not null;default:'';index"`
	ProviderEnvId   string `json:"provider_env_id" gorm:"type:varchar(191);not null;default:'';index"`
	FailureType     string `json:"failure_type" gorm:"type:varchar(64);not null;default:'';index"`
	ErrorCode       string `json:"error_code" gorm:"type:varchar(64);not null;default:''"`
	ErrorMessage    string `json:"error_message" gorm:"type:varchar(2000);not null;default:''"`
	CreatedTime     int64  `json:"created_time" gorm:"bigint;autoCreateTime;index"`
	UpdatedTime     int64  `json:"updated_time" gorm:"bigint;autoUpdateTime"`
}

type EmailReceiptEvent struct {
	Id              int    `json:"id"`
	Provider        string `json:"provider" gorm:"type:varchar(32);not null;index;uniqueIndex:idx_email_receipt_provider_event,priority:1"`
	EventId         string `json:"event_id" gorm:"type:varchar(191);not null;uniqueIndex:idx_email_receipt_provider_event,priority:2"`
	EventType       string `json:"event_type" gorm:"type:varchar(64);not null;index"`
	AttemptId       int    `json:"attempt_id" gorm:"index"`
	MessageId       string `json:"message_id" gorm:"column:message_id;type:varchar(191);not null;default:'';index"`
	NotifyMessageId string `json:"notify_message_id" gorm:"column:notify_message_id;type:varchar(191);not null;default:'';index"`
	SenderAddress   string `json:"sender_address" gorm:"type:varchar(320);not null;default:''"`
	RecipientMasked string `json:"recipient_masked" gorm:"type:varchar(320);not null;default:''"`
	RecipientDomain string `json:"recipient_domain" gorm:"type:varchar(255);not null;default:'';index"`
	Status          string `json:"status" gorm:"type:varchar(16);not null;default:''"`
	ErrorCode       string `json:"error_code" gorm:"type:varchar(64);not null;default:''"`
	ErrorMessage    string `json:"error_message" gorm:"type:varchar(2000);not null;default:''"`
	FailureType     string `json:"failure_type" gorm:"type:varchar(64);not null;default:'';index"`
	ProviderEnvId   string `json:"provider_env_id" gorm:"type:varchar(191);not null;default:''"`
	ESP             string `json:"esp" gorm:"column:esp;type:varchar(100);not null;default:''"`
	OutboundIP      string `json:"outbound_ip" gorm:"type:varchar(64);not null;default:''"`
	PayloadHash     string `json:"payload_hash" gorm:"type:char(64);not null"`
	ReceivedTime    int64  `json:"received_time" gorm:"bigint;autoCreateTime;index"`
	ProcessedTime   int64  `json:"processed_time" gorm:"bigint;not null;default:0"`
	ProcessingError string `json:"processing_error" gorm:"type:varchar(1000);not null;default:''"`
}

type EmailDeliveryThrottle struct {
	Id              int    `json:"id"`
	ScopeKey        string `json:"scope_key" gorm:"type:varchar(512);not null;uniqueIndex"`
	ScopeType       string `json:"scope_type" gorm:"type:varchar(32);not null;index"`
	SenderAccountId int    `json:"sender_account_id" gorm:"index"`
	SenderDomain    string `json:"sender_domain" gorm:"type:varchar(255);not null;default:'';index"`
	RecipientDomain string `json:"recipient_domain" gorm:"type:varchar(255);not null;default:'';index"`
	EffectiveRPM    int    `json:"effective_rpm" gorm:"not null;default:0"`
	StrikeCount     int    `json:"strike_count" gorm:"not null;default:0"`
	DisabledUntil   int64  `json:"disabled_until" gorm:"bigint;not null;default:0;index"`
	LastFailureTime int64  `json:"last_failure_time" gorm:"bigint;not null;default:0"`
	LastSuccessTime int64  `json:"last_success_time" gorm:"bigint;not null;default:0"`
	LastFailureType string `json:"last_failure_type" gorm:"type:varchar(64);not null;default:''"`
	CreatedTime     int64  `json:"created_time" gorm:"bigint;autoCreateTime"`
	UpdatedTime     int64  `json:"updated_time" gorm:"bigint;autoUpdateTime"`
}

type EmailDeliveryMinuteQuota struct {
	QuotaKey    string `json:"quota_key" gorm:"type:varchar(191);primaryKey"`
	WindowStart int64  `json:"window_start" gorm:"bigint;not null;index"`
	Used        int    `json:"used" gorm:"not null;default:0"`
	UpdatedTime int64  `json:"updated_time" gorm:"bigint;not null"`
}

type EmailReceiptEndpoint struct {
	Id               int    `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Provider         string `json:"provider" gorm:"type:varchar(32);not null"`
	Enabled          bool   `json:"enabled" gorm:"not null"`
	TokenEncrypted   string `json:"-" gorm:"type:text"`
	LastEventTime    int64  `json:"last_event_time" gorm:"bigint;not null;default:0"`
	LastVerifiedTime int64  `json:"last_verified_time" gorm:"bigint;not null;default:0"`
	LastError        string `json:"last_error" gorm:"type:varchar(1000);not null;default:''"`
	CreatedTime      int64  `json:"created_time" gorm:"bigint;autoCreateTime"`
	UpdatedTime      int64  `json:"updated_time" gorm:"bigint;autoUpdateTime"`
	TokenConfigured  bool   `json:"token_configured" gorm:"-"`
}

const emailReceiptEndpointTokenPurpose = "email-receipt-eventbridge-token"

func (account *EmailSenderAccount) Normalize() error {
	if account == nil {
		return ErrEmailSenderAccountInvalid
	}
	account.Name = strings.TrimSpace(account.Name)
	account.Profile = strings.TrimSpace(account.Profile)
	account.Provider = strings.TrimSpace(account.Provider)
	account.Server = strings.TrimSpace(account.Server)
	account.Account = strings.TrimSpace(account.Account)
	account.From = strings.TrimSpace(account.From)
	if account.Name == "" || account.Profile != EmailSenderProfileMarketing || account.Provider != EmailSenderProviderAliyunEventBridge || !validAliyunDirectMailSMTPServer(account.Server) || account.Port < 1 || account.Port > 65535 || account.Account == "" || account.From == "" {
		return ErrEmailSenderAccountInvalid
	}
	parsed, err := mail.ParseAddress(account.From)
	if err != nil || !strings.EqualFold(parsed.Address, account.From) {
		return ErrEmailSenderAccountInvalid
	}
	if account.Weight < 1 || account.Weight > 100 || account.RateLimitPerMinute < 1 || account.RateLimitPerMinute > 1000 || account.SSLEnabled && account.StartTLSEnabled {
		return ErrEmailSenderAccountInvalid
	}
	if account.HealthStatus == "" {
		account.HealthStatus = EmailSenderHealthPending
	}
	account.ConfigHash = account.ComputeConfigHash()
	return nil
}

func (account *EmailSenderAccount) ComputeConfigHash() string {
	value := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(account.Provider)),
		strings.ToLower(strings.TrimSpace(account.Server)),
		strconv.Itoa(account.Port),
		strings.ToLower(strings.TrimSpace(account.Account)),
		strings.ToLower(strings.TrimSpace(account.From)),
		strconv.FormatBool(account.SSLEnabled),
		strconv.FormatBool(account.StartTLSEnabled),
		strconv.FormatBool(account.InsecureSkipVerify),
		strconv.FormatBool(account.ForceAuthLogin),
	}, "\n")
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func (account *EmailSenderAccount) SetToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	encrypted, err := common.EncryptSensitiveValue(emailSenderAccountTokenPurpose, []byte(token))
	if err != nil {
		return err
	}
	account.TokenEncrypted = encrypted
	return nil
}

func (account *EmailSenderAccount) Token() (string, error) {
	if strings.TrimSpace(account.TokenEncrypted) == "" {
		return "", nil
	}
	plain, err := common.DecryptSensitiveValue(emailSenderAccountTokenPurpose, account.TokenEncrypted)
	return string(plain), err
}

func decorateEmailSenderAccount(account *EmailSenderAccount) {
	if account == nil {
		return
	}
	account.CredentialConfigured = strings.TrimSpace(account.TokenEncrypted) != ""
}

func ListMarketingEmailSenderAccounts() ([]*EmailSenderAccount, error) {
	rows := []*EmailSenderAccount{}
	err := DB.Where("profile = ?", EmailSenderProfileMarketing).Order("id ASC").Find(&rows).Error
	for _, row := range rows {
		decorateEmailSenderAccount(row)
	}
	return rows, err
}

func BackfillLegacyMarketingEmailSenderAccount() error {
	var count int64
	if err := DB.Model(&EmailSenderAccount{}).Where("profile = ?", EmailSenderProfileMarketing).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	keys := []string{
		"SMTPMarketingServer", "SMTPMarketingPort", "SMTPMarketingAccount", "SMTPMarketingFrom", "SMTPMarketingToken",
		"SMTPMarketingSSLEnabled", "SMTPMarketingStartTLSEnabled", "SMTPMarketingInsecureSkipVerify", "SMTPMarketingForceAuthLogin",
	}
	rows := []Option{}
	if err := DB.Where(commonKeyCol+" IN ?", keys).Find(&rows).Error; err != nil {
		return err
	}
	values := map[string]string{}
	for _, row := range rows {
		values[row.Key] = row.Value
	}
	server := strings.TrimSpace(values["SMTPMarketingServer"])
	accountName := strings.TrimSpace(values["SMTPMarketingAccount"])
	from := strings.TrimSpace(values["SMTPMarketingFrom"])
	if server == "" || accountName == "" || from == "" {
		return nil
	}
	if !validAliyunDirectMailSMTPServer(server) {
		return nil
	}
	port := 587
	if parsed, err := strconv.Atoi(values["SMTPMarketingPort"]); err == nil && parsed > 0 {
		port = parsed
	}
	rules := setting.GetEmailDeliveryRules()
	row := &EmailSenderAccount{
		Name: "Default marketing sender", Profile: EmailSenderProfileMarketing,
		Provider: EmailSenderProviderAliyunEventBridge, Server: server, Port: port,
		Account: accountName, From: from, SSLEnabled: values["SMTPMarketingSSLEnabled"] == "true",
		StartTLSEnabled:    values["SMTPMarketingStartTLSEnabled"] == "true",
		InsecureSkipVerify: values["SMTPMarketingInsecureSkipVerify"] == "true",
		ForceAuthLogin:     values["SMTPMarketingForceAuthLogin"] == "true",
		Weight:             1, RateLimitPerMinute: rules.MarketingPerMinuteLimit,
	}
	if err := row.SetToken(values["SMTPMarketingToken"]); err != nil {
		return err
	}
	return CreateEmailSenderAccount(row)
}

func GetEmailSenderAccount(id int) (*EmailSenderAccount, error) {
	if id <= 0 {
		return nil, ErrEmailSenderAccountInvalid
	}
	row := &EmailSenderAccount{}
	if err := DB.First(row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEmailSenderAccountMissing
		}
		return nil, err
	}
	decorateEmailSenderAccount(row)
	return row, nil
}

func CreateEmailSenderAccount(account *EmailSenderAccount) error {
	if err := account.Normalize(); err != nil {
		return err
	}
	account.Enabled = false
	account.TestedTime = 0
	account.ReceiptVerifiedTime = 0
	account.HealthStatus = EmailSenderHealthPending
	return DB.Create(account).Error
}

func UpdateEmailSenderAccount(account *EmailSenderAccount, configChanged bool) error {
	if err := account.Normalize(); err != nil {
		return err
	}
	updates := map[string]any{
		"name": account.Name, "provider": account.Provider, "server": account.Server, "port": account.Port,
		"account": account.Account, "from": account.From, "ssl_enabled": account.SSLEnabled,
		"start_tls_enabled": account.StartTLSEnabled, "insecure_skip_verify": account.InsecureSkipVerify,
		"force_auth_login": account.ForceAuthLogin, "weight": account.Weight,
		"rate_limit_per_minute": account.RateLimitPerMinute, "config_hash": account.ConfigHash,
	}
	if account.TokenEncrypted != "" {
		updates["token_encrypted"] = account.TokenEncrypted
	}
	if configChanged {
		updates["enabled"] = false
		updates["tested_time"] = int64(0)
		updates["receipt_verified_time"] = int64(0)
		updates["health_status"] = EmailSenderHealthPending
		updates["last_error"] = ""
	}
	return DB.Model(&EmailSenderAccount{}).Where("id = ? AND profile = ?", account.Id, EmailSenderProfileMarketing).Updates(updates).Error
}

func SetEmailSenderAccountEnabled(id int, enabled bool) error {
	if id <= 0 {
		return ErrEmailSenderAccountInvalid
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		account := &EmailSenderAccount{}
		if err := lockForUpdate(tx).Where("id = ? AND profile = ?", id, EmailSenderProfileMarketing).First(account).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEmailSenderAccountMissing
			}
			return err
		}
		now := common.GetTimestamp()
		if enabled && (account.TestedTime <= 0 || account.ReceiptVerifiedTime <= 0 || account.DisabledUntil > now || account.HealthStatus == EmailSenderHealthDegraded && !account.Enabled) {
			return ErrEmailSenderAccountInvalid
		}
		health := EmailSenderHealthDisabled
		if enabled {
			health = EmailSenderHealthHealthy
		}
		return tx.Model(account).Updates(map[string]any{
			"enabled": enabled, "health_status": health, "updated_time": now,
		}).Error
	})
}

func DeleteEmailSenderAccount(id int) error {
	result := DB.Where("id = ? AND profile = ?", id, EmailSenderProfileMarketing).Delete(&EmailSenderAccount{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrEmailSenderAccountMissing
	}
	return nil
}

func ListUsableMarketingEmailSenderAccounts(now int64) ([]*EmailSenderAccount, error) {
	rows := []*EmailSenderAccount{}
	err := DB.Where("profile = ? AND enabled = ? AND tested_time > 0 AND receipt_verified_time > 0 AND disabled_until <= ?", EmailSenderProfileMarketing, true, now).
		Order("id ASC").Find(&rows).Error
	return rows, err
}

func GetEmailReceiptEndpoint() (*EmailReceiptEndpoint, error) {
	row := &EmailReceiptEndpoint{Id: 1, Provider: EmailReceiptProviderAliyunEventBridge}
	if err := DB.FirstOrCreate(row, EmailReceiptEndpoint{Id: 1}).Error; err != nil {
		return nil, err
	}
	if row.Provider == "" {
		row.Provider = EmailReceiptProviderAliyunEventBridge
		if err := DB.Model(row).Update("provider", row.Provider).Error; err != nil {
			return nil, err
		}
	}
	row.TokenConfigured = strings.TrimSpace(row.TokenEncrypted) != ""
	return row, nil
}

func RotateEmailReceiptEndpointToken() (string, error) {
	token, err := common.GenerateRandomKey(48)
	if err != nil {
		return "", err
	}
	encrypted, err := common.EncryptSensitiveValue(emailReceiptEndpointTokenPurpose, []byte(token))
	if err != nil {
		return "", err
	}
	now := common.GetTimestamp()
	row := &EmailReceiptEndpoint{Id: 1, Provider: EmailReceiptProviderAliyunEventBridge, TokenEncrypted: encrypted, CreatedTime: now, UpdatedTime: now}
	err = DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{"token_encrypted": encrypted, "enabled": false, "last_verified_time": int64(0), "last_error": "", "updated_time": now}),
	}).Create(row).Error
	return token, err
}

func VerifyEmailReceiptEndpointToken(token string) bool {
	row, err := GetEmailReceiptEndpoint()
	if err != nil || !row.Enabled || row.TokenEncrypted == "" {
		return false
	}
	plain, err := common.DecryptSensitiveValue(emailReceiptEndpointTokenPurpose, row.TokenEncrypted)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), plain) == 1
}

func UpdateEmailReceiptEndpointEnabled(enabled bool) error {
	row, err := GetEmailReceiptEndpoint()
	if err != nil {
		return err
	}
	if enabled && row.TokenEncrypted == "" {
		return errors.New("receipt token is not configured")
	}
	return DB.Model(&EmailReceiptEndpoint{}).Where("id = ?", 1).Updates(map[string]any{"enabled": enabled, "updated_time": common.GetTimestamp()}).Error
}

func UpdateEmailReceiptEndpointActivity(verified bool, message string) error {
	now := common.GetTimestamp()
	updates := map[string]any{"last_event_time": now, "last_error": strings.TrimSpace(message), "updated_time": now}
	if verified {
		updates["last_verified_time"] = now
		updates["last_error"] = ""
	}
	return DB.Model(&EmailReceiptEndpoint{}).Where("id = ?", 1).Updates(updates).Error
}

func CreateEmailDeliveryAttempt(deliveryId int, sender *EmailSenderAccount, purpose string, recipient string, messageId string, notifyMessageId string) (*EmailDeliveryAttempt, error) {
	if sender == nil || sender.Id <= 0 || messageId == "" || notifyMessageId == "" {
		return nil, ErrEmailSenderAccountInvalid
	}
	sequence := 1
	if deliveryId > 0 {
		var count int64
		if err := DB.Model(&EmailDeliveryAttempt{}).Where("delivery_id = ?", deliveryId).Count(&count).Error; err != nil {
			return nil, err
		}
		sequence = int(count) + 1
	}
	attempt := &EmailDeliveryAttempt{
		DeliveryId:      deliveryId,
		SenderAccountId: sender.Id,
		Purpose:         strings.TrimSpace(purpose),
		Sequence:        sequence,
		MessageId:       normalizeEmailMessageId(messageId),
		NotifyMessageId: strings.TrimSpace(notifyMessageId),
		SenderAddress:   strings.TrimSpace(sender.From),
		SenderDomain:    emailAddressDomain(sender.From),
		RecipientDomain: emailAddressDomain(recipient),
		Status:          EmailAttemptStatusSubmitting,
	}
	if attempt.Purpose == "" || attempt.SenderDomain == "" || attempt.RecipientDomain == "" {
		return nil, ErrEmailSenderAccountInvalid
	}
	return attempt, DB.Create(attempt).Error
}

func MarkEmailDeliveryAttemptAccepted(attemptId int, acceptedTime int64) error {
	return DB.Model(&EmailDeliveryAttempt{}).Where("id = ? AND status = ?", attemptId, EmailAttemptStatusSubmitting).
		Updates(map[string]any{"status": EmailAttemptStatusAwaitingReceipt, "accepted_time": acceptedTime, "updated_time": acceptedTime}).Error
}

func MarkEmailDeliveryAttemptSubmissionFailure(attemptId int, message string, failedTime int64) error {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	return DB.Model(&EmailDeliveryAttempt{}).Where("id = ? AND status = ?", attemptId, EmailAttemptStatusSubmitting).
		Updates(map[string]any{"status": EmailAttemptStatusFailed, "error_message": message, "finalized_time": failedTime, "updated_time": failedTime}).Error
}

func MarkEmailSenderAccountSMTPTestAccepted(accountId int, testedTime int64) error {
	return DB.Model(&EmailSenderAccount{}).Where("id = ?", accountId).Updates(map[string]any{
		"tested_time": testedTime, "health_status": EmailSenderHealthPending,
		"last_success_time": testedTime, "last_error": "waiting for EventBridge receipt", "updated_time": testedTime,
	}).Error
}

func MarkEmailDeliveryAwaitingReceipt(deliveryId int, senderAccountId int, attemptId int, messageId string, acceptedTime int64, receiptDeadline int64) error {
	return DB.Model(&EmailDelivery{}).Where("id = ? AND expired_time = 0 AND dead_letter_time = 0", deliveryId).
		Updates(map[string]any{
			"state": EmailDeliveryStatusAwaitingReceipt, "sender_account_id": senderAccountId,
			"current_attempt_id": attemptId, "message_id": normalizeEmailMessageId(messageId),
			"accepted_time": acceptedTime, "receipt_deadline": receiptDeadline,
			"locked_until": int64(0), "last_error": "", "failure_type": "", "updated_time": acceptedTime,
		}).Error
}

func FindEmailDeliveryAttempt(messageId string, notifyMessageId string) (*EmailDeliveryAttempt, error) {
	query := DB.Model(&EmailDeliveryAttempt{})
	messageId = normalizeEmailMessageId(messageId)
	notifyMessageId = strings.TrimSpace(notifyMessageId)
	if notifyMessageId != "" {
		query = query.Where("notify_message_id = ?", notifyMessageId)
	} else if messageId != "" {
		query = query.Where("message_id = ?", messageId)
	} else {
		return nil, gorm.ErrRecordNotFound
	}
	row := &EmailDeliveryAttempt{}
	return row, query.First(row).Error
}

func StoreEmailReceiptEvent(event *EmailReceiptEvent) (bool, error) {
	if event == nil || event.Provider == "" || event.EventId == "" || event.PayloadHash == "" {
		return false, gorm.ErrInvalidData
	}
	result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(event)
	if result.Error != nil || result.RowsAffected == 1 {
		return result.RowsAffected == 1, result.Error
	}
	existing := &EmailReceiptEvent{}
	if err := DB.Where("provider = ? AND event_id = ?", event.Provider, event.EventId).First(existing).Error; err != nil {
		return false, err
	}
	*event = *existing
	return false, nil
}

func ApplySuccessfulEmailReceipt(event *EmailReceiptEvent, attempt *EmailDeliveryAttempt, finalizedTime int64) error {
	if event == nil || attempt == nil {
		return gorm.ErrInvalidData
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&EmailDeliveryAttempt{}).Where("id = ? AND finalized_time = 0", attempt.Id).Updates(map[string]any{
			"status": EmailAttemptStatusDelivered, "provider_event_id": event.EventId,
			"provider_env_id": event.ProviderEnvId, "finalized_time": finalizedTime, "updated_time": finalizedTime,
		}).Error; err != nil {
			return err
		}
		if attempt.DeliveryId > 0 {
			if err := tx.Model(&EmailDelivery{}).Where("id = ? AND current_attempt_id = ?", attempt.DeliveryId, attempt.Id).Updates(map[string]any{
				"state": EmailDeliveryStatusDelivered, "subject": "", "body": "", "delivered_time": finalizedTime,
				"finalized_time": finalizedTime, "receipt_deadline": int64(0), "last_error": "", "failure_type": "", "updated_time": finalizedTime,
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&MarketingRecipient{}).Where("email_delivery_id = ?", attempt.DeliveryId).Updates(map[string]any{
				"status": MarketingRecipientStatusDelivered, "delivered_time": finalizedTime,
				"last_error": "", "updated_time": finalizedTime,
			}).Error; err != nil {
				return err
			}
		}
		accountUpdates := map[string]any{
			"tested_time": finalizedTime, "receipt_verified_time": finalizedTime,
			"consecutive_failures": 0, "last_success_time": finalizedTime,
			"last_error": "", "updated_time": finalizedTime,
		}
		if attempt.Purpose == EmailAttemptPurposeAccountTest {
			accountUpdates["enabled"] = true
			accountUpdates["health_status"] = EmailSenderHealthHealthy
		}
		if err := tx.Model(&EmailSenderAccount{}).Where("id = ?", attempt.SenderAccountId).Updates(accountUpdates).Error; err != nil {
			return err
		}
		if attempt.Purpose == EmailAttemptPurposeDelivery {
			if err := tx.Model(&EmailSenderAccount{}).Where("id = ? AND enabled = ?", attempt.SenderAccountId, true).Updates(map[string]any{
				"health_status": EmailSenderHealthHealthy, "disabled_until": int64(0),
				"consecutive_failures": 0, "updated_time": finalizedTime,
			}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&EmailReceiptEvent{}).Where("id = ?", event.Id).Updates(map[string]any{"attempt_id": attempt.Id, "processed_time": finalizedTime, "processing_error": ""}).Error
	})
}

func ApplyFailedEmailReceipt(event *EmailReceiptEvent, attempt *EmailDeliveryAttempt, finalizedTime int64, nextAttemptTime int64, permanent bool) error {
	if event == nil || attempt == nil {
		return gorm.ErrInvalidData
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&EmailDeliveryAttempt{}).Where("id = ? AND finalized_time = 0", attempt.Id).Updates(map[string]any{
			"status": EmailAttemptStatusFailed, "provider_event_id": event.EventId,
			"failure_type": event.FailureType, "error_code": event.ErrorCode,
			"error_message": event.ErrorMessage, "finalized_time": finalizedTime, "updated_time": finalizedTime,
		}).Error; err != nil {
			return err
		}
		if attempt.DeliveryId > 0 {
			updates := map[string]any{
				"attempts": gorm.Expr("attempts + 1"), "last_error": event.ErrorMessage,
				"failure_type": event.FailureType, "locked_until": int64(0),
				"current_attempt_id": int64(0), "updated_time": finalizedTime,
			}
			if permanent {
				updates["state"] = EmailDeliveryStatusFailed
				updates["dead_letter_time"] = finalizedTime
				updates["finalized_time"] = finalizedTime
				updates["next_attempt_time"] = int64(0)
				updates["subject"] = ""
				updates["body"] = ""
			} else {
				updates["state"] = EmailDeliveryStatusRetrying
				updates["delivered_time"] = int64(0)
				updates["receipt_deadline"] = int64(0)
				updates["marketing_quota_time"] = int64(0)
				updates["next_attempt_time"] = nextAttemptTime
			}
			if err := tx.Model(&EmailDelivery{}).Where("id = ? AND current_attempt_id = ?", attempt.DeliveryId, attempt.Id).Updates(updates).Error; err != nil {
				return err
			}
			recipientUpdates := map[string]any{"last_error": event.ErrorMessage, "updated_time": finalizedTime}
			if permanent {
				recipientUpdates["status"] = MarketingRecipientStatusFailed
			} else {
				recipientUpdates["status"] = MarketingRecipientStatusQueued
			}
			if err := tx.Model(&MarketingRecipient{}).Where("email_delivery_id = ?", attempt.DeliveryId).Updates(recipientUpdates).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&EmailSenderAccount{}).Where("id = ?", attempt.SenderAccountId).Updates(map[string]any{
			"last_failure_time": finalizedTime, "last_error": event.ErrorMessage, "updated_time": finalizedTime,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&EmailReceiptEvent{}).Where("id = ?", event.Id).Updates(map[string]any{"attempt_id": attempt.Id, "processed_time": finalizedTime, "processing_error": ""}).Error
	})
}

func MarkEmailReceiptEventUnmatched(eventId int, reason string) error {
	return DB.Model(&EmailReceiptEvent{}).Where("id = ?", eventId).Updates(map[string]any{"processing_error": strings.TrimSpace(reason), "processed_time": int64(0)}).Error
}

func UpsertEmailDeliveryThrottle(throttle *EmailDeliveryThrottle) error {
	if throttle == nil || throttle.ScopeKey == "" || throttle.ScopeType == "" {
		return gorm.ErrInvalidData
	}
	now := common.GetTimestamp()
	throttle.CreatedTime = now
	throttle.UpdatedTime = now
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "scope_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"scope_type": throttle.ScopeType, "sender_account_id": throttle.SenderAccountId,
			"sender_domain": throttle.SenderDomain, "recipient_domain": throttle.RecipientDomain,
			"effective_rpm": throttle.EffectiveRPM, "strike_count": throttle.StrikeCount,
			"disabled_until": throttle.DisabledUntil, "last_failure_time": throttle.LastFailureTime,
			"last_failure_type": throttle.LastFailureType, "updated_time": now,
		}),
	}).Create(throttle).Error
}

func ActiveEmailDeliveryThrottle(scopeKey string, now int64) (*EmailDeliveryThrottle, error) {
	row := &EmailDeliveryThrottle{}
	err := DB.Where("scope_key = ? AND disabled_until > ?", strings.TrimSpace(scopeKey), now).First(row).Error
	return row, err
}

func GetEmailDeliveryThrottle(scopeKey string) (*EmailDeliveryThrottle, error) {
	row := &EmailDeliveryThrottle{}
	err := DB.Where("scope_key = ?", strings.TrimSpace(scopeKey)).First(row).Error
	return row, err
}

func RecoverEmailDeliveryThrottle(scopeKey string, maximumRPM int, recoveredTime int64) error {
	if strings.TrimSpace(scopeKey) == "" || maximumRPM <= 0 || recoveredTime <= 0 {
		return gorm.ErrInvalidData
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		row := &EmailDeliveryThrottle{}
		if err := lockForUpdate(tx).Where("scope_key = ?", strings.TrimSpace(scopeKey)).First(row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		nextRPM := row.EffectiveRPM * 2
		if nextRPM <= 0 {
			nextRPM = maximumRPM
		}
		if nextRPM > maximumRPM {
			nextRPM = maximumRPM
		}
		nextStrikes := row.StrikeCount - 1
		if nextStrikes < 0 {
			nextStrikes = 0
		}
		if nextRPM >= maximumRPM && nextStrikes == 0 {
			return tx.Delete(row).Error
		}
		return tx.Model(row).Updates(map[string]any{
			"effective_rpm": nextRPM, "strike_count": nextStrikes,
			"disabled_until": int64(0), "last_success_time": recoveredTime,
			"updated_time": recoveredTime,
		}).Error
	})
}

func ReserveEmailDeliveryMinuteQuota(scopeKey string, windowStart int64, limit int) (bool, error) {
	return reserveEmailDeliveryMinuteQuota(DB, scopeKey, windowStart, limit)
}

func reserveEmailDeliveryMinuteQuota(db *gorm.DB, scopeKey string, windowStart int64, limit int) (bool, error) {
	scopeKey = strings.TrimSpace(scopeKey)
	if db == nil || scopeKey == "" || windowStart <= 0 || limit <= 0 {
		return false, gorm.ErrInvalidData
	}
	quotaKey := scopeKey + ":" + strconv.FormatInt(windowStart, 10)
	if len(quotaKey) > 191 {
		return false, gorm.ErrInvalidData
	}
	now := common.GetTimestamp()
	quota := &EmailDeliveryMinuteQuota{QuotaKey: quotaKey, WindowStart: windowStart, UpdatedTime: now}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(quota).Error; err != nil {
		return false, err
	}
	result := db.Model(&EmailDeliveryMinuteQuota{}).
		Where("quota_key = ? AND used < ?", quotaKey, limit).
		Updates(map[string]any{"used": gorm.Expr("used + 1"), "updated_time": now})
	return result.RowsAffected == 1, result.Error
}

func CleanupEmailDeliveryMinuteQuotas(before int64) error {
	return DB.Where("window_start < ?", before).Delete(&EmailDeliveryMinuteQuota{}).Error
}

func DegradeEmailSenderAccount(accountId int, disabledUntil int64, message string) error {
	return DB.Model(&EmailSenderAccount{}).Where("id = ?", accountId).Updates(map[string]any{
		"health_status": EmailSenderHealthDegraded, "disabled_until": disabledUntil,
		"consecutive_failures": gorm.Expr("consecutive_failures + 1"),
		"last_failure_time":    common.GetTimestamp(), "last_error": strings.TrimSpace(message),
		"updated_time": common.GetTimestamp(),
	}).Error
}

func DisableEmailSenderAccountForFailure(accountId int, message string) error {
	now := common.GetTimestamp()
	return DB.Model(&EmailSenderAccount{}).Where("id = ?", accountId).Updates(map[string]any{
		"enabled": false, "health_status": EmailSenderHealthDegraded,
		"consecutive_failures": gorm.Expr("consecutive_failures + 1"),
		"last_failure_time":    now, "last_error": strings.TrimSpace(message),
		"updated_time": now,
	}).Error
}

func normalizeEmailMessageId(value string) string {
	return strings.Trim(strings.TrimSpace(value), "<>")
}

func emailAddressDomain(value string) string {
	value = strings.TrimSpace(value)
	at := strings.LastIndex(value, "@")
	if at < 0 || at == len(value)-1 {
		return ""
	}
	return strings.ToLower(value[at+1:])
}

func validAliyunDirectMailSMTPServer(server string) bool {
	server = strings.ToLower(strings.TrimSpace(server))
	return server == "smtpdm.aliyun.com" || strings.HasPrefix(server, "smtpdm-") && strings.HasSuffix(server, ".aliyun.com")
}

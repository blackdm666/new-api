package service

import (
	"errors"
	"fmt"
	"html"
	"net/mail"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var (
	ErrSMTPTestRecipientRequired = errors.New("SMTP test recipient is required")
	ErrSMTPTestRecipientInvalid  = errors.New("SMTP test recipient is invalid")
	ErrSMTPTestChannelInvalid    = errors.New("SMTP test channel is invalid")
)

func SendSMTPTestEmail(userId int, requestedRecipient string, requestedChannel string) (string, common.SMTPDeliveryResult, error) {
	channel := strings.ToLower(strings.TrimSpace(requestedChannel))
	if channel == "" {
		channel = common.SMTPChannelPrimary
	}
	if channel != common.SMTPChannelPrimary && channel != common.SMTPChannelBackup {
		return "", common.SMTPDeliveryResult{}, ErrSMTPTestChannelInvalid
	}

	recipient := strings.TrimSpace(requestedRecipient)
	if recipient == "" {
		user, err := model.GetUserById(userId, false)
		if err != nil {
			return "", common.SMTPDeliveryResult{}, err
		}
		recipient = strings.TrimSpace(user.Email)
	}
	if recipient == "" {
		return "", common.SMTPDeliveryResult{}, ErrSMTPTestRecipientRequired
	}
	address, err := mail.ParseAddress(recipient)
	if err != nil || !strings.EqualFold(address.Address, recipient) || strings.ContainsAny(recipient, ";\r\n") {
		return "", common.SMTPDeliveryResult{}, ErrSMTPTestRecipientInvalid
	}

	systemName := common.SystemNameOrDefault()
	subject := fmt.Sprintf("[%s] SMTP test email", systemName)
	content := fmt.Sprintf(
		"<h2>SMTP configuration test succeeded</h2><p>%s can send email to this address.</p><p>Test time: %s</p>",
		html.EscapeString(systemName),
		time.Now().Format(time.RFC3339),
	)
	result, err := common.SendEmailViaChannel(subject, recipient, content, channel)
	if err != nil {
		return "", common.SMTPDeliveryResult{}, err
	}
	if channel == common.SMTPChannelBackup {
		if err := model.UpdateOption("SMTPBackupEnabled", "true"); err != nil {
			return "", common.SMTPDeliveryResult{}, fmt.Errorf("activate backup SMTP channel: %w", err)
		}
	}
	return recipient, result, nil
}

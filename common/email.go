package common

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	SMTPChannelPrimary   = "primary"
	SMTPChannelBackup    = "backup"
	SMTPChannelSecurity  = "security"
	SMTPChannelMarketing = "marketing"

	SMTPProfileNotification = "notification"
	SMTPProfileSecurity     = "security"
	SMTPProfileMarketing    = "marketing"
)

type SMTPDeliveryResult struct {
	Profile   string `json:"profile"`
	Channel   string `json:"channel"`
	MessageID string `json:"message_id"`
}

type smtpChannelConfig struct {
	name               string
	enabled            bool
	server             string
	port               int
	account            string
	from               string
	token              string
	sslEnabled         bool
	startTLSEnabled    bool
	insecureSkipVerify bool
	forceAuthLogin     bool
}

func primarySMTPChannel() smtpChannelConfig {
	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()
	return smtpChannelConfig{
		name:               SMTPChannelPrimary,
		enabled:            true,
		server:             SMTPServer,
		port:               SMTPPort,
		account:            SMTPAccount,
		from:               SMTPFrom,
		token:              SMTPToken,
		sslEnabled:         SMTPSSLEnabled,
		startTLSEnabled:    SMTPStartTLSEnabled,
		insecureSkipVerify: SMTPInsecureSkipVerify,
		forceAuthLogin:     SMTPForceAuthLogin,
	}
}

func backupSMTPChannel() smtpChannelConfig {
	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()
	return smtpChannelConfig{
		name:               SMTPChannelBackup,
		enabled:            SMTPBackupEnabled,
		server:             SMTPBackupServer,
		port:               SMTPBackupPort,
		account:            SMTPBackupAccount,
		from:               SMTPBackupFrom,
		token:              SMTPBackupToken,
		sslEnabled:         SMTPBackupSSLEnabled,
		startTLSEnabled:    SMTPBackupStartTLSEnabled,
		insecureSkipVerify: SMTPBackupInsecureSkipVerify,
		forceAuthLogin:     SMTPBackupForceAuthLogin,
	}
}

func securitySMTPChannel() smtpChannelConfig {
	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()
	return smtpChannelConfig{
		name:               SMTPChannelSecurity,
		enabled:            SMTPSecurityEnabled,
		server:             SMTPSecurityServer,
		port:               SMTPSecurityPort,
		account:            SMTPSecurityAccount,
		from:               SMTPSecurityFrom,
		token:              SMTPSecurityToken,
		sslEnabled:         SMTPSecuritySSLEnabled,
		startTLSEnabled:    SMTPSecurityStartTLSEnabled,
		insecureSkipVerify: SMTPSecurityInsecureSkipVerify,
		forceAuthLogin:     SMTPSecurityForceAuthLogin,
	}
}

func marketingSMTPChannel() smtpChannelConfig {
	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()
	return smtpChannelConfig{
		name:               SMTPChannelMarketing,
		enabled:            SMTPMarketingEnabled,
		server:             SMTPMarketingServer,
		port:               SMTPMarketingPort,
		account:            SMTPMarketingAccount,
		from:               SMTPMarketingFrom,
		token:              SMTPMarketingToken,
		sslEnabled:         SMTPMarketingSSLEnabled,
		startTLSEnabled:    SMTPMarketingStartTLSEnabled,
		insecureSkipVerify: SMTPMarketingInsecureSkipVerify,
		forceAuthLogin:     SMTPMarketingForceAuthLogin,
	}
}

func smtpChannelForProfile(profile string) smtpChannelConfig {
	switch profile {
	case SMTPProfileSecurity:
		if channel := securitySMTPChannel(); channel.enabled {
			return channel
		}
	case SMTPProfileMarketing:
		if channel := marketingSMTPChannel(); channel.enabled {
			return channel
		}
	}
	return primarySMTPChannel()
}

func (config smtpChannelConfig) fromAddress() string {
	if strings.TrimSpace(config.from) != "" {
		return strings.TrimSpace(config.from)
	}
	return strings.TrimSpace(config.account)
}

func (config smtpChannelConfig) shouldUseLoginAuth() bool {
	if config.forceAuthLogin {
		return true
	}
	return isOutlookServer(config.account) || slices.Contains(EmailLoginAuthServerList, config.server)
}

func (config smtpChannelConfig) auth() smtp.Auth {
	return newSMTPAutoAuth(config.account, config.token, config.server, config.forceAuthLogin, config.shouldUseLoginAuth())
}

func (config smtpChannelConfig) shouldAuthenticate() bool {
	return config.account != "" && config.token != ""
}

func (config smtpChannelConfig) tlsConfig() *tls.Config {
	return &tls.Config{
		ServerName:         config.server,
		InsecureSkipVerify: config.insecureSkipVerify, // #nosec G402 -- admin-controlled SMTP compatibility option.
	}
}

func (config smtpChannelConfig) newClient(addr string) (*smtp.Client, error) {
	if config.sslEnabled || (config.port == 465 && !config.startTLSEnabled) {
		conn, err := tls.Dial("tcp", addr, config.tlsConfig())
		if err != nil {
			return nil, err
		}
		client, err := smtp.NewClient(conn, config.server)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		return client, nil
	}

	client, err := smtp.Dial(addr)
	if err != nil {
		return nil, err
	}

	if config.startTLSEnabled {
		startTLSSupported, _ := client.Extension("STARTTLS")
		if !startTLSSupported {
			_ = client.Close()
			return nil, fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(config.tlsConfig()); err != nil {
			_ = client.Close()
			return nil, err
		}
	}

	return client, nil
}

// The following wrappers preserve the package-level helpers used by existing
// SMTP integrations and tests while production delivery works from immutable
// channel snapshots.
func generateMessageID() (string, error) {
	return generateMessageIDFor(primarySMTPChannel().fromAddress())
}

func generateMessageIDFor(from string) (string, error) {
	at := strings.LastIndex(from, "@")
	if at <= 0 || at == len(from)-1 {
		return "", fmt.Errorf("invalid SMTP from address")
	}
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), GetRandomString(12), from[at+1:]), nil
}

func shouldUseSMTPLoginAuth() bool {
	return primarySMTPChannel().shouldUseLoginAuth()
}

func getSMTPAuth() smtp.Auth {
	return primarySMTPChannel().auth()
}

func shouldAuthenticateSMTP() bool {
	return primarySMTPChannel().shouldAuthenticate()
}

func smtpTLSConfig() *tls.Config {
	return primarySMTPChannel().tlsConfig()
}

func newSMTPClient(addr string) (*smtp.Client, error) {
	return primarySMTPChannel().newClient(addr)
}

type EmailAttachment struct {
	Filename    string
	ContentType string
	Reader      io.Reader
}

type emailAttachmentData struct {
	filename    string
	contentType string
	content     []byte
}

func readEmailAttachments(attachments []EmailAttachment) ([]emailAttachmentData, error) {
	result := make([]emailAttachmentData, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Reader == nil {
			return nil, fmt.Errorf("email attachment reader is required")
		}
		filename := strings.TrimSpace(attachment.Filename)
		if filename == "" || strings.ContainsAny(filename, "\r\n") {
			return nil, fmt.Errorf("invalid email attachment filename")
		}
		contentType := strings.TrimSpace(attachment.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		content, err := io.ReadAll(attachment.Reader)
		if err != nil {
			return nil, err
		}
		result = append(result, emailAttachmentData{
			filename:    filename,
			contentType: contentType,
			content:     content,
		})
	}
	return result, nil
}

func SendEmail(subject string, receiver string, content string) error {
	_, err := SendEmailWithProfileResult(SMTPProfileNotification, subject, receiver, content)
	return err
}

func SendEmailWithResult(subject string, receiver string, content string) (SMTPDeliveryResult, error) {
	return SendEmailWithProfileResult(SMTPProfileNotification, subject, receiver, content)
}

func SendEmailWithProfile(profile string, subject string, receiver string, content string) error {
	_, err := SendEmailWithProfileResult(profile, subject, receiver, content)
	return err
}

func SendEmailWithProfileResult(profile string, subject string, receiver string, content string) (SMTPDeliveryResult, error) {
	return sendEmailWithAttachmentsProfile(profile, subject, receiver, content, nil)
}

// SendEmailViaChannel sends a message through exactly one SMTP channel. It is
// used by the settings test action, so the backup channel can be verified
// before it is activated for automatic failover.
func SendEmailViaChannel(subject string, receiver string, content string, channel string) (SMTPDeliveryResult, error) {
	var config smtpChannelConfig
	switch channel {
	case SMTPChannelPrimary:
		config = primarySMTPChannel()
	case SMTPChannelBackup:
		config = backupSMTPChannel()
	case SMTPChannelSecurity:
		config = securitySMTPChannel()
	case SMTPChannelMarketing:
		config = marketingSMTPChannel()
	default:
		return SMTPDeliveryResult{}, fmt.Errorf("invalid SMTP channel: %s", channel)
	}
	messageID, err := sendEmailThroughChannel(config, subject, receiver, content, nil)
	if err != nil {
		return SMTPDeliveryResult{}, err
	}
	return SMTPDeliveryResult{Profile: channelProfile(channel), Channel: channel, MessageID: messageID}, nil
}

func SendEmailWithAttachments(subject string, receiver string, content string, attachments []EmailAttachment) error {
	_, err := SendEmailWithAttachmentsProfileResult(SMTPProfileNotification, subject, receiver, content, attachments)
	return err
}

func SendEmailWithAttachmentsProfileResult(profile string, subject string, receiver string, content string, attachments []EmailAttachment) (SMTPDeliveryResult, error) {
	return sendEmailWithAttachmentsProfile(profile, subject, receiver, content, attachments)
}

func channelProfile(channel string) string {
	switch channel {
	case SMTPChannelSecurity:
		return SMTPProfileSecurity
	case SMTPChannelMarketing:
		return SMTPProfileMarketing
	default:
		return SMTPProfileNotification
	}
}

func sendEmailWithAttachmentsProfile(profile string, subject string, receiver string, content string, attachments []EmailAttachment) (SMTPDeliveryResult, error) {
	if profile != SMTPProfileNotification && profile != SMTPProfileSecurity && profile != SMTPProfileMarketing {
		return SMTPDeliveryResult{}, fmt.Errorf("invalid SMTP profile: %s", profile)
	}
	attachmentData, err := readEmailAttachments(attachments)
	if err != nil {
		return SMTPDeliveryResult{}, err
	}

	primary := smtpChannelForProfile(profile)
	messageID, primaryErr := sendEmailThroughChannel(primary, subject, receiver, content, attachmentData)
	if primaryErr == nil {
		return SMTPDeliveryResult{Profile: profile, Channel: primary.name, MessageID: messageID}, nil
	}

	// A dedicated sender may fail before SMTP accepts DATA. Fall back to the
	// default transactional sender, then its existing backup, so account access
	// and operational mail remain available during provider incidents.
	if primary.name != SMTPChannelPrimary {
		fallback := primarySMTPChannel()
		messageID, fallbackErr := sendEmailThroughChannel(fallback, subject, receiver, content, attachmentData)
		if fallbackErr == nil {
			SysError(fmt.Sprintf("%s SMTP profile failed; used default primary: %v", profile, primaryErr))
			return SMTPDeliveryResult{Profile: profile, Channel: SMTPChannelPrimary, MessageID: messageID}, nil
		}
		primaryErr = fmt.Errorf("dedicated %s SMTP failed: %v; default primary failed: %w", profile, primaryErr, fallbackErr)
	}

	{
		backup := backupSMTPChannel()
		if !backup.enabled {
			return SMTPDeliveryResult{}, fmt.Errorf("primary SMTP channel failed: %w", primaryErr)
		}
		SysError(fmt.Sprintf("primary SMTP channel failed; trying backup channel: %v", primaryErr))
		messageID, backupErr := sendEmailThroughChannel(backup, subject, receiver, content, attachmentData)
		if backupErr != nil {
			return SMTPDeliveryResult{}, fmt.Errorf("primary SMTP channel failed: %v; backup SMTP channel failed: %w", primaryErr, backupErr)
		}
		return SMTPDeliveryResult{Profile: profile, Channel: SMTPChannelBackup, MessageID: messageID}, nil
	}
}

func sendEmailThroughChannel(config smtpChannelConfig, subject string, receiver string, content string, attachments []emailAttachmentData) (string, error) {
	from := config.fromAddress()
	if strings.TrimSpace(config.server) == "" {
		return "", fmt.Errorf("SMTP server is not configured")
	}
	if config.port < 1 || config.port > 65535 {
		return "", fmt.Errorf("invalid SMTP port")
	}
	if from == "" {
		return "", fmt.Errorf("SMTP from address is not configured")
	}
	senderName := SystemNameOrDefault()
	if strings.ContainsAny(subject+receiver+from+senderName, "\r\n") {
		return "", fmt.Errorf("invalid email header")
	}
	messageID, err := generateMessageIDFor(from)
	if err != nil {
		return "", err
	}
	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))
	var message bytes.Buffer
	if err := writeEmailMessageData(&message, messageID, encodedSubject, receiver, content, attachments, senderName, from); err != nil {
		return "", err
	}

	addr := net.JoinHostPort(config.server, fmt.Sprintf("%d", config.port))
	client, err := config.newClient(addr)
	if err != nil {
		return "", err
	}
	defer client.Close()
	if config.shouldAuthenticate() {
		if err = client.Auth(config.auth()); err != nil {
			return "", err
		}
	}
	if err = client.Mail(from); err != nil {
		return "", err
	}
	for _, recipient := range strings.Split(receiver, ";") {
		if err = client.Rcpt(strings.TrimSpace(recipient)); err != nil {
			return "", err
		}
	}
	w, err := client.Data()
	if err != nil {
		return "", err
	}
	if _, err = w.Write(message.Bytes()); err != nil {
		return "", err
	}
	if err = w.Close(); err != nil {
		return "", err
	}
	if err = client.Quit(); err != nil {
		SysError(fmt.Sprintf("SMTP QUIT failed after email delivery through %s channel: %v", config.name, err))
		// DATA was already accepted by the server. Treat a failed QUIT as a
		// connection-close problem so workers do not resend the same message.
		return messageID, nil
	}
	return messageID, nil
}

func writeEmailMessage(w io.Writer, messageID string, encodedSubject string, receiver string, content string, attachments []EmailAttachment, senderName string) error {
	attachmentData, err := readEmailAttachments(attachments)
	if err != nil {
		return err
	}
	return writeEmailMessageData(w, messageID, encodedSubject, receiver, content, attachmentData, senderName, primarySMTPChannel().fromAddress())
}

func writeEmailMessageData(w io.Writer, messageID string, encodedSubject string, receiver string, content string, attachments []emailAttachmentData, senderName string, from string) error {
	sender := (&mail.Address{Name: senderName, Address: from}).String()
	if len(attachments) == 0 {
		alternative := multipart.NewWriter(w)
		if _, err := fmt.Fprintf(w, "To: %s\r\nFrom: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n",
			receiver, sender, encodedSubject, time.Now().Format(time.RFC1123Z), messageID, alternative.Boundary()); err != nil {
			return err
		}
		if err := writeEmailAlternativeParts(alternative, content); err != nil {
			return err
		}
		return alternative.Close()
	}

	mixed := multipart.NewWriter(w)
	if _, err := fmt.Fprintf(w, "To: %s\r\nFrom: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=%q\r\n\r\n",
		receiver, sender, encodedSubject, time.Now().Format(time.RFC1123Z), messageID, mixed.Boundary()); err != nil {
		return err
	}
	var alternativeBody bytes.Buffer
	alternative := multipart.NewWriter(&alternativeBody)
	alternativeBoundary := alternative.Boundary()
	if err := writeEmailAlternativeParts(alternative, content); err != nil {
		return err
	}
	if err := alternative.Close(); err != nil {
		return err
	}
	bodyHeader := make(textproto.MIMEHeader)
	bodyHeader.Set("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", alternativeBoundary))
	bodyPart, err := mixed.CreatePart(bodyHeader)
	if err != nil {
		return err
	}
	if _, err := bodyPart.Write(alternativeBody.Bytes()); err != nil {
		return err
	}

	for _, attachment := range attachments {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Type", attachment.contentType+"; name="+mime.QEncoding.Encode("UTF-8", attachment.filename))
		header.Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(attachment.filename))
		header.Set("Content-Transfer-Encoding", "base64")
		part, err := mixed.CreatePart(header)
		if err != nil {
			return err
		}
		encoder := base64.NewEncoder(base64.StdEncoding, &mimeLineWriter{writer: part})
		if _, err := encoder.Write(attachment.content); err != nil {
			_ = encoder.Close()
			return err
		}
		if err := encoder.Close(); err != nil {
			return err
		}
		if _, err := io.WriteString(part, "\r\n"); err != nil {
			return err
		}
	}
	return mixed.Close()
}

var (
	emailBreakPattern = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>|</tr>|</li>|</h[1-6]>`)
	emailTagPattern   = regexp.MustCompile(`(?s)<[^>]*>`)
	emailSpacePattern = regexp.MustCompile(`[ \t\r\f\v]+`)
	emailBlankPattern = regexp.MustCompile(`\n{3,}`)
)

func emailPlainText(content string) string {
	plain := emailBreakPattern.ReplaceAllString(content, "\n")
	plain = emailTagPattern.ReplaceAllString(plain, "")
	plain = html.UnescapeString(plain)
	plain = emailSpacePattern.ReplaceAllString(plain, " ")
	plain = emailBlankPattern.ReplaceAllString(plain, "\n\n")
	return strings.TrimSpace(plain)
}

func writeEmailAlternativeParts(writer *multipart.Writer, content string) error {
	plainHeader := make(textproto.MIMEHeader)
	plainHeader.Set("Content-Type", "text/plain; charset=UTF-8")
	plainHeader.Set("Content-Transfer-Encoding", "8bit")
	plainPart, err := writer.CreatePart(plainHeader)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(plainPart, emailPlainText(content)+"\r\n"); err != nil {
		return err
	}

	htmlHeader := make(textproto.MIMEHeader)
	htmlHeader.Set("Content-Type", "text/html; charset=UTF-8")
	htmlHeader.Set("Content-Transfer-Encoding", "8bit")
	htmlPart, err := writer.CreatePart(htmlHeader)
	if err != nil {
		return err
	}
	_, err = io.WriteString(htmlPart, content+"\r\n")
	return err
}

type mimeLineWriter struct {
	writer io.Writer
	column int
}

func (w *mimeLineWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		remaining := 76 - w.column
		if remaining == 0 {
			if _, err := io.WriteString(w.writer, "\r\n"); err != nil {
				return written, err
			}
			w.column = 0
			remaining = 76
		}
		count := min(remaining, len(p))
		if _, err := w.writer.Write(p[:count]); err != nil {
			return written, err
		}
		written += count
		w.column += count
		p = p[count:]
	}
	return written, nil
}

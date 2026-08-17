package common

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	SMTPChannelPrimary = "primary"
	SMTPChannelBackup  = "backup"
)

type SMTPDeliveryResult struct {
	Channel string `json:"channel"`
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
	_, err := SendEmailWithResult(subject, receiver, content)
	return err
}

func SendEmailWithResult(subject string, receiver string, content string) (SMTPDeliveryResult, error) {
	return sendEmailWithAttachments(subject, receiver, content, nil)
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
	default:
		return SMTPDeliveryResult{}, fmt.Errorf("invalid SMTP channel: %s", channel)
	}
	if err := sendEmailThroughChannel(config, subject, receiver, content, nil); err != nil {
		return SMTPDeliveryResult{}, err
	}
	return SMTPDeliveryResult{Channel: channel}, nil
}

func SendEmailWithAttachments(subject string, receiver string, content string, attachments []EmailAttachment) error {
	_, err := sendEmailWithAttachments(subject, receiver, content, attachments)
	return err
}

func sendEmailWithAttachments(subject string, receiver string, content string, attachments []EmailAttachment) (SMTPDeliveryResult, error) {
	attachmentData, err := readEmailAttachments(attachments)
	if err != nil {
		return SMTPDeliveryResult{}, err
	}

	primary := primarySMTPChannel()
	if err := sendEmailThroughChannel(primary, subject, receiver, content, attachmentData); err == nil {
		return SMTPDeliveryResult{Channel: SMTPChannelPrimary}, nil
	} else {
		backup := backupSMTPChannel()
		if !backup.enabled {
			return SMTPDeliveryResult{}, fmt.Errorf("primary SMTP channel failed: %w", err)
		}
		SysError(fmt.Sprintf("primary SMTP channel failed; trying backup channel: %v", err))
		if backupErr := sendEmailThroughChannel(backup, subject, receiver, content, attachmentData); backupErr != nil {
			return SMTPDeliveryResult{}, fmt.Errorf("primary SMTP channel failed: %v; backup SMTP channel failed: %w", err, backupErr)
		}
	}
	return SMTPDeliveryResult{Channel: SMTPChannelBackup}, nil
}

func sendEmailThroughChannel(config smtpChannelConfig, subject string, receiver string, content string, attachments []emailAttachmentData) error {
	from := config.fromAddress()
	if strings.TrimSpace(config.server) == "" {
		return fmt.Errorf("SMTP server is not configured")
	}
	if config.port < 1 || config.port > 65535 {
		return fmt.Errorf("invalid SMTP port")
	}
	if from == "" {
		return fmt.Errorf("SMTP from address is not configured")
	}
	senderName := SystemNameOrDefault()
	if strings.ContainsAny(subject+receiver+from+senderName, "\r\n") {
		return fmt.Errorf("invalid email header")
	}
	messageID, err := generateMessageIDFor(from)
	if err != nil {
		return err
	}
	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))
	var message bytes.Buffer
	if err := writeEmailMessageData(&message, messageID, encodedSubject, receiver, content, attachments, senderName, from); err != nil {
		return err
	}

	addr := net.JoinHostPort(config.server, fmt.Sprintf("%d", config.port))
	client, err := config.newClient(addr)
	if err != nil {
		return err
	}
	defer client.Close()
	if config.shouldAuthenticate() {
		if err = client.Auth(config.auth()); err != nil {
			return err
		}
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range strings.Split(receiver, ";") {
		if err = client.Rcpt(strings.TrimSpace(recipient)); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(message.Bytes()); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	if err = client.Quit(); err != nil {
		SysError(fmt.Sprintf("SMTP QUIT failed after email delivery through %s channel: %v", config.name, err))
		// DATA was already accepted by the server. Treat a failed QUIT as a
		// connection-close problem so workers do not resend the same message.
		return nil
	}
	return nil
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
		_, err := fmt.Fprintf(w, "To: %s\r\nFrom: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n",
			receiver, sender, encodedSubject, time.Now().Format(time.RFC1123Z), messageID, content)
		return err
	}

	mixed := multipart.NewWriter(w)
	if _, err := fmt.Fprintf(w, "To: %s\r\nFrom: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=%q\r\n\r\n",
		receiver, sender, encodedSubject, time.Now().Format(time.RFC1123Z), messageID, mixed.Boundary()); err != nil {
		return err
	}
	bodyHeader := make(textproto.MIMEHeader)
	bodyHeader.Set("Content-Type", "text/html; charset=UTF-8")
	bodyHeader.Set("Content-Transfer-Encoding", "8bit")
	bodyPart, err := mixed.CreatePart(bodyHeader)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(bodyPart, content); err != nil {
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

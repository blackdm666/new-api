package common

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"slices"
	"strings"
	"time"
)

func generateMessageID() (string, error) {
	split := strings.Split(SMTPFrom, "@")
	if len(split) < 2 {
		return "", fmt.Errorf("invalid SMTP account")
	}
	domain := strings.Split(SMTPFrom, "@")[1]
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), GetRandomString(12), domain), nil
}

func shouldUseSMTPLoginAuth() bool {
	if SMTPForceAuthLogin {
		return true
	}
	return isOutlookServer(SMTPAccount) || slices.Contains(EmailLoginAuthServerList, SMTPServer)
}

func getSMTPAuth() smtp.Auth {
	return AutoSMTPAuth(SMTPAccount, SMTPToken)
}

func shouldAuthenticateSMTP() bool {
	return SMTPAccount != "" && SMTPToken != ""
}

func smtpTLSConfig() *tls.Config {
	return &tls.Config{
		ServerName:         SMTPServer,
		InsecureSkipVerify: SMTPInsecureSkipVerify, // #nosec G402 -- admin-controlled SMTP compatibility option.
	}
}

func newSMTPClient(addr string) (*smtp.Client, error) {
	if SMTPSSLEnabled || (SMTPPort == 465 && !SMTPStartTLSEnabled) {
		conn, err := tls.Dial("tcp", addr, smtpTLSConfig())
		if err != nil {
			return nil, err
		}
		client, err := smtp.NewClient(conn, SMTPServer)
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

	if SMTPStartTLSEnabled {
		startTLSSupported, _ := client.Extension("STARTTLS")
		if !startTLSSupported {
			_ = client.Close()
			return nil, fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(smtpTLSConfig()); err != nil {
			_ = client.Close()
			return nil, err
		}
	}

	return client, nil
}

type EmailAttachment struct {
	Filename    string
	ContentType string
	Reader      io.Reader
}

func SendEmail(subject string, receiver string, content string) error {
	return SendEmailWithAttachments(subject, receiver, content, nil)
}

func SendEmailWithAttachments(subject string, receiver string, content string, attachments []EmailAttachment) error {
	if SMTPFrom == "" { // for compatibility
		SMTPFrom = SMTPAccount
	}
	id, err2 := generateMessageID()
	if err2 != nil {
		return err2
	}
	if SMTPServer == "" && SMTPAccount == "" {
		return fmt.Errorf("SMTP 服务器未配置")
	}
	senderName := SystemNameOrDefault()
	if strings.ContainsAny(subject+receiver+SMTPFrom+senderName, "\r\n") {
		return fmt.Errorf("invalid email header")
	}
	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))
	auth := getSMTPAuth()
	addr := fmt.Sprintf("%s:%d", SMTPServer, SMTPPort)
	to := strings.Split(receiver, ";")
	var err error
	client, err := newSMTPClient(addr)
	if err != nil {
		return err
	}
	defer client.Close()
	if shouldAuthenticateSMTP() {
		if err = client.Auth(auth); err != nil {
			return err
		}
	}
	if err = client.Mail(SMTPFrom); err != nil {
		return err
	}
	for _, receiver := range to {
		if err = client.Rcpt(receiver); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if err = writeEmailMessage(w, id, encodedSubject, receiver, content, attachments, senderName); err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	err = client.Quit()
	if err != nil {
		SysError(fmt.Sprintf("SMTP QUIT failed after email delivery to %s: %v", receiver, err))
		// DATA was already accepted by the server. Treat a failed QUIT as a
		// connection-close problem so notification workers do not resend the
		// same invoice and create duplicate email deliveries.
		return nil
	}
	return nil
}

func writeEmailMessage(w io.Writer, messageID string, encodedSubject string, receiver string, content string, attachments []EmailAttachment, senderName string) error {
	sender := (&mail.Address{Name: senderName, Address: SMTPFrom}).String()
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
		if attachment.Reader == nil {
			return fmt.Errorf("email attachment reader is required")
		}
		filename := strings.TrimSpace(attachment.Filename)
		if filename == "" || strings.ContainsAny(filename, "\r\n") {
			return fmt.Errorf("invalid email attachment filename")
		}
		contentType := strings.TrimSpace(attachment.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Type", contentType+"; name="+mime.QEncoding.Encode("UTF-8", filename))
		header.Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename))
		header.Set("Content-Transfer-Encoding", "base64")
		part, err := mixed.CreatePart(header)
		if err != nil {
			return err
		}
		encoder := base64.NewEncoder(base64.StdEncoding, &mimeLineWriter{writer: part})
		if _, err := io.Copy(encoder, attachment.Reader); err != nil {
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

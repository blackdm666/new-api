package service

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEmailCategoriesRouteToIsolatedSMTPProfiles(t *testing.T) {
	tests := map[string]string{
		"email_verification":   common.SMTPProfileSecurity,
		"password_reset":       common.SMTPProfileSecurity,
		"user_notice":          common.SMTPProfileSecurity,
		"quota_warning_user":   common.SMTPProfileNotification,
		"invoice_issued_user":  common.SMTPProfileNotification,
		"channel_status_admin": common.SMTPProfileNotification,
		"marketing_custom":     common.SMTPProfileMarketing,
		"email_preview":        common.SMTPProfileMarketing,
	}
	for category, expected := range tests {
		assert.Equal(t, expected, smtpProfileForCategory(category), category)
	}
}

func setupUserNoticeDeliveryTest(t *testing.T) *model.EmailDelivery {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.EmailDelivery{}, &model.MarketingSuppression{}, &model.EmailDeliveryMinuteQuota{}))
	prior := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = prior
		conn, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, conn.Close())
	})
	user := model.User{Username: "notice-user", Password: "unused", Email: "recipient@example.invalid", Status: common.UserStatusDisabled}
	require.NoError(t, db.Create(&user).Error)
	delivery, _, err := model.EnqueueEmailDelivery(&model.EmailDelivery{
		DeliveryKey: "notice-fixture", Category: model.UserNoticeCategory, SMTPProfile: common.SMTPProfileSecurity,
		UserId: user.Id, Recipient: user.Email, Subject: "Usage reminder", Body: "<p>Please review retries.</p>", Priority: 50,
	})
	require.NoError(t, err)
	return delivery
}

func TestUserNoticeDeliveryRateLimitAndVerificationPriority(t *testing.T) {
	delivery := setupUserNoticeDeliveryTest(t)
	now := int64(1800000000)
	critical, _, err := model.EnqueueEmailDelivery(&model.EmailDelivery{
		DeliveryKey: "code-fixture", Category: "email_verification", Recipient: "code@example.invalid", Subject: "Code", Body: "123456", Priority: model.EmailPriorityCritical,
	})
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(critical).Update("next_attempt_time", now).Error)
	allowed, err := userNoticeDeliveryAllowed(delivery, now)
	require.NoError(t, err)
	assert.False(t, allowed, "queued verification takes precedence")
	require.NoError(t, model.DB.Delete(critical).Error)
	for range model.UserNoticePerMinute {
		allowed, err = userNoticeDeliveryAllowed(delivery, now)
		require.NoError(t, err)
		assert.True(t, allowed)
	}
	allowed, err = userNoticeDeliveryAllowed(delivery, now)
	require.NoError(t, err)
	assert.False(t, allowed, "sixth notice attempt must wait for another minute")
	allowed, err = userNoticeDeliveryAllowed(delivery, now+60)
	require.NoError(t, err)
	assert.True(t, allowed)
	require.NoError(t, model.CreateMarketingSuppression(delivery.UserId, delivery.Recipient, "permanent SMTP rejection", 1))
	allowed, err = userNoticeDeliveryAllowed(delivery, now+60)
	require.NoError(t, err)
	assert.False(t, allowed, "new restrictions also apply to queued retries")
	saved, err := model.GetEmailDeliveryById(delivery.Id)
	require.NoError(t, err)
	assert.Equal(t, model.EmailDeliveryStatusFailed, saved.State)
	assert.Contains(t, saved.LastError, "delivery restricted")
}

func TestUserNoticeDeliveryUsesSecuritySMTPAndRecordsAcceptance(t *testing.T) {
	delivery := setupUserNoticeDeliveryTest(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	messages := make(chan string, 1)
	serverErrors := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErrors <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
		_, _ = rw.WriteString("220 fixture ESMTP\r\n")
		_ = rw.Flush()
		for {
			line, err := rw.ReadString('\n')
			if err != nil {
				serverErrors <- err
				return
			}
			command := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
				_, _ = rw.WriteString("250 fixture\r\n")
			case strings.HasPrefix(command, "MAIL FROM"), strings.HasPrefix(command, "RCPT TO"):
				_, _ = rw.WriteString("250 OK\r\n")
			case command == "DATA":
				_, _ = rw.WriteString("354 continue\r\n")
				_ = rw.Flush()
				var body strings.Builder
				for {
					part, err := rw.ReadString('\n')
					if err != nil {
						serverErrors <- err
						return
					}
					if part == ".\r\n" {
						break
					}
					body.WriteString(part)
				}
				messages <- body.String()
				_, _ = rw.WriteString("250 accepted\r\n")
			case command == "QUIT":
				_, _ = rw.WriteString("221 bye\r\n")
				_ = rw.Flush()
				return
			default:
				serverErrors <- fmt.Errorf("unexpected SMTP command: %s", command)
				return
			}
			_ = rw.Flush()
		}
	}()
	enabled, server, port := common.SMTPSecurityEnabled, common.SMTPSecurityServer, common.SMTPSecurityPort
	account, from, token := common.SMTPSecurityAccount, common.SMTPSecurityFrom, common.SMTPSecurityToken
	ssl, startTLS := common.SMTPSecuritySSLEnabled, common.SMTPSecurityStartTLSEnabled
	common.SMTPSecurityEnabled, common.SMTPSecurityServer, common.SMTPSecurityPort = true, "127.0.0.1", listener.Addr().(*net.TCPAddr).Port
	common.SMTPSecurityAccount, common.SMTPSecurityFrom, common.SMTPSecurityToken = "", "security@example.invalid", ""
	common.SMTPSecuritySSLEnabled, common.SMTPSecurityStartTLSEnabled = false, false
	t.Cleanup(func() {
		common.SMTPSecurityEnabled, common.SMTPSecurityServer, common.SMTPSecurityPort = enabled, server, port
		common.SMTPSecurityAccount, common.SMTPSecurityFrom, common.SMTPSecurityToken = account, from, token
		common.SMTPSecuritySSLEnabled, common.SMTPSecurityStartTLSEnabled = ssl, startTLS
	})
	deliverSystemEmail(delivery)
	saved, err := model.GetEmailDeliveryById(delivery.Id)
	require.NoError(t, err)
	assert.Equal(t, common.SMTPProfileSecurity, saved.SMTPProfile)
	assert.Equal(t, common.SMTPChannelSecurity, saved.SMTPChannel)
	assert.Equal(t, model.EmailDeliveryStatusAcceptedUntracked, saved.State)
	assert.NotEmpty(t, saved.MessageID)
	select {
	case message := <-messages:
		assert.Contains(t, message, "security@example.invalid")
		assert.Contains(t, message, "recipient@example.invalid")
	case err := <-serverErrors:
		require.NoError(t, err)
	default:
		t.Fatal("SMTP sink did not accept the notice")
	}
	// Reprocessing an accepted outbox row must not contact SMTP again.
	deliverSystemEmail(delivery)
	assert.Empty(t, messages)
}

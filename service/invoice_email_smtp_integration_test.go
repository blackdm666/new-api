//go:build integration

package service_test

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInvoiceEmailTemplatesSMTPIntegration sends the three invoice email
// scenarios through the configured NewAPI SMTP account. It is intentionally
// opt-in so normal test runs never send real email.
func TestInvoiceEmailTemplatesSMTPIntegration(t *testing.T) {
	recipient := strings.TrimSpace(os.Getenv("INVOICE_SMTP_TEST_RECIPIENT"))
	if recipient == "" {
		t.Skip("set INVOICE_SMTP_TEST_RECIPIENT to run the SMTP integration test")
	}

	common.InitEnv()
	require.NoError(t, model.InitDB())
	model.InitOptionMap()
	require.NotEmpty(t, common.SMTPServer)
	require.NotEmpty(t, common.SMTPFrom)

	common.InvoiceApplicationNotifyAdminEnabled = true
	common.InvoiceIssuedNotifyUserEnabled = true
	common.InvoiceAdminEmail = recipient

	pending := &model.InvoiceRequest{
		Id:           1024,
		UserId:       7,
		Username:     "alice",
		CompanyName:  "示例科技有限公司",
		TaxNumber:    "91310000EXAMPLE",
		TotalMoney:   701.25,
		Status:       model.InvoiceStatusPending,
		CreatedTime:  1_787_000_000,
		OrderNumbers: "INV-202608-001",
	}
	issued := *pending
	issued.Status = model.InvoiceStatusIssued
	user := &model.User{Id: 7, Email: recipient}
	user.SetSetting(dto.UserSetting{Language: appI18n.LangZhCN})

	createdDeliveries, err := service.BuildInvoiceRequestCreatedNotifications(pending)
	require.NoError(t, err)
	require.Len(t, createdDeliveries, 1)
	expiryDeliveries, err := service.BuildInvoiceExpiryWarningNotifications(pending)
	require.NoError(t, err)
	require.Len(t, expiryDeliveries, 1)
	issuedDeliveries, err := service.BuildInvoiceIssuedNotifications(&issued, model.InvoiceStatusPending, user)
	require.NoError(t, err)
	require.Len(t, issuedDeliveries, 1)

	tests := []struct {
		name       string
		delivery   *model.InvoiceNotificationDelivery
		attachment bool
	}{
		{name: "invoice_request_admin", delivery: createdDeliveries[0]},
		{name: "invoice_issued_user", delivery: issuedDeliveries[0], attachment: true},
		{name: "invoice_expiry_admin", delivery: expiryDeliveries[0]},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.NotContains(t, test.delivery.Subject, "{{")
			assert.NotContains(t, test.delivery.Body, "{{")
			var sendErr error
			if test.attachment {
				sendErr = common.SendEmailWithAttachments(
					"[模板实测] "+test.delivery.Subject,
					recipient,
					test.delivery.Body,
					[]common.EmailAttachment{{
						Filename:    "invoice-template-test.pdf",
						ContentType: "application/pdf",
						Reader:      strings.NewReader("%PDF-1.4\n% NewAPI invoice email template integration test\n%%EOF\n"),
					}},
				)
			} else {
				sendErr = common.SendEmail("[模板实测] "+test.delivery.Subject, recipient, test.delivery.Body)
			}
			require.NoError(t, sendErr)
		})
	}
}

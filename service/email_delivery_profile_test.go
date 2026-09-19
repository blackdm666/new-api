package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestEmailCategoriesRouteToIsolatedSMTPProfiles(t *testing.T) {
	tests := map[string]string{
		"email_verification":   common.SMTPProfileSecurity,
		"password_reset":       common.SMTPProfileSecurity,
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

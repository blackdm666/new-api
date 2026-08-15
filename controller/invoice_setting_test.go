package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validInvoiceSettingsPayload() InvoiceSettingsPayload {
	return InvoiceSettingsPayload{
		InvoiceApplicationNotifyAdminEnabled: true,
		InvoiceIssuedNotifyUserEnabled:       true,
		InvoiceAdminEmail:                    "billing@example.com",
		InvoiceMinimumAmountCents:            50000,
		InvoiceDataRetentionDays:             0,
		InvoiceFileEnabled:                   true,
		InvoiceFileStorage:                   "local",
		InvoiceFileMaxSize:                   5 * 1024 * 1024,
		InvoiceFileMaxCount:                  5,
		InvoiceFileAllowedExts:               "jpg,jpeg,png,webp,pdf",
		InvoiceFileLocalPath:                 "/data/invoice_files",
		InvoiceFileSignedURLTTL:              900,
	}
}

func TestValidateInvoiceSettingsRejectsUnsafeOrIncompleteStorage(t *testing.T) {
	payload := validInvoiceSettingsPayload()
	payload.InvoiceFileMaxCount = 0
	require.Error(t, validateInvoiceSettingsPayload(&payload))

	payload = validInvoiceSettingsPayload()
	payload.InvoiceFileStorage = "s3"
	payload.InvoiceFileS3Bucket = "invoice-bucket"
	payload.InvoiceFileS3Region = "auto"
	payload.InvoiceFileS3AccessKeyId = "access-key"
	payload.InvoiceFileS3AccessKeySecret = "secret-key"
	payload.InvoiceFileS3CustomDomain = "https://files.example.com"
	err := validateInvoiceSettingsPayload(&payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "custom domain")
}

func TestValidateInvoiceSettingsNormalizesAllowedExtensions(t *testing.T) {
	payload := validInvoiceSettingsPayload()
	payload.InvoiceFileAllowedExts = " .PDF, png,PDF,jpg "
	require.NoError(t, validateInvoiceSettingsPayload(&payload))
	assert.Equal(t, "pdf,png,jpg", payload.InvoiceFileAllowedExts)
	assert.Equal(t, "application/pdf,image/png,image/jpeg", payload.InvoiceFileAllowedMimes)
}

func TestValidateInvoiceSettingsRetentionRange(t *testing.T) {
	payload := validInvoiceSettingsPayload()
	payload.InvoiceDataRetentionDays = 1
	require.Error(t, validateInvoiceSettingsPayload(&payload))

	payload = validInvoiceSettingsPayload()
	payload.InvoiceDataRetentionDays = 30
	require.NoError(t, validateInvoiceSettingsPayload(&payload))
}

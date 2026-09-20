package invoicefile

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3StorageRejectsCustomDomainThatWouldInvalidateSignature(t *testing.T) {
	previousBucket := setting.InvoiceFileS3Bucket
	previousRegion := setting.InvoiceFileS3Region
	previousAccessKey := setting.InvoiceFileS3AccessKeyId
	previousSecretKey := setting.InvoiceFileS3AccessKeySecret
	previousCustomDomain := setting.InvoiceFileS3CustomDomain
	t.Cleanup(func() {
		setting.InvoiceFileS3Bucket = previousBucket
		setting.InvoiceFileS3Region = previousRegion
		setting.InvoiceFileS3AccessKeyId = previousAccessKey
		setting.InvoiceFileS3AccessKeySecret = previousSecretKey
		setting.InvoiceFileS3CustomDomain = previousCustomDomain
	})
	setting.InvoiceFileS3Bucket = "invoice-bucket"
	setting.InvoiceFileS3Region = "auto"
	setting.InvoiceFileS3AccessKeyId = "test-access-key"
	setting.InvoiceFileS3AccessKeySecret = "test-secret-key"
	setting.InvoiceFileS3CustomDomain = "https://files.example.com"

	_, err := NewS3Storage()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrS3CustomDomainUnsupported))
}

func TestResponseContentDispositionSupportsPreviewAndDownload(t *testing.T) {
	assert.Equal(t, "attachment; filename*=UTF-8''invoice%20file.pdf", ResponseContentDisposition("invoice file.pdf", false))
	assert.Equal(t, "inline; filename*=UTF-8''invoice%20file.pdf", ResponseContentDisposition("invoice file.pdf", true))
	assert.Empty(t, ResponseContentDisposition("", true))
}

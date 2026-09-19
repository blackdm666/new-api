package invoicefile

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCOSCustomDomainParticipatesInPresignedURL(t *testing.T) {
	storageValue, err := newCOSStorage(Config{
		StorageType:  "cos",
		Bucket:       "invoice-1234567890",
		Region:       "ap-shanghai",
		AccessKeyId:  "test-secret-id",
		AccessSecret: "test-secret-key",
		CustomDomain: "https://files.example.com",
	})
	require.NoError(t, err)

	signed, err := storageValue.SignedURL(context.Background(), "2026/08/invoice.pdf", 5*time.Minute, "invoice.pdf", false)
	require.NoError(t, err)
	parsed, err := url.Parse(signed)
	require.NoError(t, err)
	assert.Equal(t, "files.example.com", parsed.Host)
	assert.NotEmpty(t, parsed.Query().Get("q-signature"))
	assert.Equal(t, "host", parsed.Query().Get("q-header-list"))
}

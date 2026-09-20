package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSensitiveValueRoundTripAndPurposeIsolation(t *testing.T) {
	original := CryptoSecret
	CryptoSecret = "stable-test-secret"
	t.Cleanup(func() { CryptoSecret = original })

	encoded, err := EncryptSensitiveValue("invoice-storage-profile", []byte("secret payload"))
	require.NoError(t, err)
	require.NotContains(t, encoded, "secret payload")

	decoded, err := DecryptSensitiveValue("invoice-storage-profile", encoded)
	require.NoError(t, err)
	require.Equal(t, "secret payload", string(decoded))

	_, err = DecryptSensitiveValue("another-purpose", encoded)
	require.Error(t, err)
}

package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const sensitiveValuePrefix = "v1:"

func sensitiveValueKey(purpose string) [32]byte {
	return sha256.Sum256([]byte("new-api:sensitive:" + purpose + ":" + CryptoSecret))
}

// EncryptSensitiveValue encrypts application-managed credentials before they
// are copied into immutable history records. CRYPTO_SECRET must stay stable
// across deployments or historical records cannot be decrypted.
func EncryptSensitiveValue(purpose string, plaintext []byte) (string, error) {
	key := sensitiveValueKey(purpose)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, []byte(purpose))
	return sensitiveValuePrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func DecryptSensitiveValue(purpose string, encoded string) ([]byte, error) {
	if !strings.HasPrefix(encoded, sensitiveValuePrefix) {
		return nil, errors.New("unsupported encrypted value version")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, sensitiveValuePrefix))
	if err != nil {
		return nil, fmt.Errorf("decode encrypted value: %w", err)
	}
	key := sensitiveValueKey(purpose)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, errors.New("encrypted value is truncated")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(purpose))
	if err != nil {
		return nil, fmt.Errorf("decrypt encrypted value: %w", err)
	}
	return plaintext, nil
}

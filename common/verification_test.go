package common

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useMemoryVerificationStore(t *testing.T) {
	t.Helper()
	previousRedisEnabled := RedisEnabled
	RedisEnabled = false
	verificationMutex.Lock()
	verificationMap = make(map[string]verificationValue)
	verificationCooldownMap = make(map[string]time.Time)
	verificationMutex.Unlock()
	t.Cleanup(func() { RedisEnabled = previousRedisEnabled })
}

func TestEmailVerificationCodesAreNumericAndPreviousCodeSurvivesOneResend(t *testing.T) {
	useMemoryVerificationStore(t)
	first := GenerateVerificationCode(6)
	second := GenerateVerificationCode(6)
	require.True(t, regexp.MustCompile(`^\d{6}$`).MatchString(first))
	require.True(t, regexp.MustCompile(`^\d{6}$`).MatchString(second))

	RegisterVerificationCodeWithKey("User@Example.com", first, EmailVerificationPurpose)
	RegisterVerificationCodeWithKey("user@example.com", second, EmailVerificationPurpose)
	assert.True(t, VerifyCodeWithKey("user@example.com", first, EmailVerificationPurpose))
	assert.True(t, VerifyCodeWithKey("user@example.com", second, EmailVerificationPurpose))

	DeleteKey("user@example.com", EmailVerificationPurpose)
	assert.False(t, VerifyCodeWithKey("user@example.com", second, EmailVerificationPurpose))
}

func TestVerificationSendCooldownRequiresReleaseBeforeAnotherImmediateSend(t *testing.T) {
	useMemoryVerificationStore(t)
	allowed, err := ReserveVerificationSend("user@example.com", EmailVerificationPurpose, time.Minute)
	require.NoError(t, err)
	assert.True(t, allowed)
	allowed, err = ReserveVerificationSend("USER@example.com", EmailVerificationPurpose, time.Minute)
	require.NoError(t, err)
	assert.False(t, allowed)

	ReleaseVerificationSend("user@example.com", EmailVerificationPurpose)
	allowed, err = ReserveVerificationSend("user@example.com", EmailVerificationPurpose, time.Minute)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestPreviousEmailVerificationCodeRemainsValidForFiveMinutes(t *testing.T) {
	useMemoryVerificationStore(t)
	first := "123456"
	second := "654321"
	RegisterVerificationCodeWithKey("user@example.com", first, EmailVerificationPurpose)
	RegisterVerificationCodeWithKey("user@example.com", second, EmailVerificationPurpose)

	mapKey := normalizedVerificationKey("user@example.com", EmailVerificationPurpose)
	verificationMutex.Lock()
	value := verificationMap[mapKey]
	value.previousTime = time.Now().Add(-4*time.Minute - 59*time.Second)
	verificationMap[mapKey] = value
	verificationMutex.Unlock()
	assert.True(t, VerifyCodeWithKey("user@example.com", first, EmailVerificationPurpose))

	verificationMutex.Lock()
	value = verificationMap[mapKey]
	value.previousTime = time.Now().Add(-5*time.Minute - time.Second)
	verificationMap[mapKey] = value
	verificationMutex.Unlock()
	assert.False(t, VerifyCodeWithKey("user@example.com", first, EmailVerificationPurpose))
}

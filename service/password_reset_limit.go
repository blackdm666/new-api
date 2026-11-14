package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Password reset email budget (OWASP Forgot Password Cheat Sheet): the
// per-IP CriticalRateLimit does not stop an attacker who rotates proxies from
// flooding one mailbox with reset emails, so each address may only receive
// PasswordResetEmailLimit reset emails per PasswordResetEmailWindow. Requests
// over the budget get the same success response and simply send nothing, so
// the endpoint still does not reveal whether an address is registered.
var (
	PasswordResetEmailLimit  = common.GetEnvOrDefault("PASSWORD_RESET_EMAIL_LIMIT", 3)
	PasswordResetEmailWindow = time.Duration(common.GetEnvOrDefault("PASSWORD_RESET_EMAIL_WINDOW_MINUTES", 60)) * time.Minute
)

const passwordResetLimitRedisPrefix = "password_reset_limit:"

// consumePasswordResetBudgetScript increments the counter, starts the window
// on the first request, and returns the new count.
const consumePasswordResetBudgetScript = `
local count = redis.call('INCR', KEYS[1])
if redis.call('TTL', KEYS[1]) < 0 then
  redis.call('EXPIRE', KEYS[1], tonumber(ARGV[1]))
end
return count
`

type passwordResetBudgetEntry struct {
	count     int
	windowEnd time.Time
}

var passwordResetBudgetMemory = struct {
	sync.Mutex
	entries map[string]*passwordResetBudgetEntry
}{entries: map[string]*passwordResetBudgetEntry{}}

func passwordResetLimitEnabled() bool {
	return PasswordResetEmailLimit > 0 && PasswordResetEmailWindow > 0
}

// passwordResetBudgetKey hashes the address so emails are not stored in Redis keys.
func passwordResetBudgetKey(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:16])
}

// ConsumePasswordResetEmailBudget counts one reset request for the address and
// reports whether a reset email may still be sent. Redis errors fail open so a
// cache outage cannot stop legitimate password resets.
func ConsumePasswordResetEmailBudget(email string) bool {
	if !passwordResetLimitEnabled() {
		return true
	}
	key := passwordResetBudgetKey(email)
	if common.RedisEnabled && common.RDB != nil {
		count, err := common.RDB.Eval(
			context.Background(),
			consumePasswordResetBudgetScript,
			[]string{passwordResetLimitRedisPrefix + key},
			int64(PasswordResetEmailWindow/time.Second),
		).Int()
		if err != nil {
			common.SysError(common.LogText("password reset limit check failed: %v", err))
			return true
		}
		return count <= PasswordResetEmailLimit
	}

	now := time.Now()
	passwordResetBudgetMemory.Lock()
	defer passwordResetBudgetMemory.Unlock()
	for k, entry := range passwordResetBudgetMemory.entries {
		if now.After(entry.windowEnd) {
			delete(passwordResetBudgetMemory.entries, k)
		}
	}
	entry := passwordResetBudgetMemory.entries[key]
	if entry == nil {
		entry = &passwordResetBudgetEntry{windowEnd: now.Add(PasswordResetEmailWindow)}
		passwordResetBudgetMemory.entries[key] = entry
	}
	entry.count++
	return entry.count <= PasswordResetEmailLimit
}

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Password login lockout (OWASP Authentication Cheat Sheet, "Account Lockout";
// ASVS 5.0 V6.3.1): after LoginFailureLimit failed passwords inside
// LoginFailureWindow, the login identifier and the matched account are locked
// for LoginLockoutDuration. Counting happens per typed identifier as well as
// per account, so switching between username and email does not reset the
// budget, and an identifier that matches no account is locked exactly like one
// that does. The lock expires on its own; a correct password does not bypass it.
var (
	LoginFailureLimit    = common.GetEnvOrDefault("LOGIN_FAILURE_LIMIT", 10)
	LoginFailureWindow   = time.Duration(common.GetEnvOrDefault("LOGIN_FAILURE_WINDOW_MINUTES", 15)) * time.Minute
	LoginLockoutDuration = time.Duration(common.GetEnvOrDefault("LOGIN_LOCKOUT_MINUTES", 15)) * time.Minute
)

const loginLockoutRedisPrefix = "login_lockout:"

// recordLoginFailureScript increments every counter key, starts its window on
// the first failure, and converts a counter that reaches the limit into a lock
// key. It returns 1 when any key became locked.
const recordLoginFailureScript = `
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local lockout = tonumber(ARGV[3])
local locked = 0
for i = 1, #KEYS do
  local counter = KEYS[i] .. ':fail'
  local count = redis.call('INCR', counter)
  if redis.call('TTL', counter) < 0 then
    redis.call('EXPIRE', counter, window)
  end
  if count >= limit then
    redis.call('SET', KEYS[i] .. ':lock', '1', 'EX', lockout)
    redis.call('DEL', counter)
    locked = 1
  end
end
return locked
`

type loginFailureEntry struct {
	count       int
	windowEnd   time.Time
	lockedUntil time.Time
}

var loginFailureMemory = struct {
	sync.Mutex
	entries map[string]*loginFailureEntry
}{entries: map[string]*loginFailureEntry{}}

func loginLockoutEnabled() bool {
	return LoginFailureLimit > 0 && LoginFailureWindow > 0 && LoginLockoutDuration > 0
}

// LoginIdentifierLockoutKey keys the lockout by what the client typed. The
// identifier is hashed so usernames and emails are not stored in Redis keys.
func LoginIdentifierLockoutKey(identifier string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(identifier))))
	return "ident:" + hex.EncodeToString(sum[:16])
}

func LoginUserLockoutKey(userID int) string {
	return "user:" + strconv.Itoa(userID)
}

// LoginLocked reports whether any of the keys is currently locked. Redis
// errors fail open so a cache outage cannot block every password login.
func LoginLocked(keys ...string) bool {
	if !loginLockoutEnabled() || len(keys) == 0 {
		return false
	}
	if common.RedisEnabled && common.RDB != nil {
		lockKeys := make([]string, len(keys))
		for i, key := range keys {
			lockKeys[i] = loginLockoutRedisPrefix + key + ":lock"
		}
		count, err := common.RDB.Exists(context.Background(), lockKeys...).Result()
		if err != nil {
			common.SysError(common.LogText("login lockout check failed: %v", err))
			return false
		}
		return count > 0
	}

	now := time.Now()
	loginFailureMemory.Lock()
	defer loginFailureMemory.Unlock()
	for _, key := range keys {
		if entry := loginFailureMemory.entries[key]; entry != nil && now.Before(entry.lockedUntil) {
			return true
		}
	}
	return false
}

// RecordLoginFailure counts one failed password attempt against every key and
// returns true when this attempt locked at least one of them.
func RecordLoginFailure(keys ...string) bool {
	if !loginLockoutEnabled() || len(keys) == 0 {
		return false
	}
	if common.RedisEnabled && common.RDB != nil {
		redisKeys := make([]string, len(keys))
		for i, key := range keys {
			redisKeys[i] = loginLockoutRedisPrefix + key
		}
		locked, err := common.RDB.Eval(
			context.Background(),
			recordLoginFailureScript,
			redisKeys,
			LoginFailureLimit,
			int64(LoginFailureWindow/time.Second),
			int64(LoginLockoutDuration/time.Second),
		).Int()
		if err != nil {
			common.SysError(common.LogText("login lockout record failed: %v", err))
			return false
		}
		return locked == 1
	}

	now := time.Now()
	locked := false
	loginFailureMemory.Lock()
	defer loginFailureMemory.Unlock()
	for key, entry := range loginFailureMemory.entries {
		if now.After(entry.windowEnd) && now.After(entry.lockedUntil) {
			delete(loginFailureMemory.entries, key)
		}
	}
	for _, key := range keys {
		entry := loginFailureMemory.entries[key]
		if entry == nil {
			entry = &loginFailureEntry{}
			loginFailureMemory.entries[key] = entry
		}
		if now.After(entry.windowEnd) {
			entry.count = 0
			entry.windowEnd = now.Add(LoginFailureWindow)
		}
		entry.count++
		if entry.count >= LoginFailureLimit {
			entry.count = 0
			entry.windowEnd = now
			entry.lockedUntil = now.Add(LoginLockoutDuration)
			locked = true
		}
	}
	return locked
}

// ClearLoginFailures resets the failure counters after a correct password.
// It never lifts an active lock.
func ClearLoginFailures(keys ...string) {
	if !loginLockoutEnabled() || len(keys) == 0 {
		return
	}
	if common.RedisEnabled && common.RDB != nil {
		counterKeys := make([]string, len(keys))
		for i, key := range keys {
			counterKeys[i] = loginLockoutRedisPrefix + key + ":fail"
		}
		if err := common.RDB.Del(context.Background(), counterKeys...).Err(); err != nil {
			common.SysError(common.LogText("login lockout reset failed: %v", err))
		}
		return
	}

	loginFailureMemory.Lock()
	defer loginFailureMemory.Unlock()
	for _, key := range keys {
		if entry := loginFailureMemory.entries[key]; entry != nil {
			entry.count = 0
		}
	}
}

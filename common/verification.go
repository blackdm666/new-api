package common

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type verificationValue struct {
	currentHash  string
	currentTime  time.Time
	previousHash string
	previousTime time.Time
}

const (
	EmailVerificationPurpose          = "v"
	PasswordResetPurpose              = "r"
	VerificationResendCooldownSeconds = 60
	verificationPreviousValidSeconds  = 5 * 60
)

var verificationMutex sync.Mutex
var verificationMap map[string]verificationValue
var verificationCooldownMap map[string]time.Time
var verificationMapMaxSize = 10
var VerificationValidMinutes = 10

func GenerateVerificationCode(length int) string {
	if length <= 0 {
		return strings.ReplaceAll(uuid.New().String(), "-", "")
	}
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(length)), nil)
	value, err := rand.Int(rand.Reader, limit)
	if err != nil {
		// uuid.New uses a cryptographically secure random source and is a safe
		// fallback if the direct random read unexpectedly fails.
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, uuid.New().String())
		for len(digits) < length {
			digits += "0"
		}
		return digits[:length]
	}
	return fmt.Sprintf("%0*d", length, value.Int64())
}

func normalizedVerificationKey(key string, purpose string) string {
	return purpose + strings.ToLower(strings.TrimSpace(key))
}

func verificationCodeHash(key string, code string, purpose string) string {
	sum := sha256.Sum256([]byte(normalizedVerificationKey(key, purpose) + "|" + strings.TrimSpace(code)))
	return fmt.Sprintf("%x", sum[:])
}

func verificationRedisKey(prefix string, key string, purpose string) string {
	sum := sha256.Sum256([]byte(normalizedVerificationKey(key, purpose)))
	return fmt.Sprintf("verification:%s:%x", prefix, sum[:])
}

func encodeVerificationValue(value verificationValue) string {
	return strings.Join([]string{
		value.currentHash,
		strconv.FormatInt(value.currentTime.Unix(), 10),
		value.previousHash,
		strconv.FormatInt(value.previousTime.Unix(), 10),
	}, "|")
}

func decodeVerificationValue(raw string) (verificationValue, bool) {
	parts := strings.Split(raw, "|")
	if len(parts) != 4 {
		return verificationValue{}, false
	}
	currentUnix, currentErr := strconv.ParseInt(parts[1], 10, 64)
	previousUnix, previousErr := strconv.ParseInt(parts[3], 10, 64)
	if currentErr != nil || previousErr != nil {
		return verificationValue{}, false
	}
	return verificationValue{
		currentHash:  parts[0],
		currentTime:  time.Unix(currentUnix, 0),
		previousHash: parts[2],
		previousTime: time.Unix(previousUnix, 0),
	}, true
}

func ReserveVerificationSend(key string, purpose string, cooldown time.Duration) (bool, error) {
	if cooldown <= 0 {
		return true, nil
	}
	redisKey := verificationRedisKey("cooldown", key, purpose)
	if RedisEnabled && RDB != nil {
		return RDB.SetNX(context.Background(), redisKey, "1", cooldown).Result()
	}

	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	now := time.Now()
	if expiresAt, ok := verificationCooldownMap[redisKey]; ok && now.Before(expiresAt) {
		return false, nil
	}
	verificationCooldownMap[redisKey] = now.Add(cooldown)
	return true, nil
}

func ReleaseVerificationSend(key string, purpose string) {
	redisKey := verificationRedisKey("cooldown", key, purpose)
	if RedisEnabled && RDB != nil {
		if err := RDB.Del(context.Background(), redisKey).Err(); err != nil {
			SysError("failed to release verification cooldown: " + err.Error())
		}
		return
	}
	verificationMutex.Lock()
	delete(verificationCooldownMap, redisKey)
	verificationMutex.Unlock()
}

func RegisterVerificationCodeWithKey(key string, code string, purpose string) {
	now := time.Now()
	hash := verificationCodeHash(key, code, purpose)
	redisKey := verificationRedisKey("code", key, purpose)
	if RedisEnabled && RDB != nil {
		value := verificationValue{currentHash: hash, currentTime: now}
		if raw, err := RDB.Get(context.Background(), redisKey).Result(); err == nil {
			if previous, ok := decodeVerificationValue(raw); ok {
				value.previousHash = previous.currentHash
				value.previousTime = previous.currentTime
			}
		}
		if err := RDB.Set(context.Background(), redisKey, encodeVerificationValue(value), time.Duration(VerificationValidMinutes)*time.Minute).Err(); err == nil {
			return
		} else {
			SysError("failed to persist verification code in Redis: " + err.Error())
		}
	}

	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	mapKey := normalizedVerificationKey(key, purpose)
	previous := verificationMap[mapKey]
	verificationMap[mapKey] = verificationValue{
		currentHash:  hash,
		currentTime:  now,
		previousHash: previous.currentHash,
		previousTime: previous.currentTime,
	}
	if len(verificationMap) > verificationMapMaxSize {
		removeExpiredPairs()
	}
}

func verificationHashMatches(actual string, expected string) bool {
	if actual == "" || expected == "" || len(actual) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func verifyValue(value verificationValue, expectedHash string, now time.Time) bool {
	if now.Sub(value.currentTime) < time.Duration(VerificationValidMinutes)*time.Minute && verificationHashMatches(value.currentHash, expectedHash) {
		return true
	}
	return now.Sub(value.previousTime) < verificationPreviousValidSeconds*time.Second && verificationHashMatches(value.previousHash, expectedHash)
}

func VerifyCodeWithKey(key string, code string, purpose string) bool {
	expectedHash := verificationCodeHash(key, code, purpose)
	now := time.Now()
	redisKey := verificationRedisKey("code", key, purpose)
	if RedisEnabled && RDB != nil {
		raw, err := RDB.Get(context.Background(), redisKey).Result()
		if err == nil {
			value, ok := decodeVerificationValue(raw)
			return ok && verifyValue(value, expectedHash, now)
		}
	}

	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	value, ok := verificationMap[normalizedVerificationKey(key, purpose)]
	return ok && verifyValue(value, expectedHash, now)
}

func DeleteKey(key string, purpose string) {
	redisKey := verificationRedisKey("code", key, purpose)
	if RedisEnabled && RDB != nil {
		if err := RDB.Del(context.Background(), redisKey).Err(); err != nil {
			SysError("failed to delete verification code from Redis: " + err.Error())
		}
	}
	verificationMutex.Lock()
	delete(verificationMap, normalizedVerificationKey(key, purpose))
	verificationMutex.Unlock()
}

// no lock inside, so the caller must lock the verificationMap before calling.
func removeExpiredPairs() {
	now := time.Now()
	for key, value := range verificationMap {
		if now.Sub(value.currentTime) >= time.Duration(VerificationValidMinutes)*time.Minute {
			delete(verificationMap, key)
		}
	}
	for key, expiresAt := range verificationCooldownMap {
		if !now.Before(expiresAt) {
			delete(verificationCooldownMap, key)
		}
	}
}

func init() {
	verificationMap = make(map[string]verificationValue)
	verificationCooldownMap = make(map[string]time.Time)
}

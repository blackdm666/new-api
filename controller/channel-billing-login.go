package controller

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

const (
	channelBalanceLoginPasswordPurpose = "channel-balance-login-password"
	// A token without a readable JWT expiry is reused for at most this long;
	// a rejected token is replaced earlier through the 401/403 retry.
	channelBalanceLoginTokenFallbackTTL  = time.Hour
	channelBalanceLoginTokenExpiryMargin = 5 * time.Minute
	// Automatic refreshes run every few minutes; without a cooldown a wrong
	// password would hit the upstream sign-in hundreds of times a day and
	// could lock the dedicated account.
	channelBalanceLoginFailureCooldown = 10 * time.Minute
	channelBalanceLoginDefaultBody     = `{"username":"{username}","password":"{password}"}`
)

type channelBalanceLoginSession struct {
	fingerprint string
	token       string
	expiresAt   time.Time
	failedAt    time.Time
	failure     string
}

// channelBalanceLoginSessions caches the upstream console token per channel
// ID in process memory only, so it is never persisted or returned by the API.
var channelBalanceLoginSessions sync.Map

// sealChannelBalanceLoginPassword encrypts a newly entered sign-in password,
// or keeps the stored ciphertext when the administrator left it blank. A
// ciphertext sent by the client is never trusted, and the stored password is
// kept only for the same sign-in origin: otherwise an editor who does not
// know it could point the sign-in elsewhere and receive it.
func sealChannelBalanceLoginPassword(login *dto.ChannelBalanceLoginConfig, origin *dto.ChannelBalanceLoginConfig) error {
	if login == nil {
		return nil
	}
	password := login.Password
	login.Password = ""
	login.PasswordConfigured = false
	login.PasswordCiphertext = ""
	if password != "" {
		ciphertext, err := common.EncryptSensitiveValue(channelBalanceLoginPasswordPurpose, []byte(password))
		if err != nil {
			return fmt.Errorf("failed to encrypt balance query login password: %w", err)
		}
		login.PasswordCiphertext = ciphertext
		return nil
	}
	if origin == nil || origin.PasswordCiphertext == "" {
		return errors.New("balance query login password is required")
	}
	currentURL, currentErr := url.Parse(strings.TrimSpace(login.URL))
	originURL, originErr := url.Parse(strings.TrimSpace(origin.URL))
	if currentErr != nil || originErr != nil || currentURL.Scheme != originURL.Scheme || !strings.EqualFold(currentURL.Host, originURL.Host) {
		return errors.New("enter the balance query login password again after changing the sign-in host")
	}
	login.PasswordCiphertext = origin.PasswordCiphertext
	return nil
}

// channelBalanceLoginToken signs in to the upstream console and returns the
// token from its response. With reuseSession the token is cached until shortly
// before its JWT expiry and a failed sign-in is not retried during the
// cooldown; a balance query test always signs in afresh and caches nothing.
// The boolean reports whether the token came from the cache.
func channelBalanceLoginToken(channel *model.Channel, login *dto.ChannelBalanceLoginConfig, reuseSession bool) (string, bool, error) {
	fingerprintSum := sha256.Sum256([]byte(strings.Join([]string{login.URL, login.Username, login.PasswordCiphertext, login.Body, login.TokenPath}, "\x00")))
	fingerprint := hex.EncodeToString(fingerprintSum[:])
	now := time.Now()
	if reuseSession {
		if value, ok := channelBalanceLoginSessions.Load(channel.Id); ok {
			session := value.(channelBalanceLoginSession)
			if session.fingerprint == fingerprint {
				if session.token != "" && now.Before(session.expiresAt) {
					return session.token, true, nil
				}
				if session.failure != "" && now.Before(session.failedAt.Add(channelBalanceLoginFailureCooldown)) {
					return "", false, fmt.Errorf("%s; next sign-in attempt after %s",
						session.failure,
						session.failedAt.Add(channelBalanceLoginFailureCooldown).Format(time.DateTime))
				}
			}
		}
	}

	token, err := signInChannelBalanceAccount(channel, login)
	if err != nil {
		if reuseSession {
			channelBalanceLoginSessions.Store(channel.Id, channelBalanceLoginSession{
				fingerprint: fingerprint,
				failedAt:    now,
				failure:     err.Error(),
			})
		}
		return "", false, err
	}
	if reuseSession {
		channelBalanceLoginSessions.Store(channel.Id, channelBalanceLoginSession{
			fingerprint: fingerprint,
			token:       token,
			expiresAt:   channelBalanceLoginTokenExpiry(token, now),
		})
	}
	return token, false, nil
}

func signInChannelBalanceAccount(channel *model.Channel, login *dto.ChannelBalanceLoginConfig) (string, error) {
	password, err := common.DecryptSensitiveValue(channelBalanceLoginPasswordPurpose, login.PasswordCiphertext)
	if err != nil {
		return "", errors.New("stored balance query login password cannot be decrypted; enter the password again")
	}
	template := strings.TrimSpace(login.Body)
	if template == "" {
		template = channelBalanceLoginDefaultBody
	}
	// Credentials are inserted as JSON string content so quotes or
	// backslashes in them cannot change the request structure.
	escapedUsername, err := common.Marshal(strings.TrimSpace(login.Username))
	if err != nil {
		return "", err
	}
	escapedPassword, err := common.Marshal(string(password))
	if err != nil {
		return "", err
	}
	body := strings.NewReplacer(
		"{username}", string(escapedUsername[1:len(escapedUsername)-1]),
		"{password}", string(escapedPassword[1:len(escapedPassword)-1]),
	).Replace(template)
	var validated json.RawMessage
	if err := common.Unmarshal([]byte(body), &validated); err != nil {
		return "", errors.New("balance query login body must be valid JSON")
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	response, err := requestBalanceJSONURLWithMethod(channel, strings.TrimSpace(login.URL), headers, http.MethodPost, body)
	if err != nil {
		return "", fmt.Errorf("upstream sign-in failed: %w", err)
	}
	token, found, err := balanceStringAt(response, login.TokenPath)
	if err != nil || !found {
		return "", fmt.Errorf("upstream sign-in response has no token string at %s", login.TokenPath)
	}
	token = strings.TrimSpace(token)
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return "", fmt.Errorf("upstream sign-in response has no usable token at %s", login.TokenPath)
	}
	return token, nil
}

func channelBalanceLoginTokenExpiry(token string, now time.Time) time.Time {
	fallback := now.Add(channelBalanceLoginTokenFallbackTTL)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fallback
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return fallback
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if common.Unmarshal(payload, &claims) != nil || claims.Exp <= 0 {
		return fallback
	}
	return time.Unix(claims.Exp, 0).Add(-channelBalanceLoginTokenExpiryMargin)
}

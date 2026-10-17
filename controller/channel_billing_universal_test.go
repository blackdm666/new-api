package controller

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChannelManagementURLKeepsVersionInQueryPath(t *testing.T) {
	t.Parallel()

	got, err := buildChannelManagementURL("https://api.example.com/", "/v1/user/balance")
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/v1/user/balance", got)

	got, err = buildChannelManagementURL("https://api.example.com/gateway", "/api/usage/token/?scope=channel")
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/gateway/api/usage/token/?scope=channel", got)
}

func TestBuildChannelManagementURLRejectsAbsoluteOrProtocolRelativePath(t *testing.T) {
	t.Parallel()

	_, err := buildChannelManagementURL("https://api.example.com", "https://evil.example/balance")
	require.Error(t, err)

	_, err = buildChannelManagementURL("https://api.example.com", "//evil.example/balance")
	require.Error(t, err)
}

func TestCustomBalanceRequestUsesVersionedPathAndChannelKeyPlaceholder(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/v1/user/balance", request.URL.Path)
		assert.Equal(t, "Bearer sk-test", request.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"balance":12.5}`))
	}))
	defer server.Close()

	config := &dto.ChannelBalanceQueryConfig{
		Mode:   dto.ChannelBalanceQueryModeCustom,
		Path:   "/v1/user/balance",
		Method: "GET",
		Auth: &dto.AdvancedCustomRouteAuth{
			Type:  dto.AdvancedCustomAuthTypeHeader,
			Name:  "Authorization",
			Value: "Bearer {api_key}",
		},
		Response: dto.ChannelBalanceResponseConfig{RemainingPath: "balance"},
	}
	headers, path, requestBody, err := buildCustomBalanceRequest(config, "sk-test", "")
	require.NoError(t, err)
	channel := &model.Channel{BaseURL: &server.URL, Key: "sk-test"}

	requestURL, err := buildChannelManagementURL(channel.GetBaseURL(), path)
	require.NoError(t, err)
	body, err := requestBalanceJSONURLWithMethod(channel, requestURL, headers, config.Method, requestBody)
	require.NoError(t, err)
	assert.JSONEq(t, `{"balance":12.5}`, string(body))
}

func TestCustomBalanceRequestUsesManualAbsoluteURL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/account/balance", request.URL.Path)
		assert.Equal(t, "Bearer sk-test", request.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"remaining":12.5}}`))
	}))
	defer server.Close()

	config := &dto.ChannelBalanceQueryConfig{
		Mode:   dto.ChannelBalanceQueryModeCustom,
		URL:    server.URL + "/account/balance",
		Method: "GET",
		Auth: &dto.AdvancedCustomRouteAuth{
			Type:  dto.AdvancedCustomAuthTypeHeader,
			Name:  "Authorization",
			Value: "Bearer {api_key}",
		},
		Response: dto.ChannelBalanceResponseConfig{RemainingPath: "data.remaining"},
	}
	headers, target, requestBody, err := buildCustomBalanceRequest(config, "sk-test", "")
	require.NoError(t, err)
	channel := &model.Channel{Key: "sk-test"}
	body, err := requestBalanceJSONURLWithMethod(channel, target, headers, config.Method, requestBody)
	require.NoError(t, err)
	assert.JSONEq(t, `{"data":{"remaining":12.5}}`, string(body))
}

func TestParseCustomBalanceMapsHaoStyleResponseWithoutProviderPreset(t *testing.T) {
	t.Parallel()

	config := &dto.ChannelBalanceQueryConfig{
		Mode:       dto.ChannelBalanceQueryModeCustom,
		Path:       "/v1/user/balance",
		Method:     "GET",
		Unit:       dto.ChannelBalanceUnitMoney,
		MetricKind: dto.ChannelBalanceMetricWallet,
		Multiplier: "1",
		Response: dto.ChannelBalanceResponseConfig{
			RemainingPath: "balance",
			TotalPath:     "total",
			UsedPath:      "used",
			CurrencyPath:  "currency",
			ActivePath:    "is_active",
		},
	}
	body := []byte(`{"is_active":true,"balance":55.002146,"total":60.002146,"used":5,"currency":"USD"}`)

	info, legacy, err := parseCustomBalance(body, config)
	require.NoError(t, err)
	require.NotNil(t, legacy)
	assert.InDelta(t, 55.002146, *legacy, 0.0000001)
	assert.Equal(t, "55.002146", info.Remaining)
	assert.Equal(t, "60.002146", info.Total)
	assert.Equal(t, "5", info.Used)
	assert.Equal(t, model.ChannelBalanceUnitMoney, info.Unit)
	assert.Equal(t, "USD", info.Currency)
	assert.Equal(t, "$", info.DisplayUnit)
	assert.Equal(t, dto.ChannelBalanceMetricWallet, info.MetricKind)
	assert.Equal(t, dto.ChannelBalanceQueryModeCustom, info.Source)
}

func TestParseCustomBalanceSubtractsFrozenMicrodollars(t *testing.T) {
	t.Parallel()

	config := &dto.ChannelBalanceQueryConfig{
		Mode:          dto.ChannelBalanceQueryModeCustom,
		Method:        http.MethodGet,
		Unit:          dto.ChannelBalanceUnitMoney,
		Currency:      "USD",
		MetricKind:    dto.ChannelBalanceMetricWallet,
		Multiplier:    "0.000001",
		RemainingMode: dto.ChannelBalanceRemainingTotalMinusUsed,
		Response: dto.ChannelBalanceResponseConfig{
			TotalPath: "balance",
			UsedPath:  "frozen_balance",
		},
	}

	info, legacy, err := parseCustomBalance([]byte(`{"balance":55002146,"frozen_balance":5000000}`), config)
	require.NoError(t, err)
	require.NotNil(t, legacy)
	assert.Equal(t, "50.002146", info.Remaining)
	assert.Equal(t, "55.002146", info.Total)
	assert.Equal(t, "5", info.Used)
	assert.InDelta(t, 50.002146, *legacy, 0.0000001)
}

func TestCustomBalancePOSTSupportsBodyHeadersAndSuccessCondition(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "Bearer account-token", request.Header.Get("Authorization"))
		assert.Equal(t, "tenant-a", request.Header.Get("X-Tenant"))
		assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"scope":"wallet"}`, string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"balance":42}`))
	}))
	defer server.Close()

	config := &dto.ChannelBalanceQueryConfig{
		Mode:   dto.ChannelBalanceQueryModeCustom,
		URL:    server.URL,
		Method: http.MethodPost,
		Body:   `{"scope":"wallet"}`,
		Auth: &dto.AdvancedCustomRouteAuth{
			Type:  dto.AdvancedCustomAuthTypeHeader,
			Name:  "Authorization",
			Value: "Bearer account-token",
		},
		Headers: []dto.ChannelBalanceRequestHeader{{Name: "X-Tenant", Value: "tenant-a"}},
		Response: dto.ChannelBalanceResponseConfig{
			RemainingPath: "balance",
			SuccessPath:   "success",
			SuccessValue:  "true",
		},
		Unit:       dto.ChannelBalanceUnitMoney,
		Currency:   "USD",
		MetricKind: dto.ChannelBalanceMetricWallet,
	}

	info, legacy, _, err := queryChannelCustomBalance(&model.Channel{}, config, false)
	require.NoError(t, err)
	require.NotNil(t, legacy)
	assert.Equal(t, "42", info.Remaining)
}

func TestParseCustomBalanceSupportsNestedArraysNumericStringsAndMultiplier(t *testing.T) {
	t.Parallel()

	config := &dto.ChannelBalanceQueryConfig{
		Mode:        dto.ChannelBalanceQueryModeCustom,
		Path:        "/balance",
		Method:      "GET",
		Unit:        dto.ChannelBalanceUnitCredits,
		DisplayUnit: "credits",
		MetricKind:  dto.ChannelBalanceMetricQuota,
		Multiplier:  "0.5",
		Response: dto.ChannelBalanceResponseConfig{
			RemainingPath: "data.accounts.0.remaining",
		},
	}
	body := []byte(`{"data":{"accounts":[{"remaining":"42.5"}]}}`)

	info, legacy, err := parseCustomBalance(body, config)
	require.NoError(t, err)
	assert.Nil(t, legacy)
	assert.Equal(t, "21.25", info.Remaining)
	assert.Equal(t, model.ChannelBalanceUnitCredits, info.Unit)
}

func TestParseCustomBalanceRejectsInactiveResponse(t *testing.T) {
	t.Parallel()

	config := &dto.ChannelBalanceQueryConfig{
		Mode:   dto.ChannelBalanceQueryModeCustom,
		Path:   "/balance",
		Method: "GET",
		Response: dto.ChannelBalanceResponseConfig{
			RemainingPath: "balance",
			ActivePath:    "is_active",
		},
	}

	_, _, err := parseCustomBalance([]byte(`{"is_active":false,"balance":10}`), config)
	require.ErrorContains(t, err, "inactive")
}

func TestNewAPIAccountBalanceRequiresDedicatedAccountToken(t *testing.T) {
	t.Parallel()

	_, err := accountBalanceAuthHeaders(nil, "NewAPI")
	require.ErrorContains(t, err, "dedicated account access token")

	_, err = accountBalanceAuthHeaders(&dto.ChannelBalanceQueryConfig{
		Auth: &dto.AdvancedCustomRouteAuth{Value: "Bearer {api_key}"},
	}, "NewAPI")
	require.ErrorContains(t, err, "not the channel API key")

	headers, err := accountBalanceAuthHeaders(&dto.ChannelBalanceQueryConfig{
		Auth:          &dto.AdvancedCustomRouteAuth{Value: "account-access-token"},
		AccountUserID: "300",
	}, "NewAPI")
	require.NoError(t, err)
	assert.Equal(t, "Bearer account-access-token", headers.Get("Authorization"))
	assert.Equal(t, "300", headers.Get("New-Api-User"))
}

func TestNewAPIAccountBalanceSurfacesMissingUserIDResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/api/user/self", request.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"success":false,"message":"Unauthorized, New-Api-User header not provided"}`))
	}))
	defer server.Close()

	channel := &model.Channel{BaseURL: &server.URL}
	_, err := updateChannelNewAPIBalance(channel, &dto.ChannelBalanceQueryConfig{
		Mode: dto.ChannelBalanceQueryModeNewAPI,
		Auth: &dto.AdvancedCustomRouteAuth{Value: "account-access-token"},
	})
	require.ErrorContains(t, err, "New-Api-User header not provided")
}

func TestChannelBalanceQueryAccessTokenIsRedactedAndRestored(t *testing.T) {
	t.Parallel()

	origin := &model.Channel{Id: 7}
	origin.SetOtherSettings(dto.ChannelOtherSettings{
		BalanceQuery: &dto.ChannelBalanceQueryConfig{
			Mode: dto.ChannelBalanceQueryModeNewAPI,
			Auth: &dto.AdvancedCustomRouteAuth{
				Type:  dto.AdvancedCustomAuthTypeHeader,
				Name:  "Authorization",
				Value: "account-token-1234567890",
			},
			Headers: []dto.ChannelBalanceRequestHeader{
				{Name: "X-Tenant-Token", Value: "tenant-secret-0987654321"},
				{Name: "X-Channel-Key", Value: "{api_key}"},
			},
		},
	})

	response := *origin
	redactChannelBalanceQuerySecret(&response)
	redacted := response.GetOtherSettings().BalanceQuery
	require.NotNil(t, redacted)
	require.NotNil(t, redacted.Auth)
	assert.Empty(t, redacted.Auth.Value)
	assert.True(t, redacted.AuthConfigured)
	assert.Equal(t, "acco••••••••7890", redacted.AuthMasked)
	assert.NotContains(t, response.OtherSettings, "account-token-1234567890")
	require.Len(t, redacted.Headers, 2)
	assert.Empty(t, redacted.Headers[0].Value)
	assert.True(t, redacted.Headers[0].Configured)
	assert.NotEmpty(t, redacted.Headers[0].Masked)
	assert.Equal(t, "{api_key}", redacted.Headers[1].Value)
	assert.NotContains(t, response.OtherSettings, "tenant-secret-0987654321")

	err := restoreChannelBalanceQuerySecret(&response, origin, map[string]any{"settings": response.OtherSettings})
	require.NoError(t, err)
	restored := response.GetOtherSettings().BalanceQuery
	require.NotNil(t, restored)
	require.NotNil(t, restored.Auth)
	assert.Equal(t, "account-token-1234567890", restored.Auth.Value)
	assert.False(t, restored.AuthConfigured)
	assert.Empty(t, restored.AuthMasked)
	require.Len(t, restored.Headers, 2)
	assert.Equal(t, "tenant-secret-0987654321", restored.Headers[0].Value)
	assert.False(t, restored.Headers[0].Configured)
	assert.Empty(t, restored.Headers[0].Masked)
}

func TestChannelBalanceQueryAPIKeyPlaceholderIsNotRedacted(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		BalanceQuery: &dto.ChannelBalanceQueryConfig{
			Mode: dto.ChannelBalanceQueryModeCustom,
			Auth: &dto.AdvancedCustomRouteAuth{Value: "Bearer {api_key}"},
		},
	})
	redactChannelBalanceQuerySecret(channel)
	config := channel.GetOtherSettings().BalanceQuery
	require.NotNil(t, config)
	require.NotNil(t, config.Auth)
	assert.Equal(t, "Bearer {api_key}", config.Auth.Value)
	assert.False(t, config.AuthConfigured)
}

func TestNormalizeNewAPIAccountBalanceConvertsAccountQuotaWithStatusRate(t *testing.T) {
	t.Parallel()

	remaining := decimal.NewFromInt(1_000_000)
	used := decimal.NewFromInt(500_000)
	quotaPerUnit := decimal.NewFromInt(500_000)
	info, legacy := normalizeAccountBalance(
		&oneAPIUserData{Quota: &remaining, UsedQuota: &used},
		&upstreamStatusData{QuotaPerUnit: &quotaPerUnit},
		dto.ChannelBalanceQueryModeNewAPI,
	)

	require.NotNil(t, legacy)
	assert.Equal(t, "2", info.Remaining)
	assert.Equal(t, "1", info.Used)
	assert.Equal(t, "3", info.Total)
	assert.Equal(t, model.ChannelBalanceUnitMoney, info.Unit)
	assert.Equal(t, "USD", info.Currency)
	assert.Equal(t, dto.ChannelBalanceQueryModeNewAPI, info.Source)
}

func TestNormalizeNewAPIAccountBalancePreservesUpstreamDisplayCurrency(t *testing.T) {
	t.Parallel()

	remaining := decimal.NewFromInt(1_000_000)
	used := decimal.NewFromInt(500_000)
	quotaPerUnit := decimal.NewFromInt(500_000)
	exchangeRate := decimal.NewFromFloat(7.2)
	info, legacy := normalizeAccountBalance(
		&oneAPIUserData{Quota: &remaining, UsedQuota: &used},
		&upstreamStatusData{
			QuotaPerUnit:     &quotaPerUnit,
			QuotaDisplayType: "CNY",
			USDExchangeRate:  &exchangeRate,
		},
		dto.ChannelBalanceQueryModeNewAPI,
	)

	assert.Nil(t, legacy)
	assert.Equal(t, "14.4", info.Remaining)
	assert.Equal(t, "7.2", info.Used)
	assert.Equal(t, "21.6", info.Total)
	assert.Equal(t, "CNY", info.Currency)
	assert.Equal(t, "¥", info.DisplayUnit)
}

func TestParseSub2APIBalancePreservesQuotaSubscriptionAndRateLimitMeaning(t *testing.T) {
	t.Parallel()

	t.Run("quota", func(t *testing.T) {
		info, legacy, err := parseSub2APIBalance([]byte(`{"mode":"quota_limited","isValid":true,"quota":{"limit":100,"used":40,"remaining":60,"unit":"USD"},"remaining":60,"unit":"USD"}`))
		require.NoError(t, err)
		require.NotNil(t, legacy)
		assert.Equal(t, dto.ChannelBalanceMetricQuota, info.MetricKind)
		assert.Equal(t, "100", info.Total)
		assert.Equal(t, "40", info.Used)
		assert.Equal(t, "60", info.Remaining)
	})

	t.Run("unlimited subscription", func(t *testing.T) {
		info, legacy, err := parseSub2APIBalance([]byte(`{"mode":"unrestricted","isValid":true,"remaining":-1,"unit":"USD","subscription":{}}`))
		require.NoError(t, err)
		assert.Nil(t, legacy)
		assert.True(t, info.Unlimited)
		assert.Equal(t, dto.ChannelBalanceMetricSubscription, info.MetricKind)
		assert.False(t, channelBalanceExhausted(&info))
	})

	t.Run("unlimited subscription with null limits", func(t *testing.T) {
		info, legacy, err := parseSub2APIBalance([]byte(`{"mode":"unrestricted","isValid":true,"remaining":-1,"subscription":{"daily_limit_usd":null,"weekly_limit_usd":null,"monthly_limit_usd":null}}`))
		require.NoError(t, err)
		assert.Nil(t, legacy)
		assert.True(t, info.Unlimited)
	})

	t.Run("overdrawn wallet is exhausted, not unlimited", func(t *testing.T) {
		info, legacy, err := parseSub2APIBalance([]byte(`{"mode":"unrestricted","isValid":true,"remaining":-3.25,"balance":-3.25,"unit":"USD"}`))
		require.NoError(t, err)
		require.NotNil(t, legacy)
		assert.InDelta(t, -3.25, *legacy, 1e-9)
		assert.False(t, info.Unlimited)
		assert.Equal(t, "-3.25", info.Remaining)
		assert.True(t, channelBalanceExhausted(&info))
	})

	t.Run("limited subscription keeps a negative remaining", func(t *testing.T) {
		info, _, err := parseSub2APIBalance([]byte(`{"isValid":true,"remaining":-1,"subscription":{"daily_limit_usd":20,"weekly_limit_usd":null,"monthly_limit_usd":null}}`))
		require.NoError(t, err)
		assert.False(t, info.Unlimited)
		assert.Equal(t, "-1", info.Remaining)
	})

	t.Run("rate limit", func(t *testing.T) {
		payload := map[string]any{
			"isValid": true,
			"rate_limits": []map[string]any{
				{"window": "5h", "limit": 100, "used": 90, "remaining": 10},
				{"window": "1d", "limit": 50, "used": 48, "remaining": 2},
			},
		}
		body, err := common.Marshal(payload)
		require.NoError(t, err)
		info, legacy, err := parseSub2APIBalance(body)
		require.NoError(t, err)
		assert.Nil(t, legacy)
		assert.Equal(t, dto.ChannelBalanceMetricRateLimit, info.MetricKind)
		assert.Equal(t, model.ChannelBalanceUnitRequests, info.Unit)
		assert.Equal(t, "2", info.Remaining)
		assert.Equal(t, "50", info.Total)
		assert.Equal(t, "48", info.Used)
	})
}

func testBalanceLoginJWT(expiresAt time.Time, nonce string) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d,"jti":"%s"}`, expiresAt.Unix(), nonce)))
	return "eyJhbGciOiJIUzI1NiJ9." + payload + ".signature"
}

func newBalanceLoginTestConfig(t *testing.T, serverURL string, password string) *dto.ChannelBalanceQueryConfig {
	t.Helper()
	login := &dto.ChannelBalanceLoginConfig{
		URL:       serverURL + "/api/v1/auth/login",
		Body:      `{"username":"{username}","password":"{password}","login_type":"password"}`,
		Username:  "balance-bot",
		Password:  password,
		TokenPath: "token",
	}
	require.NoError(t, sealChannelBalanceLoginPassword(login, nil))
	return &dto.ChannelBalanceQueryConfig{
		Mode:   dto.ChannelBalanceQueryModeCustom,
		URL:    serverURL + "/api/v1/user/wallet",
		Method: http.MethodGet,
		Auth: &dto.AdvancedCustomRouteAuth{
			Type:  dto.AdvancedCustomAuthTypeHeader,
			Name:  "Authorization",
			Value: "Bearer {login_token}",
		},
		Login:    login,
		Response: dto.ChannelBalanceResponseConfig{RemainingPath: "balance", UsedPath: "spent"},
		Unit:     dto.ChannelBalanceUnitMoney,
		Currency: "CNY",
	}
}

func TestCustomBalanceSignsInReusesTokenAndSignsInAgainWhenRejected(t *testing.T) {
	t.Parallel()

	const password = `pa"ss\word`
	var logins atomic.Int32
	var validToken atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/login":
			assert.Equal(t, http.MethodPost, request.Method)
			var credentials struct {
				Username  string `json:"username"`
				Password  string `json:"password"`
				LoginType string `json:"login_type"`
			}
			assert.NoError(t, common.DecodeJson(request.Body, &credentials))
			assert.Equal(t, "balance-bot", credentials.Username)
			assert.Equal(t, password, credentials.Password)
			assert.Equal(t, "password", credentials.LoginType)
			token := testBalanceLoginJWT(time.Now().Add(7*24*time.Hour), fmt.Sprint(logins.Add(1)))
			validToken.Store(token)
			_, _ = fmt.Fprintf(w, `{"token":%q,"user":{"id":"1"}}`, token)
		case "/api/v1/user/wallet":
			if request.Header.Get("Authorization") != "Bearer "+validToken.Load().(string) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"message":"Authentication required"},"success":false}`))
				return
			}
			_, _ = w.Write([]byte(`{"balance":500.5,"spent":24.5}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := newBalanceLoginTestConfig(t, server.URL, password)
	require.NoError(t, config.Validate())
	channel := &model.Channel{Id: 990001}
	t.Cleanup(func() { channelBalanceLoginSessions.Delete(channel.Id) })

	info, _, _, err := queryChannelCustomBalance(channel, config, true)
	require.NoError(t, err)
	assert.Equal(t, "500.5", info.Remaining)
	assert.Equal(t, "24.5", info.Used)
	assert.Equal(t, int32(1), logins.Load())

	_, _, _, err = queryChannelCustomBalance(channel, config, true)
	require.NoError(t, err)
	assert.Equal(t, int32(1), logins.Load(), "a cached token must be reused until it expires")

	// The upstream revokes the cached token, e.g. after the account signed
	// in elsewhere: the query signs in once more and succeeds.
	validToken.Store("revoked")
	info, _, _, err = queryChannelCustomBalance(channel, config, true)
	require.NoError(t, err)
	assert.Equal(t, "500.5", info.Remaining)
	assert.Equal(t, int32(2), logins.Load())

	// A test from the editor never reuses the cached session.
	_, _, _, err = queryChannelCustomBalance(channel, config, false)
	require.NoError(t, err)
	assert.Equal(t, int32(3), logins.Load())
}

func TestCustomBalanceFailedSignInWaitsForCooldownUnlessConfigChanges(t *testing.T) {
	t.Parallel()

	var logins atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		logins.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid username or password"}`))
	}))
	defer server.Close()

	config := newBalanceLoginTestConfig(t, server.URL, "wrong-password")
	channel := &model.Channel{Id: 990002}
	t.Cleanup(func() { channelBalanceLoginSessions.Delete(channel.Id) })

	_, _, _, err := queryChannelCustomBalance(channel, config, true)
	require.ErrorContains(t, err, "invalid username or password")
	assert.NotContains(t, err.Error(), "wrong-password")
	_, _, _, err = queryChannelCustomBalance(channel, config, true)
	require.ErrorContains(t, err, "next sign-in attempt after")
	assert.Equal(t, int32(1), logins.Load(), "automatic refreshes must not repeat a failed sign-in during the cooldown")

	// Saving a corrected password changes the ciphertext and signs in at once.
	corrected := newBalanceLoginTestConfig(t, server.URL, "corrected-password")
	_, _, _, err = queryChannelCustomBalance(channel, corrected, true)
	require.Error(t, err)
	assert.Equal(t, int32(2), logins.Load())
}

func TestChannelBalanceLoginPasswordIsEncryptedRedactedAndKept(t *testing.T) {
	t.Parallel()

	origin := &model.Channel{Id: 990003}
	config := newBalanceLoginTestConfig(t, "https://upstream.example", "s3cret-password")
	assert.Empty(t, config.Login.Password)
	assert.NotContains(t, config.Login.PasswordCiphertext, "s3cret-password")
	origin.SetOtherSettings(dto.ChannelOtherSettings{BalanceQuery: config})
	assert.NotContains(t, origin.OtherSettings, "s3cret-password")

	response := *origin
	redactChannelBalanceQuerySecret(&response)
	redacted := response.GetOtherSettings().BalanceQuery
	require.NotNil(t, redacted.Login)
	assert.True(t, redacted.Login.PasswordConfigured)
	assert.Empty(t, redacted.Login.PasswordCiphertext)
	assert.NotContains(t, response.OtherSettings, config.Login.PasswordCiphertext)
	assert.Equal(t, "Bearer {login_token}", redacted.Auth.Value, "the token placeholder is not a secret")
	assert.False(t, redacted.AuthConfigured)

	// Saving the redacted form keeps the stored password, and a ciphertext
	// sent by the client is ignored.
	redacted.Login.PasswordCiphertext = "v1:forged"
	response.SetOtherSettings(dto.ChannelOtherSettings{BalanceQuery: redacted})
	require.NoError(t, restoreChannelBalanceQuerySecret(&response, origin, map[string]any{"settings": response.OtherSettings}))
	kept := response.GetOtherSettings().BalanceQuery
	assert.Equal(t, config.Login.PasswordCiphertext, kept.Login.PasswordCiphertext)
	assert.False(t, kept.Login.PasswordConfigured)
	require.NoError(t, kept.Validate())

	// Moving the sign-in to another host requires the password again, so an
	// editor cannot redirect the stored password to a host they control.
	moved := *kept
	movedLogin := *kept.Login
	movedLogin.URL = "https://collector.example/api/v1/auth/login"
	moved.Login = &movedLogin
	response.SetOtherSettings(dto.ChannelOtherSettings{BalanceQuery: &moved})
	require.ErrorContains(t, restoreChannelBalanceQuerySecret(&response, origin, map[string]any{"settings": response.OtherSettings}), "password again")

	// A new password replaces the stored one.
	kept.Login.Password = "new-password"
	response.SetOtherSettings(dto.ChannelOtherSettings{BalanceQuery: kept})
	require.NoError(t, restoreChannelBalanceQuerySecret(&response, origin, map[string]any{"settings": response.OtherSettings}))
	plaintext, err := common.DecryptSensitiveValue(channelBalanceLoginPasswordPurpose, response.GetOtherSettings().BalanceQuery.Login.PasswordCiphertext)
	require.NoError(t, err)
	assert.Equal(t, "new-password", string(plaintext))

	// A channel without a stored password cannot keep one it never had.
	require.ErrorContains(t, sealChannelBalanceLoginPassword(&dto.ChannelBalanceLoginConfig{PasswordConfigured: true}, nil), "password is required")
}

func TestChannelBalanceLoginValidation(t *testing.T) {
	t.Parallel()

	require.NoError(t, newBalanceLoginTestConfig(t, "https://upstream.example", "password").Validate())

	cases := map[string]func(*dto.ChannelBalanceQueryConfig){
		"plain HTTP sign-in to a remote host": func(config *dto.ChannelBalanceQueryConfig) {
			config.Login.URL = "http://upstream.example/api/v1/auth/login"
		},
		"token placeholder unused": func(config *dto.ChannelBalanceQueryConfig) {
			config.Auth.Value = "Bearer fixed"
		},
		"login outside custom mode": func(config *dto.ChannelBalanceQueryConfig) {
			config.Mode = dto.ChannelBalanceQueryModeNewAPI
		},
		"missing token path": func(config *dto.ChannelBalanceQueryConfig) {
			config.Login.TokenPath = ""
		},
		"body without password": func(config *dto.ChannelBalanceQueryConfig) {
			config.Login.Body = `{"username":"{username}"}`
		},
		"missing password": func(config *dto.ChannelBalanceQueryConfig) {
			config.Login.PasswordCiphertext = ""
		},
	}
	for name, mutate := range cases {
		config := newBalanceLoginTestConfig(t, "https://upstream.example", "password")
		mutate(config)
		assert.Error(t, config.Validate(), name)
	}
}

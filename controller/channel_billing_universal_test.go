package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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
	headers, path, requestBody, err := buildCustomBalanceRequest(config, "sk-test")
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
	headers, target, requestBody, err := buildCustomBalanceRequest(config, "sk-test")
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

	info, legacy, _, err := queryChannelCustomBalance(&model.Channel{}, config)
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

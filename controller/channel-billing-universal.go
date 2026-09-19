package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const maxChannelBalanceResponseBytes = 256 << 10

var errBalanceAccountInactive = errors.New("upstream balance response reports an inactive account")

type testChannelBalanceQueryRequest struct {
	Config dto.ChannelBalanceQueryConfig `json:"config"`
}

func TestChannelBalanceQuery(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}
	var request testChannelBalanceQueryRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(request.Config.Mode) != dto.ChannelBalanceQueryModeCustom {
		common.ApiError(c, errors.New("only custom balance queries can be tested before saving"))
		return
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	candidate := &PatchChannel{Channel: *channel}
	settings := channel.GetOtherSettings()
	settings.BalanceQuery = &request.Config
	candidate.SetOtherSettings(settings)
	if err := restoreChannelBalanceQuerySecret(&candidate.Channel, channel, map[string]any{"settings": candidate.OtherSettings}); err != nil {
		common.ApiError(c, err)
		return
	}
	config := candidate.GetOtherSettings().BalanceQuery
	info, legacyBalance, raw, err := queryChannelCustomBalance(&candidate.Channel, config)
	if err != nil {
		if len(raw) > 0 {
			formatted, formatErr := common.IndentJson(raw)
			if formatErr == nil {
				c.JSON(http.StatusOK, gin.H{
					"success":         true,
					"mapping_success": false,
					"message":         err.Error(),
					"raw_response":    string(formatted),
				})
				return
			}
		}
		common.ApiError(c, err)
		return
	}
	response := gin.H{
		"success":         true,
		"mapping_success": true,
		"balance_info":    info,
	}
	if legacyBalance != nil {
		response["balance"] = *legacyBalance
	}
	c.JSON(http.StatusOK, response)
}

type upstreamStatusResponse struct {
	Success bool                `json:"success"`
	Data    *upstreamStatusData `json:"data"`
}

type upstreamStatusData struct {
	QuotaPerUnit               *decimal.Decimal `json:"quota_per_unit"`
	QuotaDisplayType           string           `json:"quota_display_type"`
	USDExchangeRate            *decimal.Decimal `json:"usd_exchange_rate"`
	CustomCurrencySymbol       string           `json:"custom_currency_symbol"`
	CustomCurrencyExchangeRate *decimal.Decimal `json:"custom_currency_exchange_rate"`
}

type oneAPIUserResponse struct {
	Success bool            `json:"success"`
	Data    *oneAPIUserData `json:"data"`
}

type oneAPIUserData struct {
	Quota     *decimal.Decimal `json:"quota"`
	UsedQuota *decimal.Decimal `json:"used_quota"`
}

func updateConfiguredChannelBalance(channel *model.Channel) (channelBalanceResult, bool, error) {
	settings := channel.GetOtherSettings()
	config := settings.BalanceQuery
	if config == nil {
		return channelBalanceResult{}, true, errors.New("balance query is disabled for this channel")
	}
	mode := dto.ChannelBalanceQueryModeAuto
	if config != nil && strings.TrimSpace(config.Mode) != "" {
		mode = strings.TrimSpace(config.Mode)
	}
	if mode == dto.ChannelBalanceQueryModeDisabled {
		return channelBalanceResult{}, true, errors.New("balance query is disabled for this channel")
	}
	if mode == dto.ChannelBalanceQueryModeAuto {
		switch channel.Type {
		case constant.ChannelTypeNewAPI:
			mode = dto.ChannelBalanceQueryModeNewAPI
		case constant.ChannelTypeSub2API:
			mode = dto.ChannelBalanceQueryModeSub2API
		default:
			return channelBalanceResult{}, false, nil
		}
	}

	if channel.ChannelInfo.IsMultiKey {
		return channelBalanceResult{}, true, errors.New("balance query does not support multi-key channels")
	}

	switch mode {
	case dto.ChannelBalanceQueryModeNewAPI:
		result, err := updateChannelNewAPIBalance(channel, config)
		return result, true, err
	case dto.ChannelBalanceQueryModeOneAPI:
		result, err := updateChannelOneAPIBalance(channel, config)
		return result, true, err
	case dto.ChannelBalanceQueryModeSub2API:
		result, err := updateChannelSub2APIBalance(channel)
		return result, true, err
	case dto.ChannelBalanceQueryModeGCPTrial:
		if config == nil {
			return channelBalanceResult{}, true, errors.New("Vertex trial credit configuration is required")
		}
		result, err := updateChannelGCPTrialBalance(channel, config)
		return result, true, err
	case dto.ChannelBalanceQueryModeCustom:
		if config == nil {
			return channelBalanceResult{}, true, errors.New("custom balance query configuration is required")
		}
		result, err := updateChannelCustomBalance(channel, config)
		return result, true, err
	default:
		return channelBalanceResult{}, true, fmt.Errorf("unsupported balance query mode: %s", mode)
	}
}

func channelBalanceQueryDisabled(channel *model.Channel) bool {
	config := channel.GetOtherSettings().BalanceQuery
	return config == nil || strings.TrimSpace(config.Mode) == dto.ChannelBalanceQueryModeDisabled
}

func updateChannelNewAPIBalance(channel *model.Channel, config *dto.ChannelBalanceQueryConfig) (channelBalanceResult, error) {
	headers, err := accountBalanceAuthHeaders(config, "NewAPI")
	if err != nil {
		return channelBalanceResult{}, err
	}
	body, err := requestChannelBalanceJSON(channel, "/api/user/self", headers)
	if err != nil {
		return channelBalanceResult{}, err
	}
	var response oneAPIUserResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return channelBalanceResult{}, fmt.Errorf("invalid NewAPI account response: %w", err)
	}
	if !response.Success || response.Data == nil || response.Data.Quota == nil {
		return channelBalanceResult{}, errors.New("NewAPI account response is invalid; /api/user/self requires an account access token")
	}

	var status *upstreamStatusData
	if statusBody, statusErr := requestChannelBalanceJSON(channel, "/api/status", nil); statusErr == nil {
		var parsed upstreamStatusResponse
		if common.Unmarshal(statusBody, &parsed) == nil && parsed.Success && parsed.Data != nil {
			status = parsed.Data
		}
	}
	info, legacyBalance := normalizeAccountBalance(response.Data, status, dto.ChannelBalanceQueryModeNewAPI)
	if err := channel.UpdateBalanceInfo(info, legacyBalance); err != nil {
		return channelBalanceResult{}, err
	}
	return balanceResult(info, legacyBalance), nil
}

func updateChannelOneAPIBalance(channel *model.Channel, config *dto.ChannelBalanceQueryConfig) (channelBalanceResult, error) {
	headers, err := accountBalanceAuthHeaders(config, "OneAPI")
	if err != nil {
		return channelBalanceResult{}, err
	}
	body, err := requestChannelBalanceJSON(channel, "/api/user/self", headers)
	if err != nil {
		return channelBalanceResult{}, err
	}
	var response oneAPIUserResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return channelBalanceResult{}, fmt.Errorf("invalid One API user response: %w", err)
	}
	if !response.Success || response.Data == nil || response.Data.Quota == nil {
		return channelBalanceResult{}, errors.New("One API user response is invalid; this endpoint requires an account access token")
	}
	var status *upstreamStatusData
	if statusBody, statusErr := requestChannelBalanceJSON(channel, "/api/status", nil); statusErr == nil {
		var parsed upstreamStatusResponse
		if common.Unmarshal(statusBody, &parsed) == nil && parsed.Success && parsed.Data != nil {
			status = parsed.Data
		}
	}
	info, legacyBalance := normalizeAccountBalance(response.Data, status, dto.ChannelBalanceQueryModeOneAPI)
	if err := channel.UpdateBalanceInfo(info, legacyBalance); err != nil {
		return channelBalanceResult{}, err
	}
	return balanceResult(info, legacyBalance), nil
}

func accountBalanceAuthHeaders(config *dto.ChannelBalanceQueryConfig, provider string) (http.Header, error) {
	if config == nil || config.Auth == nil {
		return nil, fmt.Errorf("%s account balance requires a dedicated account access token in the balance query configuration", provider)
	}
	value := strings.TrimSpace(config.Auth.Value)
	if value == "" || strings.Contains(value, "{api_key}") {
		return nil, fmt.Errorf("%s account balance requires an account access token, not the channel API key", provider)
	}
	if !strings.HasPrefix(strings.ToLower(value), "bearer ") {
		value = "Bearer " + value
	}
	headers := http.Header{}
	headers.Set("Authorization", value)
	if userID := strings.TrimSpace(config.AccountUserID); userID != "" {
		headers.Set("New-Api-User", userID)
	}
	return headers, nil
}

func normalizeAccountBalance(data *oneAPIUserData, status *upstreamStatusData, source string) (model.ChannelBalanceInfo, *float64) {
	remaining := *data.Quota
	if remaining.IsNegative() {
		remaining = decimal.Zero
	}
	used := decimal.Zero
	if data.UsedQuota != nil && !data.UsedQuota.IsNegative() {
		used = *data.UsedQuota
	}
	info := model.ChannelBalanceInfo{
		Remaining:   remaining.String(),
		Total:       remaining.Add(used).String(),
		Used:        used.String(),
		Unit:        model.ChannelBalanceUnitCredits,
		DisplayUnit: "credits",
		MetricKind:  dto.ChannelBalanceMetricWallet,
		Source:      source,
		UpdatedAt:   common.GetTimestamp(),
	}
	var legacyBalance *float64
	if status != nil && status.QuotaPerUnit != nil && status.QuotaPerUnit.IsPositive() {
		convert := func(value decimal.Decimal, multiplier decimal.Decimal) decimal.Decimal {
			return value.Div(*status.QuotaPerUnit).Mul(multiplier)
		}
		multiplier := decimal.NewFromInt(1)
		displayType := strings.ToUpper(strings.TrimSpace(status.QuotaDisplayType))
		switch displayType {
		case "TOKENS":
			info.Unit, info.DisplayUnit = model.ChannelBalanceUnitTokens, "tokens"
			return info, nil
		case "CNY":
			if status.USDExchangeRate == nil || !status.USDExchangeRate.IsPositive() {
				return info, nil
			}
			multiplier = *status.USDExchangeRate
			info.Unit, info.Currency, info.DisplayUnit = model.ChannelBalanceUnitMoney, "CNY", "¥"
		case "CUSTOM":
			if status.CustomCurrencyExchangeRate == nil || !status.CustomCurrencyExchangeRate.IsPositive() {
				return info, nil
			}
			multiplier = *status.CustomCurrencyExchangeRate
			info.Unit, info.Currency = model.ChannelBalanceUnitMoney, "CUSTOM"
			info.DisplayUnit = strings.TrimSpace(status.CustomCurrencySymbol)
			if info.DisplayUnit == "" {
				info.DisplayUnit = "¤"
			}
		default:
			info.Unit, info.Currency, info.DisplayUnit = model.ChannelBalanceUnitMoney, "USD", "$"
		}
		info.Remaining = convert(remaining, multiplier).String()
		info.Used = convert(used, multiplier).String()
		info.Total = convert(remaining.Add(used), multiplier).String()
		if info.Currency == "USD" {
			value := convert(remaining, multiplier).InexactFloat64()
			legacyBalance = &value
		}
	}
	return info, legacyBalance
}

func updateChannelSub2APIBalance(channel *model.Channel) (channelBalanceResult, error) {
	body, err := requestChannelBalanceJSON(channel, "/v1/usage", GetAuthHeader(channel.Key))
	if err != nil {
		return channelBalanceResult{}, err
	}
	info, legacyBalance, err := parseSub2APIBalance(body)
	if err != nil {
		return channelBalanceResult{}, err
	}
	if err := channel.UpdateBalanceInfo(info, legacyBalance); err != nil {
		return channelBalanceResult{}, err
	}
	return balanceResult(info, legacyBalance), nil
}

func parseSub2APIBalance(body []byte) (model.ChannelBalanceInfo, *float64, error) {
	raw := json.RawMessage(body)
	active, activeFound, err := firstBalanceBool(raw, "isValid", "is_active")
	if err != nil {
		return model.ChannelBalanceInfo{}, nil, err
	}
	if activeFound && !active {
		return model.ChannelBalanceInfo{}, nil, errors.New("Sub2API key is inactive")
	}

	remaining, found, err := firstBalanceDecimal(raw, "remaining", "quota.remaining", "balance")
	if err != nil {
		return model.ChannelBalanceInfo{}, nil, err
	}
	metricKind := dto.ChannelBalanceMetricWallet
	if _, quotaFound, _ := balanceRawAt(raw, "quota"); quotaFound {
		metricKind = dto.ChannelBalanceMetricQuota
	}
	if _, subscriptionFound, _ := balanceRawAt(raw, "subscription"); subscriptionFound {
		metricKind = dto.ChannelBalanceMetricSubscription
	}
	unit, _, _ := firstBalanceString(raw, "unit", "quota.unit")
	if strings.TrimSpace(unit) == "" {
		unit = "USD"
	}

	total, totalFound, totalErr := firstBalanceDecimal(raw, "quota.limit")
	if totalErr != nil {
		return model.ChannelBalanceInfo{}, nil, totalErr
	}
	used, usedFound, usedErr := firstBalanceDecimal(raw, "quota.used")
	if usedErr != nil {
		return model.ChannelBalanceInfo{}, nil, usedErr
	}

	if !found {
		remaining, total, used, found, err = parseSub2APIRateLimit(raw)
		if err != nil {
			return model.ChannelBalanceInfo{}, nil, err
		}
		if found {
			metricKind = dto.ChannelBalanceMetricRateLimit
			unit = "requests"
			totalFound, usedFound = true, true
		}
	}
	if !found {
		return model.ChannelBalanceInfo{}, nil, errors.New("Sub2API usage response does not contain remaining balance or rate limit data")
	}

	unlimited := remaining.IsNegative()
	if unlimited {
		remaining = decimal.Zero
	}
	info := model.ChannelBalanceInfo{
		Remaining:  remaining.String(),
		MetricKind: metricKind,
		Source:     dto.ChannelBalanceQueryModeSub2API,
		Unlimited:  unlimited,
		UpdatedAt:  common.GetTimestamp(),
	}
	if unlimited {
		info.Remaining = ""
	}
	if totalFound {
		info.Total = total.String()
	}
	if usedFound {
		info.Used = used.String()
	}

	var legacyBalance *float64
	switch strings.ToUpper(strings.TrimSpace(unit)) {
	case "USD", "$":
		info.Unit, info.Currency, info.DisplayUnit = model.ChannelBalanceUnitMoney, "USD", "$"
		if !unlimited {
			value := remaining.InexactFloat64()
			legacyBalance = &value
		}
	case "CNY", "RMB", "¥":
		info.Unit, info.Currency, info.DisplayUnit = model.ChannelBalanceUnitMoney, "CNY", "¥"
	case "TOKENS", "TOKEN":
		info.Unit, info.DisplayUnit = model.ChannelBalanceUnitTokens, "tokens"
	case "REQUESTS", "REQUEST":
		info.Unit, info.DisplayUnit = model.ChannelBalanceUnitRequests, "requests"
	default:
		info.Unit, info.DisplayUnit = model.ChannelBalanceUnitCredits, unit
	}
	return info, legacyBalance, nil
}

func parseSub2APIRateLimit(raw json.RawMessage) (decimal.Decimal, decimal.Decimal, decimal.Decimal, bool, error) {
	value, found, err := balanceRawAt(raw, "rate_limits")
	if err != nil || !found {
		return decimal.Zero, decimal.Zero, decimal.Zero, false, err
	}
	var entries []json.RawMessage
	if err := common.Unmarshal(value, &entries); err != nil {
		return decimal.Zero, decimal.Zero, decimal.Zero, false, fmt.Errorf("invalid Sub2API rate_limits: %w", err)
	}
	var selectedRemaining, selectedTotal, selectedUsed decimal.Decimal
	selected := false
	for _, entry := range entries {
		remaining, remainingFound, err := balanceDecimalAt(entry, "remaining")
		if err != nil {
			return decimal.Zero, decimal.Zero, decimal.Zero, false, err
		}
		if !remainingFound {
			continue
		}
		limit, limitFound, err := balanceDecimalAt(entry, "limit")
		if err != nil {
			return decimal.Zero, decimal.Zero, decimal.Zero, false, err
		}
		used, usedFound, err := balanceDecimalAt(entry, "used")
		if err != nil {
			return decimal.Zero, decimal.Zero, decimal.Zero, false, err
		}
		if !selected || remaining.LessThan(selectedRemaining) {
			selectedRemaining = remaining
			selectedTotal = decimal.Zero
			selectedUsed = decimal.Zero
			if limitFound {
				selectedTotal = limit
			}
			if usedFound {
				selectedUsed = used
			}
			selected = true
		}
	}
	return selectedRemaining, selectedTotal, selectedUsed, selected, nil
}

func updateChannelCustomBalance(channel *model.Channel, config *dto.ChannelBalanceQueryConfig) (channelBalanceResult, error) {
	info, legacyBalance, responseBody, err := queryChannelCustomBalance(channel, config)
	if err != nil {
		if errors.Is(err, errBalanceAccountInactive) {
			return channelBalanceResult{}, err
		}
		formatted, formatErr := common.IndentJson(responseBody)
		if len(responseBody) > 0 && formatErr == nil {
			return channelBalanceResult{RawResponse: string(formatted)}, nil
		}
		return channelBalanceResult{}, err
	}
	if err := channel.UpdateBalanceInfo(info, legacyBalance); err != nil {
		return channelBalanceResult{}, err
	}
	return balanceResult(info, legacyBalance), nil
}

func queryChannelCustomBalance(channel *model.Channel, config *dto.ChannelBalanceQueryConfig) (model.ChannelBalanceInfo, *float64, []byte, error) {
	if err := config.Validate(); err != nil {
		return model.ChannelBalanceInfo{}, nil, nil, err
	}
	headers, path, requestBody, err := buildCustomBalanceRequest(config, channel.Key)
	if err != nil {
		return model.ChannelBalanceInfo{}, nil, nil, err
	}
	var responseBody []byte
	if strings.TrimSpace(config.URL) != "" {
		responseBody, err = requestBalanceJSONURLWithMethod(channel, path, headers, config.Method, requestBody)
	} else {
		requestURL, buildErr := buildChannelManagementURL(channel.GetBaseURL(), path)
		if buildErr != nil {
			return model.ChannelBalanceInfo{}, nil, nil, buildErr
		}
		responseBody, err = requestBalanceJSONURLWithMethod(channel, requestURL, headers, config.Method, requestBody)
	}
	if err != nil {
		return model.ChannelBalanceInfo{}, nil, responseBody, err
	}
	info, legacyBalance, err := parseCustomBalance(responseBody, config)
	if err != nil {
		return model.ChannelBalanceInfo{}, nil, responseBody, err
	}
	return info, legacyBalance, responseBody, nil
}

func buildCustomBalanceRequest(config *dto.ChannelBalanceQueryConfig, key string) (http.Header, string, string, error) {
	headers := http.Header{}
	path := strings.TrimSpace(config.URL)
	if path == "" {
		path = strings.TrimSpace(config.Path)
	}
	auth := config.Auth
	if auth == nil {
		headers.Set("Authorization", "Bearer "+key)
	} else {
		value := replaceBalanceQueryKey(auth.Value, key)
		switch strings.TrimSpace(auth.Type) {
		case "", dto.AdvancedCustomAuthTypeNone:
		case dto.AdvancedCustomAuthTypeHeader:
			name := strings.TrimSpace(auth.Name)
			if name == "" || strings.ContainsAny(name, " \t\r\n:") || strings.ContainsAny(value, "\r\n") {
				return nil, "", "", errors.New("invalid balance query header authentication")
			}
			headers.Set(name, value)
		case dto.AdvancedCustomAuthTypeQuery:
			parsed, err := url.Parse(path)
			if err != nil {
				return nil, "", "", errors.New("invalid balance query path")
			}
			name := strings.TrimSpace(auth.Name)
			if name == "" {
				return nil, "", "", errors.New("balance query auth query name is required")
			}
			query := parsed.Query()
			query.Set(name, value)
			parsed.RawQuery = query.Encode()
			path = parsed.String()
		default:
			return nil, "", "", fmt.Errorf("invalid balance query auth type: %s", auth.Type)
		}
	}
	for _, header := range config.Headers {
		name := strings.TrimSpace(header.Name)
		value := replaceBalanceQueryKey(header.Value, key)
		if name == "" || strings.ContainsAny(name, " \t\r\n:") || strings.ContainsAny(value, "\r\n") {
			return nil, "", "", errors.New("invalid balance query additional header")
		}
		headers.Set(name, value)
	}
	requestBody := replaceBalanceQueryKey(config.Body, key)
	if strings.EqualFold(strings.TrimSpace(config.Method), http.MethodPost) && strings.TrimSpace(requestBody) != "" {
		var validatedBody json.RawMessage
		if err := common.Unmarshal([]byte(requestBody), &validatedBody); err != nil {
			return nil, "", "", fmt.Errorf("invalid balance query POST body: %w", err)
		}
	}
	return headers, path, requestBody, nil
}

func replaceBalanceQueryKey(value, key string) string {
	return strings.ReplaceAll(value, "{api_key}", strings.TrimSpace(key))
}

func parseCustomBalance(body []byte, config *dto.ChannelBalanceQueryConfig) (model.ChannelBalanceInfo, *float64, error) {
	raw := json.RawMessage(body)
	if successPath := strings.TrimSpace(config.Response.SuccessPath); successPath != "" {
		actual, found, err := balanceRawAt(raw, successPath)
		if err != nil {
			return model.ChannelBalanceInfo{}, nil, err
		}
		if !found {
			return model.ChannelBalanceInfo{}, nil, fmt.Errorf("success field not found at %s", successPath)
		}
		var actualValue any
		if err := common.Unmarshal(actual, &actualValue); err != nil {
			return model.ChannelBalanceInfo{}, nil, err
		}
		expectedText := strings.TrimSpace(config.Response.SuccessValue)
		var expectedValue any
		if err := common.Unmarshal([]byte(expectedText), &expectedValue); err != nil {
			expectedValue = expectedText
		}
		if !reflect.DeepEqual(actualValue, expectedValue) {
			return model.ChannelBalanceInfo{}, nil, fmt.Errorf("balance response success condition did not match at %s", successPath)
		}
	}
	if active, found, err := balanceBoolAt(raw, config.Response.ActivePath); err != nil {
		return model.ChannelBalanceInfo{}, nil, err
	} else if found && !active {
		return model.ChannelBalanceInfo{}, nil, errBalanceAccountInactive
	}
	unlimited := false
	if configuredUnlimited, found, err := balanceBoolAt(raw, config.Response.UnlimitedPath); err != nil {
		return model.ChannelBalanceInfo{}, nil, err
	} else if found {
		unlimited = configuredUnlimited
	}

	total, totalFound, err := balanceDecimalAt(raw, config.Response.TotalPath)
	if err != nil {
		return model.ChannelBalanceInfo{}, nil, err
	}
	used, usedFound, err := balanceDecimalAt(raw, config.Response.UsedPath)
	if err != nil {
		return model.ChannelBalanceInfo{}, nil, err
	}
	remainingMode := strings.TrimSpace(config.RemainingMode)
	if remainingMode == "" {
		remainingMode = dto.ChannelBalanceRemainingDirect
	}
	remaining := decimal.Zero
	switch remainingMode {
	case dto.ChannelBalanceRemainingDirect:
		var found bool
		remaining, found, err = balanceDecimalAt(raw, config.Response.RemainingPath)
		if err != nil {
			return model.ChannelBalanceInfo{}, nil, err
		}
		if !found {
			return model.ChannelBalanceInfo{}, nil, fmt.Errorf("balance field not found at %s", config.Response.RemainingPath)
		}
	case dto.ChannelBalanceRemainingTotalMinusUsed:
		if !totalFound || !usedFound {
			return model.ChannelBalanceInfo{}, nil, errors.New("balance total or used field was not found")
		}
		remaining = total.Sub(used)
	default:
		return model.ChannelBalanceInfo{}, nil, fmt.Errorf("unsupported balance remaining mode: %s", remainingMode)
	}
	multiplier := decimal.NewFromInt(1)
	if strings.TrimSpace(config.Multiplier) != "" {
		parsed, err := decimal.NewFromString(strings.TrimSpace(config.Multiplier))
		if err != nil || !parsed.IsPositive() {
			return model.ChannelBalanceInfo{}, nil, errors.New("balance query multiplier must be a positive number")
		}
		multiplier = parsed
	}
	remaining = remaining.Mul(multiplier)
	if !unlimited && remaining.IsNegative() {
		remaining = decimal.Zero
	}

	info := model.ChannelBalanceInfo{
		Remaining:   remaining.String(),
		Unit:        strings.TrimSpace(config.Unit),
		Currency:    strings.ToUpper(strings.TrimSpace(config.Currency)),
		DisplayUnit: strings.TrimSpace(config.DisplayUnit),
		MetricKind:  strings.TrimSpace(config.MetricKind),
		Source:      dto.ChannelBalanceQueryModeCustom,
		Unlimited:   unlimited,
		UpdatedAt:   common.GetTimestamp(),
	}
	if info.Unit == "" {
		info.Unit = model.ChannelBalanceUnitMoney
	}
	if info.MetricKind == "" {
		info.MetricKind = dto.ChannelBalanceMetricCustom
	}
	if unlimited {
		info.Remaining = ""
	}
	if currency, found, err := balanceStringAt(raw, config.Response.CurrencyPath); err != nil {
		return model.ChannelBalanceInfo{}, nil, err
	} else if found && strings.TrimSpace(currency) != "" {
		info.Currency = strings.ToUpper(strings.TrimSpace(currency))
	}
	if info.DisplayUnit == "" {
		info.DisplayUnit = defaultBalanceDisplayUnit(info.Unit, info.Currency)
	}
	if totalFound {
		info.Total = total.Mul(multiplier).String()
	}
	if usedFound {
		info.Used = used.Mul(multiplier).String()
	}

	var legacyBalance *float64
	if !unlimited && info.Unit == model.ChannelBalanceUnitMoney && info.Currency == "USD" {
		value := remaining.InexactFloat64()
		legacyBalance = &value
	}
	return info, legacyBalance, nil
}

func defaultBalanceDisplayUnit(unit, currency string) string {
	if unit == model.ChannelBalanceUnitMoney {
		switch strings.ToUpper(currency) {
		case "USD":
			return "$"
		case "CNY", "RMB":
			return "¥"
		case "EUR":
			return "€"
		}
	}
	switch unit {
	case model.ChannelBalanceUnitTokens:
		return "tokens"
	case model.ChannelBalanceUnitRequests:
		return "requests"
	case model.ChannelBalanceUnitCredits:
		return "credits"
	}
	return currency
}

func requestChannelBalanceJSON(channel *model.Channel, path string, headers http.Header) ([]byte, error) {
	requestURL, err := buildChannelManagementURL(channel.GetBaseURL(), path)
	if err != nil {
		return nil, err
	}
	return requestBalanceJSONURL(channel, requestURL, headers)
}

func requestBalanceJSONURL(channel *model.Channel, requestURL string, headers http.Header) ([]byte, error) {
	return requestBalanceJSONURLWithMethod(channel, requestURL, headers, http.MethodGet, "")
}

func requestBalanceJSONURLWithMethod(channel *model.Channel, requestURL string, headers http.Header, method string, requestBody string) ([]byte, error) {
	parsedURL, err := url.Parse(strings.TrimSpace(requestURL))
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" || parsedURL.User != nil || parsedURL.Fragment != "" {
		return nil, errors.New("invalid balance query URL")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	var bodyReader io.Reader
	if method == http.MethodPost {
		bodyReader = strings.NewReader(requestBody)
	}
	request, err := http.NewRequest(method, requestURL, bodyReader)
	if err != nil {
		return nil, sanitizeAdvancedCustomRequestError(err, channel.Key, requestURL)
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
		if strings.EqualFold(name, "Host") {
			request.Host = headers.Get(name)
		}
	}
	if method == http.MethodPost && request.Header.Get("Content-Type") == "" {
		request.Header.Set("Content-Type", "application/json")
	}
	client, err := service.GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, sanitizeAdvancedCustomRequestError(err, channel.Key, requestURL)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxChannelBalanceResponseBytes+1))
	if err != nil {
		return nil, sanitizeAdvancedCustomRequestError(err, channel.Key, requestURL)
	}
	if len(responseBody) > maxChannelBalanceResponseBytes {
		return nil, fmt.Errorf("balance response exceeds %d bytes", maxChannelBalanceResponseBytes)
	}
	if response.StatusCode != http.StatusOK {
		if message := balanceResponseErrorMessage(responseBody); message != "" {
			return nil, fmt.Errorf("balance query returned status code %d: %s", response.StatusCode, message)
		}
		return nil, fmt.Errorf("balance query returned status code %d", response.StatusCode)
	}
	var validated json.RawMessage
	if err := common.Unmarshal(responseBody, &validated); err != nil {
		return nil, fmt.Errorf("invalid balance JSON response: %w", err)
	}
	return responseBody, nil
}

func balanceResponseErrorMessage(body []byte) string {
	var payload struct {
		Message string `json:"message"`
	}
	if common.Unmarshal(body, &payload) != nil {
		return ""
	}
	message := strings.Join(strings.Fields(payload.Message), " ")
	if len(message) > 240 {
		message = message[:240]
	}
	return message
}

func buildChannelManagementURL(baseURLValue, pathValue string) (string, error) {
	baseURL, err := url.Parse(strings.TrimSpace(baseURLValue))
	if err != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return "", errors.New("invalid channel base URL")
	}
	pathURL, err := url.Parse(strings.TrimSpace(pathValue))
	if err != nil || pathURL.IsAbs() || pathURL.Host != "" || !strings.HasPrefix(pathURL.Path, "/") || strings.HasPrefix(pathURL.Path, "//") || pathURL.Fragment != "" {
		return "", errors.New("invalid balance query path")
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + pathURL.Path
	baseURL.RawPath = ""
	baseURL.RawQuery = pathURL.RawQuery
	return baseURL.String(), nil
}

func balanceResult(info model.ChannelBalanceInfo, legacyBalance *float64) channelBalanceResult {
	result := channelBalanceResult{Info: &info}
	if legacyBalance != nil {
		result.Balance = *legacyBalance
		result.HasLegacyBalance = true
	}
	return result
}

func balanceRawAt(raw json.RawMessage, path string) (json.RawMessage, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false, nil
	}
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")
	if path == "" {
		return raw, true, nil
	}
	current := raw
	for _, segment := range strings.Split(path, ".") {
		if segment == "" {
			return nil, false, fmt.Errorf("invalid JSON path: %s", path)
		}
		switch common.GetJsonType(current) {
		case "object":
			var object map[string]json.RawMessage
			if err := common.Unmarshal(current, &object); err != nil {
				return nil, false, err
			}
			next, ok := object[segment]
			if !ok {
				return nil, false, nil
			}
			current = next
		case "array":
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 {
				return nil, false, fmt.Errorf("JSON path segment %s is not a valid array index", segment)
			}
			var array []json.RawMessage
			if err := common.Unmarshal(current, &array); err != nil {
				return nil, false, err
			}
			if index >= len(array) {
				return nil, false, nil
			}
			current = array[index]
		default:
			return nil, false, nil
		}
	}
	return current, true, nil
}

func balanceDecimalAt(raw json.RawMessage, path string) (decimal.Decimal, bool, error) {
	value, found, err := balanceRawAt(raw, path)
	if err != nil || !found {
		return decimal.Zero, found, err
	}
	var text string
	switch common.GetJsonType(value) {
	case "number":
		text = string(value)
	case "string":
		if err := common.Unmarshal(value, &text); err != nil {
			return decimal.Zero, false, err
		}
	default:
		return decimal.Zero, false, fmt.Errorf("balance value at %s must be a number or numeric string", path)
	}
	parsed, err := decimal.NewFromString(strings.TrimSpace(text))
	if err != nil {
		return decimal.Zero, false, fmt.Errorf("invalid balance value at %s", path)
	}
	return parsed, true, nil
}

func balanceStringAt(raw json.RawMessage, path string) (string, bool, error) {
	value, found, err := balanceRawAt(raw, path)
	if err != nil || !found {
		return "", found, err
	}
	var text string
	if common.GetJsonType(value) != "string" {
		return "", false, fmt.Errorf("balance value at %s must be a string", path)
	}
	if err := common.Unmarshal(value, &text); err != nil {
		return "", false, err
	}
	return text, true, nil
}

func balanceBoolAt(raw json.RawMessage, path string) (bool, bool, error) {
	value, found, err := balanceRawAt(raw, path)
	if err != nil || !found {
		return false, found, err
	}
	if common.GetJsonType(value) == "boolean" {
		var result bool
		if err := common.Unmarshal(value, &result); err != nil {
			return false, false, err
		}
		return result, true, nil
	}
	if common.GetJsonType(value) == "string" {
		var text string
		if err := common.Unmarshal(value, &text); err != nil {
			return false, false, err
		}
		result, err := strconv.ParseBool(strings.TrimSpace(text))
		if err == nil {
			return result, true, nil
		}
	}
	return false, false, fmt.Errorf("balance value at %s must be a boolean", path)
}

func firstBalanceDecimal(raw json.RawMessage, paths ...string) (decimal.Decimal, bool, error) {
	for _, path := range paths {
		value, found, err := balanceDecimalAt(raw, path)
		if err != nil {
			return decimal.Zero, false, err
		}
		if found {
			return value, true, nil
		}
	}
	return decimal.Zero, false, nil
}

func firstBalanceString(raw json.RawMessage, paths ...string) (string, bool, error) {
	for _, path := range paths {
		value, found, err := balanceStringAt(raw, path)
		if err != nil {
			return "", false, err
		}
		if found {
			return value, true, nil
		}
	}
	return "", false, nil
}

func firstBalanceBool(raw json.RawMessage, paths ...string) (bool, bool, error) {
	for _, path := range paths {
		value, found, err := balanceBoolAt(raw, path)
		if err != nil {
			return false, false, err
		}
		if found {
			return value, true, nil
		}
	}
	return false, false, nil
}

func channelBalanceExhausted(info *model.ChannelBalanceInfo) bool {
	if info == nil || info.Unlimited || info.Remaining == "" {
		return false
	}
	if info.MetricKind == dto.ChannelBalanceMetricSubscription || info.MetricKind == dto.ChannelBalanceMetricRateLimit {
		return false
	}
	remaining, err := decimal.NewFromString(info.Remaining)
	return err == nil && !remaining.IsPositive()
}

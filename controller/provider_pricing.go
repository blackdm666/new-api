package controller

import (
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

const (
	providerPricingSchemaVersion         = "1.1"
	providerPricingTokenUnit             = "per_1m_tokens"
	providerPricingCallUnit              = "per_call"
	providerPricingClaudeCache1hMultiple = 6.0 / 3.75
)

type providerPricingResponse struct {
	SchemaVersion string               `json:"schema_version"`
	Success       bool                 `json:"success"`
	Message       string               `json:"message"`
	Data          *providerPricingData `json:"data,omitempty"`
}

type providerPricingData struct {
	Currency   string                 `json:"currency"`
	PriceUnit  string                 `json:"price_unit"`
	SiteName   string                 `json:"site_name,omitempty"`
	SiteDomain string                 `json:"site_domain,omitempty"`
	UpdatedAt  string                 `json:"updated_at"`
	Models     []providerPricingModel `json:"models"`
}

type providerPricingModel struct {
	ModelName          string   `json:"model_name"`
	GroupName          string   `json:"group_name"`
	PriceUnit          string   `json:"price_unit,omitempty"`
	InputPrice         *float64 `json:"input_price,omitempty"`
	OutputPrice        *float64 `json:"output_price,omitempty"`
	CacheInputPrice    *float64 `json:"cache_input_price,omitempty"`
	CacheCreatePrice   *float64 `json:"cache_create_price,omitempty"`
	CacheCreatePrice1h *float64 `json:"cache_create_price_1h,omitempty"`
	UnitPrice          *float64 `json:"unit_price,omitempty"`
	Enabled            bool     `json:"enabled"`
	Note               string   `json:"note,omitempty"`
}

// GetProviderPricing exposes NewAPI's live pricing in Hvoy Provider Pricing API
// v1.1 format. Dynamic token expressions publish the current standard tier's
// verified linear token rates; billing shapes that the schema cannot represent
// (for example per-second or task-usage expressions) remain omitted.
func GetProviderPricing(c *gin.Context) {
	usdToCNY := operation_setting.USDExchangeRate
	if !isFiniteNonNegative(usdToCNY) || usdToCNY == 0 {
		c.JSON(500, providerPricingResponse{
			SchemaVersion: providerPricingSchemaVersion,
			Success:       false,
			Message:       "invalid USD to CNY exchange rate",
		})
		return
	}

	models := buildProviderPricingModels(model.GetPricing(), ratio_setting.GetGroupRatioCopy(), usdToCNY)
	c.Header("Cache-Control", "public, max-age=60")
	c.JSON(200, providerPricingResponse{
		SchemaVersion: providerPricingSchemaVersion,
		Success:       true,
		Message:       "",
		Data: &providerPricingData{
			Currency:   "CNY",
			PriceUnit:  providerPricingTokenUnit,
			SiteName:   common.SystemName,
			SiteDomain: providerPricingSiteDomain(system_setting.ServerAddress),
			UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
			Models:     models,
		},
	})
}

func buildProviderPricingModels(pricing []model.Pricing, groupRatios map[string]float64, usdToCNY float64) []providerPricingModel {
	result := make([]providerPricingModel, 0)
	if !isFiniteNonNegative(usdToCNY) || usdToCNY == 0 {
		return result
	}

	for _, item := range pricing {
		if strings.TrimSpace(item.ModelName) == "" {
			continue
		}
		if item.QuotaType == 1 && item.BillingUnit == billing_setting.BillingUnitSecond {
			continue
		}

		for _, group := range providerPricingGroups(item.EnableGroup, groupRatios) {
			groupRatio := groupRatios[group]
			if !isFiniteNonNegative(groupRatio) {
				continue
			}

			entry := providerPricingModel{
				ModelName: item.ModelName,
				GroupName: group,
				Enabled:   true,
			}

			switch item.QuotaType {
			case 0:
				if item.BillingMode == billing_setting.BillingModeTieredExpr {
					tiered, ok := providerTieredTokenPrices(item.BillingExpr, groupRatio, usdToCNY)
					if !ok {
						continue
					}
					entry.InputPrice = tiered.InputPrice
					entry.OutputPrice = tiered.OutputPrice
					entry.CacheInputPrice = tiered.CacheInputPrice
					entry.CacheCreatePrice = tiered.CacheCreatePrice
					entry.CacheCreatePrice1h = tiered.CacheCreatePrice1h
					entry.Note = "动态计价；当前标准档"
					if tiered.Tier != "" {
						entry.Note += "：" + tiered.Tier
					}
					break
				}
				if !isFiniteNonNegative(item.ModelRatio) || !isFiniteNonNegative(item.CompletionRatio) {
					continue
				}
				inputPrice := item.ModelRatio * 2 * groupRatio * usdToCNY
				outputPrice := inputPrice * item.CompletionRatio
				if !isFiniteNonNegative(inputPrice) || !isFiniteNonNegative(outputPrice) {
					continue
				}
				entry.InputPrice = float64Pointer(inputPrice)
				entry.OutputPrice = float64Pointer(outputPrice)
				if item.CacheRatio != nil && isFiniteNonNegative(*item.CacheRatio) {
					entry.CacheInputPrice = float64Pointer(inputPrice * *item.CacheRatio)
				}
				if item.CreateCacheRatio != nil && isFiniteNonNegative(*item.CreateCacheRatio) {
					cacheCreatePrice := inputPrice * *item.CreateCacheRatio
					entry.CacheCreatePrice = float64Pointer(cacheCreatePrice)
					if isClaudeModelName(item.ModelName) {
						entry.CacheCreatePrice1h = float64Pointer(cacheCreatePrice * providerPricingClaudeCache1hMultiple)
					}
				}
			case 1:
				unitPrice := item.ModelPrice * groupRatio * usdToCNY
				if !isFiniteNonNegative(unitPrice) || unitPrice == 0 {
					continue
				}
				entry.PriceUnit = providerPricingCallUnit
				entry.UnitPrice = float64Pointer(unitPrice)
			default:
				continue
			}

			result = append(result, entry)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].ModelName == result[j].ModelName {
			return result[i].GroupName < result[j].GroupName
		}
		return result[i].ModelName < result[j].ModelName
	})
	return result
}

type providerTieredPrices struct {
	InputPrice         *float64
	OutputPrice        *float64
	CacheInputPrice    *float64
	CacheCreatePrice   *float64
	CacheCreatePrice1h *float64
	Tier               string
}

func providerTieredTokenPrices(expr string, groupRatio, usdToCNY float64) (providerTieredPrices, bool) {
	if strings.TrimSpace(expr) == "" || !isFiniteNonNegative(groupRatio) || !isFiniteNonNegative(usdToCNY) {
		return providerTieredPrices{}, false
	}
	if len(billingexpr.UsedUsageKeys(expr)) > 0 {
		return providerTieredPrices{}, false
	}

	used := billingexpr.UsedVars(expr)
	baseParams := billingexpr.TokenParams{Len: 1}
	baseCost, baseTrace, err := billingexpr.RunExpr(expr, baseParams)
	if err != nil || !approximatelyEqual(baseCost, 0) {
		return providerTieredPrices{}, false
	}

	tier := baseTrace.MatchedTier
	resolve := func(variable string) (*float64, bool) {
		if !used[variable] {
			return nil, true
		}
		one := baseParams
		two := baseParams
		setProviderTokenDimension(&one, variable, 1)
		setProviderTokenDimension(&two, variable, 2)
		oneCost, oneTrace, oneErr := billingexpr.RunExpr(expr, one)
		twoCost, twoTrace, twoErr := billingexpr.RunExpr(expr, two)
		if oneErr != nil || twoErr != nil || !sameProviderTier(tier, oneTrace.MatchedTier, twoTrace.MatchedTier) {
			return nil, false
		}
		coefficient := oneCost - baseCost
		if !isFiniteNonNegative(coefficient) || !approximatelyEqual(twoCost-baseCost, coefficient*2) {
			return nil, false
		}
		price := providerRoundedPrice(coefficient * groupRatio * usdToCNY)
		return &price, true
	}

	input, ok := resolve("p")
	if !ok {
		return providerTieredPrices{}, false
	}
	output, ok := resolve("c")
	if !ok {
		return providerTieredPrices{}, false
	}
	cacheRead, ok := resolve("cr")
	if !ok {
		return providerTieredPrices{}, false
	}
	cacheCreate, ok := resolve("cc")
	if !ok {
		return providerTieredPrices{}, false
	}
	cacheCreate1h, ok := resolve("cc1h")
	if !ok {
		return providerTieredPrices{}, false
	}
	if input == nil {
		zero := float64(0)
		input = &zero
	}
	if output == nil && *input == 0 {
		return providerTieredPrices{}, false
	}

	return providerTieredPrices{
		InputPrice:         input,
		OutputPrice:        output,
		CacheInputPrice:    cacheRead,
		CacheCreatePrice:   cacheCreate,
		CacheCreatePrice1h: cacheCreate1h,
		Tier:               tier,
	}, true
}

func setProviderTokenDimension(params *billingexpr.TokenParams, variable string, value float64) {
	switch variable {
	case "p":
		params.P = value
	case "c":
		params.C = value
	case "cr":
		params.CR = value
	case "cc":
		params.CC = value
	case "cc1h":
		params.CC1h = value
	}
}

func sameProviderTier(tiers ...string) bool {
	for _, tier := range tiers[1:] {
		if tier != tiers[0] {
			return false
		}
	}
	return true
}

func approximatelyEqual(left, right float64) bool {
	scale := math.Max(1, math.Max(math.Abs(left), math.Abs(right)))
	return math.Abs(left-right) <= 1e-9*scale
}

func providerRoundedPrice(value float64) float64 {
	return math.Round(value*1_000_000) / 1_000_000
}

func providerPricingGroups(enabledGroups []string, groupRatios map[string]float64) []string {
	groups := make(map[string]struct{})
	expandAll := false
	for _, group := range enabledGroups {
		group = strings.TrimSpace(group)
		if group == "all" {
			expandAll = true
			continue
		}
		if _, ok := groupRatios[group]; ok {
			groups[group] = struct{}{}
		}
	}
	if expandAll {
		for group := range groupRatios {
			if group != "all" && group != "auto" {
				groups[group] = struct{}{}
			}
		}
	}

	result := make([]string, 0, len(groups))
	for group := range groups {
		result = append(result, group)
	}
	sort.Strings(result)
	return result
}

func providerPricingSiteDomain(serverAddress string) string {
	serverAddress = strings.TrimSpace(serverAddress)
	if serverAddress == "" {
		return ""
	}
	parsed, err := url.Parse(serverAddress)
	if err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return strings.Trim(serverAddress, "/")
}

func isClaudeModelName(name string) bool {
	return strings.Contains(strings.ToLower(name), "claude")
}

func isFiniteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func float64Pointer(value float64) *float64 {
	return &value
}

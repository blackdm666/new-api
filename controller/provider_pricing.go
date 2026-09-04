package controller

import (
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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
}

// GetProviderPricing exposes the losslessly representable subset of NewAPI's
// live pricing in Hvoy Provider Pricing API v1.1 format. The Hvoy schema cannot
// represent per-second or dynamic expression billing, so those entries are
// deliberately omitted instead of being published with a misleading price.
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
		if strings.TrimSpace(item.ModelName) == "" || item.BillingMode == billing_setting.BillingModeTieredExpr {
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

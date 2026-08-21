package model

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	AffiliateCommissionStatusPending  = 1
	AffiliateCommissionStatusApproved = 2
	AffiliateCommissionStatusRejected = 3

	AffiliateCommissionEnabledOptionKey               = "AffiliateCommissionEnabled"
	AffiliateCommissionAutoApproveOptionKey           = "AffiliateCommissionAutoApprove"
	AffiliateCommissionDefaultRateOptionKey           = "AffiliateCommissionDefaultRateBasisPoints"
	AffiliateCommissionGroupRatesOptionKey            = "AffiliateCommissionGroupRates"
	AffiliateUpgradeInviteesThresholdOptionKey        = "AffiliateUpgradeEffectiveInviteesThreshold"
	AffiliateGoldUpgradeInviteesThresholdOptionKey    = "AffiliateGoldUpgradeEffectiveInviteesThreshold"
	AffiliateUpgradeTopUpAmountThresholdOptionKey     = "AffiliateUpgradeEffectiveTopUpAmountCents"
	AffiliateGoldUpgradeTopUpAmountThresholdOptionKey = "AffiliateGoldUpgradeEffectiveTopUpAmountCents"
	AffiliateCommissionActivatedAtOptionKey           = "AffiliateCommissionActivatedAt"
	AffiliateUpgradeEffectiveInviteesThreshold        = 50
	AffiliateGoldUpgradeEffectiveInviteesThreshold    = 500
	AffiliateUpgradeEffectiveTopUpAmountCents         = int64(200000)
	AffiliateGoldUpgradeEffectiveTopUpAmountCents     = int64(2000000)
	AffiliateCommissionMaxRateBasisPoints             = 10000
	AffiliateCommissionMaxUpgradeInviteeThreshold     = 1000000
	AffiliateCommissionMaxUpgradeTopUpAmountCents     = int64(100000000000000)
	AffiliatePromoterGroupDefault                     = "default"
	AffiliatePromoterGroupLegacyJunior                = "初级推广"
	AffiliatePromoterGroupAdvanced                    = "高级推广"
	AffiliatePromoterGroupGold                        = "金牌推广"
)

const (
	affiliateCommissionDefaultRateBasisPoints  = 500
	affiliateCommissionAdvancedRateBasisPoints = 1000
	affiliateCommissionGoldRateBasisPoints     = 1500
	affiliateCommissionDefaultGroupRatesJSON   = `{"default":500,"高级推广":1000,"金牌推广":1500}`
)

var (
	ErrAffiliateCommissionNotFound      = errors.New("affiliate commission not found")
	ErrAffiliateCommissionStatusInvalid = errors.New("affiliate commission status invalid")
	ErrAffiliateTopUpInvalid            = errors.New("affiliate top-up is no longer valid")
	ErrAffiliateUpgradeNotEligible      = errors.New("affiliate promoter is not eligible for upgrade")
	ErrAffiliateRejectReasonRequired    = errors.New("affiliate commission rejection reason is required")
)

type AffiliateCommission struct {
	Id                 int    `json:"id"`
	InviterId          int    `json:"inviter_id" gorm:"index;index:idx_affiliate_inviter_status_invitee,priority:1;not null"`
	InviteeId          int    `json:"invitee_id" gorm:"index;index:idx_affiliate_inviter_status_invitee,priority:3;not null"`
	TopUpId            int    `json:"topup_id" gorm:"uniqueIndex;not null"`
	TradeNo            string `json:"trade_no" gorm:"type:varchar(255);index"`
	TopUpAmountCents   int64  `json:"topup_amount_cents" gorm:"bigint;not null;default:0"`
	RateBasisPoints    int    `json:"rate_basis_points" gorm:"not null;default:0"`
	InviterGroup       string `json:"inviter_group" gorm:"type:varchar(64);default:''"`
	TierName           string `json:"tier_name" gorm:"type:varchar(64);default:''"`
	CommissionCents    int64  `json:"commission_cents" gorm:"bigint;not null;default:0"`
	CommissionQuota    int    `json:"commission_quota" gorm:"not null;default:0"`
	Status             int    `json:"status" gorm:"not null;default:1;index;index:idx_affiliate_inviter_status_invitee,priority:2"`
	RejectReason       string `json:"reject_reason" gorm:"type:varchar(255);default:''"`
	OperatorId         int    `json:"operator_id" gorm:"default:0"`
	ApprovedTime       int64  `json:"approved_time" gorm:"default:0"`
	CreatedTime        int64  `json:"created_time" gorm:"autoCreateTime;index"`
	UpdatedTime        int64  `json:"updated_time" gorm:"autoUpdateTime"`
	InviterUsername    string `json:"inviter_username" gorm:"->;-:migration"`
	InviterDisplayName string `json:"inviter_display_name" gorm:"->;-:migration"`
	InviteeUsername    string `json:"invitee_username" gorm:"->;-:migration"`
	InviteeDisplayName string `json:"invitee_display_name" gorm:"->;-:migration"`
}

type AffiliateUpgradeNotice struct {
	Id                        int    `json:"id"`
	InviterId                 int    `json:"inviter_id" gorm:"uniqueIndex:idx_affiliate_upgrade_notice;not null"`
	InviterUsername           string `json:"inviter_username" gorm:"->;-:migration"`
	Threshold                 int    `json:"threshold" gorm:"uniqueIndex:idx_affiliate_upgrade_notice;not null"`
	EffectiveInviteeCount     int    `json:"effective_invitee_count" gorm:"not null;default:0"`
	TopUpAmountThresholdCents int64  `json:"top_up_amount_threshold_cents" gorm:"bigint;not null;default:0"`
	EffectiveTopUpAmountCents int64  `json:"effective_top_up_amount_cents" gorm:"bigint;not null;default:0"`
	AttemptCount              int    `json:"attempt_count" gorm:"not null;default:0"`
	LastAttemptTime           int64  `json:"last_attempt_time" gorm:"not null;default:0;index"`
	NextAttemptTime           int64  `json:"next_attempt_time" gorm:"not null;default:0;index"`
	DeadLetterTime            int64  `json:"dead_letter_time" gorm:"not null;default:0;index"`
	LastError                 string `json:"last_error" gorm:"type:text"`
	CreatedTime               int64  `json:"created_time" gorm:"autoCreateTime"`
	SentTime                  int64  `json:"sent_time" gorm:"default:0"`
}

type AffiliateSummary struct {
	AutoApprove                      bool           `json:"auto_approve"`
	AvailableQuota                   int            `json:"available_quota"`
	AvailableCents                   int64          `json:"available_cents"`
	TotalApprovedQuota               int            `json:"total_approved_quota"`
	PendingCommissionCents           int64          `json:"pending_commission_cents"`
	ApprovedCommissionCents          int64          `json:"approved_commission_cents"`
	TotalTopUpCents                  int64          `json:"total_topup_cents"`
	InviteCount                      int64          `json:"invite_count"`
	EffectiveInviteeCount            int64          `json:"effective_invitee_count"`
	CommissionRecordCount            int64          `json:"commission_record_count"`
	RateBasisPoints                  int            `json:"rate_basis_points"`
	DefaultRateBasisPoints           int            `json:"default_rate_basis_points"`
	GroupRates                       map[string]int `json:"group_rates"`
	TierName                         string         `json:"tier_name"`
	UpgradeEligible                  bool           `json:"upgrade_eligible"`
	NextTierName                     string         `json:"next_tier_name"`
	NextTierRateBasisPoints          int            `json:"next_tier_rate_basis_points"`
	UpgradeThreshold                 int            `json:"upgrade_threshold"`
	UpgradeProgress                  int64          `json:"upgrade_progress"`
	UpgradeProgressRatio             float64        `json:"upgrade_progress_ratio"`
	UpgradeTopUpAmountThresholdCents int64          `json:"upgrade_top_up_amount_threshold_cents"`
	UpgradeTopUpAmountProgressCents  int64          `json:"upgrade_top_up_amount_progress_cents"`
	UpgradeTopUpAmountProgressRatio  float64        `json:"upgrade_top_up_amount_progress_ratio"`
}

type AffiliateInviteeStats struct {
	Id               int    `json:"-"`
	Username         string `json:"username"`
	DisplayName      string `json:"display_name"`
	CreatedAt        int64  `json:"created_at"`
	IsNew            bool   `json:"is_new"`
	TopUpCount       int64  `json:"topup_count"`
	TopUpAmountCents int64  `json:"topup_amount_cents"`
	CommissionCents  int64  `json:"commission_cents"`
	LastTopUpTime    int64  `json:"last_topup_time"`
}

type AffiliateCommissionQueryOptions struct {
	InviterId int
	Status    int
	Keyword   string
}

type AffiliateAdminSummary struct {
	PendingCount          int64 `json:"pending_count"`
	PendingCents          int64 `json:"pending_cents"`
	ApprovedCents         int64 `json:"approved_cents"`
	TotalInviteeCount     int64 `json:"total_invitee_count"`
	EffectiveInviteeCount int64 `json:"effective_invitee_count"`
	TopUpCents            int64 `json:"topup_cents"`
	CommissionRecordCount int64 `json:"commission_record_count"`
}

type AffiliateUpgradeCandidate struct {
	InviterId                 int    `json:"inviter_id"`
	Username                  string `json:"username"`
	DisplayName               string `json:"display_name"`
	CurrentGroup              string `json:"current_group"`
	EffectiveInviteeCount     int64  `json:"effective_invitee_count"`
	Threshold                 int    `json:"threshold"`
	EffectiveTopUpAmountCents int64  `json:"effective_top_up_amount_cents"`
	TopUpAmountThresholdCents int64  `json:"top_up_amount_threshold_cents"`
	EligibleByInvitees        bool   `json:"eligible_by_invitees"`
	EligibleByTopUpAmount     bool   `json:"eligible_by_top_up_amount"`
	NextGroup                 string `json:"next_group"`
	NextRateBasisPoints       int    `json:"next_rate_basis_points"`
}

type AffiliateUpgradeTarget struct {
	Group                     string
	Threshold                 int
	TopUpAmountThresholdCents int64
	RateBasisPoints           int
}

type AffiliateUpgradeMetrics struct {
	EffectiveInviteeCount     int64 `json:"effective_invitee_count"`
	EffectiveTopUpAmountCents int64 `json:"effective_top_up_amount_cents"`
}

type affiliatePolicy struct {
	Enabled                              bool
	AutoApprove                          bool
	DefaultRateBasisPoints               int
	GroupRates                           map[string]int
	UpgradeInviteesThreshold             int
	GoldUpgradeInviteesThreshold         int
	UpgradeTopUpAmountThresholdCents     int64
	GoldUpgradeTopUpAmountThresholdCents int64
	ActivatedAt                          int64
}

func defaultAffiliateGroupRates() map[string]int {
	return map[string]int{
		AffiliatePromoterGroupDefault:  affiliateCommissionDefaultRateBasisPoints,
		AffiliatePromoterGroupAdvanced: affiliateCommissionAdvancedRateBasisPoints,
		AffiliatePromoterGroupGold:     affiliateCommissionGoldRateBasisPoints,
	}
}

func normalizeAffiliateGroupRates(groupRates map[string]int, defaultRate int) map[string]int {
	normalized := make(map[string]int, len(groupRates)+1)
	for group, rate := range groupRates {
		normalized[strings.TrimSpace(group)] = rate
	}
	if _, ok := normalized[AffiliatePromoterGroupDefault]; !ok {
		if legacyRate, legacyExists := normalized[AffiliatePromoterGroupLegacyJunior]; legacyExists {
			normalized[AffiliatePromoterGroupDefault] = legacyRate
		} else {
			normalized[AffiliatePromoterGroupDefault] = defaultRate
		}
	}
	delete(normalized, AffiliatePromoterGroupLegacyJunior)
	return normalized
}

func getAffiliatePolicy() affiliatePolicy {
	policy := affiliatePolicy{
		Enabled:                              false,
		AutoApprove:                          false,
		DefaultRateBasisPoints:               affiliateCommissionDefaultRateBasisPoints,
		GroupRates:                           defaultAffiliateGroupRates(),
		UpgradeInviteesThreshold:             AffiliateUpgradeEffectiveInviteesThreshold,
		GoldUpgradeInviteesThreshold:         AffiliateGoldUpgradeEffectiveInviteesThreshold,
		UpgradeTopUpAmountThresholdCents:     AffiliateUpgradeEffectiveTopUpAmountCents,
		GoldUpgradeTopUpAmountThresholdCents: AffiliateGoldUpgradeEffectiveTopUpAmountCents,
		ActivatedAt:                          0,
	}
	common.OptionMapRWMutex.RLock()
	enabledRaw := common.OptionMap[AffiliateCommissionEnabledOptionKey]
	autoApproveRaw := common.OptionMap[AffiliateCommissionAutoApproveOptionKey]
	defaultRateRaw := common.OptionMap[AffiliateCommissionDefaultRateOptionKey]
	groupRatesRaw := common.OptionMap[AffiliateCommissionGroupRatesOptionKey]
	thresholdRaw := common.OptionMap[AffiliateUpgradeInviteesThresholdOptionKey]
	goldThresholdRaw := common.OptionMap[AffiliateGoldUpgradeInviteesThresholdOptionKey]
	amountThresholdRaw := common.OptionMap[AffiliateUpgradeTopUpAmountThresholdOptionKey]
	goldAmountThresholdRaw := common.OptionMap[AffiliateGoldUpgradeTopUpAmountThresholdOptionKey]
	activatedAtRaw := common.OptionMap[AffiliateCommissionActivatedAtOptionKey]
	common.OptionMapRWMutex.RUnlock()

	if enabledRaw != "" {
		policy.Enabled = enabledRaw == "true"
	}
	if autoApproveRaw != "" {
		policy.AutoApprove = autoApproveRaw == "true"
	}
	if value, err := strconv.Atoi(defaultRateRaw); err == nil && value >= 0 && value <= AffiliateCommissionMaxRateBasisPoints {
		policy.DefaultRateBasisPoints = value
	}
	if groupRatesRaw != "" {
		groupRates := map[string]int{}
		if err := common.Unmarshal([]byte(groupRatesRaw), &groupRates); err == nil {
			groupRates = normalizeAffiliateGroupRates(groupRates, policy.DefaultRateBasisPoints)
			if validateAffiliateGroupRates(groupRates) == nil {
				policy.GroupRates = groupRates
				policy.DefaultRateBasisPoints = groupRates[AffiliatePromoterGroupDefault]
			}
		}
	}
	if value, err := strconv.Atoi(thresholdRaw); err == nil && value >= 1 && value <= AffiliateCommissionMaxUpgradeInviteeThreshold {
		policy.UpgradeInviteesThreshold = value
	}
	if value, err := strconv.Atoi(goldThresholdRaw); err == nil && value >= 1 && value <= AffiliateCommissionMaxUpgradeInviteeThreshold {
		policy.GoldUpgradeInviteesThreshold = value
	}
	if value, err := strconv.ParseInt(amountThresholdRaw, 10, 64); err == nil && value >= 1 && value <= AffiliateCommissionMaxUpgradeTopUpAmountCents {
		policy.UpgradeTopUpAmountThresholdCents = value
	}
	if value, err := strconv.ParseInt(goldAmountThresholdRaw, 10, 64); err == nil && value >= 1 && value <= AffiliateCommissionMaxUpgradeTopUpAmountCents {
		policy.GoldUpgradeTopUpAmountThresholdCents = value
	}
	if value, err := strconv.ParseInt(activatedAtRaw, 10, 64); err == nil && value >= 0 {
		policy.ActivatedAt = value
	}
	return policy
}

func GetAffiliatePolicyState() (enabled bool, activatedAt int64, threshold int, advancedRate int) {
	policy := getAffiliatePolicy()
	return policy.Enabled, policy.ActivatedAt, policy.UpgradeInviteesThreshold, affiliateRateBasisPointsForPolicy(policy, AffiliatePromoterGroupAdvanced)
}

func GetAffiliateUpgradeTarget(group string) (AffiliateUpgradeTarget, bool) {
	policy := getAffiliatePolicy()
	if !policy.Enabled {
		return AffiliateUpgradeTarget{}, false
	}
	return affiliateUpgradeTargetForPolicy(policy, group)
}

func affiliateUpgradeTargetForPolicy(policy affiliatePolicy, group string) (AffiliateUpgradeTarget, bool) {
	if IsAffiliateJuniorGroup(group) {
		return AffiliateUpgradeTarget{
			Group:                     AffiliatePromoterGroupAdvanced,
			Threshold:                 policy.UpgradeInviteesThreshold,
			TopUpAmountThresholdCents: policy.UpgradeTopUpAmountThresholdCents,
			RateBasisPoints:           affiliateRateBasisPointsForPolicy(policy, AffiliatePromoterGroupAdvanced),
		}, policy.UpgradeInviteesThreshold > 0 && policy.UpgradeTopUpAmountThresholdCents > 0
	}
	if strings.TrimSpace(group) == AffiliatePromoterGroupAdvanced && policy.GoldUpgradeInviteesThreshold > policy.UpgradeInviteesThreshold && policy.GoldUpgradeTopUpAmountThresholdCents > policy.UpgradeTopUpAmountThresholdCents {
		return AffiliateUpgradeTarget{
			Group:                     AffiliatePromoterGroupGold,
			Threshold:                 policy.GoldUpgradeInviteesThreshold,
			TopUpAmountThresholdCents: policy.GoldUpgradeTopUpAmountThresholdCents,
			RateBasisPoints:           affiliateRateBasisPointsForPolicy(policy, AffiliatePromoterGroupGold),
		}, true
	}
	return AffiliateUpgradeTarget{}, false
}

func ValidateAffiliateUpgradeThresholds(advancedThreshold int, goldThreshold int, advancedAmountCents int64, goldAmountCents int64) error {
	if advancedThreshold < 1 || advancedThreshold > AffiliateCommissionMaxUpgradeInviteeThreshold {
		return fmt.Errorf("affiliate upgrade threshold must be between 1 and %d", AffiliateCommissionMaxUpgradeInviteeThreshold)
	}
	if goldThreshold < 1 || goldThreshold > AffiliateCommissionMaxUpgradeInviteeThreshold {
		return fmt.Errorf("affiliate gold upgrade threshold must be between 1 and %d", AffiliateCommissionMaxUpgradeInviteeThreshold)
	}
	if goldThreshold <= advancedThreshold {
		return errors.New("affiliate gold upgrade threshold must be greater than the advanced upgrade threshold")
	}
	if advancedAmountCents < 1 || advancedAmountCents > AffiliateCommissionMaxUpgradeTopUpAmountCents {
		return fmt.Errorf("affiliate upgrade top-up amount threshold must be between 1 and %d cents", AffiliateCommissionMaxUpgradeTopUpAmountCents)
	}
	if goldAmountCents < 1 || goldAmountCents > AffiliateCommissionMaxUpgradeTopUpAmountCents {
		return fmt.Errorf("affiliate gold upgrade top-up amount threshold must be between 1 and %d cents", AffiliateCommissionMaxUpgradeTopUpAmountCents)
	}
	if goldAmountCents <= advancedAmountCents {
		return errors.New("affiliate gold upgrade top-up amount threshold must be greater than the advanced upgrade top-up amount threshold")
	}
	return nil
}

func validateAffiliateGroupRates(groupRates map[string]int) error {
	if len(groupRates) == 0 {
		return errors.New("affiliate group rates cannot be empty")
	}
	for group, rate := range groupRates {
		if strings.TrimSpace(group) == "" {
			return errors.New("affiliate group name cannot be empty")
		}
		if rate < 0 || rate > AffiliateCommissionMaxRateBasisPoints {
			return fmt.Errorf("affiliate commission rate for group %s must be between 0 and 10000 basis points", group)
		}
	}
	for _, requiredGroup := range []string{AffiliatePromoterGroupDefault, AffiliatePromoterGroupAdvanced, AffiliatePromoterGroupGold} {
		if _, ok := groupRates[requiredGroup]; !ok {
			return fmt.Errorf("affiliate group rate for %s is required", requiredGroup)
		}
	}
	return nil
}

func GetAffiliateCommissionById(id int) (*AffiliateCommission, error) {
	record := &AffiliateCommission{}
	if err := DB.First(record, id).Error; err != nil {
		return nil, err
	}
	return record, nil
}

func ValidateAffiliateOptionValue(key string, value string) error {
	switch key {
	case AffiliateCommissionEnabledOptionKey, AffiliateCommissionAutoApproveOptionKey:
		if value != "true" && value != "false" {
			return fmt.Errorf("%s must be true or false", key)
		}
	case AffiliateCommissionDefaultRateOptionKey:
		rate, err := strconv.Atoi(value)
		if err != nil || rate < 0 || rate > AffiliateCommissionMaxRateBasisPoints {
			return errors.New("default affiliate commission rate must be between 0 and 10000 basis points")
		}
	case AffiliateCommissionGroupRatesOptionKey:
		groupRates := map[string]int{}
		if err := common.Unmarshal([]byte(value), &groupRates); err != nil {
			return errors.New("affiliate group rates must be valid JSON")
		}
		return validateAffiliateGroupRates(groupRates)
	case AffiliateUpgradeInviteesThresholdOptionKey, AffiliateGoldUpgradeInviteesThresholdOptionKey:
		threshold, err := strconv.Atoi(value)
		if err != nil || threshold < 1 || threshold > AffiliateCommissionMaxUpgradeInviteeThreshold {
			return fmt.Errorf("affiliate upgrade threshold must be between 1 and %d", AffiliateCommissionMaxUpgradeInviteeThreshold)
		}
	case AffiliateUpgradeTopUpAmountThresholdOptionKey, AffiliateGoldUpgradeTopUpAmountThresholdOptionKey:
		threshold, err := strconv.ParseInt(value, 10, 64)
		if err != nil || threshold < 1 || threshold > AffiliateCommissionMaxUpgradeTopUpAmountCents {
			return fmt.Errorf("affiliate upgrade top-up amount threshold must be between 1 and %d cents", AffiliateCommissionMaxUpgradeTopUpAmountCents)
		}
	case AffiliateCommissionActivatedAtOptionKey:
		activatedAt, err := strconv.ParseInt(value, 10, 64)
		if err != nil || activatedAt < 0 {
			return errors.New("affiliate commission activation time must be a non-negative unix timestamp")
		}
	}
	return nil
}

func AffiliateRateBasisPointsForGroup(group string) int {
	policy := getAffiliatePolicy()
	return affiliateRateBasisPointsForPolicy(policy, group)
}

func affiliateRateBasisPointsForPolicy(policy affiliatePolicy, group string) int {
	group = strings.TrimSpace(group)
	if group == AffiliatePromoterGroupLegacyJunior {
		group = AffiliatePromoterGroupDefault
	}
	if rate, ok := policy.GroupRates[group]; ok {
		return rate
	}
	return policy.DefaultRateBasisPoints
}

func AffiliateTierNameForGroup(group string) string {
	switch strings.TrimSpace(group) {
	case AffiliatePromoterGroupDefault, AffiliatePromoterGroupLegacyJunior:
		return AffiliatePromoterGroupLegacyJunior
	case AffiliatePromoterGroupAdvanced:
		return AffiliatePromoterGroupAdvanced
	case AffiliatePromoterGroupGold:
		return AffiliatePromoterGroupGold
	default:
		return "默认推广"
	}
}

func IsAffiliateJuniorGroup(group string) bool {
	group = strings.TrimSpace(group)
	return group == AffiliatePromoterGroupDefault || group == AffiliatePromoterGroupLegacyJunior
}

func localMoneyCents(value float64) int64 {
	return decimal.NewFromFloat(value).Mul(decimal.NewFromInt(100)).Round(0).IntPart()
}

func topUpMoneyCents(topUp *TopUp) int64 {
	if topUp == nil {
		return 0
	}
	if topUp.PaymentProvider == PaymentProviderAntom && topUp.MoneyMinor > 0 {
		return topUp.MoneyMinor
	}
	return localMoneyCents(topUp.Money)
}

func createAffiliateCommissionForTopUpTx(tx *gorm.DB, topUp *TopUp) (bool, int, error) {
	policy := getAffiliatePolicy()
	if !policy.Enabled || topUp == nil || topUp.Id <= 0 || topUp.UserId <= 0 || topUp.Money <= 0 || topUp.Status != common.TopUpStatusSuccess || !isPromotionalTopUpProvider(topUp.PaymentProvider) {
		return false, 0, nil
	}
	completedAt := topUp.CompleteTime
	if completedAt <= 0 {
		completedAt = topUp.CreateTime
	}
	if policy.ActivatedAt <= 0 || completedAt < policy.ActivatedAt {
		return false, 0, nil
	}

	invitee := &User{}
	if err := tx.Select("id", "username", "display_name", "inviter_id").Where("id = ?", topUp.UserId).First(invitee).Error; err != nil {
		return false, 0, err
	}
	if invitee.InviterId <= 0 || invitee.InviterId == invitee.Id {
		return false, 0, nil
	}

	inviter := &User{}
	if err := tx.Unscoped().Select("id", "username", "display_name", "email", "status", "deleted_at", commonGroupCol).Where("id = ?", invitee.InviterId).First(inviter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, 0, nil
		}
		return false, 0, err
	}
	if inviter.DeletedAt.Valid || inviter.Status != common.UserStatusEnabled {
		return false, inviter.Id, nil
	}

	topUpCents := topUpMoneyCents(topUp)
	if topUpCents <= 0 {
		return false, 0, nil
	}
	rate := affiliateRateBasisPointsForPolicy(policy, inviter.Group)
	if rate <= 0 {
		return false, inviter.Id, nil
	}
	commissionCents := decimal.NewFromInt(topUpCents).
		Mul(decimal.NewFromInt(int64(rate))).
		Div(decimal.NewFromInt(10000)).
		Round(0).
		IntPart()
	commissionQuota, err := affiliateCentsToQuota(commissionCents)
	if err != nil || commissionQuota <= 0 {
		return false, inviter.Id, nil
	}

	status := AffiliateCommissionStatusPending
	approvedTime := int64(0)
	if policy.AutoApprove {
		status = AffiliateCommissionStatusApproved
		approvedTime = common.GetTimestamp()
	}
	record := &AffiliateCommission{
		InviterId:        inviter.Id,
		InviteeId:        invitee.Id,
		TopUpId:          topUp.Id,
		TradeNo:          topUp.TradeNo,
		TopUpAmountCents: topUpCents,
		RateBasisPoints:  rate,
		InviterGroup:     inviter.Group,
		TierName:         AffiliateTierNameForGroup(inviter.Group),
		CommissionCents:  commissionCents,
		CommissionQuota:  commissionQuota,
		Status:           status,
		ApprovedTime:     approvedTime,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(record).Error; err != nil {
		return false, 0, err
	}
	if record.Id <= 0 {
		return false, inviter.Id, nil
	}
	if policy.AutoApprove {
		if err := creditAffiliateAccountTx(tx, inviter.Id, commissionCents); err != nil {
			return false, inviter.Id, err
		}
	}

	noticeCreated, err := createAffiliateUpgradeNoticeIfNeededTx(tx, inviter, policy)
	if err != nil {
		return false, inviter.Id, err
	}
	return noticeCreated, inviter.Id, nil
}

// tryCreateAffiliateCommissionForTopUpTx keeps the referral side effect from
// rolling back a successful payment. A savepoint isolates optional commission
// work so PostgreSQL can recover from a failed statement before the outer
// top-up transaction commits.
func tryCreateAffiliateCommissionForTopUpTx(tx *gorm.DB, topUp *TopUp) {
	const savepoint = "affiliate_commission_optional"
	if err := tx.SavePoint(savepoint).Error; err != nil {
		common.SysError(fmt.Sprintf("affiliate commission savepoint failed trade_no=%s: %s", topUp.TradeNo, err.Error()))
		return
	}
	if _, _, err := createAffiliateCommissionForTopUpTx(tx, topUp); err != nil {
		if rollbackErr := tx.RollbackTo(savepoint).Error; rollbackErr != nil {
			common.SysError(fmt.Sprintf("affiliate commission rollback failed trade_no=%s: %s", topUp.TradeNo, rollbackErr.Error()))
		}
		common.SysError(fmt.Sprintf("affiliate commission skipped to preserve top-up trade_no=%s: %s", topUp.TradeNo, err.Error()))
	}
}

func CreateAffiliateCommissionForTopUp(topUp *TopUp) error {
	if topUp == nil {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		_, _, err := createAffiliateCommissionForTopUpTx(tx, topUp)
		return err
	})
}

func createAffiliateUpgradeNoticeIfNeededTx(tx *gorm.DB, inviter *User, policy affiliatePolicy) (bool, error) {
	if inviter == nil || inviter.Id <= 0 {
		return false, nil
	}
	target, ok := affiliateUpgradeTargetForPolicy(policy, inviter.Group)
	if !ok {
		return false, nil
	}
	metrics, err := getAffiliateUpgradeMetrics(tx, inviter.Id)
	if err != nil {
		return false, err
	}
	if !affiliateUpgradeTargetReached(target, metrics) {
		return false, nil
	}
	notice := &AffiliateUpgradeNotice{
		InviterId:                 inviter.Id,
		Threshold:                 target.Threshold,
		EffectiveInviteeCount:     int(metrics.EffectiveInviteeCount),
		TopUpAmountThresholdCents: target.TopUpAmountThresholdCents,
		EffectiveTopUpAmountCents: metrics.EffectiveTopUpAmountCents,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(notice).Error; err != nil {
		return false, err
	}
	return notice.Id > 0, nil
}

func ListDueAffiliateUpgradeNotices(limit int, retryBefore int64) ([]*AffiliateUpgradeNotice, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	now := common.GetTimestamp()
	notices := []*AffiliateUpgradeNotice{}
	err := DB.Where("sent_time = 0 AND dead_letter_time = 0 AND ((next_attempt_time > 0 AND next_attempt_time <= ?) OR (next_attempt_time = 0 AND (last_attempt_time = 0 OR last_attempt_time <= ?)))", now, retryBefore).
		Order("id ASC").
		Limit(limit).
		Find(&notices).Error
	return notices, err
}

func ClaimAffiliateUpgradeNotice(id int, now int64, retryBefore int64) (bool, error) {
	result := DB.Model(&AffiliateUpgradeNotice{}).
		Where("id = ? AND sent_time = 0 AND dead_letter_time = 0 AND ((next_attempt_time > 0 AND next_attempt_time <= ?) OR (next_attempt_time = 0 AND (last_attempt_time = 0 OR last_attempt_time <= ?)))", id, now, retryBefore).
		Updates(map[string]interface{}{
			"last_attempt_time": now,
			"attempt_count":     gorm.Expr("attempt_count + 1"),
			"next_attempt_time": 0,
			"last_error":        "",
		})
	return result.RowsAffected == 1, result.Error
}

func CompleteAffiliateUpgradeNotice(id int) error {
	return DB.Model(&AffiliateUpgradeNotice{}).
		Where("id = ? AND sent_time = 0", id).
		Updates(map[string]interface{}{
			"sent_time":         common.GetTimestamp(),
			"last_error":        "",
			"next_attempt_time": 0,
			"dead_letter_time":  0,
		}).Error
}

func RecordAffiliateUpgradeNoticeFailure(id int, message string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		notice := &AffiliateUpgradeNotice{}
		if err := lockForUpdate(tx).Where("id = ? AND sent_time = 0", id).First(notice).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{"last_error": strings.TrimSpace(message)}
		if notice.AttemptCount >= 8 {
			updates["dead_letter_time"] = common.GetTimestamp()
			updates["next_attempt_time"] = 0
		} else {
			delay := 30 * time.Second * time.Duration(1<<max(notice.AttemptCount-1, 0))
			if delay > 24*time.Hour {
				delay = 24 * time.Hour
			}
			updates["next_attempt_time"] = time.Now().Add(delay).Unix()
		}
		return tx.Model(notice).Updates(updates).Error
	})
}

func PostponeAffiliateUpgradeNotice(id int, message string, delay time.Duration) error {
	if delay < time.Minute {
		delay = time.Minute
	}
	return DB.Model(&AffiliateUpgradeNotice{}).
		Where("id = ? AND sent_time = 0 AND dead_letter_time = 0", id).
		Updates(map[string]interface{}{
			"last_error":        strings.TrimSpace(message),
			"next_attempt_time": time.Now().Add(delay).Unix(),
		}).Error
}

func RetryAffiliateUpgradeNotice(id int) error {
	result := DB.Model(&AffiliateUpgradeNotice{}).Where("id = ? AND sent_time = 0", id).Updates(map[string]interface{}{
		"attempt_count":     0,
		"last_attempt_time": 0,
		"next_attempt_time": 0,
		"dead_letter_time":  0,
		"last_error":        "",
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func ListFailedAffiliateUpgradeNotices(pageInfo *common.PageInfo) ([]*AffiliateUpgradeNotice, int64, error) {
	query := DB.Model(&AffiliateUpgradeNotice{}).
		Joins("LEFT JOIN users ON users.id = affiliate_upgrade_notices.inviter_id").
		Where("affiliate_upgrade_notices.sent_time = 0 AND affiliate_upgrade_notices.last_error <> ''")
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []*AffiliateUpgradeNotice{}
	if err := query.
		Select("affiliate_upgrade_notices.*, COALESCE(users.username, '') AS inviter_username").
		Order("affiliate_upgrade_notices.dead_letter_time DESC, affiliate_upgrade_notices.id DESC").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func CompleteAffiliateCommission(id int, operatorId int, approve bool, reason string) error {
	if !approve && strings.TrimSpace(reason) == "" {
		return ErrAffiliateRejectReasonRequired
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		record := &AffiliateCommission{}
		if err := lockForUpdate(tx).Where("id = ?", id).First(record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAffiliateCommissionNotFound
			}
			return err
		}
		if record.Status != AffiliateCommissionStatusPending {
			return ErrAffiliateCommissionStatusInvalid
		}
		now := common.GetTimestamp()
		if approve {
			topUp := &TopUp{}
			if err := lockForUpdate(tx).Where("id = ?", record.TopUpId).First(topUp).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrAffiliateTopUpInvalid
				}
				return err
			}
			if topUp.Status != common.TopUpStatusSuccess || !isPromotionalTopUpProvider(topUp.PaymentProvider) || topUp.UserId != record.InviteeId || topUp.TradeNo != record.TradeNo || topUpMoneyCents(topUp) != record.TopUpAmountCents {
				return ErrAffiliateTopUpInvalid
			}
			record.Status = AffiliateCommissionStatusApproved
			record.OperatorId = operatorId
			record.ApprovedTime = now
			if err := tx.Save(record).Error; err != nil {
				return err
			}
			return creditAffiliateAccountTx(tx, record.InviterId, record.CommissionCents)
		}
		record.Status = AffiliateCommissionStatusRejected
		record.OperatorId = operatorId
		record.RejectReason = strings.TrimSpace(reason)
		return tx.Save(record).Error
	})
}

func GetAffiliateSummary(userId int) (*AffiliateSummary, error) {
	user, err := GetUserById(userId, false)
	if err != nil {
		return nil, err
	}
	inviteCount, err := CountUserInvitees(userId)
	if err != nil {
		return nil, err
	}
	policy := getAffiliatePolicy()
	account, err := GetAffiliateAccount(userId)
	if err != nil {
		return nil, err
	}
	availableQuota, err := affiliateCentsToQuota(account.AvailableCents)
	if err != nil {
		return nil, err
	}
	totalApprovedQuota, err := affiliateCentsToQuota(account.LifetimeEarnedCents)
	if err != nil {
		return nil, err
	}
	summary := &AffiliateSummary{
		AutoApprove:            policy.AutoApprove,
		AvailableQuota:         availableQuota,
		AvailableCents:         account.AvailableCents,
		TotalApprovedQuota:     totalApprovedQuota,
		InviteCount:            inviteCount,
		RateBasisPoints:        affiliateRateBasisPointsForPolicy(policy, user.Group),
		DefaultRateBasisPoints: policy.DefaultRateBasisPoints,
		GroupRates:             policy.GroupRates,
		TierName:               AffiliateTierNameForGroup(user.Group),
		UpgradeProgressRatio:   0,
	}
	if target, ok := affiliateUpgradeTargetForPolicy(policy, user.Group); ok {
		summary.UpgradeEligible = true
		summary.NextTierName = target.Group
		summary.NextTierRateBasisPoints = target.RateBasisPoints
		summary.UpgradeThreshold = target.Threshold
		summary.UpgradeTopUpAmountThresholdCents = target.TopUpAmountThresholdCents
	}
	type sums struct {
		PendingCents  int64
		ApprovedCents int64
		TopUpCents    int64
		RecordCount   int64
	}
	row := sums{}
	err = DB.Model(&AffiliateCommission{}).
		Select(
			"COALESCE(SUM(CASE WHEN status = ? THEN commission_cents ELSE 0 END), 0) AS pending_cents, COALESCE(SUM(CASE WHEN status = ? THEN commission_cents ELSE 0 END), 0) AS approved_cents, COALESCE(SUM(CASE WHEN status IN (?, ?) THEN top_up_amount_cents ELSE 0 END), 0) AS top_up_cents, COUNT(*) AS record_count",
			AffiliateCommissionStatusPending,
			AffiliateCommissionStatusApproved,
			AffiliateCommissionStatusPending,
			AffiliateCommissionStatusApproved,
		).
		Where("inviter_id = ?", userId).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	summary.PendingCommissionCents = row.PendingCents
	summary.ApprovedCommissionCents = row.ApprovedCents
	summary.TotalTopUpCents = row.TopUpCents
	summary.CommissionRecordCount = row.RecordCount

	if err := DB.Model(&AffiliateCommission{}).
		Where("inviter_id = ? AND status IN (?, ?)", userId, AffiliateCommissionStatusPending, AffiliateCommissionStatusApproved).
		Distinct("invitee_id").
		Count(&summary.EffectiveInviteeCount).Error; err != nil {
		return nil, err
	}
	if summary.UpgradeEligible {
		summary.UpgradeProgress = min(summary.EffectiveInviteeCount, int64(summary.UpgradeThreshold))
		summary.UpgradeTopUpAmountProgressCents = min(summary.TotalTopUpCents, summary.UpgradeTopUpAmountThresholdCents)
	}
	if summary.UpgradeEligible && summary.UpgradeThreshold > 0 {
		summary.UpgradeProgressRatio = math.Min(1, float64(summary.UpgradeProgress)/float64(summary.UpgradeThreshold))
	}
	if summary.UpgradeEligible && summary.UpgradeTopUpAmountThresholdCents > 0 {
		summary.UpgradeTopUpAmountProgressRatio = math.Min(1, float64(summary.UpgradeTopUpAmountProgressCents)/float64(summary.UpgradeTopUpAmountThresholdCents))
	}
	return summary, nil
}

func ListAffiliateCommissions(options AffiliateCommissionQueryOptions, pageInfo *common.PageInfo) ([]*AffiliateCommission, int64, error) {
	query := DB.Model(&AffiliateCommission{}).
		Select("affiliate_commissions.*, inviter.username AS inviter_username, inviter.display_name AS inviter_display_name, invitee.username AS invitee_username, invitee.display_name AS invitee_display_name").
		Joins("LEFT JOIN users AS inviter ON inviter.id = affiliate_commissions.inviter_id").
		Joins("LEFT JOIN users AS invitee ON invitee.id = affiliate_commissions.invitee_id")
	if options.InviterId > 0 {
		query = query.Where("affiliate_commissions.inviter_id = ?", options.InviterId)
	}
	if options.Status > 0 {
		query = query.Where("affiliate_commissions.status = ?", options.Status)
	}
	if keyword := strings.TrimSpace(options.Keyword); keyword != "" {
		pattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		query = query.Where(
			"(affiliate_commissions.trade_no LIKE ? ESCAPE '!' OR inviter.username LIKE ? ESCAPE '!' OR inviter.display_name LIKE ? ESCAPE '!' OR invitee.username LIKE ? ESCAPE '!' OR invitee.display_name LIKE ? ESCAPE '!')",
			pattern, pattern, pattern, pattern, pattern,
		)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	records := []*AffiliateCommission{}
	if err := query.Order("affiliate_commissions.id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func ListAffiliateInviteeStats(inviterId int, pageInfo *common.PageInfo) ([]*AffiliateInviteeStats, int64, error) {
	var total int64
	if err := DB.Unscoped().Model(&User{}).Where("inviter_id = ?", inviterId).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []*AffiliateInviteeStats{}
	err := DB.Unscoped().Table("users").
		Select("users.id, users.username, users.display_name, users.created_at, COALESCE(SUM(CASE WHEN affiliate_commissions.status IN (?, ?) THEN 1 ELSE 0 END), 0) AS top_up_count, COALESCE(SUM(CASE WHEN affiliate_commissions.status IN (?, ?) THEN affiliate_commissions.top_up_amount_cents ELSE 0 END), 0) AS top_up_amount_cents, COALESCE(SUM(CASE WHEN affiliate_commissions.status IN (?, ?) THEN affiliate_commissions.commission_cents ELSE 0 END), 0) AS commission_cents, COALESCE(MAX(CASE WHEN affiliate_commissions.status IN (?, ?) THEN affiliate_commissions.created_time ELSE 0 END), 0) AS last_top_up_time",
			AffiliateCommissionStatusPending, AffiliateCommissionStatusApproved,
			AffiliateCommissionStatusPending, AffiliateCommissionStatusApproved,
			AffiliateCommissionStatusPending, AffiliateCommissionStatusApproved,
			AffiliateCommissionStatusPending, AffiliateCommissionStatusApproved,
		).
		Joins("LEFT JOIN affiliate_commissions ON affiliate_commissions.invitee_id = users.id AND affiliate_commissions.inviter_id = ?", inviterId).
		Where("users.inviter_id = ?", inviterId).
		Group("users.id, users.username, users.display_name, users.created_at").
		Order("users.id DESC").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	now := time.Now()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	startOfTomorrow := startOfToday.AddDate(0, 0, 1)
	for _, row := range rows {
		row.IsNew = row.CreatedAt >= startOfToday.Unix() && row.CreatedAt < startOfTomorrow.Unix()
	}
	return rows, total, nil
}

func GetAffiliateAdminSummary() (*AffiliateAdminSummary, error) {
	summary := &AffiliateAdminSummary{}
	if err := DB.Unscoped().Model(&User{}).Where("inviter_id > 0").Count(&summary.TotalInviteeCount).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&AffiliateCommission{}).Where("status = ?", AffiliateCommissionStatusPending).Count(&summary.PendingCount).Error; err != nil {
		return nil, err
	}
	type sums struct {
		PendingCents  int64
		ApprovedCents int64
		TopUpCents    int64
		RecordCount   int64
	}
	row := sums{}
	if err := DB.Model(&AffiliateCommission{}).
		Select("COALESCE(SUM(CASE WHEN status = ? THEN commission_cents ELSE 0 END), 0) AS pending_cents, COALESCE(SUM(CASE WHEN status = ? THEN commission_cents ELSE 0 END), 0) AS approved_cents, COALESCE(SUM(CASE WHEN status IN (?, ?) THEN top_up_amount_cents ELSE 0 END), 0) AS top_up_cents, COUNT(*) AS record_count", AffiliateCommissionStatusPending, AffiliateCommissionStatusApproved, AffiliateCommissionStatusPending, AffiliateCommissionStatusApproved).
		Scan(&row).Error; err != nil {
		return nil, err
	}
	summary.PendingCents = row.PendingCents
	summary.ApprovedCents = row.ApprovedCents
	summary.TopUpCents = row.TopUpCents
	summary.CommissionRecordCount = row.RecordCount
	if err := DB.Model(&AffiliateCommission{}).Where("status IN (?, ?)", AffiliateCommissionStatusPending, AffiliateCommissionStatusApproved).Distinct("invitee_id").Count(&summary.EffectiveInviteeCount).Error; err != nil {
		return nil, err
	}
	return summary, nil
}

func CountEffectiveAffiliateInvitees(inviterId int) (int64, error) {
	metrics, err := GetAffiliateUpgradeMetrics(inviterId)
	return metrics.EffectiveInviteeCount, err
}

func GetAffiliateUpgradeMetrics(inviterId int) (AffiliateUpgradeMetrics, error) {
	return getAffiliateUpgradeMetrics(DB, inviterId)
}

func getAffiliateUpgradeMetrics(db *gorm.DB, inviterId int) (AffiliateUpgradeMetrics, error) {
	metrics := AffiliateUpgradeMetrics{}
	err := db.Model(&AffiliateCommission{}).
		Select("COUNT(DISTINCT invitee_id) AS effective_invitee_count, COALESCE(SUM(top_up_amount_cents), 0) AS effective_top_up_amount_cents").
		Where("inviter_id = ? AND status IN (?, ?)", inviterId, AffiliateCommissionStatusPending, AffiliateCommissionStatusApproved).
		Scan(&metrics).Error
	return metrics, err
}

func affiliateUpgradeTargetReached(target AffiliateUpgradeTarget, metrics AffiliateUpgradeMetrics) bool {
	return metrics.EffectiveInviteeCount >= int64(target.Threshold) || metrics.EffectiveTopUpAmountCents >= target.TopUpAmountThresholdCents
}

func EnsureAffiliateUpgradeNoticesForEligibleInviters() error {
	policy := getAffiliatePolicy()
	if !policy.Enabled || policy.UpgradeInviteesThreshold <= 0 || policy.UpgradeTopUpAmountThresholdCents <= 0 {
		return nil
	}
	type eligibleRow struct {
		InviterId                 int
		EffectiveInviteeCount     int64
		EffectiveTopUpAmountCents int64
	}
	rows := []eligibleRow{}
	if err := DB.Model(&AffiliateCommission{}).
		Select("inviter_id, COUNT(DISTINCT invitee_id) AS effective_invitee_count, COALESCE(SUM(top_up_amount_cents), 0) AS effective_top_up_amount_cents").
		Where("status IN (?, ?)", AffiliateCommissionStatusPending, AffiliateCommissionStatusApproved).
		Group("inviter_id").
		Having("COUNT(DISTINCT invitee_id) >= ? OR COALESCE(SUM(top_up_amount_cents), 0) >= ?", policy.UpgradeInviteesThreshold, policy.UpgradeTopUpAmountThresholdCents).
		Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		inviter := &User{}
		if err := DB.Select("id", commonGroupCol).First(inviter, row.InviterId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return err
		}
		target, ok := affiliateUpgradeTargetForPolicy(policy, inviter.Group)
		metrics := AffiliateUpgradeMetrics{EffectiveInviteeCount: row.EffectiveInviteeCount, EffectiveTopUpAmountCents: row.EffectiveTopUpAmountCents}
		if !ok || !affiliateUpgradeTargetReached(target, metrics) {
			continue
		}
		notice := &AffiliateUpgradeNotice{
			InviterId: inviter.Id, Threshold: target.Threshold,
			EffectiveInviteeCount:     int(row.EffectiveInviteeCount),
			TopUpAmountThresholdCents: target.TopUpAmountThresholdCents,
			EffectiveTopUpAmountCents: row.EffectiveTopUpAmountCents,
		}
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(notice).Error; err != nil {
			return err
		}
	}
	return nil
}

func ListAffiliateUpgradeCandidates(pageInfo *common.PageInfo) ([]*AffiliateUpgradeCandidate, int64, error) {
	policy := getAffiliatePolicy()
	if !policy.Enabled || policy.UpgradeInviteesThreshold <= 0 || policy.GoldUpgradeInviteesThreshold <= policy.UpgradeInviteesThreshold || policy.UpgradeTopUpAmountThresholdCents <= 0 || policy.GoldUpgradeTopUpAmountThresholdCents <= policy.UpgradeTopUpAmountThresholdCents {
		return []*AffiliateUpgradeCandidate{}, 0, nil
	}
	advancedEligible := DB.Model(&AffiliateCommission{}).
		Select("inviter_id").
		Where("status IN (?, ?)", AffiliateCommissionStatusPending, AffiliateCommissionStatusApproved).
		Group("inviter_id").
		Having("COUNT(DISTINCT invitee_id) >= ? OR COALESCE(SUM(top_up_amount_cents), 0) >= ?", policy.UpgradeInviteesThreshold, policy.UpgradeTopUpAmountThresholdCents)
	goldEligible := DB.Model(&AffiliateCommission{}).
		Select("inviter_id").
		Where("status IN (?, ?)", AffiliateCommissionStatusPending, AffiliateCommissionStatusApproved).
		Group("inviter_id").
		Having("COUNT(DISTINCT invitee_id) >= ? OR COALESCE(SUM(top_up_amount_cents), 0) >= ?", policy.GoldUpgradeInviteesThreshold, policy.GoldUpgradeTopUpAmountThresholdCents)
	query := DB.Model(&User{}).
		Where("status = ?", common.UserStatusEnabled).
		Where(
			"(("+commonGroupCol+" IN ? AND id IN (?)) OR ("+commonGroupCol+" = ? AND id IN (?)))",
			[]string{AffiliatePromoterGroupDefault, AffiliatePromoterGroupLegacyJunior},
			advancedEligible,
			AffiliatePromoterGroupAdvanced,
			goldEligible,
		)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	users := []*User{}
	if err := query.Select("id", "username", "display_name", commonGroupCol).Order("id ASC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]*AffiliateUpgradeCandidate, 0, len(users))
	for _, user := range users {
		target, ok := affiliateUpgradeTargetForPolicy(policy, user.Group)
		if !ok {
			continue
		}
		metrics, err := GetAffiliateUpgradeMetrics(user.Id)
		if err != nil {
			return nil, 0, err
		}
		rows = append(rows, &AffiliateUpgradeCandidate{
			InviterId: user.Id, Username: user.Username, DisplayName: user.DisplayName,
			CurrentGroup: user.Group, EffectiveInviteeCount: metrics.EffectiveInviteeCount,
			Threshold: target.Threshold, EffectiveTopUpAmountCents: metrics.EffectiveTopUpAmountCents,
			TopUpAmountThresholdCents: target.TopUpAmountThresholdCents,
			EligibleByInvitees:        metrics.EffectiveInviteeCount >= int64(target.Threshold),
			EligibleByTopUpAmount:     metrics.EffectiveTopUpAmountCents >= target.TopUpAmountThresholdCents,
			NextGroup:                 target.Group,
			NextRateBasisPoints:       target.RateBasisPoints,
		})
	}
	return rows, total, nil
}

func ApproveAffiliateUpgrade(inviterId int, expectedNextGroup string) (string, error) {
	approvedGroup := ""
	err := DB.Transaction(func(tx *gorm.DB) error {
		policy := getAffiliatePolicy()
		if !policy.Enabled {
			return ErrAffiliateUpgradeNotEligible
		}
		user := &User{}
		if err := lockForUpdate(tx).Where("id = ?", inviterId).First(user).Error; err != nil {
			return err
		}
		target, ok := affiliateUpgradeTargetForPolicy(policy, user.Group)
		if !ok || strings.TrimSpace(expectedNextGroup) != target.Group {
			return ErrAffiliateUpgradeNotEligible
		}
		if user.Status != common.UserStatusEnabled {
			return ErrAffiliateUpgradeNotEligible
		}
		metrics, err := getAffiliateUpgradeMetrics(tx, inviterId)
		if err != nil {
			return err
		}
		if !affiliateUpgradeTargetReached(target, metrics) {
			return ErrAffiliateUpgradeNotEligible
		}
		if err := tx.Model(user).Updates(map[string]interface{}{"group": target.Group}).Error; err != nil {
			return err
		}
		approvedGroup = target.Group
		return nil
	})
	if err != nil {
		return "", err
	}
	if err := EnsureAffiliateUpgradeNoticesForEligibleInviters(); err != nil {
		common.SysError(fmt.Sprintf("failed to prepare next affiliate upgrade notice for inviter %d: %s", inviterId, err.Error()))
	}
	return approvedGroup, nil
}

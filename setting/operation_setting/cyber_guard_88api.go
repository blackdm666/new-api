package operation_setting

import (
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// 88api cyberguard: CyberGuard moves users whose sub2api-attributed upstream
// content-policy refusals reach a configurable threshold into the ban group.

const (
	CyberGuardSettingName = "cyber_guard_setting"

	CyberGuardModeOff     = "off"
	CyberGuardModeObserve = "observe"
	CyberGuardModeEnforce = "enforce"

	CyberGuardCategoryCyber          = "cyber"
	CyberGuardCategoryBio            = "bio"
	CyberGuardCategoryUsage          = "usage"
	CyberGuardCategorySessionBlocked = "session_blocked"

	CyberGuardMinPollIntervalSeconds = 15
	CyberGuardMaxPollIntervalSeconds = 3600
	CyberGuardMaxLookbackMinutes     = 360
	CyberGuardMaxMergeSeconds        = 3600
	CyberGuardMaxTiers               = 5
	CyberGuardMaxTierWindowDays      = 90
	CyberGuardMaxTierMinUnits        = 50
	CyberGuardMaxTierBanHours        = 8760
	CyberGuardMaxIdListLength        = 200
)

type CyberGuardTier struct {
	WindowDays int `json:"window_days"`
	MinUnits   int `json:"min_units"`
	BanHours   int `json:"ban_hours"` // 0 = until manual unban
}

type CyberGuardSetting struct {
	Mode                  string           `json:"mode"`
	ProtectedChannelIds   []int            `json:"protected_channel_ids"`
	Sub2apiBaseUrl        string           `json:"sub2api_base_url"`
	Sub2apiAdminApiKey    string           `json:"sub2api_admin_api_key"` // secret; key ends with api_key so GET /api/option hides it
	Sub2apiApiKeyIds      []int64          `json:"sub2api_api_key_ids"`
	PollIntervalSeconds   int              `json:"poll_interval_seconds"`
	LookbackMinutes       int              `json:"lookback_minutes"`
	CountCategories       []string         `json:"count_categories"`
	NosessionMergeSeconds int              `json:"nosession_merge_seconds"`
	Tiers                 []CyberGuardTier `json:"tiers"`
	BanGroup              string           `json:"ban_group"`
	ExemptUserIds         []int            `json:"exempt_user_ids"`
	EmailEnabled          bool             `json:"email_enabled"`
	NotifyRoot            bool             `json:"notify_root"`
}

var cyberGuardSetting = DefaultCyberGuardSetting()

func init() {
	config.GlobalConfig.Register(CyberGuardSettingName, &cyberGuardSetting)
}

func DefaultCyberGuardSetting() CyberGuardSetting {
	return CyberGuardSetting{
		Mode:                  CyberGuardModeOff,
		ProtectedChannelIds:   []int{1, 9, 37},
		Sub2apiBaseUrl:        "http://sub2api:8080",
		Sub2apiApiKeyIds:      []int64{1, 6},
		PollIntervalSeconds:   30,
		LookbackMinutes:       15,
		CountCategories:       []string{CyberGuardCategoryCyber, CyberGuardCategoryBio},
		NosessionMergeSeconds: 60,
		Tiers: []CyberGuardTier{
			{WindowDays: 7, MinUnits: 2, BanHours: 72},
			{WindowDays: 30, MinUnits: 3, BanHours: 0},
		},
		BanGroup:      "您因违反使用规定已被封禁",
		ExemptUserIds: []int{},
		EmailEnabled:  true,
		NotifyRoot:    true,
	}
}

// GetCyberGuardSetting returns a deep copy taken under the option-map lock,
// which is the lock handleConfigUpdate holds while it rewrites the struct.
// Out-of-range values written through the generic option API are clamped here
// so the poller never runs with an unusable configuration.
func GetCyberGuardSetting() CyberGuardSetting {
	common.OptionMapRWMutex.RLock()
	s := cyberGuardSetting
	s.ProtectedChannelIds = slices.Clone(cyberGuardSetting.ProtectedChannelIds)
	s.Sub2apiApiKeyIds = slices.Clone(cyberGuardSetting.Sub2apiApiKeyIds)
	s.CountCategories = slices.Clone(cyberGuardSetting.CountCategories)
	s.Tiers = slices.Clone(cyberGuardSetting.Tiers)
	s.ExemptUserIds = slices.Clone(cyberGuardSetting.ExemptUserIds)
	common.OptionMapRWMutex.RUnlock()

	s.Mode = strings.TrimSpace(s.Mode)
	if s.Mode != CyberGuardModeObserve && s.Mode != CyberGuardModeEnforce {
		s.Mode = CyberGuardModeOff
	}
	s.Sub2apiBaseUrl = strings.TrimRight(strings.TrimSpace(s.Sub2apiBaseUrl), "/")
	s.Sub2apiAdminApiKey = strings.TrimSpace(s.Sub2apiAdminApiKey)
	s.BanGroup = strings.TrimSpace(s.BanGroup)
	s.ProtectedChannelIds = nonNilCyberGuardSlice(s.ProtectedChannelIds)
	s.Sub2apiApiKeyIds = nonNilCyberGuardSlice(s.Sub2apiApiKeyIds)
	s.CountCategories = nonNilCyberGuardSlice(s.CountCategories)
	s.Tiers = nonNilCyberGuardSlice(s.Tiers)
	s.ExemptUserIds = nonNilCyberGuardSlice(s.ExemptUserIds)
	s.PollIntervalSeconds = min(max(s.PollIntervalSeconds, CyberGuardMinPollIntervalSeconds), CyberGuardMaxPollIntervalSeconds)
	s.LookbackMinutes = min(max(s.LookbackMinutes, 1), CyberGuardMaxLookbackMinutes)
	s.NosessionMergeSeconds = min(max(s.NosessionMergeSeconds, 0), CyberGuardMaxMergeSeconds)
	return s
}

func nonNilCyberGuardSlice[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}

// SetCyberGuardSettingForTest replaces the live setting; tests only.
func SetCyberGuardSettingForTest(s CyberGuardSetting) {
	common.OptionMapRWMutex.Lock()
	cyberGuardSetting = s
	common.OptionMapRWMutex.Unlock()
}

func (s CyberGuardSetting) IsExempt(userId int) bool {
	return slices.Contains(s.ExemptUserIds, userId)
}

func (s CyberGuardSetting) CountsCategory(category string) bool {
	if category != CyberGuardCategoryCyber && category != CyberGuardCategoryBio {
		return false // usage and session_blocked are always record-only
	}
	return slices.Contains(s.CountCategories, category)
}

// ValidateCyberGuardSetting checks an operator-submitted configuration. The
// secret is validated by the caller because an empty value means "keep".
func ValidateCyberGuardSetting(s CyberGuardSetting) error {
	switch s.Mode {
	case CyberGuardModeOff, CyberGuardModeObserve, CyberGuardModeEnforce:
	default:
		return fmt.Errorf("mode must be off, observe or enforce")
	}
	if err := validateCyberGuardIds("protected_channel_ids", s.ProtectedChannelIds, true); err != nil {
		return err
	}
	if len(s.Sub2apiApiKeyIds) == 0 || len(s.Sub2apiApiKeyIds) > 20 {
		return fmt.Errorf("sub2api_api_key_ids must contain 1 to 20 ids")
	}
	for _, id := range s.Sub2apiApiKeyIds {
		if id <= 0 {
			return fmt.Errorf("sub2api_api_key_ids must be positive")
		}
	}
	base := strings.TrimSpace(s.Sub2apiBaseUrl)
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return fmt.Errorf("sub2api_base_url must start with http:// or https://")
	}
	if len(base) > 255 || strings.ContainsAny(base, " \r\n\t?#") {
		return fmt.Errorf("sub2api_base_url is invalid")
	}
	if s.PollIntervalSeconds < CyberGuardMinPollIntervalSeconds || s.PollIntervalSeconds > CyberGuardMaxPollIntervalSeconds {
		return fmt.Errorf("poll_interval_seconds must be between %d and %d", CyberGuardMinPollIntervalSeconds, CyberGuardMaxPollIntervalSeconds)
	}
	if s.LookbackMinutes < 1 || s.LookbackMinutes > CyberGuardMaxLookbackMinutes {
		return fmt.Errorf("lookback_minutes must be between 1 and %d", CyberGuardMaxLookbackMinutes)
	}
	if s.NosessionMergeSeconds < 0 || s.NosessionMergeSeconds > CyberGuardMaxMergeSeconds {
		return fmt.Errorf("nosession_merge_seconds must be between 0 and %d", CyberGuardMaxMergeSeconds)
	}
	for _, category := range s.CountCategories {
		if category != CyberGuardCategoryCyber && category != CyberGuardCategoryBio {
			return fmt.Errorf("count_categories may only contain cyber and bio")
		}
	}
	if len(s.Tiers) < 1 || len(s.Tiers) > CyberGuardMaxTiers {
		return fmt.Errorf("tiers must contain 1 to %d entries", CyberGuardMaxTiers)
	}
	for i, tier := range s.Tiers {
		if tier.WindowDays < 1 || tier.WindowDays > CyberGuardMaxTierWindowDays {
			return fmt.Errorf("tiers[%d].window_days must be between 1 and %d", i, CyberGuardMaxTierWindowDays)
		}
		if tier.MinUnits < 1 || tier.MinUnits > CyberGuardMaxTierMinUnits {
			return fmt.Errorf("tiers[%d].min_units must be between 1 and %d", i, CyberGuardMaxTierMinUnits)
		}
		if tier.BanHours < 0 || tier.BanHours > CyberGuardMaxTierBanHours {
			return fmt.Errorf("tiers[%d].ban_hours must be between 0 and %d", i, CyberGuardMaxTierBanHours)
		}
	}
	if strings.TrimSpace(s.BanGroup) == "" || len(s.BanGroup) > 64 {
		return fmt.Errorf("ban_group is required (at most 64 bytes)")
	}
	return validateCyberGuardIds("exempt_user_ids", s.ExemptUserIds, false)
}

func validateCyberGuardIds(name string, ids []int, required bool) error {
	if required && len(ids) == 0 {
		return fmt.Errorf("%s must not be empty", name)
	}
	if len(ids) > CyberGuardMaxIdListLength {
		return fmt.Errorf("%s may contain at most %d ids", name, CyberGuardMaxIdListLength)
	}
	for _, id := range ids {
		if id <= 0 {
			return fmt.Errorf("%s must be positive", name)
		}
	}
	return nil
}

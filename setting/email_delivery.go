package setting

import (
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

const EmailDeliveryRulesOptionKey = "EmailDeliveryRules"

type EmailDeliveryRules struct {
	MarketingDailyLimit       int `json:"marketing_daily_limit"`
	MarketingPerMinuteLimit   int `json:"marketing_per_minute_limit"`
	MarketingUserCooldownDays int `json:"marketing_user_cooldown_days"`
	MarketingSendStartHour    int `json:"marketing_send_start_hour"`
	MarketingSendEndHour      int `json:"marketing_send_end_hour"`
	EmailMaxAttempts          int `json:"email_max_attempts"`
	EmailRetryInitialSeconds  int `json:"email_retry_initial_seconds"`
	EmailRetryMaxSeconds      int `json:"email_retry_max_seconds"`
	DeliveredRetentionDays    int `json:"delivered_retention_days"`
	TerminalRetentionDays     int `json:"terminal_retention_days"`
}

var (
	emailDeliveryRulesMutex sync.RWMutex
	emailDeliveryRules      = DefaultEmailDeliveryRules()
)

func DefaultEmailDeliveryRules() EmailDeliveryRules {
	return EmailDeliveryRules{
		MarketingDailyLimit:       500,
		MarketingPerMinuteLimit:   20,
		MarketingUserCooldownDays: 7,
		MarketingSendStartHour:    9,
		MarketingSendEndHour:      20,
		EmailMaxAttempts:          8,
		EmailRetryInitialSeconds:  30,
		EmailRetryMaxSeconds:      86400,
		DeliveredRetentionDays:    30,
		TerminalRetentionDays:     90,
	}
}

func GetEmailDeliveryRules() EmailDeliveryRules {
	emailDeliveryRulesMutex.RLock()
	defer emailDeliveryRulesMutex.RUnlock()
	return emailDeliveryRules
}

func EmailDeliveryRules2JSONString() string {
	raw, err := common.Marshal(GetEmailDeliveryRules())
	if err != nil {
		common.SysError("failed to marshal email delivery rules: " + err.Error())
		return "{}"
	}
	return string(raw)
}

func ValidateEmailDeliveryRulesJSONString(raw string) error {
	_, err := parseEmailDeliveryRules(raw)
	return err
}

func UpdateEmailDeliveryRulesByJSONString(raw string) error {
	rules, err := parseEmailDeliveryRules(raw)
	if err != nil {
		return err
	}
	emailDeliveryRulesMutex.Lock()
	emailDeliveryRules = rules
	emailDeliveryRulesMutex.Unlock()
	return nil
}

func parseEmailDeliveryRules(raw string) (EmailDeliveryRules, error) {
	rules := EmailDeliveryRules{}
	if err := common.UnmarshalJsonStr(raw, &rules); err != nil {
		return rules, fmt.Errorf("invalid email delivery rules: %w", err)
	}
	if rules.MarketingDailyLimit < 1 || rules.MarketingDailyLimit > 100000 {
		return rules, fmt.Errorf("marketing daily limit must be between 1 and 100000")
	}
	if rules.MarketingPerMinuteLimit < 1 || rules.MarketingPerMinuteLimit > 1000 || rules.MarketingPerMinuteLimit > rules.MarketingDailyLimit {
		return rules, fmt.Errorf("marketing per-minute limit must be between 1 and 1000 and not exceed the daily limit")
	}
	if rules.MarketingUserCooldownDays < 0 || rules.MarketingUserCooldownDays > 365 {
		return rules, fmt.Errorf("marketing user cooldown must be between 0 and 365 days")
	}
	if rules.MarketingSendStartHour < 0 || rules.MarketingSendStartHour > 23 || rules.MarketingSendEndHour < 1 || rules.MarketingSendEndHour > 24 || rules.MarketingSendStartHour >= rules.MarketingSendEndHour {
		return rules, fmt.Errorf("marketing send window must be a valid increasing hour range")
	}
	if rules.EmailMaxAttempts < 1 || rules.EmailMaxAttempts > 20 {
		return rules, fmt.Errorf("email max attempts must be between 1 and 20")
	}
	if rules.EmailRetryInitialSeconds < 10 || rules.EmailRetryInitialSeconds > 3600 {
		return rules, fmt.Errorf("email retry initial delay must be between 10 and 3600 seconds")
	}
	if rules.EmailRetryMaxSeconds < rules.EmailRetryInitialSeconds || rules.EmailRetryMaxSeconds > 86400 {
		return rules, fmt.Errorf("email retry maximum delay must be at least the initial delay and no more than 86400 seconds")
	}
	if rules.DeliveredRetentionDays < 1 || rules.DeliveredRetentionDays > 3650 {
		return rules, fmt.Errorf("delivered email retention must be between 1 and 3650 days")
	}
	if rules.TerminalRetentionDays < 1 || rules.TerminalRetentionDays > 3650 {
		return rules, fmt.Errorf("failed or expired email retention must be between 1 and 3650 days")
	}
	return rules, nil
}

package setting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailDeliveryRulesRoundTripAndValidation(t *testing.T) {
	original := GetEmailDeliveryRules()
	t.Cleanup(func() {
		raw, err := common.Marshal(original)
		require.NoError(t, err)
		require.NoError(t, UpdateEmailDeliveryRulesByJSONString(string(raw)))
	})
	rules := DefaultEmailDeliveryRules()
	rules.MarketingDailyLimit = 750
	rules.MarketingPerMinuteLimit = 25
	rules.MarketingSendStartHour = 8
	rules.MarketingSendEndHour = 21
	rules.EmailMaxAttempts = 6

	raw, err := common.Marshal(rules)
	require.NoError(t, err)
	require.NoError(t, UpdateEmailDeliveryRulesByJSONString(string(raw)))
	assert.Equal(t, rules, GetEmailDeliveryRules())

	invalid := rules
	invalid.MarketingPerMinuteLimit = invalid.MarketingDailyLimit + 1
	raw, err = common.Marshal(invalid)
	require.NoError(t, err)
	assert.Error(t, ValidateEmailDeliveryRulesJSONString(string(raw)))

	invalid = rules
	invalid.MarketingSendStartHour = invalid.MarketingSendEndHour
	raw, err = common.Marshal(invalid)
	require.NoError(t, err)
	assert.Error(t, ValidateEmailDeliveryRulesJSONString(string(raw)))
}

func TestLegacyEmailDeliveryRulesReceiveNewDefaults(t *testing.T) {
	rules, err := parseEmailDeliveryRules(`{
		"marketing_daily_limit":500,
		"marketing_per_minute_limit":20,
		"marketing_user_cooldown_days":7,
		"marketing_send_start_hour":9,
		"marketing_send_end_hour":20,
		"email_max_attempts":8,
		"email_retry_initial_seconds":30,
		"email_retry_max_seconds":86400,
		"delivered_retention_days":30,
		"terminal_retention_days":90
	}`)
	require.NoError(t, err)
	assert.Equal(t, 24, rules.ReceiptTimeoutHours)
}

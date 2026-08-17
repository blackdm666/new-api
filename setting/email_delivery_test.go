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

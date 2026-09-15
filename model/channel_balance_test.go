package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelBalanceInfoValueAndScanPreserveStructuredUnits(t *testing.T) {
	t.Parallel()

	original := ChannelBalanceInfo{
		Remaining:   "42.1357",
		Total:       "100",
		Used:        "57.8643",
		Unit:        ChannelBalanceUnitMoney,
		Currency:    "USD",
		DisplayUnit: "$",
		MetricKind:  "wallet",
		Source:      "custom",
		UpdatedAt:   123456,
	}
	value, err := original.Value()
	require.NoError(t, err)

	var decoded ChannelBalanceInfo
	require.NoError(t, decoded.Scan(value))
	assert.Equal(t, original, decoded)
}

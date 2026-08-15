package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripeMinorUnitsToMoney(t *testing.T) {
	assert.InDelta(t, 12.34, stripeMinorUnitsToMoney(1234, "usd"), 0.000001)
	assert.InDelta(t, 1234, stripeMinorUnitsToMoney(1234, "JPY"), 0.000001)
	assert.InDelta(t, 12.34, stripeMinorUnitsToMoney(1234, "ISK"), 0.000001)
}

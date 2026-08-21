package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAntomDisplayNameUsesConfiguredValueAndDefaultFallback(t *testing.T) {
	original := AntomDisplayName
	t.Cleanup(func() { AntomDisplayName = original })

	AntomDisplayName = "  International Wallets  "
	assert.Equal(t, "International Wallets", GetAntomDisplayName())

	AntomDisplayName = "   "
	assert.Equal(t, DefaultAntomDisplayName, GetAntomDisplayName())
}

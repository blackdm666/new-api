package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func usePasswordResetBudget(t *testing.T, limit int, window time.Duration) {
	t.Helper()
	oldLimit, oldWindow, oldRedis := PasswordResetEmailLimit, PasswordResetEmailWindow, common.RedisEnabled
	PasswordResetEmailLimit, PasswordResetEmailWindow, common.RedisEnabled = limit, window, false
	passwordResetBudgetMemory.Lock()
	passwordResetBudgetMemory.entries = map[string]*passwordResetBudgetEntry{}
	passwordResetBudgetMemory.Unlock()
	t.Cleanup(func() {
		PasswordResetEmailLimit, PasswordResetEmailWindow, common.RedisEnabled = oldLimit, oldWindow, oldRedis
	})
}

func TestPasswordResetEmailBudgetPerAddress(t *testing.T) {
	usePasswordResetBudget(t, 3, time.Hour)

	for i := range 3 {
		require.True(t, ConsumePasswordResetEmailBudget("victim@example.com"), "request %d", i+1)
	}
	// Rotating proxies does not matter: the budget follows the address,
	// and case or surrounding spaces do not open a fresh budget.
	assert.False(t, ConsumePasswordResetEmailBudget("victim@example.com"))
	assert.False(t, ConsumePasswordResetEmailBudget("  Victim@Example.com "))
	assert.True(t, ConsumePasswordResetEmailBudget("other@example.com"))
}

func TestPasswordResetEmailBudgetResetsAfterWindow(t *testing.T) {
	usePasswordResetBudget(t, 1, time.Hour)

	require.True(t, ConsumePasswordResetEmailBudget("victim@example.com"))
	require.False(t, ConsumePasswordResetEmailBudget("victim@example.com"))

	passwordResetBudgetMemory.Lock()
	passwordResetBudgetMemory.entries[passwordResetBudgetKey("victim@example.com")].windowEnd = time.Now().Add(-time.Second)
	passwordResetBudgetMemory.Unlock()

	assert.True(t, ConsumePasswordResetEmailBudget("victim@example.com"))
}

func TestPasswordResetEmailBudgetDisabled(t *testing.T) {
	usePasswordResetBudget(t, 0, time.Hour)

	for range 5 {
		assert.True(t, ConsumePasswordResetEmailBudget("victim@example.com"))
	}
}

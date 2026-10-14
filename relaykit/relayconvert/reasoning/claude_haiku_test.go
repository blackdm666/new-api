package reasoning

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Claude Haiku 5.5 rejects manual thinking with budget_tokens ("omit thinking
// or use type adaptive"), so converted reasoning efforts must render adaptive.
func TestRenderClaudeHaiku55UsesAdaptiveThinking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		intent     Intent
		wantEffort Effort
	}{
		{name: "explicit effort", intent: Intent{Mode: ModeEnabled, Effort: EffortHigh}, wantEffort: EffortHigh},
		{name: "xhigh effort", intent: Intent{Mode: ModeEnabled, Effort: EffortXHigh}, wantEffort: EffortXHigh},
		{name: "enabled without effort", intent: Intent{Mode: ModeEnabled}, wantEffort: EffortMedium},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			render, err := RenderClaude("claude-haiku-5-5", tt.intent, nil, 0)
			require.NoError(t, err)
			require.NotNil(t, render.Thinking)
			assert.Equal(t, "adaptive", render.Thinking.Type)
			assert.Nil(t, render.Thinking.BudgetTokens)
			assert.Nil(t, render.MaxTokens)
			assert.Equal(t, tt.wantEffort, render.OutputEffort)
			assert.True(t, render.ClearSampling)
		})
	}
}

func TestRenderClaudeHaiku55DefaultsToMediumEffort(t *testing.T) {
	t.Parallel()

	// A channel mapping to a differently named upstream model falls back to
	// the origin model's capabilities only when the origin is a known model.
	assert.True(t, IsKnownClaudeModel("claude-haiku-5-5"))

	render, err := RenderClaude("claude-haiku-5-5-20261001", Intent{}, nil, 0)
	require.NoError(t, err)
	assert.Nil(t, render.Thinking)
	assert.Equal(t, EffortMedium, render.EffectiveEffort)
}

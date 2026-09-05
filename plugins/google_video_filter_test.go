package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoogleVideoPluginsRejectFilteredTerminalResults(t *testing.T) {
	testCases := []struct {
		key  string
		body map[string]any
	}{
		{
			key: "google",
			body: map[string]any{"done": true, "response": map[string]any{
				"generateVideoResponse": map[string]any{
					"raiMediaFilteredCount":   1,
					"raiMediaFilteredReasons": []any{"Gemini filtered the video; support code: 123"},
				},
			}},
		},
		{
			key: "vertex-ai",
			body: map[string]any{"done": true, "response": map[string]any{
				"raiMediaFilteredCount":   1,
				"raiMediaFilteredReasons": []any{"Vertex filtered the video; support code: 15236754"},
			}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.key, func(t *testing.T) {
			source, err := builtinplugins.Source(testCase.key)
			require.NoError(t, err)
			registry := jsplugin.NewRegistry()
			plugin, err := registry.RegisterFactory(source, jsplugin.Options{Key: testCase.key})
			require.NoError(t, err)

			value, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{}, testCase.body)
			require.NoError(t, err)
			encoded, err := common.Marshal(value)
			require.NoError(t, err)
			var result map[string]any
			require.NoError(t, common.Unmarshal(encoded, &result))
			assert.Equal(t, "FAILURE", result["status"])
			assert.Equal(t, "100%", result["progress"])
			assert.Contains(t, result["reason"], "support code")
		})
	}
}

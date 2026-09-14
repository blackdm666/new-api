package jsplugin

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXinMengWan3TaskPlugin(t *testing.T) {
	source, err := os.ReadFile("../../plugins/tasks/xinmeng-wan3/plugin.js")
	require.NoError(t, err)
	registry := NewRegistry()
	plugin, err := registry.Register(string(source), Options{})
	require.NoError(t, err)
	ctx := context.Background()
	call := func(t *testing.T, hook string, args ...any) map[string]any {
		t.Helper()
		value, err := plugin.Engine.Call(ctx, hook, args...)
		require.NoError(t, err)
		result, ok := value.(map[string]any)
		require.True(t, ok)
		return result
	}
	for _, resolution := range []string{"480p", "720p", "1080p"} {
		t.Run(resolution, func(t *testing.T) {
			model := "wan3.0-video-" + resolution
			input := map[string]any{"prompt": "A red toy car", "seconds": "4", "seed": 0, "metadata": map[string]any{"ratio": "9:16"}}
			decoded, err := plugin.Engine.CallPath(ctx, "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": model, "body": map[string]any{"kind": "json", "value": input}})
			require.NoError(t, err)
			request := decoded.(map[string]any)["requestBody"]
			driver := map[string]any{"model": model, "upstreamModel": model, "requestBody": request, "baseUrl": "https://example.com/v1/", "apiKey": "fixture"}
			submit := call(t, "buildSubmitRequest", driver)
			assert.Equal(t, "https://example.com/v1/videos/generations", submit["url"])
			body := submit["body"].(map[string]any)
			assert.Equal(t, resolution, body["resolution"])
			assert.Equal(t, "9:16", body["ratio"])
			assert.EqualValues(t, 0, body["seed"])
			facts := call(t, "extractUsage", driver)
			assert.EqualValues(t, 4, facts["seconds"])
			assert.Equal(t, body["duration"], facts["seconds"])
			assert.NotContains(t, input, "duration", "host inputs must not be mutated")
		})
	}
	t.Run("invalid quantities fail before upstream or billing", func(t *testing.T) {
		for _, bad := range []any{-1, 0, 3, 31, 4.5, "Infinity", "1e100", true, ""} {
			input := map[string]any{"prompt": "test", "duration": bad}
			_, err := plugin.Engine.Call(ctx, "decodeRequest", map[string]any{"model": "wan3.0-video-720p", "body": map[string]any{"kind": "json", "value": input}})
			assert.Error(t, err, "duration=%v", bad)
			_, err = plugin.Engine.Call(ctx, "extractUsage", map[string]any{"requestBody": input})
			assert.Error(t, err, "duration=%v", bad)
		}
		_, err := plugin.Engine.Call(ctx, "extractUsage", map[string]any{"requestBody": map[string]any{"duration": 4, "seconds": 30}})
		assert.ErrorContains(t, err, "must agree")
	})
	t.Run("media aliases and fixed resolution", func(t *testing.T) {
		result := call(t, "buildSubmitRequest", map[string]any{"model": "wan3.0-video-1080p", "baseUrl": "https://example.com", "requestBody": map[string]any{"duration": 5, "metadata": map[string]any{"resolution": "480p", "reference_images": []string{"https://example.com/a.png"}, "reference_videos": []string{"https://example.com/v.mp4"}, "reference_audios": []string{"https://example.com/a.mp3"}}}})
		body := result["body"].(map[string]any)
		assert.Equal(t, "1080p", body["resolution"])
		assert.Len(t, body["referenceImages"], 1)
		assert.Len(t, body["referenceVideos"], 1)
		assert.Len(t, body["referenceAudios"], 1)
	})
	t.Run("completion failure and artifact contract", func(t *testing.T) {
		pending := call(t, "parseTaskResult", map[string]any{}, map[string]any{"status": "processing", "progress": 100})
		assert.Equal(t, "99%", pending["progress"])
		body := map[string]any{"status": "completed", "result": "https://example.com/result.mp4"}
		success := call(t, "parseTaskResult", map[string]any{}, body)
		assert.Equal(t, "SUCCESS", success["status"])
		assert.Equal(t, "https://example.com/result.mp4", success["url"])
		failure := call(t, "parseTaskResult", map[string]any{}, map[string]any{"status": "failed", "error": map[string]any{"message": "provider rejected"}})
		assert.Equal(t, "FAILURE", failure["status"])
		assert.Equal(t, "provider rejected", failure["reason"])
		content := call(t, "buildContentRequest", map[string]any{"artifactKey": "video", "data": map[string]any{"data": body}, "clientRequest": map[string]any{"method": "HEAD"}})
		assert.Equal(t, true, content["credentialless"])
		assert.NotContains(t, content, "headers")
	})
	_, claimed := registry.Generation().GetByModel("doubao-seedance-2.5")
	assert.False(t, claimed, "only the three Wan3 aliases may be claimed")
}

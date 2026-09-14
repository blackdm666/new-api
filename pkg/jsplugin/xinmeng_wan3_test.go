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
		for _, bad := range []any{-1, 3, 31, 4.5, "Infinity", "1e100", true} {
			input := map[string]any{"prompt": "test", "duration": bad}
			_, err := plugin.Engine.Call(ctx, "decodeRequest", map[string]any{"model": "wan3.0-video-720p", "body": map[string]any{"kind": "json", "value": input}})
			assert.Error(t, err, "duration=%v", bad)
			_, err = plugin.Engine.Call(ctx, "extractUsage", map[string]any{"model": "wan3.0-video-720p", "requestBody": input})
			assert.Error(t, err, "duration=%v", bad)
		}
		facts := call(t, "extractUsage", map[string]any{"model": "wan3.0-video-720p", "requestBody": map[string]any{"duration": 4, "seconds": 30}})
		assert.EqualValues(t, 4, facts["seconds"], "positive duration keeps legacy precedence")
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
	assert.False(t, claimed, "unrelated provider aliases must not be claimed")
	assert.Len(t, plugin.Meta.Models, 19)
	assert.Empty(t, plugin.Meta.UsageExamples, "do not restore the removed pricing examples")
	t.Run("all sales models preserve billing identity and fixed quality", func(t *testing.T) {
		for _, tc := range []struct {
			model, upstream, quality string
			seconds                  int
		}{
			{"SD2.0 480P", "cvd-seedance-2.0", "480p", 5},
			{"SD2.0 720P", "cvd-seedance-2.0", "720p", 5},
			{"SD2.0 1080P", "cvd-seedance-2.0", "1080p", 5},
			{"SD2.5 480P", "dvc-seedance-2.5-480p", "480p", 5},
			{"SD2.5 720P", "dvc-seedance-2.5", "720p", 5},
			{"SD2.5 1080P", "dvc-seedance-2.5-1080p", "1080p", 5},
			{"seedance-2.0-mini-480p", "seedance-2.0-mini-480p", "480p", 5},
			{"seedance-2.0-mini-720p", "seedance-2.0-mini-720p", "720p", 5},
			{"kling-3.0-turbo-720p", "kling-3.0-turbo", "720p", 5},
			{"kling-3.0-turbo-1080p", "kling-3.0-turbo", "1080p", 5},
			{"kling-3.0-turbo-2k", "kling-3.0-turbo", "2k", 5},
			{"kling-3.0-turbo-4k", "kling-3.0-turbo", "4k", 5},
			{"Seedance-2.5-720p官方版", "doubao-seedance-2-5-720p", "720p", 4},
			{"Seedance-2.0-720p官方版", "doubao-seedance-2-0-720p", "720p", 4},
			{"Seedance-2.0-fast-720p官方版", "doubao-seedance-2-0-fast-720p", "720p", 4},
			{"minimax-h3-768p", "minimax-h3-768p", "768p", 4},
		} {
			t.Run(tc.model, func(t *testing.T) {
				request := call(t, "decodeRequest", map[string]any{"model": tc.model, "body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "A toy car", "metadata": `{"resolution":"4k"}`}}})
				assert.Equal(t, tc.model, request["model"])
				driver := map[string]any{"model": tc.model, "upstreamModel": tc.upstream, "requestBody": request["requestBody"], "baseUrl": "https://example.com"}
				body := call(t, "buildSubmitRequest", driver)["body"].(map[string]any)
				assert.Equal(t, tc.quality, body["resolution"])
				assert.Equal(t, tc.upstream, body["model"])
				assert.EqualValues(t, tc.seconds, body["duration"])
				assert.EqualValues(t, tc.seconds, call(t, "extractUsage", driver)["seconds"])
			})
		}
	})
	t.Run("legacy metadata and official polling contract", func(t *testing.T) {
		decoded := call(t, "decodeRequest", map[string]any{"model": "wan3.0-video-720p", "body": map[string]any{"kind": "json", "value": map[string]any{"metadata": `{"first_image":["https://example.com/first.png"],"last_image":"https://example.com/last.png"}`, "duration": "4", "seconds": "30"}}})
		request := decoded["requestBody"].(map[string]any)
		assert.NotContains(t, request, "callback_url")
		assert.NotContains(t, request, "seconds")
		body := call(t, "buildSubmitRequest", map[string]any{"model": "wan3.0-video-720p", "requestBody": request})["body"].(map[string]any)
		assert.NotContains(t, body, "callback_url", "callback delivery belongs to the host")
		assert.Equal(t, "https://example.com/last.png", body["lastFrame"])
		assert.EqualValues(t, 4, body["duration"])
	})
	t.Run("multipart aliases preserve zero false and native frame roles", func(t *testing.T) {
		decoded := call(t, "decodeRequest", map[string]any{"model": "kling-3.0-turbo-4k", "body": map[string]any{"kind": "multipart", "fields": map[string]any{"prompt": []string{"Camera pan"}, "metadata": []string{`{"firstFrame":"https://example.com/first.png"}`}, "video_urls": []string{`["https://example.com/v.mp4"]`}, "generate_audio": []string{"false"}, "seed": []string{"0"}}}})
		body := call(t, "buildSubmitRequest", map[string]any{"model": "kling-3.0-turbo-4k", "requestBody": decoded["requestBody"]})["body"].(map[string]any)
		assert.Equal(t, false, body["generate_audio"])
		assert.EqualValues(t, 0, body["seed"])
		assert.Equal(t, "kling-3.0-turbo", body["model"])
		assert.Len(t, body["videos"], 1)
		assert.Equal(t, "first_frame", body["images"].([]any)[0].(map[string]any)["role"])
		assert.NotContains(t, body, "referenceImages")
	})
	t.Run("model-specific validation", func(t *testing.T) {
		for _, model := range []string{"SD2.5 480P", "SD2.5 720P", "SD2.5 1080P"} {
			decoded := call(t, "decodeRequest", map[string]any{"model": model, "body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "test", "metadata": map[string]any{"generate_audio": false}}}})
			body := call(t, "buildSubmitRequest", map[string]any{"model": model, "requestBody": decoded["requestBody"]})["body"].(map[string]any)
			assert.Equal(t, false, body["generateAudio"], model)
		}
		refs := func(n int) []string {
			values := make([]string, n)
			for i := range values {
				values[i] = "https://example.com/reference"
			}
			return values
		}
		for _, model := range []string{"SD2.0 480P", "SD2.0 720P", "SD2.0 1080P"} {
			input := map[string]any{"prompt": "test", "images": refs(9), "videos": refs(3)}
			call(t, "decodeRequest", map[string]any{"model": model, "body": map[string]any{"kind": "json", "value": input}})
			input["audios"] = refs(1)
			_, err := plugin.Engine.Call(ctx, "decodeRequest", map[string]any{"model": model, "body": map[string]any{"kind": "json", "value": input}})
			assert.ErrorContains(t, err, "too many media references in total")
		}
		for _, tc := range []struct {
			model string
			input map[string]any
		}{
			{"minimax-h3-768p", map[string]any{"prompt": "test", "referenceAudios": []string{"https://example.com/a.mp3"}}},
			{"kling-3.0-turbo-720p", map[string]any{"prompt": "test", "duration": 30}},
			{"seedance-2.0-mini-720p", map[string]any{"prompt": "test", "generate_audio": false}},
			{"wan3.0-video-720p", map[string]any{"firstFrame": "https://example.com/a.png", "images": []string{"https://example.com/b.png"}}},
			{"wan3.0-video-720p", map[string]any{"prompt": "test", "metadata": "[]"}},
			{"__proto__", map[string]any{"prompt": "test"}},
			{"wan3.0-video-720p", map[string]any{"prompt": "test", "callback_url": "https://example.com/callback"}},
		} {
			_, err := plugin.Engine.Call(ctx, "decodeRequest", map[string]any{"model": tc.model, "body": map[string]any{"kind": "json", "value": tc.input}})
			assert.Error(t, err, "%s: %v", tc.model, tc.input)
		}
		missing := call(t, "parseTaskResult", map[string]any{}, map[string]any{"status": "completed"})
		assert.Equal(t, "FAILURE", missing["status"], "completed without video must terminate instead of polling forever")
	})
}

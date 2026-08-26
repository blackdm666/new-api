package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsClaudeTitleGenerationRequestMatchesClaudeCodeShape(t *testing.T) {
	req := &dto.ClaudeRequest{
		Model:  "claude-sonnet-4-5",
		Stream: common.GetPointer(true),
		System: "Generate a concise, sentence-case title for this session. Use 3-7 words.",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "<session>\nUser: hello"},
		},
	}

	assert.True(t, isClaudeTitleGenerationRequest(req))
}

func TestIsClaudeTitleGenerationRequestMatchesSystemMessageShape(t *testing.T) {
	req := &dto.ClaudeRequest{
		Model: "gemini-3.6-flash",
		Messages: []dto.ClaudeMessage{
			{Role: "system", Content: "Generate a concise, sentence-case title. Return 3-7 words."},
			{Role: "user", Content: "<session>\nUser: hello"},
		},
	}

	assert.True(t, isClaudeTitleGenerationRequest(req))
}

func TestIsClaudeTitleGenerationRequestRejectsNormalChat(t *testing.T) {
	req := &dto.ClaudeRequest{
		Model: "claude-sonnet-4-5",
		System: "You are a helpful assistant.",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "Generate a concise, sentence-case title for my essay."},
		},
	}

	assert.False(t, isClaudeTitleGenerationRequest(req))
}

func TestIsClaudeTitleGenerationRequestRejectsTools(t *testing.T) {
	req := &dto.ClaudeRequest{
		Model:  "claude-sonnet-4-5",
		System: "Generate a concise, sentence-case title. Use 3-7 words.",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "<session>\nUser: hello"},
		},
		Tools: []any{map[string]any{"name": "search"}},
	}

	assert.False(t, isClaudeTitleGenerationRequest(req))
}

func TestWriteClaudeTitleResponseNonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(common.RequestIdKey, "test")

	writeClaudeTitleResponse(c, &dto.ClaudeRequest{Model: "claude-sonnet-4-5"})

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp dto.ClaudeResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.Equal(t, "message", resp.Type)
	assert.Equal(t, "assistant", resp.Role)
	require.Len(t, resp.Content, 1)
	assert.Equal(t, claudeTitleShortCircuitText, resp.Content[0].GetText())
	assert.Equal(t, "end_turn", resp.StopReason)
}

func TestWriteClaudeTitleResponseStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(common.RequestIdKey, "test")

	writeClaudeTitleResponse(c, &dto.ClaudeRequest{
		Model:  "claude-sonnet-4-5",
		Stream: common.GetPointer(true),
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	assert.Contains(t, body, "event: message_start")
	assert.Contains(t, body, "event: content_block_delta")
	assert.Contains(t, body, "event: message_stop")
	assert.True(t, strings.Contains(body, claudeTitleShortCircuitText))
}

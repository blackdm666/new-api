package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const claudeTitleShortCircuitText = "New Chat Session"

func shortCircuitClaudeTitleRequest(c *gin.Context, relayFormat types.RelayFormat, request dto.Request) bool {
	if relayFormat != types.RelayFormatClaude {
		return false
	}
	claudeReq, ok := request.(*dto.ClaudeRequest)
	if !ok || !isClaudeTitleGenerationRequest(claudeReq) {
		return false
	}

	logger.LogInfo(c, "short-circuiting Claude title generation request")
	writeClaudeTitleResponse(c, claudeReq)
	return true
}

func isClaudeTitleGenerationRequest(req *dto.ClaudeRequest) bool {
	if req == nil || hasClaudeRequestTools(req) {
		return false
	}

	systemText, nonSystemMessages := claudeTitleRequestTexts(req)
	systemText = strings.ToLower(systemText)
	if !strings.Contains(systemText, "generate a concise, sentence-case title") ||
		!strings.Contains(systemText, "3-7 words") {
		return false
	}

	if len(nonSystemMessages) != 1 {
		return false
	}
	userMessage := nonSystemMessages[0]
	if strings.ToLower(strings.TrimSpace(userMessage.Role)) != "user" {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(userMessage.GetStringContent()), "<session>")
}

func hasClaudeRequestTools(req *dto.ClaudeRequest) bool {
	if req.Tools != nil {
		switch tools := req.Tools.(type) {
		case []any:
			if len(tools) > 0 {
				return true
			}
		default:
			return true
		}
	}
	return len(req.McpServers) > 0 || req.ToolChoice != nil
}

func claudeTitleRequestTexts(req *dto.ClaudeRequest) (string, []dto.ClaudeMessage) {
	var systemParts []string
	if req.System != nil {
		if req.IsStringSystem() {
			systemParts = append(systemParts, req.GetStringSystem())
		} else {
			for _, part := range req.ParseSystem() {
				if part.Type == dto.ContentTypeText {
					systemParts = append(systemParts, part.GetText())
				}
			}
		}
	}

	nonSystemMessages := make([]dto.ClaudeMessage, 0, len(req.Messages))
	for _, message := range req.Messages {
		if strings.ToLower(strings.TrimSpace(message.Role)) == "system" {
			systemParts = append(systemParts, message.GetStringContent())
			continue
		}
		nonSystemMessages = append(nonSystemMessages, message)
	}
	return strings.Join(systemParts, "\n"), nonSystemMessages
}

func writeClaudeTitleResponse(c *gin.Context, req *dto.ClaudeRequest) {
	if req.IsStream(c.Request) {
		writeClaudeTitleStreamResponse(c, req.Model)
		return
	}

	c.JSON(http.StatusOK, dto.ClaudeResponse{
		Id:         relayhelper.GetResponseID(c),
		Type:       "message",
		Role:       "assistant",
		Model:      req.Model,
		StopReason: "end_turn",
		Content: []dto.ClaudeMediaMessage{
			{
				Type: dto.ContentTypeText,
				Text: common.GetPointer(claudeTitleShortCircuitText),
			},
		},
		Usage: &dto.ClaudeUsage{OutputTokens: 3},
	})
}

func writeClaudeTitleStreamResponse(c *gin.Context, model string) {
	relayhelper.SetEventStreamHeaders(c)
	c.Status(http.StatusOK)
	responseID := relayhelper.GetResponseID(c)
	usage := &dto.ClaudeUsage{OutputTokens: 3}

	_ = relayhelper.ClaudeData(c, dto.ClaudeResponse{
		Type: "message_start",
		Message: &dto.ClaudeMediaMessage{
			Id:      responseID,
			Type:    "message",
			Role:    "assistant",
			Model:   model,
			Content: []dto.ClaudeMediaMessage{},
			Usage:   &dto.ClaudeUsage{},
		},
	})
	_ = relayhelper.ClaudeData(c, dto.ClaudeResponse{
		Type:  "content_block_start",
		Index: common.GetPointer(0),
		ContentBlock: &dto.ClaudeMediaMessage{
			Type: dto.ContentTypeText,
			Text: common.GetPointer(""),
		},
	})
	_ = relayhelper.ClaudeData(c, dto.ClaudeResponse{
		Type:  "content_block_delta",
		Index: common.GetPointer(0),
		Delta: &dto.ClaudeMediaMessage{
			Type: "text_delta",
			Text: common.GetPointer(claudeTitleShortCircuitText),
		},
	})
	_ = relayhelper.ClaudeData(c, dto.ClaudeResponse{
		Type:  "content_block_stop",
		Index: common.GetPointer(0),
	})
	_ = relayhelper.ClaudeData(c, dto.ClaudeResponse{
		Type:  "message_delta",
		Usage: usage,
		Delta: &dto.ClaudeMediaMessage{
			StopReason: common.GetPointer("end_turn"),
		},
	})
	_ = relayhelper.ClaudeData(c, dto.ClaudeResponse{Type: "message_stop"})
}

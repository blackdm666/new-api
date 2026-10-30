package service

import (
	"context"
	"fmt"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func init() {
	relayconvert.SetMediaResolver(relayconvert.MediaResolver{
		// relayconvert is gin-free; recover the gin context when the caller
		// passed one so file caching/cleanup keeps working.
		GetBase64Data: func(ctx context.Context, source types.FileSource, reason ...string) (string, string, error) {
			return GetBase64Data(ginContextFrom(ctx), source, reason...)
		},
		DecodeBase64FileData: DecodeBase64FileData,
	})
}

// ginContextFrom returns the request's gin context, also when relayconvert
// wrapped it (the conversion diagnostics collector does). Without it, disk
// cached files are never registered for cleanup at the end of the request.
func ginContextFrom(ctx context.Context) *gin.Context {
	if ctx == nil {
		return nil
	}
	if ginCtx, ok := ctx.(*gin.Context); ok {
		return ginCtx
	}
	ginCtx, _ := ctx.Value(gin.ContextKey).(*gin.Context)
	return ginCtx
}

func ConvertRequest(c *gin.Context, info *relaycommon.RelayInfo, target types.RelayFormat, request any) (*relayconvert.RequestResult, error) {
	result, err := relayconvert.ConvertRequest(c, info, target, request)
	if result != nil {
		info.RecordConversionDiagnostics(c, result.Diagnostics)
	}
	return result, err
}

func ConvertRequestByID(c *gin.Context, info *relaycommon.RelayInfo, converter string, request any) (*relayconvert.RequestResult, error) {
	result, err := relayconvert.ConvertRequestByID(c, info, converter, request)
	if result != nil {
		info.RecordConversionDiagnostics(c, result.Diagnostics)
	}
	return result, err
}

func ConvertRequestVia(c *gin.Context, info *relaycommon.RelayInfo, request any, path ...types.RelayFormat) (*relayconvert.RequestResult, error) {
	result, err := relayconvert.ConvertRequestVia(c, info, request, path...)
	if result != nil {
		info.RecordConversionDiagnostics(c, result.Diagnostics)
	}
	return result, err
}

func ClaudeToOpenAIRequest(claudeRequest dto.ClaudeRequest, info *relaycommon.RelayInfo) (*dto.GeneralOpenAIRequest, error) {
	result, err := ConvertRequest(nil, info, types.RelayFormatOpenAI, &claudeRequest)
	if err != nil {
		return nil, err
	}
	openAIRequest, ok := result.Value.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value)
	}
	return openAIRequest, nil
}

func GeminiToOpenAIRequest(geminiRequest *dto.GeminiChatRequest, info *relaycommon.RelayInfo) (*dto.GeneralOpenAIRequest, error) {
	result, err := ConvertRequest(nil, info, types.RelayFormatOpenAI, geminiRequest)
	if err != nil {
		return nil, err
	}
	openAIRequest, ok := result.Value.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value)
	}
	return openAIRequest, nil
}

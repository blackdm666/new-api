package vertex

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	rootconstant "github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	vertexTTSSampleRate    = 24000
	vertexTTSChannels      = 1
	vertexTTSBitsPerSample = 16
)

func handleVertexTTSResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, responseFormat string) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	var geminiResponse dto.GeminiChatResponse
	if err = common.Unmarshal(responseBody, &geminiResponse); err != nil {
		return nil, types.NewOpenAIError(fmt.Errorf("failed to decode Vertex AI TTS response: %w", err), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	var pcm []byte
	mimeType := ""
	for _, candidate := range geminiResponse.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData == nil || part.InlineData.Data == "" {
				continue
			}
			chunk, decodeErr := base64.StdEncoding.DecodeString(part.InlineData.Data)
			if decodeErr != nil {
				return nil, types.NewOpenAIError(fmt.Errorf("failed to decode Vertex AI TTS audio: %w", decodeErr), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
			if mimeType == "" {
				mimeType = part.InlineData.MimeType
			}
			pcm = append(pcm, chunk...)
		}
	}
	if len(pcm) == 0 {
		if geminiResponse.PromptFeedback != nil && geminiResponse.PromptFeedback.BlockReason != nil {
			reason := *geminiResponse.PromptFeedback.BlockReason
			common.SetContextKey(c, rootconstant.ContextKeyAdminRejectReason, "gemini_block_reason="+reason)
			return nil, types.NewOpenAIError(errors.New("request blocked by Vertex AI: "+reason), types.ErrorCodePromptBlocked, http.StatusBadRequest)
		}
		return nil, types.NewOpenAIError(errors.New("Vertex AI TTS response did not contain audio data"), types.ErrorCodeEmptyResponse, http.StatusInternalServerError)
	}

	normalizedMIME := strings.ToLower(strings.TrimSpace(mimeType))
	isPCM := normalizedMIME == "" || strings.Contains(normalizedMIME, "l16") || strings.Contains(normalizedMIME, "pcm")
	contentType := "audio/pcm"
	audioData := pcm
	if responseFormat == "wav" {
		if isPCM {
			audioData, err = wrapVertexPCMAsWAV(pcm)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
		} else if !strings.Contains(normalizedMIME, "wav") {
			return nil, types.NewOpenAIError(fmt.Errorf("unsupported Vertex AI TTS audio MIME type %q", mimeType), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		contentType = "audio/wav"
	} else if !isPCM {
		return nil, types.NewOpenAIError(fmt.Errorf("expected PCM audio from Vertex AI TTS, got %q", mimeType), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	usage := relayconvert.UsageFromGeminiMetadata(geminiResponse.GetUsageMetadata(), info.GetEstimatePromptTokens())
	if usage == nil {
		usage = &dto.Usage{PromptTokens: info.GetEstimatePromptTokens()}
	}
	if usage.PromptTokens > 0 && usage.PromptTokensDetails.TextTokens == 0 && usage.PromptTokensDetails.AudioTokens == 0 {
		usage.PromptTokensDetails.TextTokens = usage.PromptTokens
	}
	if usage.CompletionTokenDetails.AudioTokens == 0 {
		if usage.CompletionTokens > 0 {
			usage.CompletionTokenDetails.AudioTokens = usage.CompletionTokens
		} else {
			bytesPerSecond := vertexTTSSampleRate * vertexTTSChannels * vertexTTSBitsPerSample / 8
			duration := float64(len(pcm)) / float64(bytesPerSecond)
			audioTokens := common.QuotaRound(math.Ceil(duration) / 60 * 1000)
			usage.CompletionTokens = audioTokens
			usage.CompletionTokenDetails.AudioTokens = audioTokens
		}
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	c.Data(resp.StatusCode, contentType, audioData)
	return usage, nil
}

func wrapVertexPCMAsWAV(pcm []byte) ([]byte, error) {
	if uint64(len(pcm)) > uint64(^uint32(0))-36 {
		return nil, errors.New("Vertex AI TTS audio is too large for a WAV container")
	}

	wav := make([]byte, 44+len(pcm))
	copy(wav[0:4], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(36+len(pcm)))
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], vertexTTSChannels)
	binary.LittleEndian.PutUint32(wav[24:28], vertexTTSSampleRate)
	bytesPerSecond := vertexTTSSampleRate * vertexTTSChannels * vertexTTSBitsPerSample / 8
	binary.LittleEndian.PutUint32(wav[28:32], uint32(bytesPerSecond))
	binary.LittleEndian.PutUint16(wav[32:34], vertexTTSChannels*vertexTTSBitsPerSample/8)
	binary.LittleEndian.PutUint16(wav[34:36], vertexTTSBitsPerSample)
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], uint32(len(pcm)))
	copy(wav[44:], pcm)
	return wav, nil
}

package vertex

import (
	"bytes"
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
	"github.com/braheezy/shine-mp3/pkg/mp3"
	"github.com/gin-gonic/gin"
	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
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
	if !isPCM && !(responseFormat == "wav" && strings.Contains(normalizedMIME, "wav")) {
		return nil, types.NewOpenAIError(fmt.Errorf("expected PCM audio from Vertex AI TTS, got %q", mimeType), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	contentType := "audio/pcm"
	audioData := pcm
	switch {
	case responseFormat == "wav" && isPCM:
		contentType = "audio/wav"
		audioData, err = wrapVertexPCMAsWAV(pcm)
	case responseFormat == "wav":
		contentType = "audio/wav"
	case responseFormat == "mp3":
		contentType = "audio/mpeg"
		audioData, err = encodeVertexPCMAsMP3(pcm)
	case responseFormat == "flac":
		contentType = "audio/flac"
		audioData, err = encodeVertexPCMAsFLAC(pcm)
	}
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
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

// encodeVertexPCMAsMP3 encodes the PCM as constant 128 kbps MPEG-2 Layer III.
// The encoder only takes whole 576-sample frames, so the tail is padded with
// silence, plus two extra frames to flush the ~44 ms encoder/decoder delay.
func encodeVertexPCMAsMP3(pcm []byte) ([]byte, error) {
	const frameSamples = 576
	sampleCount := len(pcm) / 2
	samples := make([]int16, (sampleCount/frameSamples+3)*frameSamples)
	for i := range sampleCount {
		samples[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}

	var buf bytes.Buffer
	buf.Grow(len(samples) * 2 / 3) // 128 kbps at 24 kHz is 2/3 byte per sample
	if err := mp3.NewEncoder(vertexTTSSampleRate, vertexTTSChannels).Write(&buf, samples); err != nil {
		return nil, fmt.Errorf("failed to encode Vertex AI TTS audio as MP3: %w", err)
	}
	return buf.Bytes(), nil
}

func encodeVertexPCMAsFLAC(pcm []byte) ([]byte, error) {
	const blockSize = 4096
	sampleCount := len(pcm) / 2

	var buf bytes.Buffer
	encoder, err := flac.NewEncoder(&buf, &meta.StreamInfo{
		BlockSizeMin:  blockSize,
		BlockSizeMax:  blockSize,
		SampleRate:    vertexTTSSampleRate,
		NChannels:     vertexTTSChannels,
		BitsPerSample: vertexTTSBitsPerSample,
		NSamples:      uint64(sampleCount),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode Vertex AI TTS audio as FLAC: %w", err)
	}
	for frameIndex, start := 0, 0; start < sampleCount; frameIndex, start = frameIndex+1, start+blockSize {
		samples := make([]int32, min(blockSize, sampleCount-start))
		for i := range samples {
			samples[i] = int32(int16(binary.LittleEndian.Uint16(pcm[(start+i)*2:])))
		}
		err = encoder.WriteFrame(&frame.Frame{
			Header: frame.Header{
				HasFixedBlockSize: true,
				BlockSize:         uint16(len(samples)),
				SampleRate:        vertexTTSSampleRate,
				Channels:          frame.ChannelsMono,
				BitsPerSample:     vertexTTSBitsPerSample,
				Num:               uint64(frameIndex),
			},
			Subframes: []*frame.Subframe{{
				SubHeader: frame.SubHeader{Pred: frame.PredVerbatim},
				Samples:   samples,
				NSamples:  len(samples),
			}},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to encode Vertex AI TTS audio as FLAC: %w", err)
		}
	}
	if err = encoder.Close(); err != nil {
		return nil, fmt.Errorf("failed to encode Vertex AI TTS audio as FLAC: %w", err)
	}
	return buf.Bytes(), nil
}

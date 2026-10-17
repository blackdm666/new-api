package gemini

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	rootconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
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
	ttsSampleRate    = 24000
	ttsChannels      = 1
	ttsBitsPerSample = 16
)

// ttsVoices are the Gemini-TTS prebuilt voices; Gemini rejects any other
// voice name with "No matching speaker voice found".
var ttsVoices = []string{
	"Achernar", "Achird", "Algenib", "Algieba", "Alnilam", "Aoede", "Autonoe", "Callirrhoe", "Charon", "Despina",
	"Enceladus", "Erinome", "Fenrir", "Gacrux", "Iapetus", "Kore", "Laomedeia", "Leda", "Orus", "Puck",
	"Pulcherrima", "Rasalgethi", "Sadachbia", "Sadaltager", "Schedar", "Sulafat", "Umbriel", "Vindemiatrix", "Zephyr", "Zubenelgenubi",
}

// invalidTTSRequest is a client error in a speech request: 400, not retried on
// another channel, with the message in the client's language.
func invalidTTSRequest(c *gin.Context, key string, args map[string]any) error {
	return types.NewErrorWithStatusCode(errors.New(i18n.T(c, key, args)), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

// ConvertSpeechRequest turns an OpenAI /v1/audio/speech request into a Gemini
// TTS generateContent request. It returns the body and the normalized
// response_format that SpeechHandler must produce.
func ConvertSpeechRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, string, error) {
	if info.IsStream || request.StreamFormat == "sse" {
		return nil, "", invalidTTSRequest(c, i18n.MsgTTSStreamUnsupported, map[string]any{"Model": info.OriginModelName})
	}

	input := strings.TrimSpace(request.Input)
	if input == "" {
		return nil, "", invalidTTSRequest(c, i18n.MsgTTSInputRequired, nil)
	}
	voice := strings.TrimSpace(request.Voice)
	if voice == "" {
		return nil, "", invalidTTSRequest(c, i18n.MsgTTSVoiceRequired, map[string]any{"Voices": strings.Join(ttsVoices, ", ")})
	}
	voiceIndex := slices.IndexFunc(ttsVoices, func(name string) bool { return strings.EqualFold(name, voice) })
	if voiceIndex < 0 {
		return nil, "", invalidTTSRequest(c, i18n.MsgTTSVoiceUnsupported, map[string]any{"Model": info.OriginModelName, "Voice": voice, "Voices": strings.Join(ttsVoices, ", ")})
	}
	voice = ttsVoices[voiceIndex]

	responseFormat := strings.ToLower(strings.TrimSpace(request.ResponseFormat))
	if responseFormat == "" {
		responseFormat = "wav"
	}
	if responseFormat != "mp3" && responseFormat != "wav" && responseFormat != "flac" && responseFormat != "pcm" {
		return nil, "", invalidTTSRequest(c, i18n.MsgTTSResponseFormatUnsupported, map[string]any{"Model": info.OriginModelName, "Format": responseFormat})
	}

	instructions := strings.TrimSpace(request.Instructions)
	if request.Speed != nil {
		if *request.Speed < 0.25 || *request.Speed > 4 {
			return nil, "", invalidTTSRequest(c, i18n.MsgTTSSpeedOutOfRange, map[string]any{"Speed": fmt.Sprintf("%g", *request.Speed)})
		}
		if *request.Speed != 1 {
			speedInstruction := fmt.Sprintf("Speak at %gx normal speed", *request.Speed)
			if instructions == "" {
				instructions = speedInstruction
			} else {
				instructions += ". " + speedInstruction
			}
		}
	}

	prompt := input
	if instructions != "" {
		prompt = instructions + ": " + input
	}
	if len(prompt) > 8000 {
		return nil, "", invalidTTSRequest(c, i18n.MsgTTSInputTooLong, map[string]any{"Bytes": len(prompt)})
	}

	speechConfig, err := common.Marshal(map[string]any{
		"voiceConfig": map[string]any{"prebuiltVoiceConfig": map[string]any{"voiceName": voice}},
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal Gemini speech config: %w", err)
	}

	geminiRequest := dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{{
			Role:  "user",
			Parts: []dto.GeminiPart{{Text: prompt}},
		}},
		GenerationConfig: dto.GeminiChatGenerationConfig{
			ResponseModalities: []string{"AUDIO"},
			SpeechConfig:       speechConfig,
		},
	}
	jsonData, err := common.Marshal(geminiRequest)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal Gemini audio speech request: %w", err)
	}
	return bytes.NewReader(jsonData), responseFormat, nil
}

// RejectTTSModelChat is the client error for a chat request sent to a Gemini
// TTS model, which upstream only answers with "invalid argument"; nil for any
// other model.
func RejectTTSModelChat(c *gin.Context, info *relaycommon.RelayInfo) error {
	if !strings.Contains(info.UpstreamModelName, "-tts") {
		return nil
	}
	return invalidTTSRequest(c, i18n.MsgTTSChatEndpointUnsupported, map[string]any{"Model": info.OriginModelName})
}

// SpeechHandler answers an OpenAI /v1/audio/speech request from a Gemini TTS
// generateContent response: Gemini returns raw 24 kHz 16-bit mono PCM, which is
// sent as-is or encoded into responseFormat. Audio tokens come from usage
// metadata, or from the PCM length when upstream reports none.
func SpeechHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, responseFormat string) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	var geminiResponse dto.GeminiChatResponse
	if err = common.Unmarshal(responseBody, &geminiResponse); err != nil {
		return nil, types.NewOpenAIError(fmt.Errorf("failed to decode Gemini TTS response: %w", err), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
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
				return nil, types.NewOpenAIError(fmt.Errorf("failed to decode Gemini TTS audio: %w", decodeErr), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
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
			return nil, types.NewOpenAIError(errors.New("request blocked by Gemini: "+reason), types.ErrorCodePromptBlocked, http.StatusBadRequest)
		}
		finishReason := "unknown"
		for _, candidate := range geminiResponse.Candidates {
			if candidate.FinishReason != nil && *candidate.FinishReason != "" {
				finishReason = *candidate.FinishReason
				break
			}
		}
		return nil, types.NewOpenAIError(errors.New(i18n.T(c, i18n.MsgTTSNoAudio, map[string]any{"Reason": finishReason})), types.ErrorCodeEmptyResponse, http.StatusInternalServerError)
	}

	normalizedMIME := strings.ToLower(strings.TrimSpace(mimeType))
	isPCM := normalizedMIME == "" || strings.Contains(normalizedMIME, "l16") || strings.Contains(normalizedMIME, "pcm")
	if !isPCM && !(responseFormat == "wav" && strings.Contains(normalizedMIME, "wav")) {
		return nil, types.NewOpenAIError(fmt.Errorf("expected PCM audio from Gemini TTS, got %q", mimeType), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	contentType := "audio/pcm"
	audioData := pcm
	switch {
	case responseFormat == "wav" && isPCM:
		contentType = "audio/wav"
		audioData, err = wrapPCMAsWAV(pcm)
	case responseFormat == "wav":
		contentType = "audio/wav"
	case responseFormat == "mp3":
		contentType = "audio/mpeg"
		audioData, err = encodePCMAsMP3(pcm)
	case responseFormat == "flac":
		contentType = "audio/flac"
		audioData, err = encodePCMAsFLAC(pcm)
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
			bytesPerSecond := ttsSampleRate * ttsChannels * ttsBitsPerSample / 8
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

func wrapPCMAsWAV(pcm []byte) ([]byte, error) {
	if uint64(len(pcm)) > uint64(^uint32(0))-36 {
		return nil, errors.New("Gemini TTS audio is too large for a WAV container")
	}

	wav := make([]byte, 44+len(pcm))
	copy(wav[0:4], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(36+len(pcm)))
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], ttsChannels)
	binary.LittleEndian.PutUint32(wav[24:28], ttsSampleRate)
	bytesPerSecond := ttsSampleRate * ttsChannels * ttsBitsPerSample / 8
	binary.LittleEndian.PutUint32(wav[28:32], uint32(bytesPerSecond))
	binary.LittleEndian.PutUint16(wav[32:34], ttsChannels*ttsBitsPerSample/8)
	binary.LittleEndian.PutUint16(wav[34:36], ttsBitsPerSample)
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], uint32(len(pcm)))
	copy(wav[44:], pcm)
	return wav, nil
}

// encodePCMAsMP3 encodes the PCM as constant 128 kbps MPEG-2 Layer III.
// The encoder only takes whole 576-sample frames, so the tail is padded with
// silence, plus two extra frames to flush the ~44 ms encoder/decoder delay.
func encodePCMAsMP3(pcm []byte) ([]byte, error) {
	const frameSamples = 576
	sampleCount := len(pcm) / 2
	samples := make([]int16, (sampleCount/frameSamples+3)*frameSamples)
	for i := range sampleCount {
		samples[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}

	var buf bytes.Buffer
	buf.Grow(len(samples) * 2 / 3) // 128 kbps at 24 kHz is 2/3 byte per sample
	if err := mp3.NewEncoder(ttsSampleRate, ttsChannels).Write(&buf, samples); err != nil {
		return nil, fmt.Errorf("failed to encode Gemini TTS audio as MP3: %w", err)
	}
	return buf.Bytes(), nil
}

func encodePCMAsFLAC(pcm []byte) ([]byte, error) {
	const blockSize = 4096
	sampleCount := len(pcm) / 2

	var buf bytes.Buffer
	encoder, err := flac.NewEncoder(&buf, &meta.StreamInfo{
		BlockSizeMin:  blockSize,
		BlockSizeMax:  blockSize,
		SampleRate:    ttsSampleRate,
		NChannels:     ttsChannels,
		BitsPerSample: ttsBitsPerSample,
		NSamples:      uint64(sampleCount),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode Gemini TTS audio as FLAC: %w", err)
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
				SampleRate:        ttsSampleRate,
				Channels:          frame.ChannelsMono,
				BitsPerSample:     ttsBitsPerSample,
				Num:               uint64(frameIndex),
			},
			Subframes: []*frame.Subframe{{
				SubHeader: frame.SubHeader{Pred: frame.PredVerbatim},
				Samples:   samples,
				NSamples:  len(samples),
			}},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to encode Gemini TTS audio as FLAC: %w", err)
		}
	}
	if err = encoder.Close(); err != nil {
		return nil, fmt.Errorf("failed to encode Gemini TTS audio as FLAC: %w", err)
	}
	return buf.Bytes(), nil
}

package gemini

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
	rootconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// transcriptionMaxFileBytes is OpenAI's 25 MB transcription upload limit. The
// audio goes inline as base64 (a third larger), which stays well inside the
// inline request size Gemini upstreams accept.
const transcriptionMaxFileBytes = 25 << 20

// transcriptionMIMEByExt names the Gemini audio MIME type for files whose
// content is not recognized by its header.
var transcriptionMIMEByExt = map[string]string{
	".mp3":  "audio/mp3",
	".mpga": "audio/mp3",
	".mpeg": "audio/mp3",
	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
	".oga":  "audio/ogg",
	".opus": "audio/ogg",
	".flac": "audio/flac",
	".m4a":  "audio/mp4",
	".mp4":  "audio/mp4",
	".aac":  "audio/aac",
	".webm": "audio/webm",
	".aiff": "audio/aiff",
	".aif":  "audio/aiff",
}

// Transcription is what TranscriptionHandler needs from the client request.
type Transcription struct {
	ResponseFormat string
	// Language is the client's ISO-639-1 hint, empty when not given.
	Language string
	// Duration is the audio length in seconds parsed from the file for
	// verbose_json, 0 when unknown.
	Duration float64
}

// transcriptionAudioMIME detects the audio container from the file header and
// falls back to the file extension; it returns "" for an unknown format.
func transcriptionAudioMIME(audio []byte, filename string) string {
	switch {
	case len(audio) >= 12 && string(audio[0:4]) == "RIFF" && string(audio[8:12]) == "WAVE":
		return "audio/wav"
	case bytes.HasPrefix(audio, []byte("ID3")):
		return "audio/mp3"
	case bytes.HasPrefix(audio, []byte("OggS")):
		return "audio/ogg"
	case bytes.HasPrefix(audio, []byte("fLaC")):
		return "audio/flac"
	case len(audio) >= 8 && string(audio[4:8]) == "ftyp":
		return "audio/mp4"
	case bytes.HasPrefix(audio, []byte{0x1A, 0x45, 0xDF, 0xA3}):
		return "audio/webm"
	case len(audio) >= 12 && string(audio[0:4]) == "FORM" && (string(audio[8:12]) == "AIFF" || string(audio[8:12]) == "AIFC"):
		return "audio/aiff"
	case len(audio) >= 2 && audio[0] == 0xFF && audio[1]&0xF6 == 0xF0:
		// ADTS AAC: 12-bit sync and layer 00.
		return "audio/aac"
	case len(audio) >= 2 && audio[0] == 0xFF && audio[1]&0xE0 == 0xE0 && audio[1]&0x06 != 0:
		// MPEG audio frame without an ID3 tag.
		return "audio/mp3"
	}
	return transcriptionMIMEByExt[strings.ToLower(filepath.Ext(filename))]
}

// ConvertTranscriptionRequest turns an OpenAI /v1/audio/transcriptions
// multipart request into a Gemini generateContent request with the audio
// inline. language and prompt become hints in the instruction.
func ConvertTranscriptionRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, *Transcription, error) {
	if info.RelayMode == constant.RelayModeAudioTranslation {
		return nil, nil, invalidAudioRequest(c, i18n.MsgSTTTranslationUnsupported, map[string]any{"Model": info.OriginModelName})
	}
	responseFormat := strings.ToLower(strings.TrimSpace(request.ResponseFormat))
	if responseFormat == "" {
		responseFormat = "json"
	}
	if responseFormat != "json" && responseFormat != "text" && responseFormat != "verbose_json" {
		return nil, nil, invalidAudioRequest(c, i18n.MsgSTTResponseFormatUnsupported, map[string]any{"Model": info.OriginModelName, "Format": request.ResponseFormat})
	}

	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing multipart form: %w", err)
	}
	if stream := form.Value["stream"]; len(stream) > 0 && strings.EqualFold(strings.TrimSpace(stream[0]), "true") {
		return nil, nil, invalidAudioRequest(c, i18n.MsgSTTStreamUnsupported, map[string]any{"Model": info.OriginModelName})
	}
	files := form.File["file"]
	if len(files) == 0 {
		return nil, nil, invalidAudioRequest(c, i18n.MsgSTTFileRequired, nil)
	}
	fileHeader := files[0]
	if fileHeader.Size > transcriptionMaxFileBytes {
		return nil, nil, invalidAudioRequest(c, i18n.MsgSTTFileTooLarge, map[string]any{
			"Size":  fmt.Sprintf("%.1f", float64(fileHeader.Size)/(1<<20)),
			"Limit": transcriptionMaxFileBytes >> 20,
		})
	}
	file, err := fileHeader.Open()
	if err != nil {
		return nil, nil, fmt.Errorf("error opening audio file: %w", err)
	}
	defer file.Close()
	audio, err := io.ReadAll(file)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading audio file: %w", err)
	}
	mimeType := transcriptionAudioMIME(audio, fileHeader.Filename)
	if mimeType == "" {
		return nil, nil, invalidAudioRequest(c, i18n.MsgSTTFileFormatUnsupported, nil)
	}

	transcription := &Transcription{ResponseFormat: responseFormat}
	if language := form.Value["language"]; len(language) > 0 {
		transcription.Language = strings.TrimSpace(language[0])
	}
	var instruction strings.Builder
	instruction.WriteString("Transcribe the speech in this audio verbatim, in the language it is spoken. Do not translate or summarize, and do not add timestamps, speaker labels or notes. If there is no speech, the transcript is empty.")
	if transcription.Language != "" {
		fmt.Fprintf(&instruction, "\nThe audio language is %q (ISO-639-1).", transcription.Language)
	}
	if prompt := form.Value["prompt"]; len(prompt) > 0 && strings.TrimSpace(prompt[0]) != "" {
		fmt.Fprintf(&instruction, "\nContext for spelling and style only, it is not part of the audio: %s", strings.TrimSpace(prompt[0]))
	}

	var generationConfig dto.GeminiChatGenerationConfig
	if responseFormat == "verbose_json" {
		instruction.WriteString("\nReturn JSON: text is the transcript, language is the spoken language as a lowercase English name such as english or chinese.")
		generationConfig.ResponseMimeType = "application/json"
		generationConfig.ResponseSchema = map[string]any{
			"type": "OBJECT",
			"properties": map[string]any{
				"language": map[string]any{"type": "STRING"},
				"text":     map[string]any{"type": "STRING"},
			},
			"required": []string{"language", "text"},
		}
		// Display only: billing uses Gemini's token counts, never this value.
		if ext := filepath.Ext(fileHeader.Filename); ext != "" {
			seconds, durationErr := common.GetAudioDuration(c.Request.Context(), bytes.NewReader(audio), ext)
			if durationErr == nil && seconds > 0 && !math.IsInf(seconds, 0) {
				transcription.Duration = math.Round(seconds*100) / 100
			}
		}
	} else {
		instruction.WriteString("\nOutput only the transcript text.")
	}

	geminiRequest := dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{{
			Role: "user",
			Parts: []dto.GeminiPart{
				{InlineData: &dto.GeminiInlineData{MimeType: mimeType, Data: base64.StdEncoding.EncodeToString(audio)}},
				{Text: instruction.String()},
			},
		}},
		GenerationConfig: generationConfig,
	}
	body, err := common.Marshal(geminiRequest)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal Gemini transcription request: %w", err)
	}
	return bytes.NewReader(body), transcription, nil
}

// TranscriptionHandler answers an OpenAI /v1/audio/transcriptions request from
// a Gemini generateContent response. Usage is Gemini's usageMetadata, the same
// accounting as chat requests to the model; without it the prompt falls back to
// the pre-consume audio estimate and the completion to the transcript length.
func TranscriptionHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, transcription *Transcription) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	var geminiResponse dto.GeminiChatResponse
	if err = common.Unmarshal(responseBody, &geminiResponse); err != nil {
		return nil, types.NewOpenAIError(fmt.Errorf("failed to decode Gemini transcription response: %w", err), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if len(geminiResponse.Candidates) == 0 {
		if geminiResponse.PromptFeedback != nil && geminiResponse.PromptFeedback.BlockReason != nil {
			reason := *geminiResponse.PromptFeedback.BlockReason
			common.SetContextKey(c, rootconstant.ContextKeyAdminRejectReason, "gemini_block_reason="+reason)
			return nil, types.NewOpenAIError(errors.New("request blocked by Gemini: "+reason), types.ErrorCodePromptBlocked, http.StatusBadRequest)
		}
		return nil, types.NewOpenAIError(errors.New("empty response from Gemini API"), types.ErrorCodeEmptyResponse, http.StatusInternalServerError)
	}

	var transcript strings.Builder
	for _, part := range geminiResponse.Candidates[0].Content.Parts {
		if !part.Thought {
			transcript.WriteString(part.Text)
		}
	}
	text := strings.TrimSpace(transcript.String())

	var body []byte
	contentType := "application/json"
	switch transcription.ResponseFormat {
	case "text":
		contentType = "text/plain; charset=utf-8"
		body = []byte(text)
	case "verbose_json":
		// Gemini returns no timing, so segments and words are left out rather
		// than invented.
		verbose := struct {
			Task     string  `json:"task"`
			Language string  `json:"language"`
			Duration float64 `json:"duration,omitempty"`
			Text     string  `json:"text"`
		}{Task: "transcribe", Language: transcription.Language, Duration: transcription.Duration, Text: text}
		var result struct {
			Language string `json:"language"`
			Text     string `json:"text"`
		}
		if common.UnmarshalJsonStr(text, &result) == nil {
			verbose.Text = strings.TrimSpace(result.Text)
			if language := strings.ToLower(strings.TrimSpace(result.Language)); language != "" {
				verbose.Language = language
			}
		}
		body, err = common.Marshal(verbose)
	default:
		body, err = common.Marshal(dto.AudioResponse{Text: text})
	}
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	usage := buildUsageFromGeminiResponse(c, info, &geminiResponse)
	c.Data(http.StatusOK, contentType, body)
	return &usage, nil
}

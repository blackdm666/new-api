package xai

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// xAI Voice API（POST /v1/tts、POST /v1/stt）与 OpenAI 的 /v1/audio/speech、
// /v1/audio/transcriptions 字段不同，这里负责双向转换。
const (
	xaiTTSPath = "/v1/tts"
	xaiSTTPath = "/v1/stt"

	xaiDefaultVoice    = "eve"
	xaiDefaultLanguage = "auto"

	xaiTTSModel = "grok-tts"
	xaiSTTModel = "grok-stt"
)

// rejectVoiceModelChat 是用对话 / Responses 接口调 grok-tts、grok-stt、grok-voice-* 时的客户端错误：
// 上游只认 /v1/tts、/v1/stt、/v1/realtime，对话请求只会得到「没有账号支持该模型」的 404。其他模型返回 nil。
// grok-voice-* 按会话时长计价，也不能走 /v1/audio/*，否则会按字符或时长 token 套用实时价格。
func rejectVoiceModelChat(c *gin.Context, info *relaycommon.RelayInfo) error {
	if info == nil {
		return nil
	}
	var key string
	switch info.UpstreamModelName {
	case xaiTTSModel:
		key = i18n.MsgTTSChatEndpointUnsupported
	case xaiSTTModel:
		key = i18n.MsgSTTChatEndpointUnsupported
	default:
		if !isXAIRealtimeModel(info.UpstreamModelName) {
			return nil
		}
		key = i18n.MsgRealtimeEndpointUnsupported
	}
	return types.NewErrorWithStatusCode(errors.New(i18n.T(c, key, map[string]any{"Model": info.OriginModelName})), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

// OpenAI response_format → xAI output_format.codec；opus/aac/flac xAI 不支持。
var xaiTTSCodecs = map[string]string{
	"mp3":   "mp3",
	"wav":   "wav",
	"pcm":   "pcm",
	"mulaw": "mulaw",
	"alaw":  "alaw",
}

// OpenAI /v1/audio/speech 的内置音色，xAI 都不认。
var openAIBuiltinVoices = []string{"alloy", "ash", "ballad", "coral", "echo", "fable", "onyx", "nova", "sage", "shimmer", "verse", "marin", "cedar"}

// xaiBuiltinVoices 是 xAI 官方音色（GET /v1/tts/voices，2026-10-17 共 28 个）。
var xaiBuiltinVoices = []string{"altair", "ara", "atlas", "aurora", "carina", "castor", "celeste", "cosmo", "eve", "helios", "helix", "iris", "kepler", "leo", "liora", "lumen", "luna", "lux", "naksh", "orion", "perseus", "rex", "rigel", "sal", "sirius", "ursa", "zagan", "zenith"}

// xAI 自定义音色 ID 是 8 位小写字母或数字（官方示例 nlbqfwie、abc123xy）。
var xaiCustomVoicePattern = regexp.MustCompile(`^[a-z0-9]{8}$`)

// checkXAIVoice 在连上游前拒绝 xAI 不认的音色：上游对未知音色要等文字发完才报错（流式 TTS）或直接忽略（实时语音）。
// 官方音色不区分大小写，也可带 session.created 里出现的 xai_ 前缀；空值表示用上游默认音色。
func checkXAIVoice(voice string, product string) error {
	name := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(voice)), "xai_")
	if name == "" || slices.Contains(xaiBuiltinVoices, name) || xaiCustomVoicePattern.MatchString(name) {
		return nil
	}
	voices := strings.Join(xaiBuiltinVoices, ", ")
	if slices.Contains(openAIBuiltinVoices, name) {
		return badAudioRequest("voice %q is an OpenAI voice and is not available for %s, use %s or a custom voice id", voice, product, voices)
	}
	return badAudioRequest("voice %q is not available for %s, use %s or a custom voice id", voice, product, voices)
}

// xAI 支持的采样率；同时用于界定 pcm/mulaw/alaw 的时长换算。
var xaiTTSSampleRates = []int{8000, 16000, 22050, 24000, 44100, 48000}

// 流式 TTS 连接参数的取值（https://docs.x.ai/developers/model-capabilities/audio/text-to-speech）。
// 上游对这些参数只回握手失败，sub2api 再转成 502，客户看不到原因，所以在 new-api 侧先校验。
// optimize_streaming_latency 上游超出 0–2 也接受，不校验。
var (
	xaiTTSStreamCodecs   = []string{"mp3", "wav", "pcm", "mulaw", "ulaw", "alaw"}
	xaiTTSBitRates       = []int{32000, 64000, 96000, 128000, 192000}
	xaiTTSStreamBooleans = []string{"true", "false"}
)

const (
	xaiTTSMinSpeed = 0.7
	xaiTTSMaxSpeed = 1.5
)

// ValidateTTSStreamQuery 校验流式 TTS（GET /v1/tts）的连接参数；控制器在升级 WebSocket 之前调用，
// 不合法时直接回 HTTP 400。没传的参数用上游默认值。
func ValidateTTSStreamQuery(query url.Values) error {
	if err := checkXAIVoice(query.Get("voice"), "grok tts"); err != nil {
		return err
	}
	if codec := query.Get("codec"); codec != "" && !slices.Contains(xaiTTSStreamCodecs, codec) {
		return badAudioRequest("codec %q is not supported by grok tts, use %s", codec, strings.Join(xaiTTSStreamCodecs, ", "))
	}
	if raw := query.Get("sample_rate"); raw != "" {
		if rate, err := strconv.Atoi(raw); err != nil || !slices.Contains(xaiTTSSampleRates, rate) {
			return badAudioRequest("sample_rate must be one of 8000, 16000, 22050, 24000, 44100, 48000")
		}
	}
	if raw := query.Get("bit_rate"); raw != "" {
		if rate, err := strconv.Atoi(raw); err != nil || !slices.Contains(xaiTTSBitRates, rate) {
			return badAudioRequest("bit_rate must be one of 32000, 64000, 96000, 128000, 192000")
		}
	}
	if raw := query.Get("speed"); raw != "" {
		if speed, err := strconv.ParseFloat(raw, 64); err != nil || !(speed >= xaiTTSMinSpeed && speed <= xaiTTSMaxSpeed) {
			return badAudioRequest("speed must be between %.1f and %.1f", xaiTTSMinSpeed, xaiTTSMaxSpeed)
		}
	}
	for _, field := range []string{"text_normalization", "with_timestamps"} {
		if raw := query.Get(field); raw != "" && !slices.Contains(xaiTTSStreamBooleans, raw) {
			return badAudioRequest("%s must be true or false", field)
		}
	}
	return nil
}

// 透传给 xAI 的 TTS 原生字段（OpenAI 请求体里没有的部分）。
var xaiTTSPassthroughFields = []string{"text_normalization", "replace", "optimize_streaming_latency"}

// 透传给 xAI 的 STT 原生表单字段；OpenAI 专属字段（prompt、temperature 等）丢弃。
var xaiSTTPassthroughFields = map[string]struct{}{
	"language":      {},
	"format":        {},
	"multichannel":  {},
	"channels":      {},
	"diarize":       {},
	"keyterm":       {},
	"filler_words":  {},
	"vad_threshold": {},
	"audio_format":  {},
	"sample_rate":   {},
	"url":           {},
}

type xaiTTSOutputFormat struct {
	Codec      string `json:"codec"`
	SampleRate *int   `json:"sample_rate,omitempty"`
	BitRate    *int   `json:"bit_rate,omitempty"`
}

func badAudioRequest(format string, args ...any) error {
	return types.NewErrorWithStatusCode(fmt.Errorf(format, args...), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

func xaiTTSCodec(responseFormat string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(responseFormat))
	if format == "" {
		format = "mp3"
	}
	codec, ok := xaiTTSCodecs[format]
	if !ok {
		return "", badAudioRequest("response_format %q is not supported by grok tts, use mp3, wav, pcm, mulaw or alaw", responseFormat)
	}
	return codec, nil
}

func audioLanguage(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	value := gjson.ParseBytes(raw)
	if value.Type != gjson.String {
		return ""
	}
	return strings.TrimSpace(value.String())
}

// buildXAITTSRequest 返回 xAI 请求体、codec 和采样率（0 表示上游默认 24000）。
func buildXAITTSRequest(c *gin.Context, request dto.AudioRequest) (map[string]any, string, int, error) {
	if request.StreamFormat == "sse" {
		return nil, "", 0, badAudioRequest("grok tts streaming is not supported")
	}
	text := strings.TrimSpace(request.Input)
	if text == "" {
		return nil, "", 0, badAudioRequest("input is required")
	}
	codec, err := xaiTTSCodec(request.ResponseFormat)
	if err != nil {
		return nil, "", 0, err
	}

	var rawBody []byte
	if c != nil {
		if storage, err := common.GetBodyStorage(c); err == nil {
			rawBody, _ = storage.Bytes()
		}
	}

	voice := strings.TrimSpace(request.Voice)
	if voice == "" {
		voice = xaiDefaultVoice
	}
	// OpenAI SDK 示例默认用 alloy 等 OpenAI 音色；xAI 对未知音色只回 404，这里先给出明确提示。
	if err := checkXAIVoice(voice, "grok tts"); err != nil {
		return nil, "", 0, err
	}
	language := audioLanguage(request.Language)
	if language == "" {
		language = xaiDefaultLanguage
	}
	outputFormat := xaiTTSOutputFormat{Codec: codec}
	sampleRate := 0
	if v := gjson.GetBytes(rawBody, "sample_rate"); v.Exists() {
		sampleRate = int(v.Int())
		if v.Type != gjson.Number || !slices.Contains(xaiTTSSampleRates, sampleRate) {
			return nil, "", 0, badAudioRequest("sample_rate must be one of 8000, 16000, 22050, 24000, 44100, 48000")
		}
		outputFormat.SampleRate = &sampleRate
	}
	if v := gjson.GetBytes(rawBody, "bit_rate"); v.Type == gjson.Number && codec == "mp3" {
		rate := int(v.Int())
		outputFormat.BitRate = &rate
	}

	payload := map[string]any{
		"text":          request.Input,
		"voice_id":      voice,
		"language":      language,
		"output_format": outputFormat,
	}
	if request.Speed != nil {
		payload["speed"] = *request.Speed
	}
	for _, field := range xaiTTSPassthroughFields {
		if v := gjson.GetBytes(rawBody, field); v.Exists() {
			payload[field] = v.Value()
		}
	}
	return payload, codec, sampleRate, nil
}

func buildXAISTTRequest(c *gin.Context, request dto.AudioRequest) (*bytes.Buffer, string, error) {
	switch strings.ToLower(strings.TrimSpace(request.ResponseFormat)) {
	case "", "json", "text", "verbose_json":
	default:
		return nil, "", badAudioRequest("response_format %q is not supported by grok stt, use json, text or verbose_json", request.ResponseFormat)
	}

	formData, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, "", fmt.Errorf("error parsing multipart form: %w", err)
	}
	fileHeaders := formData.File["file"]
	if len(fileHeaders) == 0 && len(formData.Value["url"]) == 0 {
		return nil, "", badAudioRequest("file is required")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, values := range formData.Value {
		if _, ok := xaiSTTPassthroughFields[key]; !ok {
			continue
		}
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				return nil, "", err
			}
		}
	}
	// xAI 要求 file 必须是表单最后一个字段。
	if len(fileHeaders) > 0 {
		fileHeader := fileHeaders[0]
		file, err := fileHeader.Open()
		if err != nil {
			return nil, "", fmt.Errorf("error opening audio file: %w", err)
		}
		defer file.Close()
		part, err := writer.CreateFormFile("file", fileHeader.Filename)
		if err != nil {
			return nil, "", errors.New("create form file failed")
		}
		if _, err := io.Copy(part, file); err != nil {
			return nil, "", errors.New("copy file failed")
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return &body, writer.FormDataContentType(), nil
}

// audioTokensForSeconds 与 new-api 现有语音约定一致：每分钟 1000 tokens，向上取整到秒。
func audioTokensForSeconds(seconds float64) int {
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0
	}
	return common.QuotaRound(math.Ceil(seconds) / 60.0 * 1000)
}

func ttsAudioSeconds(c *gin.Context, audio []byte, codec string, sampleRate int) float64 {
	switch codec {
	case "pcm", "mulaw", "alaw":
		bytesPerSample := 1
		if codec == "pcm" {
			bytesPerSample = 2
		}
		if sampleRate <= 0 {
			sampleRate = 24000
		}
		return float64(len(audio)) / float64(sampleRate*bytesPerSample)
	case "wav":
		// xAI 的 wav 头按流式写法填了占位长度，GetAudioDuration 会读出几万秒；
		// 改用头里的 byte rate 和实际字节数计算。
		if len(audio) < 44 || string(audio[0:4]) != "RIFF" || string(audio[8:12]) != "WAVE" {
			return 0
		}
		byteRate := binary.LittleEndian.Uint32(audio[28:32])
		if byteRate == 0 {
			return 0
		}
		return float64(len(audio)-44) / float64(byteRate)
	case "mp3":
		seconds, err := common.GetAudioDuration(c.Request.Context(), bytes.NewReader(audio), ".mp3")
		if err != nil {
			logger.LogWarn(c, common.LogText("failed to get grok tts audio duration: %v", err))
			return 0
		}
		return seconds
	}
	return 0
}

// handleXAITTSResponse 把 xAI 返回的音频按 OpenAI /v1/audio/speech 的方式写回。
// 计费：prompt_tokens = 输入字符数（xAI 按字符计价），completion 音频 tokens 仅作记录。
func handleXAITTSResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, codec string, sampleRate int) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	audio := body
	contentType := resp.Header.Get("Content-Type")
	seconds := 0.0
	// 官方文档写的是 JSON（base64 audio），实际默认直接返回音频；两种都兼容。
	if strings.Contains(strings.ToLower(contentType), "json") {
		encoded := gjson.GetBytes(body, "audio").String()
		if encoded == "" {
			return nil, types.NewOpenAIError(errors.New("grok tts response has no audio"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		}
		audio, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, types.NewOpenAIError(fmt.Errorf("decode grok tts audio: %w", err), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		}
		contentType = gjson.GetBytes(body, "content_type").String()
		seconds = gjson.GetBytes(body, "duration").Float()
	}
	if len(audio) == 0 {
		return nil, types.NewOpenAIError(errors.New("grok tts returned empty audio"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Data(http.StatusOK, contentType, audio)

	chars := 0
	if audioReq, ok := info.Request.(*dto.AudioRequest); ok {
		chars = utf8.RuneCountInString(audioReq.Input)
	}
	if chars == 0 {
		chars = info.GetEstimatePromptTokens()
	}
	if seconds <= 0 {
		seconds = ttsAudioSeconds(c, audio, codec, sampleRate)
	}
	usage := &dto.Usage{PromptTokens: chars}
	usage.PromptTokensDetails.TextTokens = chars
	if audioTokens := audioTokensForSeconds(seconds); audioTokens > 0 {
		usage.CompletionTokens = audioTokens
		usage.CompletionTokenDetails.AudioTokens = audioTokens
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return usage, nil
}

type openAIVerboseWord struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	// Speaker 是 diarize=true 时 xAI 返回的说话人序号（从 0 开始）；上游没给时省略。
	Speaker *int64 `json:"speaker,omitempty"`
}

// handleXAISTTResponse 把 xAI STT 结果转成 OpenAI transcription 格式。
// 计费：prompt_tokens = 音频时长（每分钟 1000 tokens，与 new-api 预估口径一致）。
func handleXAISTTResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, responseFormat string) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	if !gjson.ValidBytes(body) || !gjson.GetBytes(body, "text").Exists() {
		return nil, types.NewOpenAIError(errors.New("invalid grok stt response"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	text := gjson.GetBytes(body, "text").String()
	language := gjson.GetBytes(body, "language").String()
	duration := gjson.GetBytes(body, "duration").Float()

	switch strings.ToLower(strings.TrimSpace(responseFormat)) {
	case "text":
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(text))
	case "verbose_json":
		words := make([]openAIVerboseWord, 0)
		for _, w := range gjson.GetBytes(body, "words").Array() {
			word := openAIVerboseWord{
				Word:  w.Get("text").String(),
				Start: w.Get("start").Float(),
				End:   w.Get("end").Float(),
			}
			if speaker := w.Get("speaker"); speaker.Type == gjson.Number {
				index := speaker.Int()
				word.Speaker = &index
			}
			words = append(words, word)
		}
		out := map[string]any{
			"task":     "transcribe",
			"language": language,
			"duration": duration,
			"text":     text,
			"words":    words,
		}
		if channels := gjson.GetBytes(body, "channels"); channels.Exists() {
			out["channels"] = channels.Value()
		}
		data, _ := common.Marshal(out)
		c.Data(http.StatusOK, "application/json", data)
	default:
		data, _ := common.Marshal(dto.AudioResponse{Text: text})
		c.Data(http.StatusOK, "application/json", data)
	}

	promptTokens := audioTokensForSeconds(duration)
	if promptTokens == 0 {
		promptTokens = info.GetEstimatePromptTokens()
	}
	usage := &dto.Usage{PromptTokens: promptTokens, TotalTokens: promptTokens}
	return usage, nil
}

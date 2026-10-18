package xai

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// xAI 的实时语音（wss /v1/realtime）和流式 TTS（wss /v1/tts）上游都不回用量：
// 实时语音按会话时长计费（每分钟 1000 tokens，与 grok-stt 同口径），
// 流式 TTS 按客户端 text.delta 的字符数计费（每字符 1 token，与 grok-tts 同口径），只计上游产出了音频的段落。
const (
	xaiRealtimePath        = "/v1/realtime"
	xaiRealtimeModelPrefix = "grok-voice"

	// 官方每条文字输入另收 $0.004，相当于 $0.08/分钟 的 0.05 分钟，即 50 tokens。
	xaiRealtimeTextInputTokens = 50

	// 实时会话先预留 1 分钟，之后每 30 秒续留到「已用时长 + 1 分钟」，余额不足时断开。
	xaiRealtimeReserveAhead    = time.Minute
	xaiRealtimeReserveInterval = 30 * time.Second
)

// 透传给上游的连接参数；其余参数（含可能的密钥）不转发。
var (
	xaiRealtimeQueryFields  = []string{"conversation_id", "reasoning.effort"}
	xaiTTSStreamQueryFields = []string{"voice", "language", "codec", "sample_rate", "bit_rate", "optimize_streaming_latency", "speed", "text_normalization", "with_timestamps"}
)

func isXAIRealtimeModel(model string) bool {
	return strings.HasPrefix(model, xaiRealtimeModelPrefix)
}

// xaiWebSocketURL 生成实时语音 / 流式 TTS 的上游地址，同时拒绝会按错误价格计费的模型。
func xaiWebSocketURL(info *relaycommon.RelayInfo) (string, error) {
	clientQuery := url.Values{}
	if requestURL, err := url.Parse(info.RequestURLPath); err == nil {
		clientQuery = requestURL.Query()
	}
	query := url.Values{}
	path := xaiRealtimePath
	fields := xaiRealtimeQueryFields
	if info.RelayMode == constant.RelayModeAudioSpeechStream {
		if info.UpstreamModelName != xaiTTSModel {
			return "", badAudioRequest("model %q does not support streaming tts, use %s", info.OriginModelName, xaiTTSModel)
		}
		path = xaiTTSPath
		fields = xaiTTSStreamQueryFields
		if err := ValidateTTSStreamQuery(clientQuery); err != nil {
			return "", err
		}
	} else {
		if !isXAIRealtimeModel(info.UpstreamModelName) {
			return "", badAudioRequest("model %q does not support realtime, use grok-voice-latest", info.OriginModelName)
		}
		query.Set("model", info.UpstreamModelName)
	}
	for _, field := range fields {
		if value := clientQuery.Get(field); value != "" {
			query.Set(field, value)
		}
	}
	if info.RelayMode == constant.RelayModeAudioSpeechStream {
		if query.Get("language") == "" {
			query.Set("language", xaiDefaultLanguage)
		}
		if voice, _ := normalizeXAIVoice(query.Get("voice"), "grok tts"); voice != "" {
			query.Set("voice", voice)
		}
	}
	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, path+"?"+query.Encode(), info.ChannelType), nil
}

// xaiRealtimeUsage 把实时会话时长和文字输入条数换算成计费 tokens。
func xaiRealtimeUsage(seconds float64, textInputs int) *dto.RealtimeUsage {
	audioTokens := audioTokensForSeconds(seconds)
	textTokens := textInputs * xaiRealtimeTextInputTokens
	usage := &dto.RealtimeUsage{
		InputTokens: audioTokens + textTokens,
		TotalTokens: audioTokens + textTokens,
	}
	usage.InputTokenDetails.AudioTokens = audioTokens
	usage.InputTokenDetails.TextTokens = textTokens
	return usage
}

func xaiTTSStreamUsage(chars int) *dto.RealtimeUsage {
	usage := &dto.RealtimeUsage{InputTokens: chars, TotalTokens: chars}
	usage.InputTokenDetails.TextTokens = chars
	return usage
}

// isXAIRealtimeTextInput 判断客户端事件是否为一条用户文字输入（官方按条另计费）。
func isXAIRealtimeTextInput(message []byte) bool {
	if gjson.GetBytes(message, "type").String() != "conversation.item.create" {
		return false
	}
	item := gjson.GetBytes(message, "item")
	if item.Get("type").String() != "message" || item.Get("role").String() != "user" {
		return false
	}
	for _, part := range item.Get("content").Array() {
		if part.Get("type").String() == "input_text" {
			return true
		}
	}
	return false
}

// reserveXAIRealtimeStart 在连接上游之前预留实时会话的第一分钟。
func reserveXAIRealtimeStart(info *relaycommon.RelayInfo) error {
	if err := service.ReserveWssQuota(info, xaiRealtimeUsage(xaiRealtimeReserveAhead.Seconds(), 0)); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	return nil
}

// xaiTTSUtterance 是流式 TTS 的一段文字（text.delta … text.done）。上游按顺序合成，
// 收到 audio.delta 才算合成成功；音色不存在等失败时上游只回 error，这段不计费。
type xaiTTSUtterance struct {
	chars int  // 尚未计费的字符数
	done  bool // 客户端已发 text.done
	audio bool // 上游已开始返回这段的音频
}

// xaiWebSocketSession 原样转发两端的帧（保留文本 / 二进制类型），只读取计费需要的事件；
// 实时语音的 session.update 若带了 xAI 不认的音色，直接回 error，不转发。
type xaiWebSocketSession struct {
	c         *gin.Context
	info      *relaycommon.RelayInfo
	ttsStream bool
	start     time.Time

	clientMu sync.Mutex // 两个转发协程和续留计时器都会写客户端

	mu          sync.Mutex
	textInputs  int
	chars       int               // 流式 TTS 已发给上游的字符数，用于续留
	billedChars int               // 流式 TTS 上游已产出音频的字符数，用于结算
	utterances  []xaiTTSUtterance // 流式 TTS 还没结束的段落，队首是上游正在合成的一段

	done     chan struct{}
	doneOnce sync.Once
}

func (s *xaiWebSocketSession) finish() {
	s.doneOnce.Do(func() { close(s.done) })
}

// usage 返回截至 now 的用量，已建立的实时会话至少按 1 秒计。reserve 为 true 时用于续留：
// 实时会话多算 1 分钟，流式 TTS 按已发给上游的全部字符；结算时流式 TTS 只计产出了音频的字符。
func (s *xaiWebSocketSession) usage(now time.Time, reserve bool) *dto.RealtimeUsage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ttsStream {
		if reserve {
			return xaiTTSStreamUsage(s.chars)
		}
		return xaiTTSStreamUsage(s.billedChars)
	}
	elapsed := now.Sub(s.start)
	if reserve {
		elapsed += xaiRealtimeReserveAhead
	}
	return xaiRealtimeUsage(max(elapsed.Seconds(), 1), s.textInputs)
}

func (s *xaiWebSocketSession) add(chars, textInputs int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chars += chars
	s.textInputs += textInputs
}

// trackClientTTS 把客户端 text.delta / text.done 记到对应段落；已在出音频的段落新增的字符直接计费。
func (s *xaiWebSocketSession) trackClientTTS(eventType string, chars int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	last := len(s.utterances) - 1
	switch eventType {
	case "text.delta":
		if chars <= 0 {
			return
		}
		if last < 0 || s.utterances[last].done {
			s.utterances = append(s.utterances, xaiTTSUtterance{})
			last++
		}
		if s.utterances[last].audio {
			s.billedChars += chars
		} else {
			s.utterances[last].chars += chars
		}
	case "text.done":
		if last >= 0 {
			s.utterances[last].done = true
		}
	}
}

// trackUpstreamTTS 按上游事件结算队首段落：audio.delta 表示合成成功，计入这段字符；
// audio.done / audio.clear 结束这段；error 时队首若已发完文字却没有音频，视为合成失败，不计费。
func (s *xaiWebSocketSession) trackUpstreamTTS(eventType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.utterances) == 0 {
		return
	}
	head := &s.utterances[0]
	switch eventType {
	case "audio.delta":
		if !head.audio {
			head.audio = true
			s.billedChars += head.chars
			head.chars = 0
		}
	case "audio.done", "audio.clear":
		s.utterances = s.utterances[1:]
	case "error":
		if head.done && !head.audio {
			s.utterances = s.utterances[1:]
		}
	}
}

func (s *xaiWebSocketSession) writeClient(messageType int, data []byte) error {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	return s.info.ClientWs.WriteMessage(messageType, data)
}

// rejectVoice 告知客户端 session.update 里的音色不可用；这条消息不转发给上游，会话继续。
func (s *xaiWebSocketSession) rejectVoice(err error) {
	event := dto.RealtimeEvent{
		Type:    "error",
		EventId: helper.GetLocalRealtimeID(s.c),
		Error: &types.OpenAIError{
			Message: err.Error(),
			Type:    "invalid_request_error",
			Code:    "invalid_value",
			Param:   "session.voice",
		},
	}
	if data, marshalErr := common.Marshal(event); marshalErr == nil {
		_ = s.writeClient(websocket.TextMessage, data)
	}
}

// rejectForQuota 告知客户端余额不足并结束会话。
func (s *xaiWebSocketSession) rejectForQuota(err error) {
	logger.LogWarn(s.c, common.LogText("xai websocket session stopped: %s", err.Error()))
	event := dto.RealtimeEvent{
		Type:    "error",
		EventId: helper.GetLocalRealtimeID(s.c),
		Error: &types.OpenAIError{
			Message: err.Error(),
			Type:    "insufficient_quota",
			Code:    string(types.ErrorCodeInsufficientUserQuota),
		},
	}
	if data, marshalErr := common.Marshal(event); marshalErr == nil {
		_ = s.writeClient(websocket.TextMessage, data)
	}
	deadline := time.Now().Add(time.Second)
	_ = s.info.ClientWs.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "insufficient quota"), deadline)
	_ = s.info.TargetWs.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), deadline)
	s.finish()
}

// forwardClose 把一端的关闭帧转给另一端；1005/1006 不能出现在关闭帧里，按正常关闭处理。
func forwardClose(err error, conn *websocket.Conn) {
	code, text := websocket.CloseNormalClosure, ""
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) && closeErr.Code != websocket.CloseNoStatusReceived && closeErr.Code != websocket.CloseAbnormalClosure {
		code, text = closeErr.Code, closeErr.Text
	}
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, text), time.Now().Add(time.Second))
}

func (s *xaiWebSocketSession) relayClient() {
	defer s.finish()
	for {
		messageType, message, err := s.info.ClientWs.ReadMessage()
		if err != nil {
			forwardClose(err, s.info.TargetWs)
			return
		}
		if messageType == websocket.TextMessage {
			eventType := gjson.GetBytes(message, "type").String()
			chars, textInputs := 0, 0
			if s.ttsStream {
				if eventType == "text.delta" {
					chars = utf8.RuneCountInString(gjson.GetBytes(message, "delta").String())
				}
			} else if eventType == "session.update" {
				// 上游对不认识的音色不报错、session.updated 也不回显音色，客户无从发现，这里先拦下。
				if voice := gjson.GetBytes(message, "session.voice"); voice.Type == gjson.String {
					normalized, err := normalizeXAIVoice(voice.String(), "grok voice")
					if err != nil {
						s.rejectVoice(err)
						continue
					}
					if normalized != voice.String() {
						if rewritten, setErr := sjson.SetBytes(message, "session.voice", normalized); setErr == nil {
							message = rewritten
						}
					}
				}
			} else if isXAIRealtimeTextInput(message) {
				textInputs = 1
			}
			// 先按加上这条消息的用量续留；余额不足时这条消息不计费，也不发给上游。
			if chars > 0 || textInputs > 0 {
				s.add(chars, textInputs)
				if err := service.ReserveWssQuota(s.info, s.usage(time.Now(), true)); err != nil {
					s.add(-chars, -textInputs)
					s.rejectForQuota(err)
					return
				}
			}
			if s.ttsStream {
				s.trackClientTTS(eventType, chars)
			}
		}
		if err := s.info.TargetWs.WriteMessage(messageType, message); err != nil {
			logger.LogError(s.c, common.LogText("error writing to xai upstream: %s", err.Error()))
			return
		}
	}
}

func (s *xaiWebSocketSession) relayTarget() {
	defer s.finish()
	for {
		messageType, message, err := s.info.TargetWs.ReadMessage()
		if err != nil {
			forwardClose(err, s.info.ClientWs)
			return
		}
		s.info.SetFirstResponseTime()
		if s.ttsStream && messageType == websocket.TextMessage {
			s.trackUpstreamTTS(gjson.GetBytes(message, "type").String())
		}
		if err := s.writeClient(messageType, message); err != nil {
			logger.LogError(s.c, common.LogText("error writing to client: %s", err.Error()))
			return
		}
	}
}

// xaiWebSocketHandler 转发一次实时语音或流式 TTS 会话，返回会话结束时的计费用量。
func xaiWebSocketHandler(c *gin.Context, info *relaycommon.RelayInfo) (*dto.RealtimeUsage, *types.NewAPIError) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return nil, types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse)
	}
	info.IsStream = true
	session := &xaiWebSocketSession{
		c:         c,
		info:      info,
		ttsStream: info.RelayMode == constant.RelayModeAudioSpeechStream,
		start:     time.Now(),
		done:      make(chan struct{}),
	}
	go session.relayClient()
	go session.relayTarget()

	var ticker <-chan time.Time
	if !session.ttsStream {
		t := time.NewTicker(xaiRealtimeReserveInterval)
		defer t.Stop()
		ticker = t.C
	}
	for running := true; running; {
		select {
		case <-session.done:
			running = false
		case <-c.Done():
			running = false
		case now := <-ticker:
			if err := service.ReserveWssQuota(info, session.usage(now, true)); err != nil {
				session.rejectForQuota(err)
			}
		}
	}
	return session.usage(time.Now(), false), nil
}

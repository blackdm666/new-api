package xai

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
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
)

// xAI 的实时语音（wss /v1/realtime）和流式 TTS（wss /v1/tts）上游都不回用量：
// 实时语音按会话时长计费（每分钟 1000 tokens，与 grok-stt 同口径），
// 流式 TTS 按客户端 text.delta 的字符数计费（每字符 1 token，与 grok-tts 同口径）。
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
		voice := strings.TrimSpace(clientQuery.Get("voice"))
		if slices.Contains(openAIBuiltinVoices, strings.ToLower(voice)) {
			return "", openAIVoiceError(voice)
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
	if info.RelayMode == constant.RelayModeAudioSpeechStream && query.Get("language") == "" {
		query.Set("language", xaiDefaultLanguage)
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

// xaiWebSocketSession 原样转发两端的帧（保留文本 / 二进制类型），只读取计费需要的事件。
type xaiWebSocketSession struct {
	c         *gin.Context
	info      *relaycommon.RelayInfo
	ttsStream bool
	start     time.Time

	clientMu sync.Mutex // 两个转发协程和续留计时器都会写客户端

	mu         sync.Mutex
	textInputs int
	chars      int

	done     chan struct{}
	doneOnce sync.Once
}

func (s *xaiWebSocketSession) finish() {
	s.doneOnce.Do(func() { close(s.done) })
}

// usage 返回截至 now 的计费用量；实时会话额外计入 ahead 的预留时长，已建立的会话至少按 1 秒计。
func (s *xaiWebSocketSession) usage(now time.Time, ahead time.Duration) *dto.RealtimeUsage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ttsStream {
		return xaiTTSStreamUsage(s.chars)
	}
	return xaiRealtimeUsage(max((now.Sub(s.start)+ahead).Seconds(), 1), s.textInputs)
}

func (s *xaiWebSocketSession) add(chars, textInputs int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chars += chars
	s.textInputs += textInputs
}

func (s *xaiWebSocketSession) writeClient(messageType int, data []byte) error {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	return s.info.ClientWs.WriteMessage(messageType, data)
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
			chars, textInputs := 0, 0
			if s.ttsStream {
				if gjson.GetBytes(message, "type").String() == "text.delta" {
					chars = utf8.RuneCountInString(gjson.GetBytes(message, "delta").String())
				}
			} else if isXAIRealtimeTextInput(message) {
				textInputs = 1
			}
			// 先按加上这条消息的用量续留；余额不足时这条消息不计费，也不发给上游。
			if chars > 0 || textInputs > 0 {
				s.add(chars, textInputs)
				if err := service.ReserveWssQuota(s.info, s.usage(time.Now(), s.reserveAhead())); err != nil {
					s.add(-chars, -textInputs)
					s.rejectForQuota(err)
					return
				}
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
		if err := s.writeClient(messageType, message); err != nil {
			logger.LogError(s.c, common.LogText("error writing to client: %s", err.Error()))
			return
		}
	}
}

func (s *xaiWebSocketSession) reserveAhead() time.Duration {
	if s.ttsStream {
		return 0
	}
	return xaiRealtimeReserveAhead
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
			if err := service.ReserveWssQuota(info, session.usage(now, xaiRealtimeReserveAhead)); err != nil {
				session.rejectForQuota(err)
			}
		}
	}
	return session.usage(time.Now(), 0), nil
}

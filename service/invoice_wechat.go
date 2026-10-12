package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/shopspring/decimal"
)

const (
	invoiceWechatMaxAttempts  = 3
	invoiceWechatSendBudget   = 2 * time.Minute
	invoiceWechatResponseSize = 64 * 1024
)

var (
	// Overridden in tests; production waits between attempts that never
	// reached the provider.
	invoiceWechatHTTPTimeout  = 15 * time.Second
	invoiceWechatRetryBackoff = []time.Duration{2 * time.Second, 5 * time.Second}

	// China has no daylight saving time, so a fixed offset matches
	// Asia/Shanghai without depending on the host's tzdata.
	invoiceWechatLocation = time.FixedZone("Asia/Shanghai", 8*3600)

	invoiceWechatWake = make(chan struct{}, 1)
)

// ErrInvoiceWechatNotConfigured is returned when the finance group settings
// are incomplete.
var ErrInvoiceWechatNotConfigured = errors.New("invoice wechat notification is not configured")

type invoiceWechatConfig struct {
	BaseURL string
	Token   string
	AppID   string
	GroupID string
}

func currentInvoiceWechatConfig() (invoiceWechatConfig, error) {
	if !setting.InvoiceWechatConfigured() {
		return invoiceWechatConfig{}, ErrInvoiceWechatNotConfigured
	}
	return invoiceWechatConfig{
		BaseURL: strings.TrimSpace(setting.InvoiceWechatApiBaseUrl),
		Token:   strings.TrimSpace(setting.InvoiceWechatApiToken),
		AppID:   strings.TrimSpace(setting.InvoiceWechatAppId),
		GroupID: strings.TrimSpace(setting.InvoiceWechatGroupId),
	}, nil
}

// formatInvoiceAmountCNY renders cents as 1,234.56.
func formatInvoiceAmountCNY(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	yuan := strconv.FormatInt(cents/100, 10)
	var builder strings.Builder
	builder.WriteString(sign)
	for index, digit := range yuan {
		if index > 0 && (len(yuan)-index)%3 == 0 {
			builder.WriteByte(',')
		}
		builder.WriteRune(digit)
	}
	fmt.Fprintf(&builder, ".%02d", cents%100)
	return builder.String()
}

func invoiceTotalCents(request *model.InvoiceRequest) int64 {
	if request.TotalMoneyCents > 0 || request.TotalMoney <= 0 {
		return request.TotalMoneyCents
	}
	return decimal.NewFromFloat(request.TotalMoney).Mul(decimal.NewFromInt(100)).Round(0).IntPart()
}

func formatInvoiceWechatTime(timestamp int64) string {
	return time.Unix(timestamp, 0).In(invoiceWechatLocation).Format("2006-01-02 15:04")
}

// invoiceWechatLines joins label/value pairs into message lines, leaving out
// every line whose value is empty.
func invoiceWechatLines(header string, fields [][2]string) string {
	lines := make([]string, 0, len(fields)+1)
	lines = append(lines, header)
	for _, field := range fields {
		value := strings.TrimSpace(field[1])
		if value == "" {
			continue
		}
		lines = append(lines, field[0]+"："+value)
	}
	return strings.Join(lines, "\n")
}

func buildInvoiceWechatCreatedText(request *model.InvoiceRequest) string {
	applicant := fmt.Sprintf("ID %d", request.UserId)
	if username := strings.TrimSpace(request.Username); username != "" {
		applicant = fmt.Sprintf("%s（ID %d）", username, request.UserId)
	}
	link := ""
	if base := strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/"); base != "" {
		link = fmt.Sprintf("%s/admin-invoices/%d", base, request.Id)
	}
	return invoiceWechatLines(fmt.Sprintf("【新开票申请 #%d】", request.Id), [][2]string{
		{"发票抬头", request.CompanyName},
		{"税号", request.TaxNumber},
		{"开户银行", request.BankName},
		{"银行账号", request.BankAccount},
		{"地址", request.CompanyAddress},
		{"电话", request.CompanyPhone},
		{"备注", request.Remark},
		{"开票金额", "¥" + formatInvoiceAmountCNY(invoiceTotalCents(request))},
		{"申请人", applicant},
		{"申请时间", formatInvoiceWechatTime(request.CreatedTime)},
		{"处理链接", link},
	})
}

func buildInvoiceWechatWithdrawnText(request *model.InvoiceRequest) string {
	text := invoiceWechatLines(fmt.Sprintf("【开票申请作废 #%d】", request.Id), [][2]string{
		{"发票抬头", request.CompanyName},
		{"开票金额", "¥" + formatInvoiceAmountCNY(invoiceTotalCents(request))},
	})
	return text + fmt.Sprintf("\n用户已于 %s 撤回该申请，请勿开票。", formatInvoiceWechatTime(request.UpdatedTime))
}

func newInvoiceWechatDelivery(request *model.InvoiceRequest, event string, subject string, body string) *model.InvoiceNotificationDelivery {
	groupID := strings.TrimSpace(setting.InvoiceWechatGroupId)
	seed := fmt.Sprintf("invoice:%d:%s:wechat:%s", request.Id, event, groupID)
	return &model.InvoiceNotificationDelivery{
		DeliveryKey:      hex.EncodeToString(common.Sha256Raw([]byte(seed))),
		InvoiceRequestId: request.Id,
		Kind:             model.InvoiceNotificationKindWechatGroup,
		UserId:           request.UserId,
		Recipient:        groupID,
		Subject:          subject,
		Body:             body,
	}
}

func invoiceWechatNotifyReady() bool {
	return setting.InvoiceWechatNotifyEnabled && setting.InvoiceWechatConfigured()
}

// BuildInvoiceWithdrawnNotifications tells the finance group that a pending
// request was withdrawn and must not be invoiced.
func BuildInvoiceWithdrawnNotifications(request *model.InvoiceRequest) ([]*model.InvoiceNotificationDelivery, error) {
	if request == nil || request.Status != model.InvoiceStatusWithdrawn || !invoiceWechatNotifyReady() {
		return nil, nil
	}
	return []*model.InvoiceNotificationDelivery{newInvoiceWechatDelivery(
		request, "withdrawn", fmt.Sprintf("开票申请作废 #%d", request.Id), buildInvoiceWechatWithdrawnText(request),
	)}, nil
}

// WakeInvoiceWechatDelivery asks the delivery worker to send queued WeChat
// messages now instead of at its next tick. It never blocks.
func WakeInvoiceWechatDelivery() {
	select {
	case invoiceWechatWake <- struct{}{}:
	default:
	}
}

// deliverPendingInvoiceWechatNotifications sends queued messages while the
// finance group notification is enabled and fully configured. Otherwise it
// leaves every message untouched, so messages queued before the switch was
// turned off are sent once it is turned on again.
func deliverPendingInvoiceWechatNotifications() {
	if !invoiceWechatNotifyReady() {
		return
	}
	now := common.GetTimestamp()
	interrupted, err := model.FailInterruptedInvoiceWechatNotifications(now)
	if err != nil {
		common.SysError("failed to fail interrupted invoice wechat notifications: " + err.Error())
	} else if interrupted > 0 {
		common.SysError(fmt.Sprintf("%d invoice wechat notification(s) were interrupted while sending and need a manual retry", interrupted))
	}
	deliveries, err := model.ListPendingInvoiceWechatNotifications(50, now)
	if err != nil {
		common.SysError("failed to list pending invoice wechat notifications: " + err.Error())
		return
	}
	for _, delivery := range deliveries {
		// Re-read the settings for every message so turning the switch off
		// stops the batch.
		config, err := currentInvoiceWechatConfig()
		if err != nil || !setting.InvoiceWechatNotifyEnabled {
			return
		}
		deliverInvoiceWechatNotification(delivery, config)
	}
}

// deliverInvoiceWechatNotification sends one queued message and records the
// final outcome. The claim marks the message in flight before anything is
// posted, so it is never sent twice automatically: a message that was not
// accepted, or whose result was never recorded, is marked failed and is only
// sent again when an administrator retries it, because the provider may still
// have delivered it.
func deliverInvoiceWechatNotification(delivery *model.InvoiceNotificationDelivery, config invoiceWechatConfig) {
	now := common.GetTimestamp()
	claimed, err := model.ClaimInvoiceWechatNotification(delivery.Id, now, now+int64((invoiceWechatSendBudget+time.Minute)/time.Second))
	if err != nil || !claimed {
		if err != nil {
			common.SysError(fmt.Sprintf("failed to claim invoice wechat notification %d: %s", delivery.Id, err.Error()))
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), invoiceWechatSendBudget)
	attempts, err := sendInvoiceWechatText(ctx, config, delivery.Recipient, delivery.Body)
	cancel()
	failure := ""
	if err != nil {
		failure = err.Error()
		common.SysError(fmt.Sprintf("invoice wechat notification %d failed after %d attempt(s): %s", delivery.Id, attempts, failure))
	}
	if recordErr := model.RecordInvoiceWechatNotificationResult(delivery.Id, now, delivery.Attempts+attempts, failure); recordErr != nil {
		common.SysError(fmt.Sprintf("failed to record invoice wechat notification %d: %s", delivery.Id, recordErr.Error()))
	}
}

// SendInvoiceWechatTestMessage posts a timestamped test message to the
// configured group with the saved settings.
func SendInvoiceWechatTestMessage(ctx context.Context) error {
	config, err := currentInvoiceWechatConfig()
	if err != nil {
		return err
	}
	content := fmt.Sprintf("88API 开票通知测试（%s）", time.Now().In(invoiceWechatLocation).Format("2006-01-02 15:04:05"))
	_, err = sendInvoiceWechatText(ctx, config, config.GroupID, content)
	return err
}

type invoiceWechatResponse struct {
	Ret int    `json:"ret"`
	Msg string `json:"msg"`
}

// sendInvoiceWechatText posts a text message and returns the number of HTTP
// attempts. It retries only when the request never reached the provider
// (DNS, connect or TLS failures). Once the request was written, a timeout or
// any response other than HTTP 200 with ret 200 is final, so the group never
// receives the same message twice. The token is removed from returned errors.
func sendInvoiceWechatText(ctx context.Context, config invoiceWechatConfig, groupID string, content string) (int, error) {
	payload, err := common.Marshal(map[string]string{
		"appId":   config.AppID,
		"toWxid":  groupID,
		"content": content,
	})
	if err != nil {
		return 0, err
	}
	endpoint := strings.TrimRight(config.BaseURL, "/") + "/message/postText"
	client := &http.Client{Timeout: invoiceWechatHTTPTimeout}
	attempts := 0
	for {
		attempts++
		requestSent, sendErr := postInvoiceWechatText(ctx, client, endpoint, config.Token, payload)
		if sendErr == nil {
			return attempts, nil
		}
		if config.Token != "" {
			sendErr = errors.New(strings.ReplaceAll(sendErr.Error(), config.Token, "[redacted]"))
		}
		if requestSent || attempts >= invoiceWechatMaxAttempts {
			return attempts, sendErr
		}
		var delay time.Duration
		if len(invoiceWechatRetryBackoff) > 0 {
			delay = invoiceWechatRetryBackoff[min(attempts-1, len(invoiceWechatRetryBackoff)-1)]
		}
		select {
		case <-ctx.Done():
			return attempts, sendErr
		case <-time.After(delay):
		}
	}
}

// postInvoiceWechatText performs one HTTP attempt. requestSent reports whether
// the request was fully written, after which the provider may have acted on it.
func postInvoiceWechatText(ctx context.Context, client *http.Client, endpoint string, token string, payload []byte) (bool, error) {
	var wrote atomic.Bool
	trace := &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				wrote.Store(true)
			}
		},
	}
	request, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("wechat-token", token)
	response, err := client.Do(request)
	if err != nil {
		return wrote.Load(), err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, invoiceWechatResponseSize))
	if err != nil {
		return true, fmt.Errorf("read wechat api response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		snippet := []rune(strings.TrimSpace(string(body)))
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return true, fmt.Errorf("wechat api returned HTTP %d: %s", response.StatusCode, string(snippet))
	}
	var result invoiceWechatResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return true, errors.New("wechat api returned an invalid response")
	}
	if result.Ret != http.StatusOK {
		message := strings.TrimSpace(result.Msg)
		if message == "" {
			message = fmt.Sprintf("ret %d", result.Ret)
		}
		return true, fmt.Errorf("wechat api rejected the message: %s", message)
	}
	return true, nil
}

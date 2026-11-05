package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicefile"
	"github.com/QuantumNous/new-api/setting"
	"github.com/ledongthuc/pdf"
	"github.com/shopspring/decimal"
	"golang.org/x/text/width"
	"gorm.io/gorm"
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

// invoiceWechatRegionLabel names the kind of invoice the finance group has to
// issue.
func invoiceWechatRegionLabel(region string) string {
	if region == model.InvoiceRegionHongKong {
		return "帳單 INVOICE（中国香港）"
	}
	return "增值税电子普通发票（中国大陆）"
}

func buildInvoiceWechatCreatedText(request *model.InvoiceRequest) string {
	return invoiceWechatLines(fmt.Sprintf("【新开票申请 #%d】", request.Id), [][2]string{
		{"开票类型", invoiceWechatRegionLabel(request.Region)},
		{"发票抬头", request.CompanyName},
		{"税号", request.TaxNumber},
		{"开户银行", request.BankName},
		{"银行账号", request.BankAccount},
		{"地址", request.CompanyAddress},
		{"电话", request.CompanyPhone},
		{"备注", request.Remark},
		{"开票金额", "¥" + formatInvoiceAmountCNY(invoiceTotalCents(request))},
	})
}

func buildInvoiceWechatWithdrawnText(request *model.InvoiceRequest) string {
	text := invoiceWechatLines(fmt.Sprintf("【开票请求作废 #%d】", request.Id), [][2]string{
		{"发票抬头", request.CompanyName},
		{"开票金额", "¥" + formatInvoiceAmountCNY(invoiceTotalCents(request))},
	})
	return text + fmt.Sprintf("\n用户已于 %s 撤回该申请，请勿开票。", formatInvoiceWechatTime(request.UpdatedTime))
}

// buildInvoiceWechatIssuedText confirms in the finance group that an invoice
// PDF posted there completed its request.
func buildInvoiceWechatIssuedText(request *model.InvoiceRequest, fileName string) string {
	return invoiceWechatLines(fmt.Sprintf("【开票工单已完成 #%d】", request.Id), [][2]string{
		{"发票抬头", request.CompanyName},
		{"开票金额", "¥" + formatInvoiceAmountCNY(invoiceTotalCents(request))},
		{"发票文件", fileName},
	})
}

// buildInvoiceWechatAutoBilledText tells the finance group that the system
// generated and sent a Hong Kong bill, so nothing is left to issue.
func buildInvoiceWechatAutoBilledText(request *model.InvoiceRequest, fileName string) string {
	text := invoiceWechatLines(fmt.Sprintf("【帳單已自动开具 #%d】", request.Id), [][2]string{
		{"开票类型", invoiceWechatRegionLabel(request.Region)},
		{"收件人", request.CompanyName},
		{"开票金额", "¥" + formatInvoiceAmountCNY(invoiceTotalCents(request))},
		{"帳單文件", fileName},
	})
	return text + "\n系统已生成帳單并发给用户，无需人工开具。"
}

// invoiceWechatRepostWindowSeconds is how long after an invoice PDF issued a
// request the same file is taken as the bot posting it again, not as a resend
// by the finance group.
const invoiceWechatRepostWindowSeconds = 10 * 60

// invoiceWechatPendingList lists pending requests for the finance group, at
// most five of them.
func invoiceWechatPendingList(requests []*model.InvoiceRequest) string {
	const shown = 5
	items := make([]string, 0, shown)
	for _, request := range requests[:min(len(requests), shown)] {
		items = append(items, fmt.Sprintf("#%d ¥%s（%s 提交）", request.Id,
			formatInvoiceAmountCNY(invoiceTotalCents(request)),
			time.Unix(request.CreatedTime, 0).In(invoiceWechatLocation).Format("01-02")))
	}
	list := strings.Join(items, "、")
	if len(requests) > shown {
		list += fmt.Sprintf(" 等 %d 张", len(requests))
	}
	return list
}

func buildInvoiceWechatDuplicateText(holder *model.InvoiceRequest, invoiceNumber string, fileName string) string {
	field := [2]string{"发票号码", invoiceNumber}
	if invoiceNumber == "" {
		field = [2]string{"发票文件", fileName}
	}
	text := invoiceWechatLines("【重复发票，未处理】", [][2]string{field})
	target := fmt.Sprintf("#%d（%s ¥%s）", holder.Id, holder.CompanyName, formatInvoiceAmountCNY(invoiceTotalCents(holder)))
	if holder.Status == model.InvoiceStatusIssued && holder.IssuedTime > 0 {
		return text + fmt.Sprintf("\n这张发票已于 %s 开给 %s，本次不重复处理。", formatInvoiceWechatTime(holder.IssuedTime), target)
	}
	return text + fmt.Sprintf("\n这张发票已上传到 %s，本次不重复处理。", target)
}

func buildInvoiceWechatAmountMismatchText(pending []*model.InvoiceRequest, invoice invoicePDF) string {
	text := invoiceWechatLines("【未开票：金额对不上】", [][2]string{
		{"发票抬头", pending[0].CompanyName},
		{"发票号码", invoice.number},
		{"价税合计", "¥" + formatInvoiceAmountCNY(invoice.totalCents)},
		{"待开申请", invoiceWechatPendingList(pending)},
	})
	target := "这张申请"
	if len(pending) > 1 {
		target = "其中一张申请"
	}
	return text + fmt.Sprintf("\n没有金额相等的申请，未自动开票。线下付款的票无需处理；如果是开给%s的，请核对金额后重开。", target)
}

func buildInvoiceWechatUnreadableText(pending []*model.InvoiceRequest, fileName string) string {
	base := path.Base(strings.ReplaceAll(strings.TrimSpace(fileName), "\\", "/"))
	ext := path.Ext(base)
	example := fmt.Sprintf("%s_#%d%s", strings.TrimSuffix(base, ext), pending[0].Id, ext)
	text := invoiceWechatLines("【未开票：读不出发票内容】", [][2]string{{"发票文件", base}})
	return text + fmt.Sprintf("\n同一抬头有 %d 张待开申请：%s\n文件里读不到发票金额（可能是扫描件），请在文件名末尾加上申请号重新发送，例如：%s",
		len(pending), invoiceWechatPendingList(pending), example)
}

// enqueueInvoiceWechatFileReply tells the finance group why an invoice PDF it
// posted was not issued. The reply is filed under the request it is about and
// sent once per file.
func enqueueInvoiceWechatFileReply(request *model.InvoiceRequest, event string, sha256Value string, subject string, body string) error {
	delivery := newInvoiceWechatDelivery(request, event+":"+sha256Value, fmt.Sprintf("%s #%d", subject, request.Id), body)
	if _, _, err := model.EnqueueInvoiceNotification(delivery); err != nil {
		return err
	}
	WakeInvoiceWechatDelivery()
	return nil
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
		request, "withdrawn", fmt.Sprintf("开票请求作废 #%d", request.Id), buildInvoiceWechatWithdrawnText(request),
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

// Outcomes of an invoice PDF posted in the finance group.
const (
	InvoiceWechatFileIssued         = "issued"
	InvoiceWechatFileNoMatch        = "no_match"
	InvoiceWechatFileAmbiguous      = "ambiguous"
	InvoiceWechatFileAmountMismatch = "amount_mismatch"
	InvoiceWechatFileDuplicate      = "duplicate"
	InvoiceWechatFileNotPDF         = "not_pdf"
)

// InvoiceWechatFileResult reports what an invoice PDF posted in the finance
// group did. RequestId is set when a request was issued, or for a resend, the
// request that already holds the invoice.
type InvoiceWechatFileResult struct {
	Outcome   string `json:"outcome"`
	RequestId int    `json:"request_id,omitempty"`
}

var invoiceWechatFileRequestID = regexp.MustCompile(`#(\d+)`)

// invoiceShortCompanyNameLength is the length below which a company name, such
// as a person's name, must be the whole file name instead of part of it.
const invoiceShortCompanyNameLength = 4

// invoiceWechatFileName is an invoice PDF's file name prepared for matching
// company names: folded like invoiceCompanyMatchKey, with boundary[i] telling
// whether rune i starts the name or follows a space or a non-letter.
type invoiceWechatFileName struct {
	runes     []rune
	boundary  []bool
	requestId int
}

// parseInvoiceWechatFileName prepares a ".pdf" file name for matching and
// reads an optional "#<request id>" that picks one of several pending
// requests for the same company. ok is false for other file types.
func parseInvoiceWechatFileName(fileName string) (name invoiceWechatFileName, ok bool) {
	base := width.Fold.String(strings.TrimSpace(path.Base(strings.ReplaceAll(fileName, "\\", "/"))))
	ext := path.Ext(base)
	if !strings.EqualFold(ext, ".pdf") {
		return name, false
	}
	stem := strings.TrimSuffix(base, ext)
	if match := invoiceWechatFileRequestID.FindStringSubmatchIndex(stem); match != nil {
		name.requestId, _ = strconv.Atoi(stem[match[2]:match[3]])
		stem = stem[:match[0]] + " " + stem[match[1]:]
	}
	atBoundary := true
	for _, r := range stem {
		if unicode.IsSpace(r) {
			atBoundary = true
			continue
		}
		name.runes = append(name.runes, unicode.ToLower(r))
		name.boundary = append(name.boundary, atBoundary)
		atBoundary = !unicode.IsLetter(r)
	}
	return name, len(name.runes) > 0
}

// namesCompany reports whether the file name contains the company name right
// after its start, a space or a non-letter, so "云帆科技有限公司" is not found
// in a file named for "上海云帆科技有限公司". Anything may follow the name,
// such as "发票" or a date. Short names must be the whole file name.
func (name invoiceWechatFileName) namesCompany(companyName string) bool {
	company := []rune(invoiceCompanyMatchKey(companyName))
	if len(company) == 0 {
		return false
	}
	if len(company) < invoiceShortCompanyNameLength {
		return slices.Equal(name.runes, company)
	}
	for start := 0; start+len(company) <= len(name.runes); start++ {
		if name.boundary[start] && slices.Equal(name.runes[start:start+len(company)], company) {
			return true
		}
	}
	return false
}

// invoiceCompanyMatchKey folds the differences between a company name typed
// by an applicant and the same name in a file name: full-width characters,
// spaces and letter case.
func invoiceCompanyMatchKey(name string) string {
	var builder strings.Builder
	for _, r := range width.Fold.String(name) {
		if unicode.IsSpace(r) {
			continue
		}
		builder.WriteRune(unicode.ToLower(r))
	}
	return builder.String()
}

var (
	invoicePDFNumber    = regexp.MustCompile(`(?m)^\s*(\d{20})\s*$`)
	invoicePDFAmount    = regexp.MustCompile(`[¥￥]\s*(\d[\d,]*(?:\.\d{1,2})?)`)
	invoicePDFTaxNumber = regexp.MustCompile(`(?m)^\s*([0-9A-Z]{15,20})\s*$`)
)

// invoicePDF is what an e-invoice PDF says about itself.
type invoicePDF struct {
	number     string
	totalCents int64
	// taxNumbers holds every taxpayer number printed on the invoice: the
	// buyer's, when the buyer has one, and the seller's.
	taxNumbers []string
}

// namesTaxNumber reports whether the invoice carries the taxpayer number an
// applicant typed, ignoring spaces, letter case and full-width characters.
func (invoice invoicePDF) namesTaxNumber(taxNumber string) bool {
	key := strings.ToUpper(invoiceCompanyMatchKey(taxNumber))
	return key != "" && slices.Contains(invoice.taxNumbers, key)
}

// readInvoicePDF reads the 20-digit invoice number, the taxpayer numbers and
// the total including tax from a fully digitalized e-invoice (数电票) PDF. The
// total is its largest "¥" amount: line items carry no currency sign, and the
// amount and tax totals are smaller. ok is false when there is no amount, as
// in a scanned invoice.
func readInvoicePDF(content []byte) (invoice invoicePDF, ok bool) {
	defer func() {
		// The PDF reader panics on some malformed files.
		if recover() != nil {
			invoice, ok = invoicePDF{}, false
		}
	}()
	reader, err := pdf.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return invoice, false
	}
	plain, err := reader.GetPlainText()
	if err != nil {
		return invoice, false
	}
	text, err := io.ReadAll(plain)
	if err != nil {
		return invoice, false
	}
	if match := invoicePDFNumber.FindSubmatch(text); match != nil {
		invoice.number = string(match[1])
	}
	for _, match := range invoicePDFTaxNumber.FindAllSubmatch(text, -1) {
		if taxNumber := string(match[1]); taxNumber != invoice.number && !slices.Contains(invoice.taxNumbers, taxNumber) {
			invoice.taxNumbers = append(invoice.taxNumbers, taxNumber)
		}
	}
	for _, match := range invoicePDFAmount.FindAllSubmatch(text, -1) {
		amount, err := decimal.NewFromString(strings.ReplaceAll(string(match[1]), ",", ""))
		if err != nil {
			continue
		}
		invoice.totalCents = max(invoice.totalCents, amount.Shift(2).Round(0).IntPart())
	}
	return invoice, invoice.totalCents > 0
}

// InvoiceWechatAutoIssueReady reports whether invoice PDFs from the finance
// group can be issued: the switch is on, invoice files are enabled, and the
// group notification that announces the result is enabled and configured.
func InvoiceWechatAutoIssueReady() bool {
	return setting.InvoiceWechatAutoIssueEnabled && setting.InvoiceFileEnabled && invoiceWechatNotifyReady()
}

// IssueInvoiceFromWechatFile handles an invoice PDF posted in the finance
// group and attaches it to a pending request: the request is marked issued,
// the applicant is emailed as for a manual issue, and the group is told the
// order is complete.
//
// A pending request is a candidate when the invoice carries its taxpayer
// number or the file name contains its company name (a company name found
// only inside a longer matching one, such as a head office inside its branch,
// does not count). A "#<request id>" in the file name limits the candidates to
// that request and issues it whatever the amount.
//
// When the invoice total can be read, the oldest candidate with that amount is
// issued. Candidates of which none has the amount are left alone and the group
// is asked to check them; an invoice for a company with no pending request was
// paid offline and is ignored. When the total cannot be read, as in a scanned
// invoice, a single candidate is issued and several are left for the group to
// pick with "#<request id>".
//
// An invoice some request already holds (the same file or invoice number sent
// again) only resumes that request if it is a candidate. Otherwise the group
// is told it was a resend, so a resend never issues another request.
func IssueInvoiceFromWechatFile(ctx context.Context, fileName string, content []byte) (*InvoiceWechatFileResult, error) {
	name, ok := parseInvoiceWechatFileName(fileName)
	if !ok || invoicefile.DetectMime(content) != "application/pdf" {
		return &InvoiceWechatFileResult{Outcome: InvoiceWechatFileNotPDF}, nil
	}
	pending, err := model.ListPendingInvoiceRequests()
	if err != nil {
		return nil, err
	}
	invoice, readable := readInvoicePDF(content)
	var named []*model.InvoiceRequest
	for _, request := range pending {
		if name.namesCompany(request.CompanyName) {
			named = append(named, request)
		}
	}
	var matched []*model.InvoiceRequest
	for _, request := range pending {
		if name.requestId != 0 && request.Id != name.requestId {
			continue
		}
		key := invoiceCompanyMatchKey(request.CompanyName)
		byName := slices.Contains(named, request) && !slices.ContainsFunc(named, func(other *model.InvoiceRequest) bool {
			otherKey := invoiceCompanyMatchKey(other.CompanyName)
			return len(otherKey) > len(key) && strings.Contains(otherKey, key)
		})
		if byName || invoice.namesTaxNumber(request.TaxNumber) {
			matched = append(matched, request)
		}
	}
	digest := sha256.Sum256(content)
	sha256Value := hex.EncodeToString(digest[:])

	holders, err := model.ListInvoiceRequestIdsHoldingInvoice(sha256Value, invoice.number)
	if err != nil {
		return nil, err
	}
	if len(holders) > 0 {
		resumed := slices.IndexFunc(matched, func(request *model.InvoiceRequest) bool {
			return slices.Contains(holders, request.Id)
		})
		if resumed < 0 {
			holder, err := model.GetInvoiceRequestById(holders[0])
			if err != nil {
				return nil, err
			}
			// The bot posts a file again when it lost the answer to a post
			// that went through; the group already has the completion message.
			sameFile, err := model.InvoiceFileExists(holder.Id, sha256Value)
			if err != nil {
				return nil, err
			}
			if sameFile && holder.Status == model.InvoiceStatusIssued && common.GetTimestamp()-holder.IssuedTime < invoiceWechatRepostWindowSeconds {
				return &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: holder.Id}, nil
			}
			err = enqueueInvoiceWechatFileReply(holder, "duplicate", sha256Value, "重复发票",
				buildInvoiceWechatDuplicateText(holder, invoice.number, fileName))
			if err != nil {
				return nil, err
			}
			return &InvoiceWechatFileResult{Outcome: InvoiceWechatFileDuplicate, RequestId: holder.Id}, nil
		}
		matched = []*model.InvoiceRequest{matched[resumed]}
	}
	if len(matched) == 0 {
		return &InvoiceWechatFileResult{Outcome: InvoiceWechatFileNoMatch}, nil
	}
	if readable && name.requestId == 0 && len(holders) == 0 {
		picked := slices.IndexFunc(matched, func(request *model.InvoiceRequest) bool {
			return invoiceTotalCents(request) == invoice.totalCents
		})
		if picked < 0 {
			err := enqueueInvoiceWechatFileReply(matched[0], "amount-mismatch", sha256Value, "未开票：金额对不上",
				buildInvoiceWechatAmountMismatchText(matched, invoice))
			if err != nil {
				return nil, err
			}
			return &InvoiceWechatFileResult{Outcome: InvoiceWechatFileAmountMismatch}, nil
		}
		matched = []*model.InvoiceRequest{matched[picked]}
	}
	if len(matched) > 1 {
		err := enqueueInvoiceWechatFileReply(matched[0], "unreadable", sha256Value, "未开票：读不出发票内容",
			buildInvoiceWechatUnreadableText(matched, fileName))
		if err != nil {
			return nil, err
		}
		return &InvoiceWechatFileResult{Outcome: InvoiceWechatFileAmbiguous}, nil
	}
	request := matched[0]

	attached, err := model.InvoiceFileExists(request.Id, sha256Value)
	if err != nil {
		return nil, err
	}
	if !attached {
		_, err := StoreInvoiceFile(ctx, InvoiceFileContent{
			RequestId:    request.Id,
			FileName:     fileName,
			DeclaredMime: "application/pdf",
			Size:         int64(len(content)),
			Body:         bytes.NewReader(content),
		})
		if err != nil {
			return nil, err
		}
	}

	// An applicant whose account is gone cannot be emailed, but the invoice
	// is still issued.
	var user *model.User
	if common.InvoiceIssuedNotifyUserEnabled {
		user, err = model.GetUserById(request.UserId, false)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	issued, err := model.UpdateInvoiceRequestStatusWithNotifications(request.Id, model.InvoiceStatusIssued, 0, "",
		func(updated *model.InvoiceRequest) ([]*model.InvoiceNotificationDelivery, error) {
			deliveries, err := BuildInvoiceIssuedNotifications(updated, model.InvoiceStatusPending, user)
			if err != nil {
				return nil, err
			}
			return append(deliveries, newInvoiceWechatDelivery(
				updated, "issued", fmt.Sprintf("开票工单已完成 #%d", updated.Id), buildInvoiceWechatIssuedText(updated, fileName),
			)), nil
		})
	if err != nil {
		return nil, err
	}
	WakeInvoiceWechatDelivery()
	return &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: issued.Id}, nil
}

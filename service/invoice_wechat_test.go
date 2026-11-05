package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/ledongthuc/pdf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testInvoiceWechatToken = "wechat-secret-token-7f3a"

type fakeWechatAPI struct {
	server   *httptest.Server
	mu       sync.Mutex
	response string
	delay    time.Duration
	messages []map[string]string
	tokens   []string
}

func newFakeWechatAPI(t *testing.T) *fakeWechatAPI {
	t.Helper()
	api := &fakeWechatAPI{response: `{"ret":200,"msg":"ok","data":{"toWxid":"finance@chatroom","msgId":1}}`}
	api.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var message map[string]string
		_ = common.Unmarshal(body, &message)
		api.mu.Lock()
		api.messages = append(api.messages, message)
		api.tokens = append(api.tokens, r.Header.Get("wechat-token"))
		response, delay := api.response, api.delay
		api.mu.Unlock()
		if r.URL.Path != "/finder/v2/api/message/postText" {
			http.NotFound(w, r)
			return
		}
		time.Sleep(delay)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(api.server.Close)
	return api
}

func (api *fakeWechatAPI) received() []map[string]string {
	api.mu.Lock()
	defer api.mu.Unlock()
	return append([]map[string]string(nil), api.messages...)
}

func withInvoiceWechatSettings(t *testing.T, baseURL string) {
	t.Helper()
	previous := []any{
		setting.InvoiceWechatNotifyEnabled, setting.InvoiceWechatApiBaseUrl, setting.InvoiceWechatApiToken,
		setting.InvoiceWechatAppId, setting.InvoiceWechatGroupId, system_setting.ServerAddress,
		common.InvoiceApplicationNotifyAdminEnabled, setting.InvoiceTaxRateBasisPoints,
		invoiceWechatHTTPTimeout, invoiceWechatRetryBackoff,
	}
	setting.InvoiceWechatNotifyEnabled = true
	setting.InvoiceWechatApiBaseUrl = baseURL
	setting.InvoiceWechatApiToken = testInvoiceWechatToken
	setting.InvoiceWechatAppId = "wx_app_1"
	setting.InvoiceWechatGroupId = "finance@chatroom"
	system_setting.ServerAddress = "https://88api.ai/"
	common.InvoiceApplicationNotifyAdminEnabled = false
	setting.InvoiceTaxRateBasisPoints = 0
	invoiceWechatHTTPTimeout = 300 * time.Millisecond
	invoiceWechatRetryBackoff = []time.Duration{0}
	t.Cleanup(func() {
		setting.InvoiceWechatNotifyEnabled = previous[0].(bool)
		setting.InvoiceWechatApiBaseUrl = previous[1].(string)
		setting.InvoiceWechatApiToken = previous[2].(string)
		setting.InvoiceWechatAppId = previous[3].(string)
		setting.InvoiceWechatGroupId = previous[4].(string)
		system_setting.ServerAddress = previous[5].(string)
		common.InvoiceApplicationNotifyAdminEnabled = previous[6].(bool)
		setting.InvoiceTaxRateBasisPoints = previous[7].(int)
		invoiceWechatHTTPTimeout = previous[8].(time.Duration)
		invoiceWechatRetryBackoff = previous[9].([]time.Duration)
	})
}

func TestInvoiceWechatMessagesOmitEmptyLines(t *testing.T) {
	withInvoiceWechatSettings(t, "http://wechat.invalid/finder/v2/api")
	createdAt := time.Date(2026, time.October, 12, 1, 5, 0, 0, time.UTC).Unix()
	request := &model.InvoiceRequest{
		Id: 42, UserId: 7, Username: "alice", CompanyName: "示例科技有限公司", TaxNumber: "91310000EXAMPLE",
		CompanyPhone: "021-88888888", TotalMoneyCents: 123456780, CreatedTime: createdAt,
		UpdatedTime: createdAt + 3600, Status: model.InvoiceStatusWithdrawn,
	}

	assert.Equal(t, "【新开票申请 #42】\n"+
		"开票类型：增值税电子普通发票（中国大陆）\n"+
		"发票抬头：示例科技有限公司\n"+
		"税号：91310000EXAMPLE\n"+
		"电话：021-88888888\n"+
		"开票金额：¥1,234,567.80", buildInvoiceWechatCreatedText(request))
	hongKong := &model.InvoiceRequest{Id: 43, Region: model.InvoiceRegionHongKong, CompanyName: "Harbour Trading Limited", TotalMoneyCents: 95000}
	assert.Equal(t, "【新开票申请 #43】\n"+
		"开票类型：帳單 INVOICE（中国香港）\n"+
		"发票抬头：Harbour Trading Limited\n"+
		"开票金额：¥950.00", buildInvoiceWechatCreatedText(hongKong))
	assert.Equal(t, "【开票请求作废 #42】\n"+
		"发票抬头：示例科技有限公司\n"+
		"开票金额：¥1,234,567.80\n"+
		"用户已于 2026-10-12 10:05 撤回该申请，请勿开票。", buildInvoiceWechatWithdrawnText(request))

	for cents, expected := range map[int64]string{0: "0.00", 5: "0.05", 50000: "500.00", 100000: "1,000.00", 70125: "701.25"} {
		assert.Equal(t, expected, formatInvoiceAmountCNY(cents))
	}
}

func TestSendInvoiceWechatTextRetriesOnlyUnsentRequests(t *testing.T) {
	api := newFakeWechatAPI(t)
	withInvoiceWechatSettings(t, api.server.URL+"/finder/v2/api")
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	tests := []struct {
		name         string
		baseURL      string
		response     string
		delay        time.Duration
		wantAttempts int
		wantHits     int
		wantErr      string
	}{
		{name: "accepted", baseURL: api.server.URL + "/finder/v2/api", response: `{"ret":200,"msg":"ok"}`, wantAttempts: 1, wantHits: 1},
		{name: "rejected by provider", baseURL: api.server.URL + "/finder/v2/api/", response: `{"ret":500,"msg":"wxid not in group, token ` + testInvoiceWechatToken + `"}`, wantAttempts: 1, wantHits: 1, wantErr: "wxid not in group"},
		{name: "http error", baseURL: api.server.URL + "/wrong", response: `{"ret":200}`, wantAttempts: 1, wantHits: 1, wantErr: "HTTP 404"},
		{name: "timeout after send", baseURL: api.server.URL + "/finder/v2/api", response: `{"ret":200}`, delay: time.Second, wantAttempts: 1, wantHits: 1, wantErr: "Timeout"},
		{name: "connection refused", baseURL: closedURL + "/finder/v2/api", wantAttempts: invoiceWechatMaxAttempts, wantErr: "connect"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api.mu.Lock()
			api.messages, api.tokens = nil, nil
			api.response, api.delay = test.response, test.delay
			api.mu.Unlock()
			config := invoiceWechatConfig{BaseURL: test.baseURL, Token: testInvoiceWechatToken, AppID: "wx_app_1", GroupID: "finance@chatroom"}

			attempts, err := sendInvoiceWechatText(context.Background(), config, "finance@chatroom", "hello")

			assert.Equal(t, test.wantAttempts, attempts)
			assert.Len(t, api.received(), test.wantHits)
			if test.wantErr == "" {
				require.NoError(t, err)
				require.Len(t, api.received(), 1)
				assert.Equal(t, map[string]string{"appId": "wx_app_1", "toWxid": "finance@chatroom", "content": "hello"}, api.received()[0])
				assert.Equal(t, []string{testInvoiceWechatToken}, api.tokens)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
			assert.NotContains(t, err.Error(), testInvoiceWechatToken)
		})
	}
}

func TestSendInvoiceWechatTestMessageUsesSavedSettings(t *testing.T) {
	api := newFakeWechatAPI(t)
	withInvoiceWechatSettings(t, api.server.URL+"/finder/v2/api")
	setting.InvoiceWechatNotifyEnabled = false

	require.NoError(t, SendInvoiceWechatTestMessage(context.Background()))
	require.Len(t, api.received(), 1)
	assert.Equal(t, "finance@chatroom", api.received()[0]["toWxid"])
	assert.Regexp(t, `^88API 开票通知测试（\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}）$`, api.received()[0]["content"])

	api.mu.Lock()
	api.response = `{"ret":401,"msg":"token expired"}`
	api.mu.Unlock()
	err := SendInvoiceWechatTestMessage(context.Background())
	require.ErrorContains(t, err, "token expired")

	setting.InvoiceWechatAppId = ""
	require.ErrorIs(t, SendInvoiceWechatTestMessage(context.Background()), ErrInvoiceWechatNotConfigured)
}

func TestInvoiceWechatNotificationsQueueForCreateAndWithdrawOnly(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(
		&model.InvoiceRequest{}, &model.InvoiceOrderClaim{}, &model.InvoiceSearchTerm{}, &model.InvoiceRequestEvent{},
		&model.InvoiceNotificationDelivery{}, &model.InvoiceFile{}, &model.InvoiceFileUpload{}, &model.InvoiceFileCleanup{},
	))
	t.Cleanup(func() {
		for _, table := range []string{
			"invoice_notification_deliveries", "invoice_request_events", "invoice_search_terms", "invoice_order_claims",
			"invoice_requests", "email_deliveries", "top_ups", "logs", "users",
		} {
			model.DB.Exec("DELETE FROM " + table)
		}
	})
	api := newFakeWechatAPI(t)
	withInvoiceWechatSettings(t, api.server.URL+"/finder/v2/api")

	user := &model.User{Username: "wechat-applicant", Email: "applicant@example.com", AffCode: "wechat-applicant", Status: common.UserStatusEnabled, Quota: 1_000_000}
	require.NoError(t, model.DB.Create(user).Error)
	submit := func(tradeNo string) *model.InvoiceRequest {
		order := &model.TopUp{UserId: user.Id, Money: 600, Amount: 100, TradeNo: tradeNo, Status: common.TopUpStatusSuccess}
		require.NoError(t, model.DB.Create(order).Error)
		request, _, err := model.CreateInvoiceRequestWithNotifications(model.CreateInvoiceRequestParams{
			UserId: user.Id, Username: user.Username, CompanyName: "微信通知有限公司", TaxNumber: "91310000WECHAT",
			BankName: "招商银行", Email: user.Email, TopUpOrderIds: []int{order.Id},
		}, BuildInvoiceRequestCreatedNotifications)
		require.NoError(t, err)
		return request
	}
	wechatDeliveries := func() []*model.InvoiceNotificationDelivery {
		var deliveries []*model.InvoiceNotificationDelivery
		require.NoError(t, model.DB.Where("kind = ?", model.InvoiceNotificationKindWechatGroup).Order("id ASC").Find(&deliveries).Error)
		return deliveries
	}

	setting.InvoiceWechatNotifyEnabled = false
	submit("WECHAT-DISABLED")
	setting.InvoiceWechatNotifyEnabled = true
	setting.InvoiceWechatGroupId = ""
	submit("WECHAT-UNCONFIGURED")
	setting.InvoiceWechatGroupId = "finance@chatroom"
	require.Empty(t, wechatDeliveries())

	withdrawnRequest := submit("WECHAT-WITHDRAWN")
	_, err := model.WithdrawInvoiceRequest(withdrawnRequest.Id, user.Id, BuildInvoiceWithdrawnNotifications)
	require.NoError(t, err)
	rejectedRequest := submit("WECHAT-REJECTED")
	_, err = model.UpdateInvoiceRequestStatusWithNotifications(rejectedRequest.Id, model.InvoiceStatusRejected, 1, "资料错误", func(updated *model.InvoiceRequest) ([]*model.InvoiceNotificationDelivery, error) {
		return BuildInvoiceClosedNotifications(updated, user)
	})
	require.NoError(t, err)
	expiredRequest := submit("WECHAT-EXPIRED")
	require.NoError(t, model.DB.Model(&model.InvoiceRequest{}).Where("id = ?", expiredRequest.Id).Update("created_time", 100).Error)
	_, err = model.ExpirePendingInvoiceRequests(200, 100, BuildInvoiceClosedNotifications)
	require.NoError(t, err)

	queued := wechatDeliveries()
	require.Len(t, queued, 4, "created x3 and one withdrawal")
	subjects := make([]string, 0, len(queued))
	for _, delivery := range queued {
		subjects = append(subjects, delivery.Subject)
		assert.Equal(t, model.InvoiceNotificationChannelWechat, delivery.Channel)
		assert.Equal(t, "finance@chatroom", delivery.Recipient)
		assert.Zero(t, delivery.EmailDeliveryId)
	}
	assert.Contains(t, subjects, "开票请求作废 #"+strconv.Itoa(withdrawnRequest.Id))
	var emailCount int64
	require.NoError(t, model.DB.Model(&model.EmailDelivery{}).Where("category = ?", "invoice_admin_email").Count(&emailCount).Error)
	assert.Zero(t, emailCount)

	deliverPendingInvoiceWechatNotifications()
	require.Len(t, api.received(), 4)
	assert.Contains(t, api.received()[0]["content"], "【新开票申请 #")
	assert.Contains(t, api.received()[0]["content"], "开户银行：招商银行")
	for _, delivery := range wechatDeliveries() {
		assert.NotZero(t, delivery.DeliveredTime)
		assert.Zero(t, delivery.FailedTime)
		assert.Equal(t, 1, delivery.Attempts)
		assert.Empty(t, delivery.Body, "billing details are dropped once delivered")
		assert.Equal(t, "finance@chatroom", delivery.Recipient)
	}

	api.mu.Lock()
	api.response = `{"ret":500,"msg":"group closed"}`
	api.mu.Unlock()
	failed := submit("WECHAT-FAILED")
	deliverPendingInvoiceWechatNotifications()
	deliverPendingInvoiceWechatNotifications()
	require.Len(t, api.received(), 5, "a rejected message is not sent again automatically")
	var failedDelivery model.InvoiceNotificationDelivery
	require.NoError(t, model.DB.Where("invoice_request_id = ? AND kind = ?", failed.Id, model.InvoiceNotificationKindWechatGroup).First(&failedDelivery).Error)
	assert.NotZero(t, failedDelivery.FailedTime)
	assert.Zero(t, failedDelivery.DeliveredTime)
	assert.Equal(t, 1, failedDelivery.Attempts)
	assert.Contains(t, failedDelivery.LastError, "group closed")
	assert.NotContains(t, failedDelivery.LastError, testInvoiceWechatToken)
}

func TestInvoiceWechatDeliveryWaitsWhileDisabledAndNeverResendsAnInterruptedSend(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(&model.InvoiceNotificationDelivery{}))
	t.Cleanup(func() { model.DB.Exec("DELETE FROM invoice_notification_deliveries") })
	api := newFakeWechatAPI(t)
	withInvoiceWechatSettings(t, api.server.URL+"/finder/v2/api")
	enqueue := func(key string) *model.InvoiceNotificationDelivery {
		delivery, created, err := model.EnqueueInvoiceNotification(&model.InvoiceNotificationDelivery{
			DeliveryKey: key, InvoiceRequestId: 1, Kind: model.InvoiceNotificationKindWechatGroup,
			Recipient: "finance@chatroom", Subject: key, Body: "body " + key,
		})
		require.NoError(t, err)
		require.True(t, created)
		return delivery
	}
	reload := func(id int) model.InvoiceNotificationDelivery {
		var delivery model.InvoiceNotificationDelivery
		require.NoError(t, model.DB.First(&delivery, id).Error)
		return delivery
	}

	// Switched off, or switched on without a token: nothing is claimed,
	// failed or sent, and the message stays queued.
	queued := enqueue("wechat-queued")
	setting.InvoiceWechatNotifyEnabled = false
	deliverPendingInvoiceWechatNotifications()
	setting.InvoiceWechatNotifyEnabled = true
	setting.InvoiceWechatApiToken = ""
	deliverPendingInvoiceWechatNotifications()
	assert.Empty(t, api.received())
	waiting := reload(queued.Id)
	assert.Zero(t, waiting.FailedTime)
	assert.Zero(t, waiting.SendStartedTime)
	assert.Zero(t, waiting.LockedUntil)
	assert.Zero(t, waiting.Attempts)
	assert.Empty(t, waiting.LastError)

	// One sender stopped after claiming a message and its claim expired;
	// another sender is still posting a second message.
	now := common.GetTimestamp()
	interrupted := enqueue("wechat-interrupted")
	claimed, err := model.ClaimInvoiceWechatNotification(interrupted.Id, now-300, now-120)
	require.NoError(t, err)
	require.True(t, claimed)
	inFlight := enqueue("wechat-in-flight")
	claimed, err = model.ClaimInvoiceWechatNotification(inFlight.Id, now, now+600)
	require.NoError(t, err)
	require.True(t, claimed)

	setting.InvoiceWechatApiToken = testInvoiceWechatToken
	deliverPendingInvoiceWechatNotifications()
	require.Len(t, api.received(), 1, "only the message that was never claimed is posted")
	assert.Equal(t, "body wechat-queued", api.received()[0]["content"])
	assert.NotZero(t, reload(queued.Id).DeliveredTime)
	unknown := reload(interrupted.Id)
	assert.NotZero(t, unknown.FailedTime)
	assert.Zero(t, unknown.DeliveredTime)
	assert.Equal(t, model.InvoiceWechatOutcomeUnknownError, unknown.LastError)
	sending := reload(inFlight.Id)
	assert.Zero(t, sending.FailedTime)
	assert.Zero(t, sending.DeliveredTime)

	_, err = model.RetryInvoiceNotification(inFlight.Id)
	require.ErrorIs(t, err, model.ErrInvoiceNotificationSending)
	_, err = model.RetryInvoiceNotification(interrupted.Id)
	require.NoError(t, err)
	deliverPendingInvoiceWechatNotifications()
	require.Len(t, api.received(), 2, "an administrator retry posts it once more")
	assert.Equal(t, "body wechat-interrupted", api.received()[1]["content"])
	assert.NotZero(t, reload(interrupted.Id).DeliveredTime)
}

func TestInvoiceWechatFileNameNamesCompany(t *testing.T) {
	const yunfan = "上海云帆网络信息技术有限公司"
	tests := []struct {
		fileName  string
		company   string
		names     bool
		requestId int
	}{
		{"上海云帆网络信息技术有限公司.pdf", yunfan, true, 0},
		{"上海云帆网络信息技术有限公司发票（1）.PDF", yunfan, true, 0},
		{"dzfp_26442000000012345678_上海云帆网络信息技术有限公司_20261012153012.pdf", yunfan, true, 0},
		{"电子发票 上海云帆网络信息技术有限公司.pdf", yunfan, true, 0},
		{"上海云帆网络信息技术有限公司＃63.pdf", yunfan, true, 63},
		{"发票#63上海云帆网络信息技术有限公司.pdf", yunfan, true, 63},
		{"上海云帆网络信息技术有限公司.pdf", "云帆网络信息技术有限公司", false, 0},
		{"Ａｃｍｅ Trading（Shanghai） Co_invoice.pdf", "Acme Trading (Shanghai) Co", true, 0},
		{"张三.pdf", "张三", true, 0},
		{"张三丰工作室.pdf", "张三", false, 0},
		{"上海云帆网络信息技术有限公司.ofd", yunfan, false, 0},
	}
	for _, test := range tests {
		t.Run(test.fileName+"/"+test.company, func(t *testing.T) {
			name, ok := parseInvoiceWechatFileName(test.fileName)
			assert.Equal(t, test.names, ok && name.namesCompany(test.company))
			assert.Equal(t, test.requestId, name.requestId)
		})
	}
}

// invoiceTestPDF builds a one-page PDF laid out like a fully digitalized
// e-invoice: the invoice number and each "¥" amount on lines of their own.
func invoiceTestPDF(number string, amounts ...string) []byte {
	var stream strings.Builder
	for i, line := range append([]string{number}, amounts...) {
		line = strings.ReplaceAll(line, "¥", `\245`)
		fmt.Fprintf(&stream, "BT /F1 12 Tf 50 %d Td (%s) Tj ET\n", 750-20*i, line)
	}
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", stream.Len(), stream.String()),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
	var out strings.Builder
	out.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return []byte(out.String())
}

func TestReadInvoicePDF(t *testing.T) {
	invoice, ok := readInvoicePDF(invoiceTestPDF("26612000002217482656", "¥940.59", "¥9.41", "¥ 1,950.00"))
	require.True(t, ok)
	assert.Equal(t, invoicePDF{number: "26612000002217482656", totalCents: 195000}, invoice)
	invoice, ok = readInvoicePDF(invoiceTestPDF("26612000002005097971", "91330109MAG0554P7G", "91610132396092369H", "¥10,000.00"))
	require.True(t, ok)
	assert.Equal(t, []string{"91330109MAG0554P7G", "91610132396092369H"}, invoice.taxNumbers)
	assert.True(t, invoice.namesTaxNumber(" 91330109mag0554p7g"), "the number an applicant typed matches whatever its case")
	assert.False(t, invoice.namesTaxNumber(""))

	_, ok = readInvoicePDF([]byte("%PDF-1.7\n% scanned invoice"))
	assert.False(t, ok, "a PDF without text has no total")
	_, ok = readInvoicePDF(invoiceTestPDF("26612000002217482656"))
	assert.False(t, ok, "a PDF without amounts has no total")
}

func TestIssueInvoiceFromWechatFile(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(
		&model.InvoiceRequest{}, &model.InvoiceRequestEvent{}, &model.InvoiceNotificationDelivery{},
		&model.InvoiceFile{}, &model.InvoiceFileUpload{}, &model.InvoiceFileCleanup{}, &model.InvoiceStorageProfile{},
	))
	t.Cleanup(func() {
		for _, table := range []string{
			"invoice_notification_deliveries", "invoice_request_events", "invoice_files", "invoice_file_uploads",
			"invoice_requests", "invoice_storage_profiles", "email_deliveries", "users",
		} {
			model.DB.Exec("DELETE FROM " + table)
		}
	})
	api := newFakeWechatAPI(t)
	withInvoiceWechatSettings(t, api.server.URL+"/finder/v2/api")
	previous := []any{
		setting.InvoiceFileEnabled, setting.InvoiceFileStorage, setting.InvoiceFileLocalPath, setting.InvoiceFileAllowedExts,
		setting.InvoiceFileAllowedMimes, setting.InvoiceFileMaxSize, setting.InvoiceFileMaxCount, common.InvoiceIssuedNotifyUserEnabled,
	}
	t.Cleanup(func() {
		setting.InvoiceFileEnabled = previous[0].(bool)
		setting.InvoiceFileStorage = previous[1].(string)
		setting.InvoiceFileLocalPath = previous[2].(string)
		setting.InvoiceFileAllowedExts = previous[3].(string)
		setting.InvoiceFileAllowedMimes = previous[4].(string)
		setting.InvoiceFileMaxSize = previous[5].(int64)
		setting.InvoiceFileMaxCount = previous[6].(int)
		common.InvoiceIssuedNotifyUserEnabled = previous[7].(bool)
	})
	setting.InvoiceFileEnabled = true
	setting.InvoiceFileStorage = "local"
	setting.InvoiceFileLocalPath = t.TempDir()
	setting.InvoiceFileAllowedExts = "pdf,png"
	setting.InvoiceFileAllowedMimes = "application/pdf,image/png"
	setting.InvoiceFileMaxSize = 5 * 1024 * 1024
	setting.InvoiceFileMaxCount = 5
	common.InvoiceIssuedNotifyUserEnabled = true

	applicant := &model.User{Username: "yunfan", Email: "finance@yunfan.example", AffCode: "yunfan", Status: common.UserStatusEnabled}
	other := &model.User{Username: "acme", AffCode: "acme", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(applicant).Error)
	require.NoError(t, model.DB.Create(other).Error)
	submitted := time.Date(2026, 11, 5, 10, 0, 0, 0, invoiceWechatLocation).Unix()
	pendingRequest := func(user *model.User, company string, taxNumber string, totalCents int64) *model.InvoiceRequest {
		request := &model.InvoiceRequest{
			UserId: user.Id, Username: user.Username, CompanyName: company, TaxNumber: taxNumber,
			TotalMoneyCents: totalCents, Status: model.InvoiceStatusPending, CreatedTime: submitted,
		}
		require.NoError(t, model.DB.Create(request).Error)
		return request
	}
	yunfan := pendingRequest(applicant, "上海云帆网络信息技术有限公司", "91310000YUNFAN", 120000)
	acmeFirst := pendingRequest(other, "Acme Trading Co", "91310000ACME", 120000)
	acmeSecond := pendingRequest(other, "Acme Trading Co", "91310000ACME", 120000)
	retried := pendingRequest(other, "重试科技有限公司", "91310000RETRY", 120000)
	yunfanSuffix := pendingRequest(other, "云帆网络信息技术有限公司", "91310000SUFFIX", 120000)
	headOffice := pendingRequest(other, "成都蜀道智能科技有限公司", "91510000HEAD", 120000)
	branch := pendingRequest(other, "成都蜀道智能科技有限公司北京分公司", "91110000BRANCH", 120000)
	splitSmall := pendingRequest(other, "浙江拆单大学", "12330000SPLIT", 68800)
	splitOlder := pendingRequest(other, "浙江拆单大学", "12330000SPLIT", 95000)
	splitNewer := pendingRequest(other, "浙江拆单大学", "12330000SPLIT", 95000)
	hiyou := pendingRequest(other, "杭州嗨游米网络科技有限公司", " 91330109mag0554p7g", 624900)

	// Files without text stand for scanned invoices, one per invoice.
	scanned := func(name string) []byte { return []byte("%PDF-1.7\n% scanned " + name) }
	retriedPDF := scanned("retried")
	retriedDigest := sha256.Sum256(retriedPDF)
	require.NoError(t, model.DB.Create(&model.InvoiceFile{
		InvoiceRequestId: retried.Id, FileName: "重试科技有限公司.pdf", StoredName: "attached.pdf", MimeType: "application/pdf",
		Size: int64(len(retriedPDF)), StorageType: "local", StorageKey: "attached.pdf", Sha256: hex.EncodeToString(retriedDigest[:]),
	}).Error)

	issue := func(fileName string, content []byte) *InvoiceWechatFileResult {
		result, err := IssueInvoiceFromWechatFile(context.Background(), fileName, content)
		require.NoError(t, err)
		return result
	}
	assert.Equal(t, InvoiceWechatFileNoMatch, issue("未知公司.pdf", scanned("unknown")).Outcome)
	assert.Equal(t, InvoiceWechatFileAmbiguous, issue("acme trading co.pdf", scanned("acme")).Outcome,
		"a scanned invoice cannot pick among several requests")
	assert.Equal(t, InvoiceWechatFileNotPDF, issue("Acme Trading Co.pdf", []byte("plain text")).Outcome)
	assert.Equal(t, InvoiceWechatFileNotPDF, issue("Acme Trading Co.png", scanned("acme")).Outcome)
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: acmeSecond.Id},
		issue("Acme Trading Co#"+strconv.Itoa(acmeSecond.Id)+".pdf", scanned("acme second")))
	yunfanPDF := scanned("yunfan")
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: yunfan.Id},
		issue("上海云帆网络信息技术有限公司(1).pdf", yunfanPDF))
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: yunfan.Id},
		issue("上海云帆网络信息技术有限公司(1).pdf", yunfanPDF), "the bot posting the file again right after the issue")
	assert.Equal(t, InvoiceWechatFileNoMatch, issue("上海云帆网络信息技术有限公司.pdf", scanned("another")).Outcome,
		"an issued request takes no more files")
	issuedAt := time.Date(2026, 11, 4, 14, 2, 0, 0, invoiceWechatLocation).Unix()
	require.NoError(t, model.DB.Model(yunfan).Update("issued_time", issuedAt).Error)
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileDuplicate, RequestId: yunfan.Id},
		issue("上海云帆网络信息技术有限公司(1).pdf", yunfanPDF), "the finance group sending the file again later")
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: retried.Id},
		issue("重试科技有限公司.pdf", retriedPDF), "a file attached before a failed issue is reused")
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: branch.Id},
		issue("dzfp_26442000000012345678_成都蜀道智能科技有限公司北京分公司_20261012.pdf", scanned("branch")),
		"the head office name inside the branch name does not count")

	firstInvoice := invoiceTestPDF("26612000000000000001", "¥940.59", "¥9.41", "¥950.00")
	firstName := "dzfp_26612000000000000001_浙江拆单大学_20261104102159.pdf"
	wrongTotal := invoiceTestPDF("26612000000000000009", "¥123.00")
	assert.Equal(t, InvoiceWechatFileAmountMismatch, issue(firstName, wrongTotal).Outcome, "no split request has the invoice total")
	assert.Equal(t, InvoiceWechatFileAmountMismatch, issue(firstName, wrongTotal).Outcome, "the group is asked only once")
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: splitOlder.Id},
		issue(firstName, firstInvoice), "the oldest request with the invoice total is issued")
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: splitOlder.Id},
		issue(firstName, firstInvoice), "posting the same invoice again does not issue the next request")
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileDuplicate, RequestId: splitOlder.Id},
		issue("dzfp_26612000000000000001_浙江拆单大学_20261104110000.pdf", invoiceTestPDF("26612000000000000001", "¥950.00")),
		"a regenerated file of the same invoice number does not either")
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: splitNewer.Id},
		issue("dzfp_26612000000000000002_浙江拆单大学_20261104102211.pdf", invoiceTestPDF("26612000000000000002", "¥940.59", "¥9.41", "¥950.00")))

	offline := invoiceTestPDF("26612000002005097971", "91330109MAG0554P7G", "91610132396092369H", "¥9,900.99", "¥99.01", "¥10,000.00")
	offlineName := "dzfp_26612000002005097971_杭州嗨游米网络科技有限公司_20261009110422.pdf"
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileAmountMismatch},
		issue(offlineName, offline), "an offline invoice does not issue the pending online request")
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: hiyou.Id},
		issue("发票.pdf", invoiceTestPDF("26612000002004948151", "91330109MAG0554P7G", "91610132396092369H", "¥6,187.13", "¥61.87", "¥6,249.00")),
		"the taxpayer number on the invoice finds the request")
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileNoMatch},
		issue("dzfp_26612000002061024076_杭州嗨游米网络科技有限公司_20261016090741.pdf",
			invoiceTestPDF("26612000002061024076", "91330109MAG0554P7G", "91610132396092369H", "¥10,000.00")),
		"an offline invoice for a company with no pending request is ignored")

	statuses := map[int]int{}
	var requests []*model.InvoiceRequest
	require.NoError(t, model.DB.Find(&requests).Error)
	for _, request := range requests {
		statuses[request.Id] = request.Status
	}
	assert.Equal(t, map[int]int{
		yunfan.Id: model.InvoiceStatusIssued, acmeFirst.Id: model.InvoiceStatusPending,
		acmeSecond.Id: model.InvoiceStatusIssued, retried.Id: model.InvoiceStatusIssued,
		yunfanSuffix.Id: model.InvoiceStatusPending, headOffice.Id: model.InvoiceStatusPending, branch.Id: model.InvoiceStatusIssued,
		splitSmall.Id: model.InvoiceStatusPending, splitOlder.Id: model.InvoiceStatusIssued, splitNewer.Id: model.InvoiceStatusIssued,
		hiyou.Id: model.InvoiceStatusIssued,
	}, statuses)
	for request, content := range map[*model.InvoiceRequest][]byte{yunfan: yunfanPDF, retried: retriedPDF} {
		files, err := model.ListInvoiceFiles(request.Id)
		require.NoError(t, err)
		require.Len(t, files, 1)
		digest := sha256.Sum256(content)
		assert.Equal(t, hex.EncodeToString(digest[:]), files[0].Sha256)
	}

	olderIssued, err := model.GetInvoiceRequestById(splitOlder.Id)
	require.NoError(t, err)

	var deliveries []*model.InvoiceNotificationDelivery
	require.NoError(t, model.DB.Where("invoice_request_id = ?", yunfan.Id).Order("id ASC").Find(&deliveries).Error)
	require.Len(t, deliveries, 3)
	assert.Equal(t, model.InvoiceNotificationKindUserEmail, deliveries[0].Kind)
	assert.Equal(t, "finance@yunfan.example", deliveries[0].Recipient)
	assert.Equal(t, model.InvoiceNotificationKindWechatGroup, deliveries[1].Kind)
	assert.Equal(t, "finance@chatroom", deliveries[1].Recipient)
	assert.Equal(t, "重复发票 #"+strconv.Itoa(yunfan.Id), deliveries[2].Subject)

	deliverPendingInvoiceWechatNotifications()
	var contents []string
	for _, message := range api.received() {
		assert.Equal(t, "finance@chatroom", message["toWxid"])
		contents = append(contents, message["content"])
	}
	completions := 0
	for _, content := range contents {
		if strings.HasPrefix(content, "【开票工单已完成 #") {
			completions++
		}
	}
	assert.Equal(t, 7, completions, "one completion message per issued request")
	assert.Len(t, contents, 12, "plus one reply for each file the group has to look at")
	assert.Contains(t, contents, "【开票工单已完成 #"+strconv.Itoa(yunfan.Id)+"】\n"+
		"发票抬头：上海云帆网络信息技术有限公司\n"+
		"开票金额：¥1,200.00\n"+
		"发票文件：上海云帆网络信息技术有限公司(1).pdf")
	assert.Contains(t, contents, "【重复发票，未处理】\n"+
		"发票文件：上海云帆网络信息技术有限公司(1).pdf\n"+
		"这张发票已于 2026-11-04 14:02 开给 #"+strconv.Itoa(yunfan.Id)+"（上海云帆网络信息技术有限公司 ¥1,200.00），本次不重复处理。")
	assert.Contains(t, contents, "【重复发票，未处理】\n"+
		"发票号码：26612000000000000001\n"+
		"这张发票已于 "+formatInvoiceWechatTime(olderIssued.IssuedTime)+" 开给 #"+strconv.Itoa(splitOlder.Id)+"（浙江拆单大学 ¥950.00），本次不重复处理。")
	assert.Contains(t, contents, "【未开票：金额对不上】\n"+
		"发票抬头：杭州嗨游米网络科技有限公司\n"+
		"发票号码：26612000002005097971\n"+
		"价税合计：¥10,000.00\n"+
		"待开申请：#"+strconv.Itoa(hiyou.Id)+" ¥6,249.00（11-05 提交）\n"+
		"没有金额相等的申请，未自动开票。线下付款的票无需处理；如果是开给这张申请的，请核对金额后重开。")
	assert.Contains(t, contents, "【未开票：金额对不上】\n"+
		"发票抬头：浙江拆单大学\n"+
		"发票号码：26612000000000000009\n"+
		"价税合计：¥123.00\n"+
		"待开申请：#"+strconv.Itoa(splitSmall.Id)+" ¥688.00（11-05 提交）、#"+strconv.Itoa(splitOlder.Id)+" ¥950.00（11-05 提交）、#"+
		strconv.Itoa(splitNewer.Id)+" ¥950.00（11-05 提交）\n"+
		"没有金额相等的申请，未自动开票。线下付款的票无需处理；如果是开给其中一张申请的，请核对金额后重开。")
	assert.Contains(t, contents, "【未开票：读不出发票内容】\n"+
		"发票文件：acme trading co.pdf\n"+
		"同一抬头有 2 张待开申请：#"+strconv.Itoa(acmeFirst.Id)+" ¥1,200.00（11-05 提交）、#"+strconv.Itoa(acmeSecond.Id)+" ¥1,200.00（11-05 提交）\n"+
		"文件里读不到发票金额（可能是扫描件），请在文件名末尾加上申请号重新发送，例如：acme trading co_#"+strconv.Itoa(acmeFirst.Id)+".pdf")
}

func TestInvoiceWechatReminderFollowsTheWorkingDayCalendar(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(&model.InvoiceRequest{}, &model.InvoiceNotificationDelivery{}))
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM invoice_notification_deliveries")
		model.DB.Exec("DELETE FROM invoice_requests")
	})
	withInvoiceWechatSettings(t, newFakeWechatAPI(t).server.URL+"/finder/v2/api")
	calendar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 2026-11-02 (Monday) is a day off and 2026-11-07 (Saturday) a working day.
		_, _ = w.Write([]byte(`{"year":2026,"days":[{"date":"2026-11-02","isOffDay":true},{"date":"2026-11-07","isOffDay":false}]}`))
	}))
	t.Cleanup(calendar.Close)
	previousURL, previousCache := chinaHolidayCalendarURL, chinaHolidayCalendar
	chinaHolidayCalendarURL = calendar.URL + "/%d.json"
	chinaHolidayCalendar = &holidayCalendarCache{years: map[int]holidayCalendarYear{}}
	t.Cleanup(func() { chinaHolidayCalendarURL, chinaHolidayCalendar = previousURL, previousCache })

	at := func(value string) time.Time {
		parsed, err := time.ParseInLocation("2006-01-02 15:04", value, invoiceWechatLocation)
		require.NoError(t, err)
		return parsed
	}
	requests := []*model.InvoiceRequest{
		{UserId: 1, CompanyName: "济南即刻创想网络科技有限公司", TotalMoneyCents: 130700, Status: model.InvoiceStatusPending, CreatedTime: at("2026-10-26 17:35").Unix()},
		{UserId: 2, CompanyName: "已开票公司", TotalMoneyCents: 100, Status: model.InvoiceStatusIssued, CreatedTime: at("2026-10-27 10:00").Unix()},
		{UserId: 3, CompanyName: "当天申请公司", TotalMoneyCents: 200, Status: model.InvoiceStatusPending, CreatedTime: at("2026-11-03 08:30").Unix()},
	}
	for _, request := range requests {
		require.NoError(t, model.DB.Create(request).Error)
	}
	reminders := func() []*model.InvoiceNotificationDelivery {
		var deliveries []*model.InvoiceNotificationDelivery
		require.NoError(t, model.DB.Where("subject LIKE ?", "开票提醒%").Order("id ASC").Find(&deliveries).Error)
		return deliveries
	}

	queueInvoiceWechatReminder(at("2026-10-31 10:00")) // Saturday
	queueInvoiceWechatReminder(at("2026-11-02 10:00")) // holiday Monday
	queueInvoiceWechatReminder(at("2026-11-03 08:59")) // before 09:00
	queueInvoiceWechatReminder(at("2026-11-03 12:00")) // after the morning window
	require.Empty(t, reminders())

	queueInvoiceWechatReminder(at("2026-11-03 09:00"))
	queueInvoiceWechatReminder(at("2026-11-03 09:01"))
	queued := reminders()
	require.Len(t, queued, 1, "one reminder per working day")
	assert.Equal(t, "开票提醒 2026-11-03", queued[0].Subject)
	assert.Equal(t, "finance@chatroom", queued[0].Recipient)
	assert.Equal(t, requests[0].Id, queued[0].InvoiceRequestId)
	assert.Equal(t, "【开票提醒】有 1 张发票申请还未完成，请及时跟进：\n#"+strconv.Itoa(requests[0].Id)+" 济南即刻创想网络科技有限公司 ¥1,307.00（2026-10-26 17:35 申请）", queued[0].Body)

	queueInvoiceWechatReminder(at("2026-11-07 09:30")) // make-up working Saturday
	queued = reminders()
	require.Len(t, queued, 2)
	assert.Contains(t, queued[1].Body, "有 2 张发票申请")
	assert.Contains(t, queued[1].Body, "当天申请公司")

	setting.InvoiceWechatNotifyEnabled = false
	queueInvoiceWechatReminder(at("2026-11-09 09:30"))
	assert.Len(t, reminders(), 2, "nothing is queued while the group notification is off")
}

func TestIssueInvoiceHongKongBill(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(
		&model.InvoiceRequest{}, &model.InvoiceRequestEvent{}, &model.InvoiceNotificationDelivery{},
		&model.InvoiceFile{}, &model.InvoiceFileUpload{}, &model.InvoiceFileCleanup{}, &model.InvoiceStorageProfile{},
	))
	t.Cleanup(func() {
		for _, table := range []string{
			"invoice_notification_deliveries", "invoice_request_events", "invoice_files", "invoice_file_uploads",
			"invoice_requests", "invoice_storage_profiles", "email_deliveries", "users",
		} {
			model.DB.Exec("DELETE FROM " + table)
		}
	})
	api := newFakeWechatAPI(t)
	withInvoiceWechatSettings(t, api.server.URL+"/finder/v2/api")
	previous := []any{
		setting.InvoiceFileEnabled, setting.InvoiceFileStorage, setting.InvoiceFileLocalPath, setting.InvoiceFileAllowedExts,
		setting.InvoiceFileAllowedMimes, setting.InvoiceFileMaxSize, setting.InvoiceFileMaxCount, common.InvoiceIssuedNotifyUserEnabled,
		setting.InvoiceHongKongAutoBillEnabled, setting.InvoiceHongKongIssuerName, setting.InvoiceHongKongIssuerRegistrationNo,
		setting.InvoiceHongKongIssuerAddress,
	}
	t.Cleanup(func() {
		setting.InvoiceFileEnabled = previous[0].(bool)
		setting.InvoiceFileStorage = previous[1].(string)
		setting.InvoiceFileLocalPath = previous[2].(string)
		setting.InvoiceFileAllowedExts = previous[3].(string)
		setting.InvoiceFileAllowedMimes = previous[4].(string)
		setting.InvoiceFileMaxSize = previous[5].(int64)
		setting.InvoiceFileMaxCount = previous[6].(int)
		common.InvoiceIssuedNotifyUserEnabled = previous[7].(bool)
		setting.InvoiceHongKongAutoBillEnabled = previous[8].(bool)
		setting.InvoiceHongKongIssuerName = previous[9].(string)
		setting.InvoiceHongKongIssuerRegistrationNo = previous[10].(string)
		setting.InvoiceHongKongIssuerAddress = previous[11].(string)
	})
	setting.InvoiceFileEnabled = true
	setting.InvoiceFileStorage = "local"
	setting.InvoiceFileLocalPath = t.TempDir()
	setting.InvoiceFileAllowedExts = "pdf"
	setting.InvoiceFileAllowedMimes = "application/pdf"
	setting.InvoiceFileMaxSize = 5 * 1024 * 1024
	setting.InvoiceFileMaxCount = 5
	common.InvoiceIssuedNotifyUserEnabled = true
	setting.InvoiceHongKongAutoBillEnabled = true
	setting.InvoiceHongKongIssuerName = ""
	assert.False(t, InvoiceHongKongAutoBillReady(), "a bill needs its issuer")
	setting.InvoiceHongKongIssuerName = "DAYFOLD HK LIMITED"
	setting.InvoiceHongKongIssuerRegistrationNo = "78129797"
	setting.InvoiceHongKongIssuerAddress = "SUITE 6503, 65/F, CENTRAL PLAZA, 18 HARBOUR ROAD, WAN CHAI, HONG KONG"
	assert.True(t, InvoiceHongKongAutoBillReady())

	applicant := &model.User{Username: "harbour", Email: "accounts@harbour.example", AffCode: "harbour", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(applicant).Error)
	// 24 orders of ¥875 run past the first page.
	orders := make([]*model.TopUp, 24)
	for index := range orders {
		orders[index] = &model.TopUp{
			Id: index + 1, UserId: applicant.Id, Money: 875, TradeNo: fmt.Sprintf("USR%dNOHK%03d", applicant.Id, index+1),
			CompleteTime: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC).Unix(), Status: common.TopUpStatusSuccess,
		}
	}
	request := &model.InvoiceRequest{
		UserId: applicant.Id, Username: applicant.Username, Region: model.InvoiceRegionHongKong,
		CompanyName: "海港貿易有限公司 Harbour Trading Limited", TotalMoney: 21000, TotalMoneyCents: 2100000,
		Status: model.InvoiceStatusPending,
	}
	require.NoError(t, model.DB.Create(request).Error)

	content, number, err := RenderInvoiceHongKongBill(request, orders, time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("88API-HK-20261028-%06d", request.Id), number, "the bill is dated in Hong Kong time")
	reader, err := pdf.NewReader(bytes.NewReader(content), int64(len(content)))
	require.NoError(t, err)
	assert.Greater(t, reader.NumPage(), 1)
	plain, err := reader.GetPlainText()
	require.NoError(t, err)
	text, err := io.ReadAll(plain)
	require.NoError(t, err)
	for _, want := range []string{
		number, "2026-10-28", "DAYFOLD HK LIMITED", "78129797", "CENTRAL PLAZA", "Harbour Trading Limited",
		"CNY 875.00", "CNY 21,000.00", orders[0].TradeNo, orders[23].TradeNo,
	} {
		assert.Contains(t, string(text), want)
	}
	_, _, err = RenderInvoiceHongKongBill(&model.InvoiceRequest{Region: model.InvoiceRegionMainland}, orders, time.Now())
	assert.Error(t, err, "mainland requests get a VAT e-invoice from finance")

	issued, err := IssueInvoiceHongKongBill(context.Background(), request, orders)
	require.NoError(t, err)
	assert.Equal(t, model.InvoiceStatusIssued, issued.Status)
	files, err := model.ListInvoiceFiles(request.Id)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Regexp(t, fmt.Sprintf(`^帳單-88API-HK-\d{8}-%06d\.pdf$`, request.Id), files[0].FileName)
	assert.Equal(t, "application/pdf", files[0].MimeType)

	var deliveries []*model.InvoiceNotificationDelivery
	require.NoError(t, model.DB.Where("invoice_request_id = ?", request.Id).Order("id ASC").Find(&deliveries).Error)
	require.Len(t, deliveries, 2)
	assert.Equal(t, model.InvoiceNotificationKindUserEmail, deliveries[0].Kind)
	assert.Equal(t, "accounts@harbour.example", deliveries[0].Recipient)
	assert.Equal(t, model.InvoiceNotificationKindWechatGroup, deliveries[1].Kind)
	deliverPendingInvoiceWechatNotifications()
	messages := api.received()
	require.Len(t, messages, 1)
	assert.Equal(t, "【帳單已自动开具 #"+strconv.Itoa(request.Id)+"】\n"+
		"开票类型：帳單 INVOICE（中国香港）\n"+
		"收件人：海港貿易有限公司 Harbour Trading Limited\n"+
		"开票金额：¥21,000.00\n"+
		"帳單文件："+files[0].FileName+"\n"+
		"系统已生成帳單并发给用户，无需人工开具。", messages[0]["content"])

	// A bill that cannot be generated leaves the request to finance.
	failed := &model.InvoiceRequest{
		UserId: applicant.Id, Username: applicant.Username, Region: model.InvoiceRegionHongKong,
		CompanyName: "Kowloon Supplies Limited", TotalMoney: 600, TotalMoneyCents: 60000, Status: model.InvoiceStatusPending,
	}
	require.NoError(t, model.DB.Create(failed).Error)
	require.NoError(t, QueueInvoiceRequestCreatedNotifications(failed))
	deliverPendingInvoiceWechatNotifications()
	messages = api.received()
	require.Len(t, messages, 2)
	assert.True(t, strings.HasPrefix(messages[1]["content"], "【新开票申请 #"+strconv.Itoa(failed.Id)+"】"))
	assert.True(t, strings.HasSuffix(messages[1]["content"], "\n系统自动生成帳單失败，请人工开具。"))
}

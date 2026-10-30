package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
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
	pendingRequest := func(user *model.User, company string) *model.InvoiceRequest {
		request := &model.InvoiceRequest{
			UserId: user.Id, Username: user.Username, CompanyName: company, TaxNumber: "91310000TEST",
			TotalMoney: 1200, TotalMoneyCents: 120000, Status: model.InvoiceStatusPending,
		}
		require.NoError(t, model.DB.Create(request).Error)
		return request
	}
	yunfan := pendingRequest(applicant, "上海云帆网络信息技术有限公司")
	acmeFirst := pendingRequest(other, "Acme Trading Co")
	acmeSecond := pendingRequest(other, "Acme Trading Co")
	retried := pendingRequest(other, "重试科技有限公司")
	yunfanSuffix := pendingRequest(other, "云帆网络信息技术有限公司")
	headOffice := pendingRequest(other, "成都蜀道智能科技有限公司")
	branch := pendingRequest(other, "成都蜀道智能科技有限公司北京分公司")
	pdf := []byte("%PDF-1.7\n% issued invoice")
	digest := sha256.Sum256(pdf)
	require.NoError(t, model.DB.Create(&model.InvoiceFile{
		InvoiceRequestId: retried.Id, FileName: "重试科技有限公司.pdf", StoredName: "attached.pdf", MimeType: "application/pdf",
		Size: int64(len(pdf)), StorageType: "local", StorageKey: "attached.pdf", Sha256: hex.EncodeToString(digest[:]),
	}).Error)

	issue := func(fileName string, content []byte) *InvoiceWechatFileResult {
		result, err := IssueInvoiceFromWechatFile(context.Background(), fileName, content)
		require.NoError(t, err)
		return result
	}
	assert.Equal(t, InvoiceWechatFileNoMatch, issue("未知公司.pdf", pdf).Outcome)
	assert.Equal(t, InvoiceWechatFileAmbiguous, issue("acme trading co.pdf", pdf).Outcome)
	assert.Equal(t, InvoiceWechatFileNotPDF, issue("Acme Trading Co.pdf", []byte("plain text")).Outcome)
	assert.Equal(t, InvoiceWechatFileNotPDF, issue("Acme Trading Co.png", pdf).Outcome)
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: acmeSecond.Id},
		issue("Acme Trading Co#"+strconv.Itoa(acmeSecond.Id)+".pdf", pdf))
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: yunfan.Id},
		issue("上海云帆网络信息技术有限公司(1).pdf", pdf))
	assert.Equal(t, InvoiceWechatFileNoMatch, issue("上海云帆网络信息技术有限公司.pdf", pdf).Outcome, "an issued request takes no more files")
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: retried.Id},
		issue("重试科技有限公司.pdf", pdf), "a file attached before a failed issue is reused")
	assert.Equal(t, &InvoiceWechatFileResult{Outcome: InvoiceWechatFileIssued, RequestId: branch.Id},
		issue("dzfp_26442000000012345678_成都蜀道智能科技有限公司北京分公司_20261012.pdf", pdf),
		"the head office name inside the branch name does not count")

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
	}, statuses)
	for _, request := range []*model.InvoiceRequest{yunfan, retried} {
		files, err := model.ListInvoiceFiles(request.Id)
		require.NoError(t, err)
		require.Len(t, files, 1)
		assert.Equal(t, hex.EncodeToString(digest[:]), files[0].Sha256)
	}

	var deliveries []*model.InvoiceNotificationDelivery
	require.NoError(t, model.DB.Where("invoice_request_id = ?", yunfan.Id).Order("id ASC").Find(&deliveries).Error)
	require.Len(t, deliveries, 2)
	assert.Equal(t, model.InvoiceNotificationKindUserEmail, deliveries[0].Kind)
	assert.Equal(t, "finance@yunfan.example", deliveries[0].Recipient)
	assert.Equal(t, model.InvoiceNotificationKindWechatGroup, deliveries[1].Kind)
	assert.Equal(t, "finance@chatroom", deliveries[1].Recipient)

	deliverPendingInvoiceWechatNotifications()
	contents := make([]string, 0)
	for _, message := range api.received() {
		assert.Equal(t, "finance@chatroom", message["toWxid"])
		contents = append(contents, message["content"])
	}
	assert.Len(t, contents, 4, "one completion message per issued request")
	assert.Contains(t, contents, "【开票工单已完成 #"+strconv.Itoa(yunfan.Id)+"】\n"+
		"发票抬头：上海云帆网络信息技术有限公司\n"+
		"开票金额：¥1,200.00\n"+
		"发票文件：上海云帆网络信息技术有限公司(1).pdf")
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

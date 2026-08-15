//go:build integration

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicefile"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/require"
)

// TestInvoiceBusinessEmailWorkflowIntegration exercises the same durable
// business records, storage adapter, notification queue, and delivery worker
// used by the HTTP invoice flow. It is opt-in and additionally requires an
// explicit local-preview confirmation because it intentionally creates data.
func TestInvoiceBusinessEmailWorkflowIntegration(t *testing.T) {
	if os.Getenv("INVOICE_BUSINESS_EMAIL_CONFIRM") != "local-preview" {
		t.Skip("set INVOICE_BUSINESS_EMAIL_CONFIRM=local-preview to run this stateful integration test")
	}
	recipient := strings.TrimSpace(os.Getenv("INVOICE_BUSINESS_EMAIL_RECIPIENT"))
	require.NotEmpty(t, recipient)
	require.Contains(t, strings.ToLower(os.Getenv("SQLITE_PATH")), "invoice-preview")

	common.InitEnv()
	require.NoError(t, model.InitDB())
	model.InitOptionMap()
	require.NoError(t, InitializeInvoiceStorageProfiles())
	require.NotEmpty(t, common.SMTPServer)
	require.NotEmpty(t, common.SMTPFrom)

	require.NoError(t, model.UpdateOptionsBulk(map[string]string{
		"InvoiceApplicationNotifyAdminEnabled": "true",
		"InvoiceIssuedNotifyUserEnabled":       "true",
		"InvoiceAdminEmail":                    recipient,
	}))

	username := strings.TrimSpace(os.Getenv("INVOICE_BUSINESS_TEST_USERNAME"))
	if username == "" {
		username = "invoice_demo"
	}
	var user model.User
	require.NoError(t, model.DB.Where("username = ?", username).First(&user).Error)
	userSettings := user.GetSetting()
	userSettings.Language = appI18n.LangZhCN
	user.SetSetting(userSettings)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Updates(map[string]any{
		"email":   recipient,
		"setting": user.Setting,
	}).Error)
	user.Email = recipient

	now := time.Now()
	tag := now.Format("20060102-150405")
	issuedOrder := createInvoiceBusinessTopUp(t, user.Id, "BUSINESS-ISSUED-"+tag, 688, now.Unix())
	issuedRequest, _, err := model.CreateInvoiceRequestWithNotifications(model.CreateInvoiceRequestParams{
		UserId:         user.Id,
		Username:       user.Username,
		CompanyName:    "88API 邮件业务实测（已开具）",
		TaxNumber:      "91310000EMAILISSUED",
		BankName:       "测试银行",
		BankAccount:    "6222000000000000",
		CompanyAddress: "上海市测试路 88 号",
		CompanyPhone:   "021-88888888",
		Email:          recipient,
		Remark:         "真实业务触发：申请通知与开具通知",
		TopUpOrderIds:  []int{issuedOrder.Id},
	}, BuildInvoiceRequestCreatedNotifications)
	require.NoError(t, err)

	profile, err := invoicefile.EnsureCurrentProfile()
	require.NoError(t, err)
	storage, err := invoicefile.ForProfile(profile.Id, profile.StorageType)
	require.NoError(t, err)
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	require.NoError(t, err)
	storageKey := path.Join(
		strconv.Itoa(issuedRequest.Id),
		fmt.Sprintf("%04d", now.UTC().Year()),
		fmt.Sprintf("%02d", now.UTC().Month()),
		"invoice-business-email-"+tag+".png",
	)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, storage.Put(ctx, storageKey, bytes.NewReader(pngBytes), int64(len(pngBytes)), "image/png"))
	digest := sha256.Sum256(pngBytes)
	require.NoError(t, model.CreateInvoiceFileWithinLimit(&model.InvoiceFile{
		InvoiceRequestId: issuedRequest.Id,
		UploaderId:       user.Id,
		FileName:         "invoice-business-email.png",
		StoredName:       "invoice-business-email-" + tag + ".png",
		MimeType:         "image/png",
		Size:             int64(len(pngBytes)),
		StorageProfileId: profile.Id,
		StorageType:      storage.Kind(),
		StorageKey:       storageKey,
		Sha256:           hex.EncodeToString(digest[:]),
	}, setting.InvoiceFileMaxCount))

	previousStatus := issuedRequest.Status
	_, err = model.UpdateInvoiceRequestStatusWithNotifications(
		issuedRequest.Id,
		model.InvoiceStatusIssued,
		user.Id,
		"",
		func(updated *model.InvoiceRequest) ([]*model.InvoiceNotificationDelivery, error) {
			return BuildInvoiceIssuedNotifications(updated, previousStatus, &user)
		},
	)
	require.NoError(t, err)

	expiryOrder := createInvoiceBusinessTopUp(t, user.Id, "BUSINESS-EXPIRY-"+tag, 788, now.Unix())
	expiryRequest, _, err := model.CreateInvoiceRequestWithNotifications(model.CreateInvoiceRequestParams{
		UserId:        user.Id,
		Username:      user.Username,
		CompanyName:   "88API 邮件业务实测（即将过期）",
		TaxNumber:     "91310000EMAILEXPIRY",
		Email:         recipient,
		Remark:        "真实业务触发：到期前 24 小时管理员提醒",
		TopUpOrderIds: []int{expiryOrder.Id},
	}, nil)
	require.NoError(t, err)

	const previewExpiryDays = 30
	previousExpiryDays := setting.InvoicePendingExpiryDays
	setting.InvoicePendingExpiryDays = previewExpiryDays
	defer func() { setting.InvoicePendingExpiryDays = previousExpiryDays }()
	backdatedCreatedTime := now.Add(-time.Duration(previewExpiryDays-1)*24*time.Hour - time.Hour).Unix()
	require.NoError(t, model.DB.Model(&model.InvoiceRequest{}).Where("id = ?", expiryRequest.Id).Updates(map[string]any{
		"created_time": backdatedCreatedTime,
		"updated_time": backdatedCreatedTime,
	}).Error)
	expiryRequest.CreatedTime = backdatedCreatedTime
	warningCutoff := now.Add(-time.Duration(previewExpiryDays-1) * 24 * time.Hour).Unix()
	expiryCutoff := now.Add(-time.Duration(previewExpiryDays) * 24 * time.Hour).Unix()
	queued, err := model.QueuePendingInvoiceExpiryWarnings(
		warningCutoff,
		expiryCutoff,
		100,
		BuildInvoiceExpiryWarningNotifications,
	)
	require.NoError(t, err)
	require.Equal(t, 1, queued)

	createdDelivery := invoiceBusinessDelivery(t, issuedRequest.Id, model.InvoiceNotificationKindAdminEmail)
	issuedDelivery := invoiceBusinessDelivery(t, issuedRequest.Id, model.InvoiceNotificationKindUserEmail)
	expiryDelivery := invoiceBusinessDelivery(t, expiryRequest.Id, model.InvoiceNotificationKindAdminEmail)
	for _, delivery := range []*model.InvoiceNotificationDelivery{createdDelivery, issuedDelivery, expiryDelivery} {
		require.Equal(t, recipient, delivery.Recipient)
		require.NotContains(t, delivery.Subject, "{{")
		require.NotContains(t, delivery.Body, "{{")
		deliverInvoiceNotification(delivery)
		waitForInvoiceBusinessDelivery(t, delivery.Id)
	}

	t.Logf(
		"business email workflow delivered: issued_request_id=%d expiry_request_id=%d storage=%s",
		issuedRequest.Id,
		expiryRequest.Id,
		storage.Kind(),
	)
}

func createInvoiceBusinessTopUp(t *testing.T, userID int, tradeNo string, money float64, timestamp int64) *model.TopUp {
	t.Helper()
	topUp := &model.TopUp{
		UserId:          userID,
		Amount:          int64(money),
		Money:           money,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodBalance,
		PaymentProvider: model.PaymentProviderBalance,
		CreateTime:      timestamp,
		CompleteTime:    timestamp,
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, topUp.Insert())
	return topUp
}

func invoiceBusinessDelivery(t *testing.T, requestID int, kind string) *model.InvoiceNotificationDelivery {
	t.Helper()
	var delivery model.InvoiceNotificationDelivery
	require.NoError(t, model.DB.Where(
		"invoice_request_id = ? AND kind = ?",
		requestID,
		kind,
	).Order("id DESC").First(&delivery).Error)
	return &delivery
}

func waitForInvoiceBusinessDelivery(t *testing.T, deliveryID int) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var delivery model.InvoiceNotificationDelivery
		require.NoError(t, model.DB.First(&delivery, "id = ?", deliveryID).Error)
		if delivery.DeliveredTime > 0 {
			require.Zero(t, delivery.Attempts)
			require.Empty(t, delivery.LastError)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("invoice notification %d was not delivered", deliveryID)
}

package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testInvoiceNotificationRequest(status int) *model.InvoiceRequest {
	return &model.InvoiceRequest{
		Id:           42,
		UserId:       7,
		Username:     "alice",
		CompanyName:  "示例科技有限公司",
		TaxNumber:    "91310000EXAMPLE",
		Email:        "alice@example.com",
		TotalMoney:   500,
		Status:       status,
		CreatedTime:  1_787_000_000,
		OrderNumbers: "INV-202608-001",
	}
}

func withInvoiceIssuedNotificationSetting(t *testing.T, enabled bool) {
	t.Helper()
	previous := common.InvoiceIssuedNotifyUserEnabled
	common.InvoiceIssuedNotifyUserEnabled = enabled
	t.Cleanup(func() { common.InvoiceIssuedNotifyUserEnabled = previous })
}

func withInvoiceNotificationTestSettings(t *testing.T) {
	t.Helper()
	previousAdminEnabled := common.InvoiceApplicationNotifyAdminEnabled
	previousAdminEmail := common.InvoiceAdminEmail
	previousIssuedEnabled := common.InvoiceIssuedNotifyUserEnabled
	previousSystemName := common.SystemName
	previousServerAddress := system_setting.ServerAddress
	previousExpiryDays := setting.InvoicePendingExpiryDays
	common.InvoiceApplicationNotifyAdminEnabled = true
	common.InvoiceAdminEmail = "billing@example.com, finance@example.com"
	common.InvoiceIssuedNotifyUserEnabled = true
	common.SystemName = "88API"
	system_setting.ServerAddress = "https://88api.ai/"
	setting.InvoicePendingExpiryDays = 30
	t.Cleanup(func() {
		common.InvoiceApplicationNotifyAdminEnabled = previousAdminEnabled
		common.InvoiceAdminEmail = previousAdminEmail
		common.InvoiceIssuedNotifyUserEnabled = previousIssuedEnabled
		common.SystemName = previousSystemName
		system_setting.ServerAddress = previousServerAddress
		setting.InvoicePendingExpiryDays = previousExpiryDays
	})
}

func assertRenderedInvoiceEmail(t *testing.T, delivery *model.InvoiceNotificationDelivery) {
	t.Helper()
	assert.NotEmpty(t, delivery.Subject)
	assert.NotEmpty(t, delivery.Body)
	assert.NotContains(t, delivery.Subject, "{{")
	assert.NotContains(t, delivery.Body, "{{")
	assert.Contains(t, delivery.Body, "88API")
	assert.Contains(t, delivery.Body, "示例科技有限公司")
}

func TestBuildInvoiceRequestCreatedNotificationsRendersAdminTemplate(t *testing.T) {
	withInvoiceNotificationTestSettings(t)
	deliveries, err := BuildInvoiceRequestCreatedNotifications(
		testInvoiceNotificationRequest(model.InvoiceStatusPending),
	)
	require.NoError(t, err)
	require.Len(t, deliveries, 2)
	assert.Equal(t, "billing@example.com", deliveries[0].Recipient)
	assert.Equal(t, "finance@example.com", deliveries[1].Recipient)
	for _, delivery := range deliveries {
		assert.Equal(t, model.InvoiceNotificationKindAdminEmail, delivery.Kind)
		assert.Contains(t, delivery.Subject, "新发票申请 #42")
		assert.Contains(t, delivery.Body, "alice 提交了一份新的发票申请")
		assert.Contains(t, delivery.Body, "https://88api.ai/admin-invoices/42")
		assertRenderedInvoiceEmail(t, delivery)
	}
}

func TestBuildInvoiceExpiryWarningNotificationsRendersAdminTemplate(t *testing.T) {
	withInvoiceNotificationTestSettings(t)
	request := testInvoiceNotificationRequest(model.InvoiceStatusPending)
	deliveries, err := BuildInvoiceExpiryWarningNotifications(request)
	require.NoError(t, err)
	require.Len(t, deliveries, 2)
	for _, delivery := range deliveries {
		assert.Equal(t, model.InvoiceNotificationKindAdminEmail, delivery.Kind)
		assert.Contains(t, delivery.Subject, "发票申请 #42 将在 24 小时内过期")
		assert.Contains(t, delivery.Body, "将在 24 小时内自动过期")
		assert.Contains(t, delivery.Body, "预计过期时间")
		assert.Contains(t, delivery.Body, "https://88api.ai/admin-invoices/42")
		assertRenderedInvoiceEmail(t, delivery)
	}
}

func TestBuildInvoiceIssuedNotificationsRequiresDedicatedSetting(t *testing.T) {
	withInvoiceIssuedNotificationSetting(t, false)
	deliveries, err := BuildInvoiceIssuedNotifications(
		testInvoiceNotificationRequest(model.InvoiceStatusIssued),
		model.InvoiceStatusPending,
		&model.User{Id: 7, Email: "account@example.com"},
	)
	require.NoError(t, err)
	assert.Empty(t, deliveries)
}

func TestBuildInvoiceIssuedNotificationsIgnoresOtherStatuses(t *testing.T) {
	withInvoiceIssuedNotificationSetting(t, true)
	deliveries, err := BuildInvoiceIssuedNotifications(
		testInvoiceNotificationRequest(model.InvoiceStatusRejected),
		model.InvoiceStatusPending,
		&model.User{Id: 7, Email: "alice@example.com"},
	)
	require.NoError(t, err)
	assert.Empty(t, deliveries)
}

func TestBuildInvoiceIssuedNotificationsCreatesOneUserDelivery(t *testing.T) {
	withInvoiceNotificationTestSettings(t)
	user := &model.User{Id: 7, Email: "account@example.com"}
	user.SetSetting(dto.UserSetting{Language: appI18n.LangEn})
	deliveries, err := BuildInvoiceIssuedNotifications(
		testInvoiceNotificationRequest(model.InvoiceStatusIssued),
		model.InvoiceStatusPending,
		user,
	)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, model.InvoiceNotificationKindUser, deliveries[0].Kind)
	assert.Equal(t, "account@example.com", deliveries[0].Recipient)
	assert.Equal(t, 7, deliveries[0].UserId)
	assert.Contains(t, deliveries[0].Subject, "Invoice application #42 has been issued")
	assert.Contains(t, deliveries[0].Body, "electronic invoice is attached")
	assert.Contains(t, deliveries[0].Body, "https://88api.ai/invoices/42")
	assert.NotContains(t, strings.ToLower(deliveries[0].Body), "admin-invoices")
	assertRenderedInvoiceEmail(t, deliveries[0])
}

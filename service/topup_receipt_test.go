package service

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTopUpReceiptTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.EmailDelivery{}))
	prior := model.DB
	model.DB = db
	paymentSetting := operation_setting.GetPaymentSetting()
	priorEnabled, priorMethods := paymentSetting.TopUpReceiptEmailEnabled, paymentSetting.TopUpReceiptPaymentMethods
	t.Cleanup(func() {
		model.DB = prior
		paymentSetting.TopUpReceiptEmailEnabled, paymentSetting.TopUpReceiptPaymentMethods = priorEnabled, priorMethods
		conn, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, conn.Close())
	})
	return db
}

// Each recent paid order of an allowed payment method is queued once, even
// when the scan runs again; old orders, balance conversions, other payment
// methods and accounts without an email are skipped.
func TestRecentTopUpsGetOneReceiptEmailEach(t *testing.T) {
	db := setupTopUpReceiptTest(t)
	now := time.Unix(1795430000, 0)
	withEmail := model.User{Username: "nexight", Password: "unused", Email: "payer@example.invalid", AffCode: "aff-payer"}
	noEmail := model.User{Username: "no-email", Password: "unused", AffCode: "aff-none"}
	require.NoError(t, db.Create(&withEmail).Error)
	require.NoError(t, db.Create(&noEmail).Error)
	recent := now.Add(-10 * time.Minute).Unix()
	topUps := []model.TopUp{
		{UserId: withEmail.Id, TradeNo: "PAID-STRIPE", Money: 1900, PaymentMethod: "88fkstripe", PaymentProvider: model.PaymentProviderEpay, Status: common.TopUpStatusSuccess, CompleteTime: recent},
		{UserId: withEmail.Id, TradeNo: "PAID-ALIPAY", Money: 950, PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay, Status: common.TopUpStatusSuccess, CompleteTime: recent},
		{UserId: withEmail.Id, TradeNo: "OLD-STRIPE", Money: 950, PaymentMethod: "88fkstripe", PaymentProvider: model.PaymentProviderEpay, Status: common.TopUpStatusSuccess, CompleteTime: now.Add(-3 * time.Hour).Unix()},
		{UserId: withEmail.Id, TradeNo: "PENDING-STRIPE", Money: 950, PaymentMethod: "88fkstripe", PaymentProvider: model.PaymentProviderEpay, Status: common.TopUpStatusPending},
		{UserId: withEmail.Id, TradeNo: "BALANCE", Money: 10, PaymentMethod: model.PaymentMethodBalance, PaymentProvider: model.PaymentProviderBalance, Status: common.TopUpStatusSuccess, CompleteTime: recent},
		{UserId: noEmail.Id, TradeNo: "NO-EMAIL", Money: 99, PaymentMethod: "88fkstripe", PaymentProvider: model.PaymentProviderEpay, Status: common.TopUpStatusSuccess, CompleteTime: recent},
	}
	require.NoError(t, db.Create(&topUps).Error)

	paymentSetting := operation_setting.GetPaymentSetting()
	paymentSetting.TopUpReceiptEmailEnabled = false
	queueRecentTopUpReceiptEmails(now)
	var count int64
	require.NoError(t, db.Model(&model.EmailDelivery{}).Count(&count).Error)
	assert.Zero(t, count, "nothing is queued while the setting is off")

	paymentSetting.TopUpReceiptEmailEnabled = true
	paymentSetting.TopUpReceiptPaymentMethods = []string{"88fkstripe"}
	queueRecentTopUpReceiptEmails(now)
	queueRecentTopUpReceiptEmails(now.Add(time.Minute))

	var deliveries []model.EmailDelivery
	require.NoError(t, db.Find(&deliveries).Error)
	require.Len(t, deliveries, 1)
	assert.Equal(t, TopUpReceiptEmailCategory, deliveries[0].Category)
	assert.Equal(t, topUps[0].Id, deliveries[0].RelatedId)
	assert.Equal(t, withEmail.Id, deliveries[0].UserId)
	assert.Equal(t, "payer@example.invalid", deliveries[0].Recipient)
	assert.Contains(t, deliveries[0].Subject, "PAID-STRIPE")
	assert.Contains(t, deliveries[0].Body, "1900.00")

	paymentSetting.TopUpReceiptPaymentMethods = nil
	queueRecentTopUpReceiptEmails(now.Add(2 * time.Minute))
	require.NoError(t, db.Model(&model.EmailDelivery{}).Count(&count).Error)
	assert.EqualValues(t, 2, count, "an empty method list covers every payment method")
}

func TestRenderTopUpReceiptOnlyForPaidOrders(t *testing.T) {
	user := &model.User{Id: 13992, Username: "nexight", Email: "payer@example.invalid"}
	topUp := &model.TopUp{Id: 26659, UserId: user.Id, TradeNo: "USR13992NO5UpC2w1795427480", Money: 1900, PaymentMethod: "88fkstripe",
		PaymentProvider: model.PaymentProviderEpay, Status: common.TopUpStatusSuccess, CompleteTime: 1795427515}
	assert.Equal(t, "88API-RC-20261123-026659", TopUpReceiptNumber(topUp))
	priorIssuer := []string{setting.InvoiceHongKongIssuerName, setting.InvoiceHongKongIssuerRegistrationNo, setting.InvoiceHongKongIssuerAddress}
	setting.InvoiceHongKongIssuerName = "88 DIGITAL TECHNOLOGY LIMITED"
	setting.InvoiceHongKongIssuerRegistrationNo = "80862628"
	setting.InvoiceHongKongIssuerAddress = "RM 18 2/F, TUSPARK, 118 WAI YIP STREET, KWUN TONG, HONG KONG"
	t.Cleanup(func() {
		setting.InvoiceHongKongIssuerName, setting.InvoiceHongKongIssuerRegistrationNo, setting.InvoiceHongKongIssuerAddress = priorIssuer[0], priorIssuer[1], priorIssuer[2]
	})
	for _, lang := range []string{"zh-CN", "zh-TW", "en", "fr", "ja", "ru", "vi"} {
		content, err := RenderTopUpReceipt(topUp, user, lang)
		require.NoError(t, err)
		assert.True(t, bytes.HasPrefix(content, []byte("%PDF")), lang)
		if dir := os.Getenv("TOPUP_RECEIPT_SAMPLE_DIR"); dir != "" {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "receipt-"+lang+".pdf"), content, 0o600))
		}
	}
	pending := *topUp
	pending.Status = common.TopUpStatusPending
	_, err := RenderTopUpReceipt(&pending, user, "zh-CN")
	assert.ErrorIs(t, err, ErrTopUpReceiptUnavailable)
}

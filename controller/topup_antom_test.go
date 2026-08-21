package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

type fakeAntomGateway struct {
	verifyErr     error
	inquiryResult *service.AntomPaymentResult
	inquiryErr    error
	inquiryCalls  int
}

func (gateway *fakeAntomGateway) CreatePaymentSession(input service.AntomPaymentSessionInput) (*service.AntomPaymentSession, error) {
	return nil, errors.New("not implemented")
}

func (gateway *fakeAntomGateway) InquiryPayment(string) (*service.AntomPaymentResult, error) {
	gateway.inquiryCalls++
	return gateway.inquiryResult, gateway.inquiryErr
}

func setupAntomControllerDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousDatabaseType := common.MainDatabaseType()
	previousQuotaPerUnit := common.QuotaPerUnit
	previousRedisEnabled := common.RedisEnabled
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.TopUp{},
		&model.Log{},
		&model.MarketingRecipient{},
		&model.MarketingEvent{},
	))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	model.LOG_DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.QuotaPerUnit = 500000
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SetMainDatabaseType(previousDatabaseType)
		common.QuotaPerUnit = previousQuotaPerUnit
		common.RedisEnabled = previousRedisEnabled
		require.NoError(t, sqlDB.Close())
	})
}

func createAntomControllerOrder(t *testing.T, userID int, tradeNo string) (*model.User, *model.TopUp) {
	t.Helper()
	user := &model.User{Id: userID, Username: "antom-user-" + tradeNo, Quota: 0, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
	order := &model.TopUp{
		UserId:          user.Id,
		Amount:          10,
		Money:           10,
		MoneyMinor:      1000,
		CreditedQuota:   5_000_000,
		Currency:        "CNY",
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodAntom,
		PaymentProvider: model.PaymentProviderAntom,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, model.DB.Create(order).Error)
	return user, order
}

func (gateway *fakeAntomGateway) VerifyWebhook(string, string, string, string, string) error {
	return gateway.verifyErr
}

func configureAntomControllerTest(t *testing.T, gateway service.AntomGateway) {
	t.Helper()
	originalFactory := newAntomGateway
	originalEnabled := setting.AntomEnabled
	originalGatewayURL := setting.AntomGateway
	originalClientID := setting.AntomClientId
	originalPrivateKey := setting.AntomMerchantPrivateKey
	originalPublicKey := setting.AntomPublicKey
	originalServerAddress := system_setting.ServerAddress
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTermsVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		newAntomGateway = originalFactory
		setting.AntomEnabled = originalEnabled
		setting.AntomGateway = originalGatewayURL
		setting.AntomClientId = originalClientID
		setting.AntomMerchantPrivateKey = originalPrivateKey
		setting.AntomPublicKey = originalPublicKey
		system_setting.ServerAddress = originalServerAddress
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTermsVersion
	})

	newAntomGateway = func() (service.AntomGateway, error) { return gateway, nil }
	setting.AntomEnabled = true
	setting.AntomGateway = setting.DefaultAntomGateway
	setting.AntomClientId = "SANDBOX_CLIENT"
	setting.AntomMerchantPrivateKey = "private"
	setting.AntomPublicKey = "public"
	system_setting.ServerAddress = "https://merchant.example"
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
}

func TestAntomMoneyToMinorUnitsRoundsCurrencyAmount(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		expected int64
	}{
		{name: "CNY cents", value: "10", expected: 1000},
		{name: "USD cents", value: "1.235", expected: 124},
		{name: "round down", value: "8.004", expected: 800},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual, err := antomMoneyToMinorUnits(decimal.RequireFromString(testCase.value))
			require.NoError(t, err)
			assert.Equal(t, testCase.expected, actual)
		})
	}

	_, err := antomMoneyToMinorUnits(decimal.Zero)
	require.Error(t, err)
}

func TestGetOptionsHidesAntomKeysAndReturnsConfiguredState(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{
		"AntomMerchantPrivateKey":           "merchant-private-key-material",
		"AntomPublicKey":                    "antom-public-key-material",
		"AntomMerchantPrivateKeyConfigured": "true",
		"AntomPublicKeyConfigured":          "true",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	GetOptions(ctx)

	var response struct {
		Data []model.Option `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	values := make(map[string]string, len(response.Data))
	for _, option := range response.Data {
		values[option.Key] = option.Value
	}
	_, hasPrivateKey := values["AntomMerchantPrivateKey"]
	_, hasPublicKey := values["AntomPublicKey"]
	assert.False(t, hasPrivateKey)
	assert.False(t, hasPublicKey)
	assert.Equal(t, "true", values["AntomMerchantPrivateKeyConfigured"])
	assert.Equal(t, "true", values["AntomPublicKeyConfigured"])
}

func TestAntomPricingReusesPriceGroupRatioAndDiscountForCNYAndUSD(t *testing.T) {
	originalPrice := operation_setting.Price
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalDiscounts := operation_setting.GetPaymentSetting().AmountDiscount
	originalRatios := common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		operation_setting.Price = originalPrice
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalRatios))
	})

	operation_setting.Price = 2.5
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{10: 0.8}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"vip":1.2}`))

	for _, currency := range []string{operation_setting.QuotaDisplayTypeCNY, operation_setting.QuotaDisplayTypeUSD} {
		operation_setting.GetGeneralSetting().QuotaDisplayType = currency
		assert.True(t, decimal.RequireFromString("24").Equal(getPayMoneyDecimal(10, "vip")))
	}
}

func TestAntomNotifyRejectsInvalidSignature(t *testing.T) {
	configureAntomControllerTest(t, &fakeAntomGateway{verifyErr: errors.New("invalid signature")})
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/antom/notify", strings.NewReader(`{"notifyType":"PAYMENT_PENDING"}`))

	AntomNotify(ctx)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestAntomNotifyAcknowledgesPendingPaymentWithoutCrediting(t *testing.T) {
	setupAntomControllerDB(t)
	configureAntomControllerTest(t, &fakeAntomGateway{})
	_, order := createAntomControllerOrder(t, 9801, "ANTOM-PENDING")
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/antom/notify",
		strings.NewReader(`{"notifyType":"PAYMENT_PENDING","paymentRequestId":"ANTOM-PENDING","paymentAmount":{"value":"1000","currency":"CNY"},"result":{"resultCode":"SUCCESS","resultStatus":"S","resultMessage":"pending"}}`),
	)

	AntomNotify(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"result":{"resultCode":"SUCCESS","resultStatus":"S","resultMessage":"success"}}`, recorder.Body.String())
	assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(order.TradeNo).Status)
}

func TestAntomNotifyRejectsUnknownTypeAndMismatchedAmount(t *testing.T) {
	setupAntomControllerDB(t)
	configureAntomControllerTest(t, &fakeAntomGateway{})
	_, _ = createAntomControllerOrder(t, 9802, "ANTOM-VALIDATION")

	testCases := []struct {
		name string
		body string
	}{
		{name: "unknown type", body: `{"notifyType":"CAPTURE_RESULT","paymentRequestId":"ANTOM-VALIDATION","paymentAmount":{"value":"1000","currency":"CNY"},"result":{"resultStatus":"S"}}`},
		{name: "amount mismatch", body: `{"notifyType":"PAYMENT_RESULT","paymentRequestId":"ANTOM-VALIDATION","paymentAmount":{"value":"999","currency":"CNY"},"result":{"resultStatus":"S"}}`},
		{name: "currency mismatch", body: `{"notifyType":"PAYMENT_RESULT","paymentRequestId":"ANTOM-VALIDATION","paymentAmount":{"value":"1000","currency":"USD"},"result":{"resultStatus":"S"}}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/antom/notify", strings.NewReader(testCase.body))
			AntomNotify(ctx)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
		})
	}
}

func TestAntomNotifyMarksFinalFailure(t *testing.T) {
	setupAntomControllerDB(t)
	configureAntomControllerTest(t, &fakeAntomGateway{})
	_, order := createAntomControllerOrder(t, 9804, "ANTOM-FAILED")
	body := `{"notifyType":"PAYMENT_RESULT","paymentRequestId":"ANTOM-FAILED","paymentAmount":{"value":"1000","currency":"CNY"},"paymentMethodType":"ALIPAY_CN","result":{"resultCode":"USER_BALANCE_NOT_ENOUGH","resultStatus":"F","resultMessage":"failed"}}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/antom/notify", strings.NewReader(body))

	AntomNotify(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, common.TopUpStatusFailed, model.GetTopUpByTradeNo(order.TradeNo).Status)
}

func TestAntomNotifyConcurrentSuccessCreditsOnce(t *testing.T) {
	setupAntomControllerDB(t)
	configureAntomControllerTest(t, &fakeAntomGateway{})
	user, order := createAntomControllerOrder(t, 9803, "ANTOM-CONCURRENT-NOTIFY")
	body := `{"notifyType":"PAYMENT_RESULT","paymentRequestId":"ANTOM-CONCURRENT-NOTIFY","paymentAmount":{"value":"1000","currency":"CNY"},"paymentMethodType":"ALIPAY_HK","result":{"resultCode":"SUCCESS","resultStatus":"S","resultMessage":"success"}}`

	const attempts = 8
	codes := make(chan int, attempts)
	var waitGroup sync.WaitGroup
	waitGroup.Add(attempts)
	for range attempts {
		go func() {
			defer waitGroup.Done()
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/antom/notify", strings.NewReader(body))
			AntomNotify(ctx)
			codes <- recorder.Code
		}()
	}
	waitGroup.Wait()
	close(codes)
	for code := range codes {
		assert.Equal(t, http.StatusOK, code)
	}

	reloadedUser := &model.User{}
	require.NoError(t, model.DB.First(reloadedUser, user.Id).Error)
	assert.Equal(t, order.CreditedQuota, reloadedUser.Quota)
	assert.Equal(t, common.TopUpStatusSuccess, model.GetTopUpByTradeNo(order.TradeNo).Status)
}

func TestAntomQueryFallbackAndOwnership(t *testing.T) {
	t.Run("successful inquiry credits owner", func(t *testing.T) {
		setupAntomControllerDB(t)
		gateway := &fakeAntomGateway{inquiryResult: &service.AntomPaymentResult{
			PaymentRequestID: "ANTOM-QUERY-SUCCESS",
			PaymentStatus:    "SUCCESS",
			AmountMinor:      1000,
			Currency:         "CNY",
			PaymentMethod:    "GCASH",
		}}
		configureAntomControllerTest(t, gateway)
		user, order := createAntomControllerOrder(t, 9810, "ANTOM-QUERY-SUCCESS")

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("id", user.Id)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/antom/query", strings.NewReader(`{"trade_no":"ANTOM-QUERY-SUCCESS"}`))
		ctx.Request.Header.Set("Content-Type", "application/json")
		RequestAntomQuery(ctx)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"status":"success"`)
		assert.Equal(t, order.CreditedQuota, func() int {
			var reloaded model.User
			require.NoError(t, model.DB.First(&reloaded, user.Id).Error)
			return reloaded.Quota
		}())
	})

	t.Run("temporary inquiry failure leaves order pending", func(t *testing.T) {
		setupAntomControllerDB(t)
		gateway := &fakeAntomGateway{inquiryErr: errors.New("temporary upstream failure")}
		configureAntomControllerTest(t, gateway)
		user, order := createAntomControllerOrder(t, 9811, "ANTOM-QUERY-PENDING")

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("id", user.Id)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/antom/query", strings.NewReader(`{"trade_no":"ANTOM-QUERY-PENDING"}`))
		ctx.Request.Header.Set("Content-Type", "application/json")
		RequestAntomQuery(ctx)

		assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(order.TradeNo).Status)
		assert.NotContains(t, recorder.Body.String(), `"success":true`)
	})

	t.Run("non owner cannot inquire", func(t *testing.T) {
		setupAntomControllerDB(t)
		gateway := &fakeAntomGateway{inquiryErr: errors.New("must not be called")}
		configureAntomControllerTest(t, gateway)
		_, _ = createAntomControllerOrder(t, 9812, "ANTOM-QUERY-PRIVATE")

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("id", 9999)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/antom/query", strings.NewReader(`{"trade_no":"ANTOM-QUERY-PRIVATE"}`))
		ctx.Request.Header.Set("Content-Type", "application/json")
		RequestAntomQuery(ctx)

		assert.Zero(t, gateway.inquiryCalls)
		assert.NotContains(t, recorder.Body.String(), `"success":true`)
	})
}

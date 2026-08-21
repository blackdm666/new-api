package service

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	antommodel "github.com/alipay/global-open-sdk-go/com/alipay/api/model"
	"github.com/alipay/global-open-sdk-go/com/alipay/api/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

func TestBuildAntomPaymentSessionRequestLeavesWalletSelectionToAntom(t *testing.T) {
	request, params, err := buildAntomPaymentSessionRequest(AntomPaymentSessionInput{
		PaymentRequestID: "ANTOM-ORDER-1",
		AmountMinor:      1000,
		Currency:         "CNY",
		NotifyURL:        "https://merchant.example/api/user/antom/notify",
		RedirectURL:      "https://merchant.example/wallet?trade_no=ANTOM-ORDER-1",
		BuyerReferenceID: "42",
		ClientIP:         "203.0.113.10",
		UserAgent:        "test-agent",
	})

	require.NoError(t, err)
	require.NotNil(t, request)
	require.NotNil(t, params)
	assert.Equal(t, "ANTOM-ORDER-1", params.PaymentRequestId)
	assert.Equal(t, antommodel.CASHIER_PAYMENT, params.ProductCode)
	assert.Equal(t, "CHECKOUT_PAYMENT", params.ProductScene)
	assert.Equal(t, "https://merchant.example/api/user/antom/notify", params.PaymentNotifyUrl)
	assert.Equal(t, "https://merchant.example/wallet?trade_no=ANTOM-ORDER-1", params.PaymentRedirectUrl)
	require.NotNil(t, params.PaymentAmount)
	assert.Equal(t, "1000", params.PaymentAmount.Value)
	assert.Equal(t, "CNY", params.PaymentAmount.Currency)
	require.NotNil(t, params.Order)
	assert.Equal(t, params.PaymentAmount, params.Order.OrderAmount)
	assert.Nil(t, params.PaymentMethod)
	assert.Nil(t, params.AvailablePaymentMethod)
	assert.Empty(t, params.AllowedPaymentMethodRegions)
	assert.Nil(t, params.PaymentQuote)
	assert.Nil(t, params.ProcessingAmount)
}

func TestResolveAntomURLsUsesDefaultsAndPreservesCustomQuery(t *testing.T) {
	originalClientID := setting.AntomClientId
	originalNotifyURL := setting.AntomNotifyURL
	originalRedirectURL := setting.AntomRedirectURL
	originalServerAddress := system_setting.ServerAddress
	t.Cleanup(func() {
		setting.AntomClientId = originalClientID
		setting.AntomNotifyURL = originalNotifyURL
		setting.AntomRedirectURL = originalRedirectURL
		system_setting.ServerAddress = originalServerAddress
	})

	setting.AntomClientId = "SANDBOX_CLIENT"
	setting.AntomNotifyURL = ""
	setting.AntomRedirectURL = ""
	system_setting.ServerAddress = "https://merchant.example/"

	notifyURL, err := ResolveAntomNotifyURL()
	require.NoError(t, err)
	assert.Equal(t, "https://merchant.example/api/user/antom/notify", notifyURL)
	redirectURL, err := ResolveAntomRedirectURL("ORDER-1")
	require.NoError(t, err)
	assert.Equal(t, "https://merchant.example/wallet?pay=pending&trade_no=ORDER-1", redirectURL)

	setting.AntomNotifyURL = "https://hooks.example/antom?source=newapi"
	setting.AntomRedirectURL = "https://app.example/balance?source=newapi"
	notifyURL, err = ResolveAntomNotifyURL()
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.example/antom?source=newapi", notifyURL)
	redirectURL, err = ResolveAntomRedirectURL("ORDER-2")
	require.NoError(t, err)
	assert.Equal(t, "https://app.example/balance?pay=pending&source=newapi&trade_no=ORDER-2", redirectURL)
}

func TestAntomOrderCurrencyAcceptsOnlyCNYAndUSD(t *testing.T) {
	original := operation_setting.GetGeneralSetting().QuotaDisplayType
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = original
	})

	for _, currency := range []string{operation_setting.QuotaDisplayTypeCNY, operation_setting.QuotaDisplayTypeUSD} {
		operation_setting.GetGeneralSetting().QuotaDisplayType = currency
		actual, err := AntomOrderCurrency()
		require.NoError(t, err)
		assert.Equal(t, currency, actual)
	}

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	_, err := AntomOrderCurrency()
	require.Error(t, err)
}

func TestValidateAntomURLRequiresHTTPSExceptSandboxLocalhost(t *testing.T) {
	_, err := validateAntomURL("http://merchant.example/notify", false, false)
	require.Error(t, err)
	_, err = validateAntomURL("http://localhost:3000/notify", false, false)
	require.Error(t, err)

	actual, err := validateAntomURL("http://localhost:3000/notify", true, false)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:3000/notify", actual)

	_, err = validateAntomURL("https://gateway.example/v1", false, true)
	require.Error(t, err)
	_, err = validateAntomURL("https://user:password@gateway.example", false, true)
	require.Error(t, err)
}

func TestSDKAntomGatewayVerifyWebhook(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	privatePEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}))
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)
	publicPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}))

	const (
		path        = "/api/user/antom/notify"
		clientID    = "CLIENT_123"
		requestTime = "1787000000000"
		body        = `{"notifyType":"PAYMENT_RESULT"}`
	)
	signature, err := tools.GenSign("POST", path, clientID, requestTime, body, normalizeAntomKey(privatePEM))
	require.NoError(t, err)

	gateway := &sdkAntomGateway{clientID: clientID, publicKey: normalizeAntomKey(publicPEM)}
	require.NoError(t, gateway.VerifyWebhook(path, clientID, requestTime, body, "algorithm=RSA256,keyVersion=1,signature="+signature))
	require.Error(t, gateway.VerifyWebhook(path, "OTHER_CLIENT", requestTime, body, "algorithm=RSA256,keyVersion=1,signature="+signature))
	require.Error(t, gateway.VerifyWebhook(path, clientID, requestTime, body+" ", "algorithm=RSA256,keyVersion=1,signature="+signature))
}

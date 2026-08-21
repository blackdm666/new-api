package service

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	antomapi "github.com/alipay/global-open-sdk-go/com/alipay/api"
	antommodel "github.com/alipay/global-open-sdk-go/com/alipay/api/model"
	"github.com/alipay/global-open-sdk-go/com/alipay/api/request"
	"github.com/alipay/global-open-sdk-go/com/alipay/api/request/pay"
	antomresponse "github.com/alipay/global-open-sdk-go/com/alipay/api/response/pay"
	"github.com/alipay/global-open-sdk-go/com/alipay/api/tools"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

const antomCheckoutProductScene = "CHECKOUT_PAYMENT"

type AntomPaymentSessionInput struct {
	PaymentRequestID string
	AmountMinor      int64
	Currency         string
	NotifyURL        string
	RedirectURL      string
	BuyerReferenceID string
	ClientIP         string
	UserAgent        string
}

type AntomPaymentSession struct {
	NormalURL  string
	SessionID  string
	ExpiryTime string
}

type AntomPaymentResult struct {
	PaymentRequestID string
	PaymentStatus    string
	AmountMinor      int64
	Currency         string
	PaymentMethod    string
}

type AntomGateway interface {
	CreatePaymentSession(input AntomPaymentSessionInput) (*AntomPaymentSession, error)
	InquiryPayment(paymentRequestID string) (*AntomPaymentResult, error)
	VerifyWebhook(requestPath, clientID, requestTime, body, signature string) error
}

type sdkAntomGateway struct {
	client    *antomapi.DefaultAlipayClient
	clientID  string
	publicKey string
}

func AntomOrderCurrency() (string, error) {
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		return operation_setting.QuotaDisplayTypeCNY, nil
	case operation_setting.QuotaDisplayTypeUSD:
		return operation_setting.QuotaDisplayTypeUSD, nil
	default:
		return "", errors.New("Antom supports only CNY or USD pricing modes")
	}
}

func AntomConfigurationReady() bool {
	if !setting.AntomEnabled {
		return false
	}
	return ValidateAntomConfiguration() == nil
}

func ValidateAntomConfiguration() error {
	if strings.TrimSpace(setting.AntomClientId) == "" {
		return errors.New("Antom client ID is required")
	}
	if strings.TrimSpace(setting.AntomMerchantPrivateKey) == "" {
		return errors.New("Antom merchant private key is required")
	}
	if strings.TrimSpace(setting.AntomPublicKey) == "" {
		return errors.New("Antom public key is required")
	}
	if _, err := AntomOrderCurrency(); err != nil {
		return err
	}

	sandbox := strings.HasPrefix(strings.TrimSpace(setting.AntomClientId), "SANDBOX_")
	if _, err := validateAntomURL(antomGatewayURL(), sandbox, true); err != nil {
		return fmt.Errorf("invalid Antom gateway: %w", err)
	}
	if _, err := ResolveAntomNotifyURL(); err != nil {
		return err
	}
	if _, err := ResolveAntomRedirectURL("configuration-check"); err != nil {
		return err
	}
	return nil
}

func ResolveAntomNotifyURL() (string, error) {
	rawURL := strings.TrimSpace(setting.AntomNotifyURL)
	if rawURL == "" {
		rawURL = strings.TrimRight(system_setting.ServerAddress, "/") + "/api/user/antom/notify"
	}
	sandbox := strings.HasPrefix(strings.TrimSpace(setting.AntomClientId), "SANDBOX_")
	validated, err := validateAntomURL(rawURL, sandbox, false)
	if err != nil {
		return "", fmt.Errorf("invalid Antom notification URL: %w", err)
	}
	return validated, nil
}

func ResolveAntomRedirectURL(paymentRequestID string) (string, error) {
	rawURL := strings.TrimSpace(setting.AntomRedirectURL)
	if rawURL == "" {
		rawURL = strings.TrimRight(system_setting.ServerAddress, "/") + "/wallet"
	}
	sandbox := strings.HasPrefix(strings.TrimSpace(setting.AntomClientId), "SANDBOX_")
	validated, err := validateAntomURL(rawURL, sandbox, false)
	if err != nil {
		return "", fmt.Errorf("invalid Antom redirect URL: %w", err)
	}

	parsed, err := url.Parse(validated)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("pay", "pending")
	query.Set("trade_no", paymentRequestID)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func NewAntomGateway() (AntomGateway, error) {
	if err := ValidateAntomConfiguration(); err != nil {
		return nil, err
	}
	clientID := strings.TrimSpace(setting.AntomClientId)
	privateKey := normalizeAntomKey(setting.AntomMerchantPrivateKey)
	publicKey := normalizeAntomKey(setting.AntomPublicKey)
	client := antomapi.NewDefaultAlipayClient(
		antomGatewayURL(),
		clientID,
		privateKey,
		publicKey,
	)
	return &sdkAntomGateway{
		client:    client,
		clientID:  clientID,
		publicKey: publicKey,
	}, nil
}

func (gateway *sdkAntomGateway) CreatePaymentSession(input AntomPaymentSessionInput) (*AntomPaymentSession, error) {
	request, _, err := buildAntomPaymentSessionRequest(input)
	if err != nil {
		return nil, err
	}

	rawResponse, err := gateway.client.Execute(request)
	if err != nil {
		return nil, err
	}
	response, ok := rawResponse.(*antomresponse.AlipayPaymentSessionResponse)
	if !ok {
		return nil, errors.New("unexpected Antom payment session response")
	}
	resultStatus, resultCode, resultMessage := antomResponseResult(
		response.AlipayResponse.Result.ResultStatus,
		response.AlipayResponse.Result.ResultCode,
		response.AlipayResponse.Result.ResultMessage,
		response.Result,
	)
	if resultStatus != "S" {
		return nil, fmt.Errorf("Antom payment session failed: %s %s", resultCode, resultMessage)
	}
	normalURL := strings.TrimSpace(response.NormalUrl)
	if normalURL == "" {
		normalURL = strings.TrimSpace(response.Url)
	}
	validatedURL, err := validateAntomURL(normalURL, gateway.client.IsSandboxMode, false)
	if err != nil {
		return nil, errors.New("Antom payment session response did not include a valid checkout URL")
	}
	return &AntomPaymentSession{
		NormalURL:  validatedURL,
		SessionID:  response.PaymentSessionId,
		ExpiryTime: response.PaymentSessionExpiryTime,
	}, nil
}

func buildAntomPaymentSessionRequest(input AntomPaymentSessionInput) (*request.AlipayRequest, *pay.AlipayPaymentSessionRequest, error) {
	if input.PaymentRequestID == "" || input.AmountMinor <= 0 || input.Currency == "" {
		return nil, nil, errors.New("invalid Antom payment session input")
	}

	antomRequest, params := pay.NewAlipayPaymentSessionRequest()
	amount := antommodel.NewAmount(strconv.FormatInt(input.AmountMinor, 10), input.Currency)
	params.PaymentRequestId = input.PaymentRequestID
	params.ProductCode = antommodel.CASHIER_PAYMENT
	params.ProductScene = antomCheckoutProductScene
	params.PaymentAmount = amount
	params.PaymentNotifyUrl = input.NotifyURL
	params.PaymentRedirectUrl = input.RedirectURL
	params.Order = &antommodel.Order{
		ReferenceOrderId: input.PaymentRequestID,
		OrderDescription: "Wallet top-up",
		OrderAmount:      amount,
	}
	if input.BuyerReferenceID != "" {
		params.Order.Buyer = &antommodel.Buyer{ReferenceBuyerId: input.BuyerReferenceID}
	}
	params.Env = &antommodel.Env{
		TerminalType: antommodel.WEB,
		ClientIp:     input.ClientIP,
		UserAgent:    input.UserAgent,
	}

	return antomRequest, params, nil
}

func (gateway *sdkAntomGateway) InquiryPayment(paymentRequestID string) (*AntomPaymentResult, error) {
	if strings.TrimSpace(paymentRequestID) == "" {
		return nil, errors.New("payment request ID is required")
	}
	request, params := pay.NewAlipayPayQueryRequest()
	params.PaymentRequestId = paymentRequestID

	rawResponse, err := gateway.client.Execute(request)
	if err != nil {
		return nil, err
	}
	response, ok := rawResponse.(*antomresponse.AlipayPayQueryResponse)
	if !ok {
		return nil, errors.New("unexpected Antom inquiry response")
	}
	resultStatus, resultCode, resultMessage := antomResponseResult(
		response.AlipayResponse.Result.ResultStatus,
		response.AlipayResponse.Result.ResultCode,
		response.AlipayResponse.Result.ResultMessage,
		response.Result,
	)
	if resultStatus != "S" {
		return nil, fmt.Errorf("Antom inquiry failed: %s %s", resultCode, resultMessage)
	}
	amountMinor, currency, err := antomAmount(response.PaymentAmount)
	if err != nil && response.PaymentStatus == antommodel.TransactionStatusType_SUCCESS {
		return nil, err
	}
	return &AntomPaymentResult{
		PaymentRequestID: response.PaymentRequestId,
		PaymentStatus:    string(response.PaymentStatus),
		AmountMinor:      amountMinor,
		Currency:         currency,
		PaymentMethod:    response.PaymentMethodType,
	}, nil
}

func (gateway *sdkAntomGateway) VerifyWebhook(requestPath, clientID, requestTime, body, signature string) error {
	if strings.TrimSpace(clientID) != gateway.clientID {
		return errors.New("Antom webhook client ID mismatch")
	}
	valid, err := tools.CheckSignature(
		requestPath,
		"POST",
		clientID,
		requestTime,
		body,
		signature,
		gateway.publicKey,
	)
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("Antom webhook signature verification failed")
	}
	return nil
}

func antomAmount(amount *antommodel.Amount) (int64, string, error) {
	if amount == nil || strings.TrimSpace(amount.Currency) == "" || strings.TrimSpace(amount.Value) == "" {
		return 0, "", errors.New("Antom response did not include a payment amount")
	}
	value, err := strconv.ParseInt(amount.Value, 10, 64)
	if err != nil || value <= 0 {
		return 0, "", errors.New("Antom response included an invalid payment amount")
	}
	return value, strings.ToUpper(strings.TrimSpace(amount.Currency)), nil
}

func antomResponseResult(status, code, message string, fallback *antommodel.Result) (string, string, string) {
	if fallback != nil && fallback.ResultStatus != "" {
		return string(fallback.ResultStatus), fallback.ResultCode, fallback.ResultMessage
	}
	return status, code, message
}

func antomGatewayURL() string {
	gateway := strings.TrimSpace(setting.AntomGateway)
	if gateway == "" {
		gateway = setting.DefaultAntomGateway
	}
	return strings.TrimRight(gateway, "/")
}

func normalizeAntomKey(value string) string {
	cleaned := strings.TrimSpace(value)
	for _, marker := range []string{
		"-----BEGIN PRIVATE KEY-----",
		"-----END PRIVATE KEY-----",
		"-----BEGIN PUBLIC KEY-----",
		"-----END PUBLIC KEY-----",
	} {
		cleaned = strings.ReplaceAll(cleaned, marker, "")
	}
	return strings.Join(strings.Fields(cleaned), "")
}

func validateAntomURL(rawURL string, sandbox bool, originOnly bool) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", err
	}
	if parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("URL must include a host and must not include credentials or fragments")
	}
	isLocalhost := parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1"
	if parsed.Scheme != "https" && !(sandbox && isLocalhost && parsed.Scheme == "http") {
		return "", errors.New("URL must use HTTPS; sandbox localhost may use HTTP")
	}
	if originOnly && (parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "") {
		return "", errors.New("URL must be an origin without a path or query")
	}
	return parsed.String(), nil
}

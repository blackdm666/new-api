package service

import (
	"context"
	"crypto"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const affiliateAlipayGateway = "https://openapi.alipay.com/gateway.do"

const (
	affiliateAlipayProductCode           = "TRANS_ACCOUNT_NO_PWD"
	affiliateAlipayBizScene              = "DIRECT_TRANSFER"
	affiliateAlipayTransferSceneName     = "业务结算"
	affiliateAlipayTransferSceneInfoType = "结算款项名称"
	affiliateAlipayIdentityType          = "ALIPAY_LOGON_ID"
)

type AffiliateAlipayAPIError struct {
	Code      string
	SubCode   string
	Message   string
	Ambiguous bool
}

func (err *AffiliateAlipayAPIError) Error() string {
	code := err.DiagnosticCode()
	if code == "" {
		code = "UNKNOWN"
	}
	if err.Message == "" {
		return fmt.Sprintf("Alipay payout failed (%s)", code)
	}
	return fmt.Sprintf("Alipay payout failed (%s): %s", code, err.Message)
}

func (err *AffiliateAlipayAPIError) DiagnosticCode() string {
	if err == nil {
		return ""
	}
	if strings.TrimSpace(err.SubCode) != "" {
		return strings.TrimSpace(err.SubCode)
	}
	return strings.TrimSpace(err.Code)
}

type affiliateAlipayClient struct {
	appId            string
	privateKey       *rsa.PrivateKey
	publicKey        *rsa.PublicKey
	appCertSN        string
	alipayCertSN     string
	alipayRootCertSN string
	endpoint         string
	httpClient       *http.Client
	now              func() time.Time
}

type affiliateAlipayResponse struct {
	Code           string `json:"code"`
	Msg            string `json:"msg"`
	SubCode        string `json:"sub_code"`
	SubMsg         string `json:"sub_msg"`
	OutBizNo       string `json:"out_biz_no"`
	OrderId        string `json:"order_id"`
	PayFundOrderId string `json:"pay_fund_order_id"`
	Status         string `json:"status"`
	PayDate        string `json:"pay_date"`
	TransDate      string `json:"trans_date"`
	ErrorCode      string `json:"error_code"`
	FailReason     string `json:"fail_reason"`
}

type AffiliateAlipayPayoutResult struct {
	Status         string
	OrderId        string
	PayFundOrderId string
}

func newAffiliateAlipayClient(config *model.AffiliateAlipayPayoutConfig, endpoint string, httpClient *http.Client) (*affiliateAlipayClient, error) {
	if config == nil || !config.Configured() {
		return nil, model.ErrAffiliateAlipayNotConfigured
	}
	privateKey, err := parseAffiliateAlipayPrivateKey(config.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parse Alipay private key: %w", err)
	}
	appCertificate, err := parseAffiliateAlipayCertificate(config.AppCertificate)
	if err != nil {
		return nil, fmt.Errorf("parse Alipay application certificate: %w", err)
	}
	applicationPublicKey, ok := appCertificate.PublicKey.(*rsa.PublicKey)
	if !ok || applicationPublicKey.E != privateKey.PublicKey.E || applicationPublicKey.N.Cmp(privateKey.PublicKey.N) != 0 {
		return nil, errors.New("Alipay application certificate does not match the application private key")
	}
	alipayCertificates, err := parseAffiliateAlipayCertificates(config.AlipayPublicCertificate)
	if err != nil {
		return nil, fmt.Errorf("parse Alipay public certificate: %w", err)
	}
	// Alipay's official PHP Easy SDK accepts a certificate chain file and uses
	// the first certificate for response verification. Keep the same behavior
	// so the certificate file exported by Alipay can be pasted without splitting.
	alipayCertificate := alipayCertificates[0]
	publicKey, ok := alipayCertificate.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("Alipay public certificate does not contain an RSA public key")
	}
	rootCertificates, err := parseAffiliateAlipayRootCertificates(config.AlipayRootCertificate)
	if err != nil {
		return nil, fmt.Errorf("parse Alipay root certificate: %w", err)
	}
	appCertSN := affiliateAlipayCertificateSN(appCertificate)
	alipayCertSN := affiliateAlipayCertificateSN(alipayCertificate)
	alipayRootCertSN := affiliateAlipayRootCertificateSN(rootCertificates)
	if appCertSN == "" || alipayCertSN == "" || alipayRootCertSN == "" {
		return nil, errors.New("Alipay certificate serial number is unavailable")
	}
	if endpoint == "" {
		endpoint = affiliateAlipayGateway
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &affiliateAlipayClient{
		appId:            config.AppId,
		privateKey:       privateKey,
		publicKey:        publicKey,
		appCertSN:        appCertSN,
		alipayCertSN:     alipayCertSN,
		alipayRootCertSN: alipayRootCertSN,
		endpoint:         endpoint,
		httpClient:       httpClient,
		now:              time.Now,
	}, nil
}

func (client *affiliateAlipayClient) transfer(ctx context.Context, payout *model.AffiliatePayout, title string) (*AffiliateAlipayPayoutResult, error) {
	response, err := client.execute(ctx, "alipay.fund.trans.uni.transfer", map[string]interface{}{
		"out_biz_no":   payout.PaymentReference,
		"trans_amount": affiliatePayoutAmount(payout.AmountCents),
		"product_code": affiliateAlipayProductCode,
		"biz_scene":    affiliateAlipayBizScene,
		"order_title":  title,
		"payee_info": map[string]interface{}{
			"identity":      payout.Account,
			"identity_type": affiliateAlipayIdentityType,
			"name":          payout.AccountName,
		},
		"remark":              title,
		"transfer_scene_name": affiliateAlipayTransferSceneName,
		"transfer_scene_report_infos": []map[string]string{
			{
				"info_type":    affiliateAlipayTransferSceneInfoType,
				"info_content": title,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	status := strings.ToUpper(strings.TrimSpace(response.Status))
	if status == "" {
		status = "UNKNOWN"
	}
	if status == "FAIL" || status == "REFUND" {
		message := strings.TrimSpace(response.FailReason)
		if message == "" {
			message = "Alipay reported a failed transfer"
		}
		return nil, &AffiliateAlipayAPIError{
			Code:    response.ErrorCode,
			SubCode: response.ErrorCode,
			Message: message,
		}
	}
	return &AffiliateAlipayPayoutResult{
		Status:         status,
		OrderId:        response.OrderId,
		PayFundOrderId: response.PayFundOrderId,
	}, nil
}

func (client *affiliateAlipayClient) query(ctx context.Context, reference string) (*AffiliateAlipayPayoutResult, error) {
	response, err := client.execute(ctx, "alipay.fund.trans.common.query", map[string]interface{}{
		"out_biz_no":   reference,
		"product_code": affiliateAlipayProductCode,
		"biz_scene":    affiliateAlipayBizScene,
	})
	if err != nil {
		return nil, err
	}
	status := strings.ToUpper(strings.TrimSpace(response.Status))
	if status == "" {
		status = "UNKNOWN"
	}
	if status == "FAIL" {
		return nil, &AffiliateAlipayAPIError{
			Code:    response.ErrorCode,
			SubCode: response.ErrorCode,
			Message: response.FailReason,
		}
	}
	return &AffiliateAlipayPayoutResult{
		Status:         status,
		OrderId:        response.OrderId,
		PayFundOrderId: response.PayFundOrderId,
	}, nil
}

func (client *affiliateAlipayClient) execute(ctx context.Context, method string, bizContent map[string]interface{}) (*affiliateAlipayResponse, error) {
	bizJSON, err := common.Marshal(bizContent)
	if err != nil {
		return nil, err
	}
	params := map[string]string{
		"app_id":      client.appId,
		"biz_content": string(bizJSON),
		"charset":     "utf-8",
		"format":      "JSON",
		"method":      method,
		"sign_type":   "RSA2",
		"timestamp":   client.now().In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02 15:04:05"),
		"version":     "1.0",
	}
	if client.appCertSN != "" {
		params["app_cert_sn"] = client.appCertSN
		params["alipay_root_cert_sn"] = client.alipayRootCertSN
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	canonical := make([]string, 0, len(keys))
	for _, key := range keys {
		canonical = append(canonical, key+"="+params[key])
	}
	digest := sha256.Sum256([]byte(strings.Join(canonical, "&")))
	signature, err := rsa.SignPKCS1v15(rand.Reader, client.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	form.Set("sign", base64.StdEncoding.EncodeToString(signature))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, &AffiliateAlipayAPIError{Message: "network request failed", Ambiguous: true}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, &AffiliateAlipayAPIError{Message: "read response failed", Ambiguous: true}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, &AffiliateAlipayAPIError{Code: fmt.Sprintf("HTTP_%d", response.StatusCode), Message: "gateway returned a non-success status", Ambiguous: response.StatusCode >= 500}
	}
	var envelope map[string]json.RawMessage
	if err := common.Unmarshal(body, &envelope); err != nil {
		return nil, &AffiliateAlipayAPIError{Message: "invalid gateway response", Ambiguous: true}
	}
	responseKey := strings.ReplaceAll(method, ".", "_") + "_response"
	rawResponse := envelope[responseKey]
	if len(rawResponse) == 0 {
		return nil, &AffiliateAlipayAPIError{Message: "gateway response payload is missing", Ambiguous: true}
	}
	var encodedSignature string
	if rawSignature := envelope["sign"]; len(rawSignature) > 0 {
		_ = common.Unmarshal(rawSignature, &encodedSignature)
	}
	if client.alipayCertSN != "" {
		var responseCertSN string
		if rawCertSN := envelope["alipay_cert_sn"]; len(rawCertSN) > 0 {
			_ = common.Unmarshal(rawCertSN, &responseCertSN)
		}
		if responseCertSN == "" {
			return nil, &AffiliateAlipayAPIError{Code: "RESPONSE_CERT_SN_MISSING", Message: "Alipay certificate serial number is missing", Ambiguous: true}
		}
		if responseCertSN != client.alipayCertSN {
			return nil, &AffiliateAlipayAPIError{Code: "RESPONSE_CERT_SN_MISMATCH", Message: "Alipay response certificate does not match the configured certificate", Ambiguous: true}
		}
	}
	if err := verifyAffiliateAlipayResponse(client.publicKey, rawResponse, encodedSignature); err != nil {
		return nil, err
	}
	parsed := &affiliateAlipayResponse{}
	if err := common.Unmarshal(rawResponse, parsed); err != nil {
		return nil, &AffiliateAlipayAPIError{Message: "invalid signed response", Ambiguous: true}
	}
	if parsed.Code != "10000" {
		message := parsed.SubMsg
		if message == "" {
			message = parsed.Msg
		}
		return nil, &AffiliateAlipayAPIError{
			Code:      parsed.Code,
			SubCode:   parsed.SubCode,
			Message:   message,
			Ambiguous: affiliateAlipayErrorIsAmbiguous(parsed.Code, parsed.SubCode),
		}
	}
	return parsed, nil
}

func ExecuteAffiliateAlipayPayout(ctx context.Context, payoutId int, operatorId int) (*model.AffiliatePayout, error) {
	config, err := model.GetAffiliateAlipayPayoutConfig()
	if err != nil {
		return nil, err
	}
	if !config.Enabled || !config.Configured() {
		return nil, model.ErrAffiliateAlipayNotConfigured
	}
	client, err := newAffiliateAlipayClient(config, "", nil)
	if err != nil {
		return nil, err
	}
	payout, _, err := model.BeginAffiliatePayoutDisbursement(payoutId, operatorId)
	if err != nil {
		return nil, err
	}
	if payout.Status == model.AffiliatePayoutStatusPaid {
		return payout, nil
	}

	// Match the proven certificate implementation: query the deterministic
	// merchant reference before every transfer attempt. Existing success or
	// processing states are reconciled locally; only a signed ORDER_NOT_EXIST
	// response is allowed to proceed to the transfer API.
	existing, queryErr := client.query(ctx, payout.PaymentReference)
	if queryErr == nil {
		if existing.Status == "SUCCESS" {
			if err := model.CompleteAffiliatePayoutDisbursement(payout.Id, operatorId, payout.PaymentReference, existing.OrderId, existing.PayFundOrderId, existing.Status); err != nil {
				return nil, err
			}
			return model.GetAffiliatePayoutById(payout.Id)
		}
		if err := model.UpdateAffiliatePayoutDisbursementProcessing(payout.Id, payout.PaymentReference, existing.Status, existing.OrderId, "", ""); err != nil {
			return nil, err
		}
		return model.GetAffiliatePayoutById(payout.Id)
	}
	queryAPIError := &AffiliateAlipayAPIError{}
	if !errors.As(queryErr, &queryAPIError) || queryAPIError.SubCode != "ORDER_NOT_EXIST" {
		if errors.As(queryErr, &queryAPIError) && queryAPIError.Ambiguous {
			if stateErr := model.UpdateAffiliatePayoutDisbursementProcessing(payout.Id, payout.PaymentReference, "UNKNOWN", payout.ProviderOrderId, queryAPIError.DiagnosticCode(), queryAPIError.Message); stateErr != nil {
				return nil, stateErr
			}
			return model.GetAffiliatePayoutById(payout.Id)
		}
		if errors.As(queryErr, &queryAPIError) {
			if stateErr := model.FailAffiliatePayoutDisbursement(payout.Id, payout.PaymentReference, queryAPIError.DiagnosticCode(), queryAPIError.Message); stateErr != nil {
				return nil, stateErr
			}
		}
		return nil, queryErr
	}

	result, transferErr := client.transfer(ctx, payout, config.TransferTitle)
	if transferErr == nil {
		if result.Status == "SUCCESS" {
			if err := model.CompleteAffiliatePayoutDisbursement(payout.Id, operatorId, payout.PaymentReference, result.OrderId, result.PayFundOrderId, result.Status); err != nil {
				return nil, err
			}
			return model.GetAffiliatePayoutById(payout.Id)
		}
		if err := model.UpdateAffiliatePayoutDisbursementProcessing(payout.Id, payout.PaymentReference, result.Status, result.OrderId, "", ""); err != nil {
			return nil, err
		}
		return model.GetAffiliatePayoutById(payout.Id)
	}
	transferAPIError := &AffiliateAlipayAPIError{}
	if errors.As(transferErr, &transferAPIError) && !transferAPIError.Ambiguous {
		if stateErr := model.FailAffiliatePayoutDisbursement(payout.Id, payout.PaymentReference, transferAPIError.DiagnosticCode(), transferAPIError.Message); stateErr != nil {
			return nil, stateErr
		}
		return nil, transferErr
	}
	if stateErr := model.UpdateAffiliatePayoutDisbursementProcessing(payout.Id, payout.PaymentReference, "UNKNOWN", "", transferAPIError.DiagnosticCode(), transferAPIError.Message); stateErr != nil {
		return nil, stateErr
	}
	return model.GetAffiliatePayoutById(payout.Id)
}

func RefreshAffiliateAlipayPayout(ctx context.Context, payoutId int, operatorId int) (*model.AffiliatePayout, error) {
	config, err := model.GetAffiliateAlipayPayoutConfig()
	if err != nil {
		return nil, err
	}
	if !config.Enabled || !config.Configured() {
		return nil, model.ErrAffiliateAlipayNotConfigured
	}
	client, err := newAffiliateAlipayClient(config, "", nil)
	if err != nil {
		return nil, err
	}
	payout, err := model.GetAffiliatePayoutById(payoutId)
	if err != nil {
		return nil, err
	}
	if payout.Status == model.AffiliatePayoutStatusPaid {
		return payout, nil
	}
	if payout.Status != model.AffiliatePayoutStatusProcessing || payout.DisbursementMode != model.AffiliatePayoutDisbursementAlipayDirect {
		return nil, model.ErrAffiliatePayoutStatusInvalid
	}
	return refreshAffiliateAlipayPayoutWithClient(ctx, client, payout, operatorId)
}

func refreshAffiliateAlipayPayoutWithClient(ctx context.Context, client *affiliateAlipayClient, payout *model.AffiliatePayout, operatorId int) (*model.AffiliatePayout, error) {
	result, err := client.query(ctx, payout.PaymentReference)
	if err != nil {
		apiErr := &AffiliateAlipayAPIError{}
		if errors.As(err, &apiErr) {
			if apiErr.SubCode == "ORDER_NOT_EXIST" || apiErr.Ambiguous {
				if stateErr := model.UpdateAffiliatePayoutDisbursementProcessing(payout.Id, payout.PaymentReference, "UNKNOWN", payout.ProviderOrderId, apiErr.DiagnosticCode(), apiErr.Message); stateErr != nil {
					return nil, stateErr
				}
				return model.GetAffiliatePayoutById(payout.Id)
			}
			if stateErr := model.FailAffiliatePayoutDisbursement(payout.Id, payout.PaymentReference, apiErr.DiagnosticCode(), apiErr.Message); stateErr != nil {
				return nil, stateErr
			}
		}
		return nil, err
	}
	if result.Status == "SUCCESS" {
		if err := model.CompleteAffiliatePayoutDisbursement(payout.Id, operatorId, payout.PaymentReference, result.OrderId, result.PayFundOrderId, result.Status); err != nil {
			return nil, err
		}
		return model.GetAffiliatePayoutById(payout.Id)
	}
	if result.Status == "FAIL" {
		if stateErr := model.FailAffiliatePayoutDisbursement(payout.Id, payout.PaymentReference, "TRANSFER_FAIL", "Alipay reported a failed transfer"); stateErr != nil {
			return nil, stateErr
		}
		return nil, &AffiliateAlipayAPIError{SubCode: "TRANSFER_FAIL", Message: "Alipay reported a failed transfer"}
	}
	if err := model.UpdateAffiliatePayoutDisbursementProcessing(payout.Id, payout.PaymentReference, result.Status, result.OrderId, "", ""); err != nil {
		return nil, err
	}
	return model.GetAffiliatePayoutById(payout.Id)
}

func TestAffiliateAlipayPayoutConfig(ctx context.Context) error {
	config, err := model.GetAffiliateAlipayPayoutConfig()
	if err != nil {
		return err
	}
	if !config.Configured() {
		return model.ErrAffiliateAlipayNotConfigured
	}
	client, err := newAffiliateAlipayClient(config, "", nil)
	if err != nil {
		return err
	}
	reference := fmt.Sprintf("AFFCFG-%d", time.Now().UnixNano())
	_, err = client.query(ctx, reference)
	apiErr := &AffiliateAlipayAPIError{}
	if errors.As(err, &apiErr) && apiErr.SubCode == "ORDER_NOT_EXIST" {
		return nil
	}
	return err
}

func ValidateAffiliateAlipayPayoutKeyMaterial(privateKey string, appCertificate string, alipayPublicCertificate string, alipayRootCertificate string) error {
	if strings.TrimSpace(privateKey) != "" {
		if _, err := parseAffiliateAlipayPrivateKey(privateKey); err != nil {
			return model.ErrAffiliateAlipayConfigInvalid
		}
	}
	if strings.TrimSpace(appCertificate) != "" {
		if _, err := parseAffiliateAlipayCertificate(appCertificate); err != nil {
			return model.ErrAffiliateAlipayConfigInvalid
		}
	}
	if strings.TrimSpace(alipayPublicCertificate) != "" {
		certificates, err := parseAffiliateAlipayCertificates(alipayPublicCertificate)
		if err != nil {
			return model.ErrAffiliateAlipayConfigInvalid
		}
		if _, ok := certificates[0].PublicKey.(*rsa.PublicKey); !ok {
			return model.ErrAffiliateAlipayConfigInvalid
		}
	}
	if strings.TrimSpace(alipayRootCertificate) != "" {
		certificates, err := parseAffiliateAlipayRootCertificates(alipayRootCertificate)
		if err != nil || affiliateAlipayRootCertificateSN(certificates) == "" {
			return model.ErrAffiliateAlipayConfigInvalid
		}
	}
	return nil
}

func parseAffiliateAlipayPrivateKey(value string) (*rsa.PrivateKey, error) {
	der, err := decodeAffiliateAlipayKey(value, "PRIVATE KEY")
	if err != nil {
		return nil, err
	}
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		if privateKey, ok := key.(*rsa.PrivateKey); ok {
			return privateKey, nil
		}
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	return nil, errors.New("unsupported RSA private key")
}

func parseAffiliateAlipayCertificate(value string) (*x509.Certificate, error) {
	certificates, err := parseAffiliateAlipayCertificates(value)
	if err != nil {
		return nil, err
	}
	if len(certificates) != 1 {
		return nil, errors.New("expected one X.509 certificate")
	}
	return certificates[0], nil
}

func parseAffiliateAlipayCertificates(value string) ([]*x509.Certificate, error) {
	return parseAffiliateAlipayCertificateBundle(value, false)
}

func parseAffiliateAlipayRootCertificates(value string) ([]*x509.Certificate, error) {
	return parseAffiliateAlipayCertificateBundle(value, true)
}

func parseAffiliateAlipayCertificateBundle(value string, skipUnsupported bool) ([]*x509.Certificate, error) {
	remaining := []byte(strings.TrimSpace(value))
	certificates := make([]*x509.Certificate, 0, 3)
	var firstParseError error
	for len(remaining) > 0 {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}
		remaining = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			if !skipUnsupported {
				return nil, err
			}
			if firstParseError == nil {
				firstParseError = err
			}
			continue
		}
		certificates = append(certificates, certificate)
	}
	if len(certificates) == 0 {
		if firstParseError != nil {
			return nil, firstParseError
		}
		return nil, errors.New("no X.509 certificate found")
	}
	return certificates, nil
}

func affiliateAlipayCertificateSN(certificate *x509.Certificate) string {
	if certificate == nil {
		return ""
	}
	attributes := certificate.Issuer.Names
	principal := make([]string, 0, len(attributes))
	for index := len(attributes) - 1; index >= 0; index-- {
		attribute := attributes[index]
		shortName := affiliateAlipayAttributeShortName(attribute.Type.String())
		principal = append(principal, shortName+"="+fmt.Sprint(attribute.Value))
	}
	digest := md5.Sum([]byte(strings.Join(principal, ",") + certificate.SerialNumber.String()))
	return fmt.Sprintf("%x", digest)
}

func affiliateAlipayRootCertificateSN(certificates []*x509.Certificate) string {
	serialNumbers := make([]string, 0, len(certificates))
	for _, certificate := range certificates {
		if !affiliateAlipayRSASignatureAlgorithm(certificate.SignatureAlgorithm) {
			continue
		}
		serialNumbers = append(serialNumbers, affiliateAlipayCertificateSN(certificate))
	}
	return strings.Join(serialNumbers, "_")
}

func affiliateAlipayAttributeShortName(oid string) string {
	switch oid {
	case "2.5.4.3":
		return "CN"
	case "2.5.4.6":
		return "C"
	case "2.5.4.7":
		return "L"
	case "2.5.4.8":
		return "ST"
	case "2.5.4.9":
		return "STREET"
	case "2.5.4.10":
		return "O"
	case "2.5.4.11":
		return "OU"
	case "2.5.4.17":
		return "postalCode"
	default:
		return oid
	}
}

func affiliateAlipayRSASignatureAlgorithm(algorithm x509.SignatureAlgorithm) bool {
	switch algorithm {
	case x509.MD2WithRSA, x509.MD5WithRSA, x509.SHA1WithRSA, x509.SHA256WithRSA, x509.SHA384WithRSA, x509.SHA512WithRSA:
		return true
	default:
		return false
	}
}

func decodeAffiliateAlipayKey(value string, pemType string) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	if block, _ := pem.Decode([]byte(trimmed)); block != nil {
		return block.Bytes, nil
	}
	compact := strings.NewReplacer("\r", "", "\n", "", " ", "", "\t", "").Replace(trimmed)
	der, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", strings.ToLower(pemType), err)
	}
	return der, nil
}

func verifyAffiliateAlipayResponse(publicKey *rsa.PublicKey, payload json.RawMessage, encodedSignature string) error {
	if encodedSignature == "" {
		return &AffiliateAlipayAPIError{Code: "RESPONSE_SIGNATURE_MISSING", Message: "signed response is missing", Ambiguous: true}
	}
	signature, err := base64.StdEncoding.DecodeString(encodedSignature)
	if err != nil {
		return &AffiliateAlipayAPIError{Code: "RESPONSE_SIGNATURE_INVALID", Message: "invalid response signature", Ambiguous: true}
	}
	digest := sha256.Sum256(payload)
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature); err != nil {
		return &AffiliateAlipayAPIError{Code: "RESPONSE_SIGNATURE_VERIFICATION_FAILED", Message: "response signature verification failed", Ambiguous: true}
	}
	return nil
}

func affiliateAlipayErrorIsAmbiguous(code string, subCode string) bool {
	value := strings.ToUpper(code + " " + subCode)
	return code == "20000" || strings.Contains(value, "SYSTEM_ERROR") || strings.Contains(value, "UNKNOWN")
}

func affiliatePayoutAmount(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

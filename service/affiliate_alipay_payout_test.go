package service

import (
	"context"
	"crypto"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAffiliateAlipayClientSignsTransferAndVerifiesResponse(t *testing.T) {
	appPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	alipayPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	config, _, alipaySN, _ := createAffiliateAlipayTestConfig(t, appPrivate, alipayPrivate, "88API commission")

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.NoError(t, request.ParseForm())
		signature, err := base64.StdEncoding.DecodeString(request.Form.Get("sign"))
		require.NoError(t, err)
		keys := make([]string, 0, len(request.Form)-1)
		for key := range request.Form {
			if key != "sign" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		canonical := make([]string, 0, len(keys))
		for _, key := range keys {
			canonical = append(canonical, key+"="+request.Form.Get(key))
		}
		digest := sha256.Sum256([]byte(strings.Join(canonical, "&")))
		require.NoError(t, rsa.VerifyPKCS1v15(&appPrivate.PublicKey, crypto.SHA256, digest[:], signature))

		var bizContent map[string]interface{}
		require.NoError(t, common.UnmarshalJsonStr(request.Form.Get("biz_content"), &bizContent))
		assert.Equal(t, "alipay.fund.trans.uni.transfer", request.Form.Get("method"))
		outBizNo, ok := bizContent["out_biz_no"].(string)
		require.True(t, ok)
		assert.Contains(t, []string{"AFFP-1-1-test", "AFFP-1-2-dealing"}, outBizNo)
		assert.Equal(t, "100.00", bizContent["trans_amount"])
		assert.Equal(t, "TRANS_ACCOUNT_NO_PWD", bizContent["product_code"])
		assert.Equal(t, "DIRECT_TRANSFER", bizContent["biz_scene"])
		assert.Equal(t, "88API commission", bizContent["order_title"])
		assert.Equal(t, affiliateAlipayTransferSceneName, bizContent["transfer_scene_name"])
		reportInfos, ok := bizContent["transfer_scene_report_infos"].([]interface{})
		require.True(t, ok)
		require.Len(t, reportInfos, 1)
		reportInfo, ok := reportInfos[0].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, affiliateAlipayTransferSceneInfoType, reportInfo["info_type"])
		assert.Equal(t, "88API commission", reportInfo["info_content"])
		payeeInfo, ok := bizContent["payee_info"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "recipient@example.com", payeeInfo["identity"])
		assert.Equal(t, "ALIPAY_LOGON_ID", payeeInfo["identity_type"])
		assert.Equal(t, "Recipient", payeeInfo["name"])

		status := "SUCCESS"
		if outBizNo == "AFFP-1-2-dealing" {
			status = "DEALING"
		}
		payload, err := common.Marshal(affiliateAlipayResponse{
			Code:           "10000",
			OutBizNo:       outBizNo,
			OrderId:        "202608150001",
			PayFundOrderId: "FUND-001",
			Status:         status,
		})
		require.NoError(t, err)
		responseDigest := sha256.Sum256(payload)
		responseSignature, err := rsa.SignPKCS1v15(rand.Reader, alipayPrivate, crypto.SHA256, responseDigest[:])
		require.NoError(t, err)
		responseKey := strings.ReplaceAll(request.Form.Get("method"), ".", "_") + "_response"
		envelope, err := common.Marshal(map[string]interface{}{
			responseKey:      json.RawMessage(payload),
			"alipay_cert_sn": alipaySN,
			"sign":           base64.StdEncoding.EncodeToString(responseSignature),
		})
		require.NoError(t, err)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(envelope)
	}))
	defer server.Close()

	client, err := newAffiliateAlipayClient(config, server.URL, server.Client())
	require.NoError(t, err)
	client.now = func() time.Time { return time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC) }

	result, err := client.transfer(context.Background(), &model.AffiliatePayout{
		AmountCents:      10_000,
		AccountName:      "Recipient",
		Account:          "recipient@example.com",
		PaymentReference: "AFFP-1-1-test",
	}, "88API commission")
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", result.Status)
	assert.Equal(t, "202608150001", result.OrderId)
	assert.Equal(t, "FUND-001", result.PayFundOrderId)

	result, err = client.transfer(context.Background(), &model.AffiliatePayout{
		AmountCents:      10_000,
		AccountName:      "Recipient",
		Account:          "recipient@example.com",
		PaymentReference: "AFFP-1-2-dealing",
	}, "88API commission")
	require.NoError(t, err)
	assert.Equal(t, "DEALING", result.Status)
}

func TestAffiliateAlipayClientRejectsUnsignedResponse(t *testing.T) {
	appPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	alipayPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	config, _, alipaySN, _ := createAffiliateAlipayTestConfig(t, appPrivate, alipayPrivate, "88API commission")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		payload, marshalErr := common.Marshal(map[string]interface{}{"code": "10000", "status": "SUCCESS"})
		require.NoError(t, marshalErr)
		envelope, marshalErr := common.Marshal(map[string]interface{}{
			"alipay_fund_trans_common_query_response": json.RawMessage(payload),
			"alipay_cert_sn": alipaySN,
		})
		require.NoError(t, marshalErr)
		_, _ = writer.Write(envelope)
	}))
	defer server.Close()

	client, err := newAffiliateAlipayClient(config, server.URL, server.Client())
	require.NoError(t, err)

	_, err = client.query(context.Background(), "AFFP-test")
	require.Error(t, err)
	apiErr := &AffiliateAlipayAPIError{}
	require.ErrorAs(t, err, &apiErr)
	assert.True(t, apiErr.Ambiguous)
	assert.Equal(t, "RESPONSE_SIGNATURE_MISSING", apiErr.DiagnosticCode())
}

func TestAffiliateAlipayClientCertificateModeAddsSerialNumbersAndVerifiesCertificateResponse(t *testing.T) {
	appPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rootPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	alipayPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	rootName := pkix.Name{Country: []string{"CN"}, Organization: []string{"Alipay"}, OrganizationalUnit: []string{"Open Platform"}, CommonName: "Alipay Root"}
	appCertificate := createAffiliateAlipayCertificate(t, &x509.Certificate{
		SerialNumber: big.NewInt(1001),
		Subject:      pkix.Name{Country: []string{"CN"}, Organization: []string{"88API"}, CommonName: "Affiliate Payout App"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(1, 0, 0),
	}, nil, &appPrivate.PublicKey, appPrivate)
	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(2001),
		Subject:               rootName,
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	rootCertificate := createAffiliateAlipayCertificate(t, rootTemplate, nil, &rootPrivate.PublicKey, rootPrivate)
	alipayCertificate := createAffiliateAlipayCertificate(t, &x509.Certificate{
		SerialNumber: big.NewInt(3001),
		Subject:      pkix.Name{Country: []string{"CN"}, Organization: []string{"Alipay"}, CommonName: "Alipay Public RSA2"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}, rootTemplate, &alipayPrivate.PublicKey, rootPrivate)

	privateDER, err := x509.MarshalPKCS8PrivateKey(appPrivate)
	require.NoError(t, err)
	client, err := newAffiliateAlipayClient(&model.AffiliateAlipayPayoutConfig{
		Enabled:                 true,
		AppId:                   "2026000000000000",
		PrivateKey:              base64.StdEncoding.EncodeToString(privateDER),
		AppCertificate:          appCertificate,
		AlipayPublicCertificate: alipayCertificate + alipayCertificate,
		AlipayRootCertificate:   rootCertificate,
		TransferTitle:           "88API commission",
	}, "", nil)
	require.NoError(t, err)
	appIssuer := "CN=Affiliate Payout App,O=88API,C=CN"
	appSN := fmt.Sprintf("%x", md5.Sum([]byte(appIssuer+"1001")))
	rootIssuer := "CN=Alipay Root,OU=Open Platform,O=Alipay,C=CN"
	rootSN := fmt.Sprintf("%x", md5.Sum([]byte(rootIssuer+"2001")))
	alipaySN := fmt.Sprintf("%x", md5.Sum([]byte(rootIssuer+"3001")))
	assert.Equal(t, appSN, client.appCertSN)
	assert.Equal(t, rootSN, client.alipayRootCertSN)
	assert.Equal(t, alipaySN, client.alipayCertSN)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.NoError(t, request.ParseForm())
		assert.Equal(t, appSN, request.Form.Get("app_cert_sn"))
		assert.Equal(t, rootSN, request.Form.Get("alipay_root_cert_sn"))
		payload, marshalErr := common.Marshal(affiliateAlipayResponse{Code: "10000", Status: "SUCCESS"})
		require.NoError(t, marshalErr)
		digest := sha256.Sum256(payload)
		signature, signErr := rsa.SignPKCS1v15(rand.Reader, alipayPrivate, crypto.SHA256, digest[:])
		require.NoError(t, signErr)
		envelope, marshalErr := common.Marshal(map[string]interface{}{
			"alipay_fund_trans_common_query_response": json.RawMessage(payload),
			"alipay_cert_sn": alipaySN,
			"sign":           base64.StdEncoding.EncodeToString(signature),
		})
		require.NoError(t, marshalErr)
		_, _ = writer.Write(envelope)
	}))
	defer server.Close()
	client.endpoint = server.URL
	client.httpClient = server.Client()
	client.now = func() time.Time { return now }

	result, err := client.query(context.Background(), "AFFP-CERT-1")
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", result.Status)
}

func createAffiliateAlipayCertificate(t *testing.T, template *x509.Certificate, parent *x509.Certificate, publicKey *rsa.PublicKey, signer *rsa.PrivateKey) string {
	t.Helper()
	if parent == nil {
		parent = template
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, publicKey, signer)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func createAffiliateAlipayTestConfig(t *testing.T, appPrivate *rsa.PrivateKey, alipayPrivate *rsa.PrivateKey, title string) (*model.AffiliateAlipayPayoutConfig, string, string, string) {
	t.Helper()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	rootPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(4001),
		Subject:               pkix.Name{Country: []string{"CN"}, Organization: []string{"Alipay"}, OrganizationalUnit: []string{"Open Platform"}, CommonName: "Alipay Root"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	appCertificate := createAffiliateAlipayCertificate(t, &x509.Certificate{
		SerialNumber: big.NewInt(4002),
		Subject:      pkix.Name{Country: []string{"CN"}, Organization: []string{"88API"}, CommonName: "Affiliate Payout App"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(1, 0, 0),
	}, nil, &appPrivate.PublicKey, appPrivate)
	rootCertificate := createAffiliateAlipayCertificate(t, rootTemplate, nil, &rootPrivate.PublicKey, rootPrivate)
	alipayCertificate := createAffiliateAlipayCertificate(t, &x509.Certificate{
		SerialNumber: big.NewInt(4003),
		Subject:      pkix.Name{Country: []string{"CN"}, Organization: []string{"Alipay"}, CommonName: "Alipay Public RSA2"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}, rootTemplate, &alipayPrivate.PublicKey, rootPrivate)
	privateDER, err := x509.MarshalPKCS8PrivateKey(appPrivate)
	require.NoError(t, err)
	appParsed, err := parseAffiliateAlipayCertificate(appCertificate)
	require.NoError(t, err)
	alipayParsed, err := parseAffiliateAlipayCertificate(alipayCertificate)
	require.NoError(t, err)
	rootParsed, err := parseAffiliateAlipayRootCertificates(rootCertificate)
	require.NoError(t, err)
	return &model.AffiliateAlipayPayoutConfig{
		Enabled:                 true,
		AppId:                   "2026000000000000",
		PrivateKey:              base64.StdEncoding.EncodeToString(privateDER),
		AppCertificate:          appCertificate,
		AlipayPublicCertificate: alipayCertificate,
		AlipayRootCertificate:   rootCertificate,
		TransferTitle:           title,
	}, affiliateAlipayCertificateSN(appParsed), affiliateAlipayCertificateSN(alipayParsed), affiliateAlipayRootCertificateSN(rootParsed)
}

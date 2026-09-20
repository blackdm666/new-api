package controller

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type gcpTrialRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn gcpTrialRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestSignGCPServiceAccountJWT(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	privateKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}))
	credential := &gcpServiceAccountCredential{
		Type:        "service_account",
		ClientEmail: "billing-reader@example.iam.gserviceaccount.com",
		PrivateKey:  privateKey,
		TokenURI:    gcpDefaultTokenURI,
	}
	assertion, err := signGCPServiceAccountJWT(credential, time.Unix(1_700_000_000, 0))
	require.NoError(t, err)
	assert.Len(t, strings.Split(assertion, "."), 3)
}

func TestQueryGCPPromotionUsageBuildsBoundedBillingQuery(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: gcpTrialRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, "Bearer access-token", request.Header.Get("Authorization"))
		assert.Contains(t, request.URL.String(), "/projects/api-505117/queries")
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		queryBody := string(body)
		assert.Contains(t, queryBody, "api-505117.billing_export.gcp_billing_export_v1_0112D2_3D1562_101A70")
		assert.Contains(t, queryBody, "PROMOTION")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"jobComplete":true,"rows":[{"f":[{"v":"12.5"}]}]}`)),
		}, nil
	})}
	used, err := queryGCPPromotionUsage(client, "access-token", &dto.ChannelBalanceGCPTrialConfig{
		BillingAccountID: "0112D2-3D1562-101A70",
		QueryProjectID:   "api-505117",
		DatasetID:        "billing_export",
		BaselineAt:       1_700_000_000,
	})
	require.NoError(t, err)
	assert.Equal(t, "12.5", used.String())
}

func TestQueryGCPPromotionUsageReportsPendingBillingTable(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: gcpTrialRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"error":{"code":404,"message":"Not found: Table api-505117:billing_export.gcp_billing_export_v1_0112D2_3D1562_101A70","status":"NOT_FOUND"}}`,
			)),
		}, nil
	})}
	_, err := queryGCPPromotionUsage(client, "access-token", &dto.ChannelBalanceGCPTrialConfig{
		BillingAccountID: "0112D2-3D1562-101A70",
		QueryProjectID:   "api-505117",
		DatasetID:        "billing_export",
		BaselineAt:       1_700_000_000,
	})
	require.ErrorContains(t, err, "billing table is not ready yet")
}

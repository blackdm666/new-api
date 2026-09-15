package controller

import (
	"crypto"
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
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/shopspring/decimal"
)

const (
	gcpBigQueryScope               = "https://www.googleapis.com/auth/bigquery"
	gcpDefaultTokenURI             = "https://oauth2.googleapis.com/token"
	gcpBigQueryQueryResponseMaxLen = 256 << 10
)

var (
	errGCPBillingTableNotReady = errors.New("Google Cloud billing table is not ready yet")
	gcpBillingAccountPattern   = regexp.MustCompile(`^[A-Za-z0-9]{6}-[A-Za-z0-9]{6}-[A-Za-z0-9]{6}$`)
	gcpProjectIDPattern        = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
	gcpDatasetIDPattern        = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

type gcpServiceAccountCredential struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type gcpOAuthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

type gcpBigQueryResponse struct {
	JobComplete bool `json:"jobComplete"`
	Rows        []struct {
		F []struct {
			V json.RawMessage `json:"v"`
		} `json:"f"`
	} `json:"rows"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func updateChannelGCPTrialBalance(channel *model.Channel, config *dto.ChannelBalanceQueryConfig) (channelBalanceResult, error) {
	if config == nil || config.GCPTrial == nil {
		return channelBalanceResult{}, errors.New("Vertex trial credit configuration is required")
	}
	if err := config.Validate(); err != nil {
		return channelBalanceResult{}, err
	}
	gcp := config.GCPTrial
	credentialChannel := channel
	if gcp.CredentialChannelID > 0 && gcp.CredentialChannelID != channel.Id {
		var err error
		credentialChannel, err = model.GetChannelById(gcp.CredentialChannelID, true)
		if err != nil {
			return channelBalanceResult{}, fmt.Errorf("failed to load Vertex credential channel %d: %w", gcp.CredentialChannelID, err)
		}
	}
	if credentialChannel.Type != constant.ChannelTypeVertexAi {
		return channelBalanceResult{}, errors.New("Google Cloud trial credit queries require a Vertex AI credential channel")
	}
	credential, err := parseGCPServiceAccountCredential(credentialChannel.Key)
	if err != nil {
		return channelBalanceResult{}, err
	}
	// Billing metadata is independent of the model-serving route. In particular,
	// production Vertex channels may use Docker-only proxies that are unavailable
	// to local maintenance instances, so Google OAuth and BigQuery go direct.
	client, err := service.GetHttpClientWithProxy("")
	if err != nil {
		return channelBalanceResult{}, err
	}
	accessToken, err := exchangeGCPServiceAccountToken(client, credential)
	if err != nil {
		return channelBalanceResult{}, err
	}
	usedAfterBaseline, err := queryGCPPromotionUsage(client, accessToken, gcp)
	source := dto.ChannelBalanceQueryModeGCPTrial
	if errors.Is(err, errGCPBillingTableNotReady) {
		usedAfterBaseline = decimal.Zero
		source += "_baseline"
	} else if err != nil {
		return channelBalanceResult{}, err
	}
	total, err := decimal.NewFromString(strings.TrimSpace(gcp.TotalAmount))
	if err != nil {
		return channelBalanceResult{}, err
	}
	baseline, err := decimal.NewFromString(strings.TrimSpace(gcp.BaselineUsed))
	if err != nil {
		return channelBalanceResult{}, err
	}
	used := baseline.Add(usedAfterBaseline)
	if used.IsNegative() {
		used = decimal.Zero
	}
	if used.GreaterThan(total) {
		used = total
	}
	remaining := total.Sub(used)
	info := model.ChannelBalanceInfo{
		Remaining:   remaining.String(),
		Total:       total.String(),
		Used:        used.String(),
		Unit:        model.ChannelBalanceUnitMoney,
		Currency:    "USD",
		DisplayUnit: "$",
		MetricKind:  dto.ChannelBalanceMetricWallet,
		Source:      source,
		UpdatedAt:   common.GetTimestamp(),
	}
	legacyBalance := remaining.InexactFloat64()
	if err := channel.UpdateBalanceInfo(info, &legacyBalance); err != nil {
		return channelBalanceResult{}, err
	}
	return balanceResult(info, &legacyBalance), nil
}

func parseGCPServiceAccountCredential(key string) (*gcpServiceAccountCredential, error) {
	key = strings.TrimSpace(key)
	if strings.HasPrefix(key, "[") {
		var credentials []json.RawMessage
		if err := common.Unmarshal([]byte(key), &credentials); err != nil || len(credentials) == 0 {
			return nil, errors.New("Vertex credential channel does not contain a valid service account JSON")
		}
		key = string(credentials[0])
	}
	credential := &gcpServiceAccountCredential{}
	if err := common.Unmarshal([]byte(key), credential); err != nil {
		return nil, errors.New("Vertex credential channel does not contain a valid service account JSON")
	}
	if credential.Type != "service_account" || strings.TrimSpace(credential.ClientEmail) == "" || strings.TrimSpace(credential.PrivateKey) == "" {
		return nil, errors.New("Vertex credential channel must use a Google service account JSON")
	}
	if strings.TrimSpace(credential.TokenURI) == "" {
		credential.TokenURI = gcpDefaultTokenURI
	}
	return credential, nil
}

func exchangeGCPServiceAccountToken(client *http.Client, credential *gcpServiceAccountCredential) (string, error) {
	assertion, err := signGCPServiceAccountJWT(credential, time.Now())
	if err != nil {
		return "", err
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)
	request, err := http.NewRequest(http.MethodPost, credential.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", errors.New("failed to create Google OAuth request")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("Google OAuth request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return "", errors.New("failed to read Google OAuth response")
	}
	parsed := gcpOAuthTokenResponse{}
	if common.Unmarshal(body, &parsed) != nil {
		return "", fmt.Errorf("Google OAuth returned status code %d", response.StatusCode)
	}
	if response.StatusCode != http.StatusOK || strings.TrimSpace(parsed.AccessToken) == "" {
		message := strings.TrimSpace(parsed.Description)
		if message == "" {
			message = strings.TrimSpace(parsed.Error)
		}
		if message == "" {
			message = fmt.Sprintf("status code %d", response.StatusCode)
		}
		return "", fmt.Errorf("Google OAuth rejected the Vertex service account: %s", message)
	}
	return parsed.AccessToken, nil
}

func signGCPServiceAccountJWT(credential *gcpServiceAccountCredential, now time.Time) (string, error) {
	block, _ := pem.Decode([]byte(credential.PrivateKey))
	if block == nil {
		return "", errors.New("Vertex service account private key is invalid")
	}
	var privateKey *rsa.PrivateKey
	parsedPKCS8, pkcs8Err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if pkcs8Err == nil {
		var ok bool
		privateKey, ok = parsedPKCS8.(*rsa.PrivateKey)
		if !ok {
			return "", errors.New("Vertex service account private key is not RSA")
		}
	} else {
		var err error
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return "", errors.New("Vertex service account private key is invalid")
		}
	}
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iss":   credential.ClientEmail,
		"scope": gcpBigQueryScope,
		"aud":   credential.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", errors.New("failed to sign Google OAuth assertion")
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func queryGCPPromotionUsage(client *http.Client, accessToken string, config *dto.ChannelBalanceGCPTrialConfig) (decimal.Decimal, error) {
	projectID := strings.TrimSpace(config.QueryProjectID)
	datasetID := strings.TrimSpace(config.DatasetID)
	billingAccountID := strings.TrimSpace(config.BillingAccountID)
	if !gcpProjectIDPattern.MatchString(projectID) || !gcpDatasetIDPattern.MatchString(datasetID) || len(datasetID) > 1024 || !gcpBillingAccountPattern.MatchString(billingAccountID) {
		return decimal.Zero, errors.New("invalid Google Cloud billing export configuration")
	}
	tableID := "gcp_billing_export_v1_" + strings.ReplaceAll(billingAccountID, "-", "_")
	baselineAt := time.Unix(config.BaselineAt, 0).UTC().Format(time.RFC3339)
	query := fmt.Sprintf("SELECT CAST(COALESCE(-SUM(IFNULL((SELECT SUM(c.amount) FROM UNNEST(credits) c WHERE c.type = 'PROMOTION'), 0)), 0) AS STRING) FROM `%s.%s.%s` WHERE usage_start_time >= TIMESTAMP('%s')", projectID, datasetID, tableID, baselineAt)
	payload, _ := common.Marshal(map[string]any{
		"query":        query,
		"useLegacySql": false,
		"maxResults":   1,
		"timeoutMs":    15000,
		"location":     "US",
	})
	endpoint := fmt.Sprintf("https://bigquery.googleapis.com/bigquery/v2/projects/%s/queries", url.PathEscape(projectID))
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return decimal.Zero, errors.New("failed to create BigQuery request")
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return decimal.Zero, fmt.Errorf("BigQuery request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, gcpBigQueryQueryResponseMaxLen+1))
	if err != nil {
		return decimal.Zero, errors.New("failed to read BigQuery response")
	}
	if len(body) > gcpBigQueryQueryResponseMaxLen {
		return decimal.Zero, errors.New("BigQuery response is too large")
	}
	parsed := gcpBigQueryResponse{}
	_ = common.Unmarshal(body, &parsed)
	if response.StatusCode != http.StatusOK {
		message := ""
		if parsed.Error != nil {
			message = strings.TrimSpace(parsed.Error.Message)
		}
		if response.StatusCode == http.StatusNotFound || strings.Contains(strings.ToLower(message), "not found") {
			return decimal.Zero, fmt.Errorf("%w: %s.%s.%s", errGCPBillingTableNotReady, projectID, datasetID, tableID)
		}
		if message == "" {
			message = fmt.Sprintf("status code %d", response.StatusCode)
		}
		return decimal.Zero, fmt.Errorf("BigQuery rejected the billing query: %s", message)
	}
	if !parsed.JobComplete {
		return decimal.Zero, errors.New("BigQuery billing query is still running; try again shortly")
	}
	if len(parsed.Rows) == 0 || len(parsed.Rows[0].F) == 0 {
		return decimal.Zero, nil
	}
	var value string
	if err := common.Unmarshal(parsed.Rows[0].F[0].V, &value); err != nil {
		return decimal.Zero, errors.New("BigQuery returned an invalid promotional credit total")
	}
	used, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil || used.IsNegative() {
		return decimal.Zero, errors.New("BigQuery returned an invalid promotional credit total")
	}
	return used, nil
}

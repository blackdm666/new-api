package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupReferralApplicationTest prepares an outbox database, an SMTP server
// address that refuses connections (so background delivery fails fast) and
// the same middleware chain as POST /api/public/referral-application.
func setupReferralApplicationTest(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:referral-application-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.EmailDelivery{}))
	model.DB = db

	previousServer, previousPort := common.SMTPServer, common.SMTPPort
	previousRecipient := common.ReferralApplicationRecipient
	previousTurnstile := common.TurnstileCheckEnabled
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	common.SMTPServer = "127.0.0.1"
	common.SMTPPort = 1
	common.ReferralApplicationRecipient = common.DefaultReferralApplicationRecipient
	common.TurnstileCheckEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.SMTPServer, common.SMTPPort = previousServer, previousPort
		common.ReferralApplicationRecipient = previousRecipient
		common.TurnstileCheckEnabled = previousTurnstile
		common.RedisEnabled = previousRedis
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/public/referral-application",
		middleware.ReferralApplicationRateLimit(),
		middleware.AnonymousRequestBodyLimit(),
		middleware.TurnstileCheckFromBody(),
		SubmitReferralApplication,
	)
	return router, db
}

func validReferralApplicationPayload() map[string]any {
	return map[string]any{
		"account":         "10086",
		"display_name":    "Alice <b>Studio</b>",
		"telegram":        "t.me/alice_ai",
		"wechat":          "",
		"email":           "alice@example.com",
		"channels":        []any{"bilibili", "tg_group"},
		"channel_url":     "https://space.bilibili.com/123",
		"audience_size":   "10k-100k",
		"message":         "Line one\nLine two",
		"consent_rules":   true,
		"consent_privacy": true,
		"website":         "",
		"source_page":     "https://88api.ai/business/referral/?aff=abcd#apply",
		"locale":          "zh-CN",
	}
}

func postReferralApplication(t *testing.T, router *gin.Engine, ip string, payload map[string]any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/public/referral-application", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = ip + ":40000"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	response := map[string]any{}
	if recorder.Body.Len() > 0 {
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	}
	return recorder, response
}

func TestSubmitReferralApplicationQueuesEscapedEmailToDefaultRecipient(t *testing.T) {
	router, db := setupReferralApplicationTest(t)

	recorder, response := postReferralApplication(t, router, "198.51.100.10", validReferralApplicationPayload())

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, true, response["success"])
	data, _ := response["data"].(map[string]any)
	reference, _ := data["reference"].(string)
	assert.Regexp(t, `^RA-[A-Z0-9]{8}$`, reference)

	deliveries := []model.EmailDelivery{}
	require.NoError(t, db.Find(&deliveries).Error)
	require.Len(t, deliveries, 1)
	delivery := deliveries[0]
	assert.Equal(t, "laohei@88fk.org", delivery.Recipient)
	assert.Equal(t, service.ReferralApplicationEmailCategory, delivery.Category)
	assert.Contains(t, delivery.Subject, reference)
	assert.NotContains(t, delivery.Subject, "Alice")
	for _, expected := range []string{
		"Alice &lt;b&gt;Studio&lt;/b&gt;", "10086", "@alice_ai", "alice@example.com",
		"B站、Telegram 群", "1 万–10 万", "Line one<br/>Line two",
		"https://88api.ai/business/referral/", "198.51.100.10", "北京时间", "zh-CN", reference,
	} {
		assert.Contains(t, delivery.Body, expected)
	}
	assert.NotContains(t, delivery.Body, "<b>Studio</b>")
	assert.NotContains(t, delivery.Body, "aff=abcd")
}

func TestSubmitReferralApplicationSendsToEveryConfiguredRecipient(t *testing.T) {
	router, db := setupReferralApplicationTest(t)
	common.ReferralApplicationRecipient = "ops@example.com; sales@example.com"

	recorder, _ := postReferralApplication(t, router, "198.51.100.11", validReferralApplicationPayload())

	require.Equal(t, http.StatusOK, recorder.Code)
	recipients := []string{}
	require.NoError(t, db.Model(&model.EmailDelivery{}).Order("recipient ASC").Pluck("recipient", &recipients).Error)
	assert.Equal(t, []string{"ops@example.com", "sales@example.com"}, recipients)
}

func TestSubmitReferralApplicationRejectsInvalidFieldsWithoutSending(t *testing.T) {
	router, db := setupReferralApplicationTest(t)
	cases := []struct {
		name  string
		edit  func(payload map[string]any)
		field string
		code  string
	}{
		{"missing name", func(p map[string]any) { p["display_name"] = "  " }, "display_name", service.ReferralApplicationErrorRequired},
		{"name too long", func(p map[string]any) { p["display_name"] = string(bytes.Repeat([]byte("名"), 51)) }, "display_name", service.ReferralApplicationErrorTooLong},
		{"invalid account", func(p map[string]any) { p["account"] = "alice" }, "account", service.ReferralApplicationErrorInvalid},
		{"invalid email", func(p map[string]any) { p["email"] = "alice@localhost" }, "email", service.ReferralApplicationErrorInvalid},
		{"invalid telegram", func(p map[string]any) { p["telegram"] = "@ab" }, "telegram", service.ReferralApplicationErrorInvalid},
		{"no contact", func(p map[string]any) { p["telegram"], p["wechat"], p["email"] = "", "", "" }, "contact", service.ReferralApplicationErrorContactRequired},
		{"no channel", func(p map[string]any) { p["channels"] = []any{} }, "channels", service.ReferralApplicationErrorRequired},
		{"unknown channel", func(p map[string]any) { p["channels"] = []any{"bilibili", "tiktok"} }, "channels", service.ReferralApplicationErrorInvalid},
		{"unknown audience", func(p map[string]any) { p["audience_size"] = "huge" }, "audience_size", service.ReferralApplicationErrorInvalid},
		{"too many links", func(p map[string]any) {
			p["message"] = "https://a.example https://b.example http://c.example https://d.example"
		}, "message", service.ReferralApplicationErrorTooManyLinks},
		{"control character", func(p map[string]any) { p["wechat"] = "wx\u0000id" }, "wechat", service.ReferralApplicationErrorInvalid},
		{"rules not accepted", func(p map[string]any) { p["consent_rules"] = false }, "consent_rules", service.ReferralApplicationErrorRequired},
		{"privacy not accepted", func(p map[string]any) { delete(p, "consent_privacy") }, "consent_privacy", service.ReferralApplicationErrorRequired},
		{"wrong type", func(p map[string]any) { p["display_name"] = 42 }, "display_name", service.ReferralApplicationErrorInvalid},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := validReferralApplicationPayload()
			tc.edit(payload)

			recorder, response := postReferralApplication(t, router, fmt.Sprintf("198.51.101.%d", index+1), payload)

			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Equal(t, false, response["success"])
			data, _ := response["data"].(map[string]any)
			assert.Equal(t, tc.field, data["field"])
			assert.Equal(t, tc.code, data["code"])
		})
	}
	var count int64
	require.NoError(t, db.Model(&model.EmailDelivery{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestSubmitReferralApplicationDropsHoneypotSubmissionsSilently(t *testing.T) {
	router, db := setupReferralApplicationTest(t)
	payload := validReferralApplicationPayload()
	payload["website"] = "https://spam.example"

	recorder, response := postReferralApplication(t, router, "198.51.100.12", payload)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, true, response["success"])
	var count int64
	require.NoError(t, db.Model(&model.EmailDelivery{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestSubmitReferralApplicationLimitsRequestsPerIP(t *testing.T) {
	router, db := setupReferralApplicationTest(t)

	for range middleware.ReferralApplicationRateLimitNum {
		recorder, _ := postReferralApplication(t, router, "198.51.100.13", validReferralApplicationPayload())
		require.Equal(t, http.StatusOK, recorder.Code)
	}
	limited, _ := postReferralApplication(t, router, "198.51.100.13", validReferralApplicationPayload())
	otherIP, _ := postReferralApplication(t, router, "198.51.100.14", validReferralApplicationPayload())

	assert.Equal(t, http.StatusTooManyRequests, limited.Code)
	assert.Equal(t, http.StatusOK, otherIP.Code)
	var count int64
	require.NoError(t, db.Model(&model.EmailDelivery{}).Count(&count).Error)
	assert.EqualValues(t, middleware.ReferralApplicationRateLimitNum+1, count)
}

func TestSubmitReferralApplicationRequiresHumanVerificationWhenEnabled(t *testing.T) {
	router, db := setupReferralApplicationTest(t)
	verifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		success := r.PostForm.Get("response") == "valid-token" && r.PostForm.Get("secret") == "custom-secret"
		_, _ = fmt.Fprintf(w, `{"success":%t}`, success)
	}))
	t.Cleanup(verifier.Close)
	previous := common.CurrentTurnstileConfig()
	common.TurnstileCheckEnabled = true
	common.TurnstileProvider = common.TurnstileProviderCustom
	common.TurnstileSecretKey = "custom-secret"
	common.TurnstileWidgetScriptURL = "https://captcha.example/widget.js"
	common.TurnstileWidgetEndpoint = "https://captcha.example"
	common.TurnstileVerifyURL = verifier.URL
	t.Cleanup(func() {
		common.TurnstileProvider = previous.Provider
		common.TurnstileSecretKey = previous.SecretKey
		common.TurnstileWidgetScriptURL = previous.WidgetScriptURL
		common.TurnstileWidgetEndpoint = previous.WidgetEndpoint
		common.TurnstileVerifyURL = previous.VerifyURL
	})

	missing, missingResponse := postReferralApplication(t, router, "198.51.100.15", validReferralApplicationPayload())
	wrongPayload := validReferralApplicationPayload()
	wrongPayload["turnstile"] = "forged-token"
	wrong, wrongResponse := postReferralApplication(t, router, "198.51.100.15", wrongPayload)
	validPayload := validReferralApplicationPayload()
	validPayload["turnstile"] = "valid-token"
	valid, validResponse := postReferralApplication(t, router, "198.51.100.15", validPayload)

	assert.Equal(t, http.StatusOK, missing.Code)
	assert.Equal(t, false, missingResponse["success"])
	assert.Equal(t, http.StatusOK, wrong.Code)
	assert.Equal(t, false, wrongResponse["success"])
	assert.Equal(t, http.StatusOK, valid.Code)
	assert.Equal(t, true, validResponse["success"])
	var count int64
	require.NoError(t, db.Model(&model.EmailDelivery{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestSubmitReferralApplicationReportsUnavailableWithoutSMTP(t *testing.T) {
	router, db := setupReferralApplicationTest(t)
	common.SMTPServer = ""

	recorder, response := postReferralApplication(t, router, "198.51.100.16", validReferralApplicationPayload())

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, false, response["success"])
	var count int64
	require.NoError(t, db.Model(&model.EmailDelivery{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestValidateReferralApplicationRecipients(t *testing.T) {
	assert.NoError(t, service.ValidateReferralApplicationRecipients("laohei@88fk.org"))
	assert.NoError(t, service.ValidateReferralApplicationRecipients("a@example.com, b@example.com;c@example.com"))
	assert.Error(t, service.ValidateReferralApplicationRecipients(""))
	assert.Error(t, service.ValidateReferralApplicationRecipients("a@example.com, not-an-email"))
	assert.Error(t, service.ValidateReferralApplicationRecipients("Alice <a@example.com>"))
}

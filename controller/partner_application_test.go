package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	partnerApplicationPath = "/api/public/partner-application"
	// partnerApplicationTestUserHeader carries the signed-in user ID in tests.
	partnerApplicationTestUserHeader = "X-Test-User-Id"
)

// nextPartnerApplicantID hands every test its own account IDs, because the
// in-memory per-account rate limit outlives each test's database.
var nextPartnerApplicantID atomic.Int64

func init() { nextPartnerApplicantID.Store(30000) }

// partnerApplicationTestAuth stands in for UserAuth on the test router: it
// sets the signed-in user ID like UserAuth does, or answers 401 when the
// request carries none. UserAuth itself is covered by the auth tests, and the
// route table test requires the real route to declare a session rule.
func partnerApplicationTestAuth(c *gin.Context) {
	id, err := strconv.Atoi(c.GetHeader(partnerApplicationTestUserHeader))
	if err != nil || id <= 0 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Set("id", id)
}

// createPartnerApplicants stores count enabled accounts with consecutive IDs
// and returns the first, for tests that submit more often than one account may.
func createPartnerApplicants(t *testing.T, db *gorm.DB, count int) int {
	t.Helper()
	first := int(nextPartnerApplicantID.Add(int64(count))) - count + 1
	for index := range count {
		id := first + index
		require.NoError(t, db.Create(&model.User{
			Id: id, Username: fmt.Sprintf("partner_%d", id), Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("pb%d", id),
		}).Error)
	}
	return first
}

// createPartnerApplicant stores an enabled signed-in applicant and returns its ID.
func createPartnerApplicant(t *testing.T, db *gorm.DB) int {
	t.Helper()
	id := int(nextPartnerApplicantID.Add(1))
	require.NoError(t, db.Create(&model.User{
		Id: id, Username: fmt.Sprintf("partner_a%d", id), Group: "vip", Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("pa%d", id),
		Quota: int(10 * common.QuotaPerUnit), UsedQuota: int(2.5 * common.QuotaPerUnit), RequestCount: 42,
		CreatedAt: 1767196800, // 2026-01-01 00:00:00 UTC+8
	}).Error)
	return id
}

func validPartnerApplicationPayload() map[string]any {
	return map[string]any{
		"display_name":    "Acme <b>Relay</b>",
		"business_type":   "relay_running",
		"site_url":        "https://relay.example.com",
		"system":          "new_api",
		"monthly_spend":   "10k-50k",
		"prepay":          "paid",
		"models":          []any{"claude", "gpt", "claude"},
		"needs":           []any{"price", "fapiao"},
		"telegram":        "@acme_relay",
		"wechat":          "",
		"email":           "",
		"message":         "Line one\nLine two",
		"consent_privacy": true,
		"website":         "",
		"source_page":     "https://88api.ai/business/partner/?utm_source=x#apply",
		"locale":          "zh-CN",
	}
}

func postPartnerApplication(t *testing.T, router *gin.Engine, ip string, userID int, payload map[string]any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, partnerApplicationPath, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if userID > 0 {
		request.Header.Set(partnerApplicationTestUserHeader, strconv.Itoa(userID))
	}
	request.RemoteAddr = ip + ":40000"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	response := map[string]any{}
	if recorder.Body.Len() > 0 {
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	}
	return recorder, response
}

// The account overview always describes the signed-in applicant: a UID in the
// payload (the old form field) is ignored, so nobody can file under another
// account.
func TestSubmitPartnerApplicationQueuesGradedEmailWithAccountOverview(t *testing.T) {
	router, db := setupReferralApplicationTest(t)
	applicant := createPartnerApplicant(t, db)
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "site_owner", Group: "internal", Status: common.UserStatusEnabled, AffCode: "own1"}).Error)
	payload := validPartnerApplicationPayload()
	payload["account"] = "1"

	recorder, response := postPartnerApplication(t, router, "203.0.113.10", applicant, payload)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, true, response["success"])
	data, _ := response["data"].(map[string]any)
	reference, _ := data["reference"].(string)
	assert.Regexp(t, `^PA-[A-Z0-9]{8}$`, reference)

	// Only the administrator notification is queued: no applicant reply.
	deliveries := []model.EmailDelivery{}
	require.NoError(t, db.Find(&deliveries).Error)
	require.Len(t, deliveries, 1)
	delivery := deliveries[0]
	assert.Equal(t, service.PartnerApplicationEmailCategory, delivery.Category)
	assert.Equal(t, "laohei@88fk.org", delivery.Recipient)
	assert.Equal(t, fmt.Sprintf("[%s] 代理合作申请 A级 %s", common.SystemNameOrDefault(), reference), delivery.Subject)
	for _, expected := range []string{
		"官网代理合作页收到一份新申请；账号概况取自提交时登录的账号，评级只按申请人自报的月消耗，联系前请结合账号概况核实。",
		"88API UID", strconv.Itoa(applicant), "Acme &lt;b&gt;Relay&lt;/b&gt;", "已在运营 API 中转站", "https://relay.example.com",
		"New API", "¥1 万–5 万", "单次预存不少于 ¥2,000", "已预存 ¥2,000 以上", "Claude、GPT / Codex", "专属价格、开发票", "@acme_relay", "Line one<br/>Line two",
		"账号概况", "partner_a", "vip", "$2.50", "$10.00", "42", "2026-01-01 00:00:00（北京时间）",
		"https://88api.ai/business/partner/", "203.0.113.10", reference,
	} {
		assert.Contains(t, delivery.Body, expected)
	}
	assert.NotContains(t, delivery.Body, "<b>Relay</b>")
	assert.NotContains(t, delivery.Body, "site_owner")
	assert.NotContains(t, delivery.Body, "utm_source")
	assert.NotContains(t, delivery.Body, "自动回信")
	assert.Less(t, strings.Index(delivery.Body, "账号概况"), strings.Index(delivery.Body, "提交信息"))
}

func TestSubmitPartnerApplicationGradeAndOptionalSite(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(payload map[string]any)
		grade string
	}{
		{"spend over 50k", func(p map[string]any) { p["monthly_spend"] = "gt50k" }, "A"},
		{"spend 5k to 10k", func(p map[string]any) { p["monthly_spend"] = "5k-10k" }, "B"},
		{"willing to prepay", func(p map[string]any) { p["prepay"] = "willing" }, "A"},
		{"reseller without site", func(p map[string]any) { p["business_type"], p["site_url"] = "reseller", "" }, "A"},
		{"building without site", func(p map[string]any) { p["business_type"], p["site_url"] = "relay_building", "" }, "A"},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, db := setupReferralApplicationTest(t)
			applicant := createPartnerApplicant(t, db)
			payload := validPartnerApplicationPayload()
			tc.edit(payload)

			recorder, response := postPartnerApplication(t, router, fmt.Sprintf("203.0.113.%d", 20+index), applicant, payload)

			require.Equal(t, http.StatusOK, recorder.Code)
			data, _ := response["data"].(map[string]any)
			reference, _ := data["reference"].(string)
			delivery := model.EmailDelivery{}
			require.NoError(t, db.Where("category = ?", service.PartnerApplicationEmailCategory).First(&delivery).Error)
			assert.Equal(t, fmt.Sprintf("[%s] 代理合作申请 %s级 %s", common.SystemNameOrDefault(), tc.grade, reference), delivery.Subject)
		})
	}
}

func TestSubmitPartnerApplicationRejectsInvalidFieldsWithoutSending(t *testing.T) {
	router, db := setupReferralApplicationTest(t)
	cases := []struct {
		name  string
		edit  func(payload map[string]any)
		field string
		code  string
	}{
		{"missing name", func(p map[string]any) { delete(p, "display_name") }, "display_name", service.ReferralApplicationErrorRequired},
		{"name too long", func(p map[string]any) { p["display_name"] = strings.Repeat("名", 51) }, "display_name", service.ReferralApplicationErrorTooLong},
		{"missing business type", func(p map[string]any) { p["business_type"] = "" }, "business_type", service.ReferralApplicationErrorRequired},
		{"unknown business type", func(p map[string]any) { p["business_type"] = "agency" }, "business_type", service.ReferralApplicationErrorInvalid},
		{"running relay without site", func(p map[string]any) { p["site_url"] = "" }, "site_url", service.ReferralApplicationErrorRequired},
		{"saas without site", func(p map[string]any) { p["business_type"], p["site_url"] = "saas", " " }, "site_url", service.ReferralApplicationErrorRequired},
		{"too many site links", func(p map[string]any) {
			p["site_url"] = "https://a.example https://b.example https://c.example https://d.example"
		}, "site_url", service.ReferralApplicationErrorTooManyLinks},
		{"missing system", func(p map[string]any) { p["system"] = "" }, "system", service.ReferralApplicationErrorRequired},
		{"missing spend", func(p map[string]any) { p["monthly_spend"] = "" }, "monthly_spend", service.ReferralApplicationErrorRequired},
		{"no models", func(p map[string]any) { p["models"] = []any{} }, "models", service.ReferralApplicationErrorRequired},
		{"unknown model", func(p map[string]any) { p["models"] = []any{"codex"} }, "models", service.ReferralApplicationErrorInvalid},
		{"old invoice need value", func(p map[string]any) { p["needs"] = []any{"invoice"} }, "needs", service.ReferralApplicationErrorInvalid},
		{"invalid telegram", func(p map[string]any) { p["telegram"] = "@ab" }, "telegram", service.ReferralApplicationErrorInvalid},
		{"no contact", func(p map[string]any) { p["telegram"] = "" }, "contact", service.ReferralApplicationErrorContactRequired},
		{"too many message links", func(p map[string]any) {
			p["message"] = "https://a.example https://b.example https://c.example https://d.example"
		}, "message", service.ReferralApplicationErrorTooManyLinks},
		{"privacy not accepted", func(p map[string]any) { p["consent_privacy"] = false }, "consent_privacy", service.ReferralApplicationErrorRequired},
		{"personal use", func(p map[string]any) { p["business_type"] = "personal" }, "business_type", service.PartnerApplicationErrorNotEligible},
		{"in-house use", func(p map[string]any) { p["business_type"] = "studio" }, "business_type", service.PartnerApplicationErrorNotEligible},
		{"spend under 5k", func(p map[string]any) { p["monthly_spend"] = "lt5k" }, "monthly_spend", service.PartnerApplicationErrorNotEligible},
		{"old 3k tier value", func(p map[string]any) { p["monthly_spend"] = "3k-10k" }, "monthly_spend", service.ReferralApplicationErrorInvalid},
		{"missing prepay", func(p map[string]any) { p["prepay"] = "" }, "prepay", service.ReferralApplicationErrorRequired},
		{"prepay declined", func(p map[string]any) { p["prepay"] = "decline" }, "prepay", service.PartnerApplicationErrorNotEligible},
		// Order: field formats, then contact, then eligibility.
		{"site checked before contact", func(p map[string]any) { p["site_url"], p["telegram"] = "", "" }, "site_url", service.ReferralApplicationErrorRequired},
		{"contact checked before eligibility", func(p map[string]any) { p["business_type"], p["telegram"] = "personal", "" }, "contact", service.ReferralApplicationErrorContactRequired},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			applicant := createPartnerApplicant(t, db)
			payload := validPartnerApplicationPayload()
			tc.edit(payload)

			recorder, response := postPartnerApplication(t, router, fmt.Sprintf("203.0.114.%d", index+1), applicant, payload)

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

func TestSubmitPartnerApplicationDropsHoneypotSubmissionsSilently(t *testing.T) {
	router, db := setupReferralApplicationTest(t)
	applicant := createPartnerApplicant(t, db)
	payload := validPartnerApplicationPayload()
	payload["website"] = "https://spam.example"

	recorder, response := postPartnerApplication(t, router, "203.0.113.30", applicant, payload)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, true, response["success"])
	var count int64
	require.NoError(t, db.Model(&model.EmailDelivery{}).Count(&count).Error)
	assert.Zero(t, count)
}

// The partner form has its own per-IP budget, so filling it does not use up
// the referral form's budget.
func TestSubmitPartnerApplicationLimitsRequestsPerIPSeparately(t *testing.T) {
	router, db := setupReferralApplicationTest(t)
	first := createPartnerApplicants(t, db, middleware.PartnerApplicationRateLimitNum+1)

	for index := range middleware.PartnerApplicationRateLimitNum {
		recorder, _ := postPartnerApplication(t, router, "203.0.113.31", first+index, validPartnerApplicationPayload())
		require.Equal(t, http.StatusOK, recorder.Code)
	}
	limited, _ := postPartnerApplication(t, router, "203.0.113.31", first+middleware.PartnerApplicationRateLimitNum, validPartnerApplicationPayload())
	referral, _ := postReferralApplication(t, router, "203.0.113.31", validReferralApplicationPayload())

	assert.Equal(t, http.StatusTooManyRequests, limited.Code)
	assert.Equal(t, http.StatusOK, referral.Code)
}

// The partner form has its own daily email budget: past it a submission still
// looks accepted but nothing is emailed, and the referral form is unaffected.
func TestSubmitPartnerApplicationStopsEmailingAtDailyLimit(t *testing.T) {
	router, db := setupReferralApplicationTest(t)
	first := createPartnerApplicants(t, db, service.ApplicationDailyLimit+1)

	for index := range service.ApplicationDailyLimit {
		recorder, _ := postPartnerApplication(t, router, fmt.Sprintf("203.0.114.%d", index+1), first+index, validPartnerApplicationPayload())
		require.Equal(t, http.StatusOK, recorder.Code)
	}
	recorder, response := postPartnerApplication(t, router, "203.0.114.200", first+service.ApplicationDailyLimit, validPartnerApplicationPayload())
	referral, _ := postReferralApplication(t, router, "203.0.114.201", validReferralApplicationPayload())

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, true, response["success"])
	assert.Equal(t, http.StatusOK, referral.Code)
	var partner, referralNotified int64
	require.NoError(t, db.Model(&model.EmailDelivery{}).Where("category = ?", service.PartnerApplicationEmailCategory).Count(&partner).Error)
	require.NoError(t, db.Model(&model.EmailDelivery{}).Where("category = ?", service.ReferralApplicationEmailCategory).Count(&referralNotified).Error)
	assert.EqualValues(t, service.ApplicationDailyLimit, partner)
	assert.EqualValues(t, 1, referralNotified)
}

func TestSubmitPartnerApplicationReportsUnavailableWithoutSMTP(t *testing.T) {
	router, db := setupReferralApplicationTest(t)
	applicant := createPartnerApplicant(t, db)
	common.SMTPServer = ""

	recorder, response := postPartnerApplication(t, router, "203.0.113.32", applicant, validPartnerApplicationPayload())

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, false, response["success"])
	var count int64
	require.NoError(t, db.Model(&model.EmailDelivery{}).Count(&count).Error)
	assert.Zero(t, count)
}

// Without a signed-in account nothing is validated or emailed; a disabled
// account is refused; and one account cannot repeat the form from rotating
// IPs past its own budget.
func TestSubmitPartnerApplicationRequiresEnabledSignedInAccount(t *testing.T) {
	router, db := setupReferralApplicationTest(t)
	applicant := createPartnerApplicant(t, db)
	disabledID := createPartnerApplicant(t, db)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", disabledID).Update("status", common.UserStatusDisabled).Error)

	anonymous, _ := postPartnerApplication(t, router, "203.0.115.1", 0, validPartnerApplicationPayload())
	disabled, disabledResponse := postPartnerApplication(t, router, "203.0.115.2", disabledID, validPartnerApplicationPayload())

	assert.Equal(t, http.StatusUnauthorized, anonymous.Code)
	assert.Equal(t, http.StatusForbidden, disabled.Code)
	assert.Equal(t, false, disabledResponse["success"])

	for index := range middleware.PartnerApplicationUserRateLimitNum {
		recorder, _ := postPartnerApplication(t, router, fmt.Sprintf("203.0.115.%d", 10+index), applicant, validPartnerApplicationPayload())
		require.Equal(t, http.StatusOK, recorder.Code)
	}
	limited, _ := postPartnerApplication(t, router, "203.0.115.99", applicant, validPartnerApplicationPayload())
	assert.Equal(t, http.StatusTooManyRequests, limited.Code)

	var count int64
	require.NoError(t, db.Model(&model.EmailDelivery{}).Where("category = ?", service.PartnerApplicationEmailCategory).Count(&count).Error)
	assert.EqualValues(t, middleware.PartnerApplicationUserRateLimitNum, count)
}

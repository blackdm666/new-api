package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacyInvoiceSettingsPayload is what a console bundle built before the
// rejected/expired notices and the WeChat settings submits.
func legacyInvoiceSettingsPayload() InvoiceSettingsPayload {
	return InvoiceSettingsPayload{
		InvoiceApplicationNotifyAdminEnabled: true,
		InvoiceIssuedNotifyUserEnabled:       true,
		InvoiceAdminEmail:                    "billing@example.com",
		InvoiceMinimumAmountCents:            50000,
		InvoiceTaxRateBasisPoints:            300,
		InvoiceDataRetentionDays:             0,
		InvoiceFileEnabled:                   true,
		InvoiceFileStorage:                   "local",
		InvoiceFileMaxSize:                   5 * 1024 * 1024,
		InvoiceFileMaxCount:                  5,
		InvoiceFileAllowedExts:               "jpg,jpeg,png,webp,pdf",
		InvoiceFileLocalPath:                 "/data/invoice_files",
		InvoiceFileSignedURLTTL:              900,
	}
}

func validInvoiceSettingsPayload() InvoiceSettingsPayload {
	payload := legacyInvoiceSettingsPayload()
	payload.InvoiceRejectedNotifyUserEnabled = common.GetPointer(true)
	payload.InvoiceExpiredNotifyUserEnabled = common.GetPointer(true)
	payload.InvoiceWechatNotifyEnabled = common.GetPointer(false)
	payload.InvoiceWechatApiBaseUrl = common.GetPointer("")
	payload.InvoiceWechatApiToken = common.GetPointer("")
	payload.InvoiceWechatAppId = common.GetPointer("")
	payload.InvoiceWechatGroupId = common.GetPointer("")
	payload.InvoiceWechatAutoIssueEnabled = common.GetPointer(false)
	return payload
}

// restoreAfterTest puts the given settings back when the test finishes.
func restoreAfterTest[T any](t *testing.T, settings ...*T) {
	t.Helper()
	saved := make([]T, len(settings))
	for i, target := range settings {
		saved[i] = *target
	}
	t.Cleanup(func() {
		for i, target := range settings {
			*target = saved[i]
		}
	})
}

func TestValidateInvoiceSettingsRejectsUnsafeOrIncompleteStorage(t *testing.T) {
	payload := validInvoiceSettingsPayload()
	payload.InvoiceFileMaxCount = 0
	require.Error(t, validateInvoiceSettingsPayload(&payload))

	payload = validInvoiceSettingsPayload()
	payload.InvoiceFileStorage = "s3"
	payload.InvoiceFileS3Bucket = "invoice-bucket"
	payload.InvoiceFileS3Region = "auto"
	payload.InvoiceFileS3AccessKeyId = "access-key"
	payload.InvoiceFileS3AccessKeySecret = "secret-key"
	payload.InvoiceFileS3CustomDomain = "https://files.example.com"
	err := validateInvoiceSettingsPayload(&payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "custom domain")
}

func TestValidateInvoiceSettingsNormalizesAllowedExtensions(t *testing.T) {
	payload := validInvoiceSettingsPayload()
	payload.InvoiceFileAllowedExts = " .PDF, png,PDF,jpg "
	require.NoError(t, validateInvoiceSettingsPayload(&payload))
	assert.Equal(t, "pdf,png,jpg", payload.InvoiceFileAllowedExts)
	assert.Equal(t, "application/pdf,image/png,image/jpeg", payload.InvoiceFileAllowedMimes)
}

func TestValidateInvoiceSettingsRetentionRange(t *testing.T) {
	payload := validInvoiceSettingsPayload()
	payload.InvoiceDataRetentionDays = 1
	require.Error(t, validateInvoiceSettingsPayload(&payload))

	payload = validInvoiceSettingsPayload()
	payload.InvoiceDataRetentionDays = 30
	require.NoError(t, validateInvoiceSettingsPayload(&payload))
}

func TestValidateInvoiceSettingsTaxRateRange(t *testing.T) {
	payload := validInvoiceSettingsPayload()
	payload.InvoiceTaxRateBasisPoints = 301
	require.NoError(t, validateInvoiceSettingsPayload(&payload))

	payload.InvoiceTaxRateBasisPoints = 10001
	require.ErrorContains(t, validateInvoiceSettingsPayload(&payload), "tax rate")
}

func TestValidateInvoiceSettingsWechatGroup(t *testing.T) {
	restoreAfterTest(t, &setting.InvoiceWechatApiToken)
	setting.InvoiceWechatApiToken = "stored-token"

	payload := validInvoiceSettingsPayload()
	require.NoError(t, validateInvoiceSettingsPayload(&payload))
	assert.Equal(t, setting.DefaultInvoiceWechatApiBaseUrl, *payload.InvoiceWechatApiBaseUrl)

	payload = validInvoiceSettingsPayload()
	payload.InvoiceWechatGroupId = common.GetPointer("12345@im.wechat")
	require.ErrorContains(t, validateInvoiceSettingsPayload(&payload), "@chatroom")

	payload = validInvoiceSettingsPayload()
	payload.InvoiceWechatApiBaseUrl = common.GetPointer("ftp://api.wechatapi.net")
	require.ErrorContains(t, validateInvoiceSettingsPayload(&payload), "HTTP or HTTPS")

	payload = validInvoiceSettingsPayload()
	payload.InvoiceWechatNotifyEnabled = common.GetPointer(true)
	payload.InvoiceWechatAppId = common.GetPointer("wx_app")
	payload.InvoiceWechatGroupId = common.GetPointer("12345@chatroom")
	require.Error(t, validateInvoiceSettingsPayload(&payload), "enabling requires a token")

	for _, submitted := range []string{"", "  ", "********"} {
		payload.InvoiceWechatApiToken = common.GetPointer(submitted)
		preserveStoredInvoiceSettings(&payload)
		require.NoError(t, validateInvoiceSettingsPayload(&payload))
		assert.Equal(t, "stored-token", payload.optionValues()["InvoiceWechatApiToken"])
	}
	payload.InvoiceWechatApiToken = common.GetPointer("new-token")
	preserveStoredInvoiceSettings(&payload)
	assert.Equal(t, "new-token", *payload.InvoiceWechatApiToken)
}

func TestValidateInvoiceSettingsWechatAutoIssue(t *testing.T) {
	restoreAfterTest(t, &setting.InvoiceWechatApiToken, &setting.InvoiceWechatInboundToken)
	setting.InvoiceWechatApiToken = "stored-token"
	setting.InvoiceWechatInboundToken = ""
	botToken := strings.Repeat("ab", 16)

	payload := validInvoiceSettingsPayload()
	payload.InvoiceWechatAutoIssueEnabled = common.GetPointer(true)
	payload.InvoiceWechatInboundToken = common.GetPointer(botToken)
	require.ErrorContains(t, validateInvoiceSettingsPayload(&payload), "auto-issue", "the completed order is posted to the group")

	payload.InvoiceWechatNotifyEnabled = common.GetPointer(true)
	payload.InvoiceWechatAppId = common.GetPointer("wx_app")
	payload.InvoiceWechatGroupId = common.GetPointer("12345@chatroom")
	payload.InvoiceWechatInboundToken = nil
	preserveStoredInvoiceSettings(&payload)
	require.ErrorContains(t, validateInvoiceSettingsPayload(&payload), "auto-issue", "a bot token is required")

	payload.InvoiceWechatInboundToken = common.GetPointer("short-token")
	require.ErrorContains(t, validateInvoiceSettingsPayload(&payload), "bot token")

	payload.InvoiceWechatInboundToken = common.GetPointer(" " + botToken + " ")
	payload.InvoiceFileEnabled = false
	require.ErrorContains(t, validateInvoiceSettingsPayload(&payload), "auto-issue", "the PDF is stored as an invoice file")

	payload.InvoiceFileEnabled = true
	require.NoError(t, validateInvoiceSettingsPayload(&payload))
	digest := payload.optionValues()["InvoiceWechatInboundToken"]
	assert.Equal(t, setting.InvoiceWechatInboundTokenDigest(botToken), digest)
	assert.NotContains(t, digest, botToken, "only the digest is stored")

	setting.InvoiceWechatInboundToken = digest
	assert.True(t, setting.MatchInvoiceWechatInboundToken(botToken))
	assert.False(t, setting.MatchInvoiceWechatInboundToken(digest), "the stored digest is not a token")
	for _, submitted := range []string{"", "  ", "********"} {
		payload.InvoiceWechatInboundToken = common.GetPointer(submitted)
		preserveStoredInvoiceSettings(&payload)
		require.NoError(t, validateInvoiceSettingsPayload(&payload))
		assert.Equal(t, digest, payload.optionValues()["InvoiceWechatInboundToken"])
	}
}

func TestUpdateInvoiceSettingsKeepsSettingsAnOlderConsoleOmits(t *testing.T) {
	require.NoError(t, i18n.Init())
	setupInvoiceControllerTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}, &model.InvoiceStorageProfile{}, &model.InvoiceFileUpload{}, &model.InvoiceFileCleanup{}))
	restoreAfterTest(t, &common.RedisEnabled)
	common.RedisEnabled = false
	// Every global an invoice settings PUT writes.
	restoreAfterTest(t, &common.InvoiceApplicationNotifyAdminEnabled, &common.InvoiceIssuedNotifyUserEnabled,
		&common.InvoiceRejectedNotifyUserEnabled, &common.InvoiceExpiredNotifyUserEnabled,
		&setting.InvoiceFileEnabled, &setting.InvoiceWechatNotifyEnabled, &setting.InvoiceWechatAutoIssueEnabled)
	restoreAfterTest(t, &setting.InvoiceTaxRateBasisPoints, &setting.InvoiceDataRetentionDays,
		&setting.InvoicePendingExpiryDays, &setting.InvoiceFileMaxCount)
	restoreAfterTest(t, &setting.InvoiceMinimumAmountCents, &setting.InvoiceFileMaxSize, &setting.InvoiceFileSignedURLTTL)
	restoreAfterTest(t, &common.InvoiceAdminEmail, &setting.InvoiceFileStorage, &setting.InvoiceFileAllowedExts,
		&setting.InvoiceFileAllowedMimes, &setting.InvoiceFileLocalPath,
		&setting.InvoiceFileOSSEndpoint, &setting.InvoiceFileOSSBucket, &setting.InvoiceFileOSSRegion,
		&setting.InvoiceFileOSSAccessKeyId, &setting.InvoiceFileOSSAccessKeySecret, &setting.InvoiceFileOSSCustomDomain,
		&setting.InvoiceFileS3Endpoint, &setting.InvoiceFileS3Bucket, &setting.InvoiceFileS3Region,
		&setting.InvoiceFileS3AccessKeyId, &setting.InvoiceFileS3AccessKeySecret, &setting.InvoiceFileS3CustomDomain,
		&setting.InvoiceFileCOSEndpoint, &setting.InvoiceFileCOSBucket, &setting.InvoiceFileCOSRegion,
		&setting.InvoiceFileCOSSecretId, &setting.InvoiceFileCOSSecretKey, &setting.InvoiceFileCOSCustomDomain,
		&setting.InvoiceWechatApiBaseUrl, &setting.InvoiceWechatApiToken, &setting.InvoiceWechatAppId, &setting.InvoiceWechatGroupId,
		&setting.InvoiceWechatInboundToken)
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})

	common.InvoiceRejectedNotifyUserEnabled = true
	common.InvoiceExpiredNotifyUserEnabled = false
	setting.InvoiceWechatNotifyEnabled = true
	setting.InvoiceWechatApiBaseUrl = "http://wechat.example.com/finder/v2/api"
	setting.InvoiceWechatApiToken = "stored-token"
	setting.InvoiceWechatAppId = "wx_stored"
	setting.InvoiceWechatGroupId = "stored@chatroom"

	put := func(payload InvoiceSettingsPayload) string {
		body, err := common.Marshal(payload)
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPut, "/api/invoice/settings", bytes.NewReader(body))
		UpdateInvoiceSettings(context)
		require.Equal(t, http.StatusOK, recorder.Code)
		return recorder.Body.String()
	}
	stored := func() map[string]string {
		var options []model.Option
		require.NoError(t, model.DB.Find(&options).Error)
		values := make(map[string]string)
		for _, option := range options {
			switch option.Key {
			case "InvoiceRejectedNotifyUserEnabled", "InvoiceExpiredNotifyUserEnabled", "InvoiceWechatNotifyEnabled",
				"InvoiceWechatApiBaseUrl", "InvoiceWechatApiToken", "InvoiceWechatAppId", "InvoiceWechatGroupId":
				values[option.Key] = option.Value
			}
		}
		return values
	}

	// The merged values satisfy "enabled needs token, app ID and group",
	// so the omitted switch stays on instead of failing or turning off.
	require.Contains(t, put(legacyInvoiceSettingsPayload()), `"success":true`)
	assert.Equal(t, map[string]string{
		"InvoiceRejectedNotifyUserEnabled": "true",
		"InvoiceExpiredNotifyUserEnabled":  "false",
		"InvoiceWechatNotifyEnabled":       "true",
		"InvoiceWechatApiBaseUrl":          "http://wechat.example.com/finder/v2/api",
		"InvoiceWechatApiToken":            "stored-token",
		"InvoiceWechatAppId":               "wx_stored",
		"InvoiceWechatGroupId":             "stored@chatroom",
	}, stored())
	assert.True(t, setting.InvoiceWechatNotifyEnabled)
	assert.False(t, common.InvoiceExpiredNotifyUserEnabled)

	current := validInvoiceSettingsPayload()
	current.InvoiceRejectedNotifyUserEnabled = common.GetPointer(false)
	current.InvoiceExpiredNotifyUserEnabled = common.GetPointer(true)
	current.InvoiceWechatNotifyEnabled = common.GetPointer(false)
	current.InvoiceWechatApiBaseUrl = common.GetPointer(" http://api.wechatapi.net/finder/v2/api/ ")
	current.InvoiceWechatApiToken = common.GetPointer("new-token")
	current.InvoiceWechatAppId = common.GetPointer("wx_new")
	current.InvoiceWechatGroupId = common.GetPointer("new@chatroom")
	require.Contains(t, put(current), `"success":true`)
	assert.Equal(t, map[string]string{
		"InvoiceRejectedNotifyUserEnabled": "false",
		"InvoiceExpiredNotifyUserEnabled":  "true",
		"InvoiceWechatNotifyEnabled":       "false",
		"InvoiceWechatApiBaseUrl":          "http://api.wechatapi.net/finder/v2/api",
		"InvoiceWechatApiToken":            "new-token",
		"InvoiceWechatAppId":               "wx_new",
		"InvoiceWechatGroupId":             "new@chatroom",
	}, stored())
	assert.False(t, setting.InvoiceWechatNotifyEnabled)
	assert.Equal(t, "wx_new", setting.InvoiceWechatAppId)

	enableOnly := legacyInvoiceSettingsPayload()
	enableOnly.InvoiceWechatNotifyEnabled = common.GetPointer(true)
	enableOnly.InvoiceWechatAppId = common.GetPointer("")
	require.Contains(t, put(enableOnly), `"success":false`, "the switch needs an app ID after merging")
	assert.False(t, setting.InvoiceWechatNotifyEnabled)
	assert.Equal(t, "wx_new", stored()["InvoiceWechatAppId"])
}

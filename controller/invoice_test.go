package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicefile"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRespondInvoiceInternalErrorDoesNotExposeTechnicalDetails(t *testing.T) {
	require.NoError(t, i18n.Init())
	tests := []struct {
		language string
		message  string
	}{
		{language: "zh-CN", message: "发票操作失败，请稍后重试"},
		{language: "en", message: "The invoice operation failed. Please try again later"},
	}

	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest("GET", "/api/invoice/requests/1", nil)
			context.Request.Header.Set("Accept-Language", test.language)

			respondInvoiceInternalError(context, i18n.MsgInvoiceOperationFailed, errors.New("decrypt encrypted value: cipher: message authentication failed"))

			assert.Contains(t, recorder.Body.String(), test.message)
			assert.NotContains(t, recorder.Body.String(), "cipher")
			assert.NotContains(t, recorder.Body.String(), "decrypt encrypted value")
		})
	}
}

func setupInvoiceControllerTest(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.TopUp{},
		&model.InvoiceRequest{},
		&model.InvoiceFile{},
		&model.InvoiceRequestEvent{},
		&model.InvoiceNotificationDelivery{},
	))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
	})
}

func TestBuildInvoiceRequestDetailProtectsPendingFilesAndOperatorIdentity(t *testing.T) {
	setupInvoiceControllerTest(t)
	order := &model.TopUp{
		UserId: 8,
		Money:  600,
		Status: common.TopUpStatusSuccess,
	}
	require.NoError(t, model.DB.Create(order).Error)
	request := &model.InvoiceRequest{
		UserId:          8,
		Username:        "invoice-user",
		CompanyName:     "Invoice Company",
		TaxNumber:       "91310000TEST",
		Email:           "billing@example.com",
		TotalMoney:      600,
		TotalMoneyCents: 60000,
		Status:          model.InvoiceStatusPending,
	}
	require.NoError(t, request.SetTopUpOrderIDs([]int{order.Id}))
	require.NoError(t, model.DB.Create(request).Error)
	require.NoError(t, model.DB.Create(&model.InvoiceFile{
		InvoiceRequestId: request.Id,
		UploaderId:       77,
		FileName:         "invoice.pdf",
		StoredName:       "invoice.pdf",
		MimeType:         "application/pdf",
		Size:             128,
		StorageType:      "local",
		StorageKey:       "invoice.pdf",
	}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 8, Username: "invoice-user", AffCode: "invoice-user-code"}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 77, Username: "finance-admin", AffCode: "finance-admin-code"}).Error)
	for _, event := range []*model.InvoiceRequestEvent{
		{InvoiceRequestId: request.Id, FromStatus: 0, ToStatus: model.InvoiceStatusPending, OperatorId: 8, CreatedTime: 1},
		{InvoiceRequestId: request.Id, FromStatus: model.InvoiceStatusPending, ToStatus: model.InvoiceStatusPending, OperatorId: 77, CreatedTime: 2},
		{InvoiceRequestId: request.Id, FromStatus: model.InvoiceStatusPending, ToStatus: model.InvoiceStatusPending, OperatorId: 0, CreatedTime: 3},
	} {
		require.NoError(t, model.DB.Create(event).Error)
	}

	userDetail, err := buildInvoiceRequestDetail(request, false)
	require.NoError(t, err)
	assert.Empty(t, userDetail.Files)
	require.Len(t, userDetail.Events, 3)
	for _, event := range userDetail.Events {
		assert.Zero(t, event.OperatorId)
	}
	assert.Equal(t, "invoice-user", userDetail.Events[0].OperatorName)
	assert.Empty(t, userDetail.Events[1].OperatorName, "applicants must not learn which staff account acted")
	assert.Empty(t, userDetail.Events[2].OperatorName)

	adminDetail, err := buildInvoiceRequestDetail(request, true)
	require.NoError(t, err)
	require.Len(t, adminDetail.Files, 1)
	require.Len(t, adminDetail.Events, 3)
	assert.Equal(t, 77, adminDetail.Events[1].OperatorId)
	assert.Equal(t, "invoice-user", adminDetail.Events[0].OperatorName)
	assert.Equal(t, "finance-admin", adminDetail.Events[1].OperatorName)
	assert.Empty(t, adminDetail.Events[2].OperatorName)

	request.Status = model.InvoiceStatusIssued
	userDetail, err = buildInvoiceRequestDetail(request, false)
	require.NoError(t, err)
	require.Len(t, userDetail.Files, 1)
}

func TestDownloadInvoiceFileStreamsObjectStorageWithoutRedirect(t *testing.T) {
	setupInvoiceControllerTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.InvoiceStorageProfile{}))

	content := []byte("%PDF-1.7 issued invoice")
	var objectPath string
	objectStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		objectPath = r.URL.Path
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		_, _ = w.Write(content)
	}))
	t.Cleanup(objectStore.Close)

	profile, err := invoicefile.EnsureProfile(invoicefile.Config{
		StorageType:  "s3",
		Endpoint:     objectStore.URL,
		Bucket:       "invoices",
		Region:       "auto",
		AccessKeyId:  "test-access-key",
		AccessSecret: "test-secret-key",
	})
	require.NoError(t, err)
	request := &model.InvoiceRequest{
		UserId:          8,
		Username:        "invoice-user",
		TotalMoney:      600,
		TotalMoneyCents: 60000,
		Status:          model.InvoiceStatusIssued,
	}
	require.NoError(t, model.DB.Create(request).Error)
	file := &model.InvoiceFile{
		InvoiceRequestId: request.Id,
		StorageProfileId: profile.Id,
		StorageType:      "s3",
		StorageKey:       "1/2026/10/invoice.pdf",
		FileName:         "invoice.pdf",
		StoredName:       "invoice.pdf",
		MimeType:         "application/pdf",
		Size:             int64(len(content)),
	}
	require.NoError(t, model.DB.Create(file).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/invoice/requests/1/files/1", nil)
	context.Params = gin.Params{
		{Key: "id", Value: strconv.Itoa(request.Id)},
		{Key: "file_id", Value: strconv.Itoa(file.Id)},
	}
	context.Set("id", 8)
	context.Set("role", common.RoleCommonUser)

	DownloadInvoiceFile(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Empty(t, recorder.Header().Get("Location"))
	assert.Equal(t, content, recorder.Body.Bytes())
	assert.Equal(t, "application/pdf", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "attachment; filename*=UTF-8''invoice.pdf", recorder.Header().Get("Content-Disposition"))
	assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "/invoices/1/2026/10/invoice.pdf", objectPath)
}

func TestGetInvoiceConfigDescribesApplicantAndUploadLimits(t *testing.T) {
	setupInvoiceControllerTest(t)
	previousExpiryDays := setting.InvoicePendingExpiryDays
	previousExts := setting.InvoiceFileAllowedExts
	previousCount := setting.InvoiceFileMaxCount
	previousSize := setting.InvoiceFileMaxSize
	setting.InvoicePendingExpiryDays = 30
	setting.InvoiceFileAllowedExts = "pdf,PNG, .jpg"
	setting.InvoiceFileMaxCount = 3
	setting.InvoiceFileMaxSize = 2 * 1024 * 1024
	t.Cleanup(func() {
		setting.InvoicePendingExpiryDays = previousExpiryDays
		setting.InvoiceFileAllowedExts = previousExts
		setting.InvoiceFileMaxCount = previousCount
		setting.InvoiceFileMaxSize = previousSize
	})
	bound := &model.User{Username: "config-bound", Email: "billing@example.com", AffCode: "config-bound-code"}
	unbound := &model.User{Username: "config-unbound", AffCode: "config-unbound-code"}
	require.NoError(t, model.DB.Create(bound).Error)
	require.NoError(t, model.DB.Create(unbound).Error)

	type invoiceConfig struct {
		Email             string   `json:"email"`
		EmailBound        bool     `json:"email_bound"`
		PendingExpiryDays int      `json:"pending_expiry_days"`
		FileAllowedExts   []string `json:"file_allowed_exts"`
		FileMaxCount      int      `json:"file_max_count"`
		FileMaxSize       int64    `json:"file_max_size"`
		MinimumAmount     int64    `json:"minimum_amount_cents"`
	}
	for _, test := range []struct {
		user  *model.User
		email string
	}{
		{user: bound, email: "billing@example.com"},
		{user: unbound, email: ""},
	} {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodGet, "/api/invoice/config", nil)
		context.Set("id", test.user.Id)

		GetInvoiceConfig(context)

		var response struct {
			Success bool          `json:"success"`
			Data    invoiceConfig `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.True(t, response.Success)
		assert.Equal(t, test.email, response.Data.Email)
		assert.Equal(t, test.email != "", response.Data.EmailBound)
		assert.Equal(t, 30, response.Data.PendingExpiryDays)
		assert.Equal(t, []string{"pdf", "png", "jpg"}, response.Data.FileAllowedExts)
		assert.Equal(t, 3, response.Data.FileMaxCount)
		assert.Equal(t, int64(2*1024*1024), response.Data.FileMaxSize)
		assert.Equal(t, setting.InvoiceMinimumAmountCents, response.Data.MinimumAmount)
	}
}

func TestDestructiveInvoiceMaintenanceRequiresRoot(t *testing.T) {
	require.NoError(t, i18n.Init())
	for name, handler := range map[string]gin.HandlerFunc{
		"orphan cleanup": CleanupInvoiceOrphans,
		"wechat test":    TestInvoiceWechatNotification,
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/invoice/admin/maintenance",
				strings.NewReader(`{"keys":[{"storage_profile_id":1,"storage_key":"avatars/1.png"}]}`))
			context.Set("role", common.RoleAdminUser)

			handler(context)

			assert.Equal(t, http.StatusForbidden, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "AUTH_INSUFFICIENT_PRIVILEGE")
		})
	}
}

package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicefile"
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
	require.NoError(t, model.DB.Create(&model.InvoiceRequestEvent{
		InvoiceRequestId: request.Id,
		FromStatus:       0,
		ToStatus:         model.InvoiceStatusPending,
		OperatorId:       77,
		CreatedTime:      1,
	}).Error)

	userDetail, err := buildInvoiceRequestDetail(request, false)
	require.NoError(t, err)
	assert.Empty(t, userDetail.Files)
	require.Len(t, userDetail.Events, 1)
	assert.Zero(t, userDetail.Events[0].OperatorId)

	adminDetail, err := buildInvoiceRequestDetail(request, true)
	require.NoError(t, err)
	require.Len(t, adminDetail.Files, 1)
	require.Len(t, adminDetail.Events, 1)
	assert.Equal(t, 77, adminDetail.Events[0].OperatorId)

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
	assert.Equal(t, "/invoices/1/2026/10/invoice.pdf", objectPath)
}

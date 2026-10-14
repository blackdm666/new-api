package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/invoicefile"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type invoiceFileUploadResponse struct {
	Id          int    `json:"id"`
	FileName    string `json:"file_name"`
	MimeType    string `json:"mime_type"`
	Size        int64  `json:"size"`
	Sha256      string `json:"sha256"`
	StorageType string `json:"storage_type"`
	Previewable bool   `json:"previewable"`
	CreatedTime int64  `json:"created_time"`
}

func UploadInvoiceFile(c *gin.Context) {
	requestId, ok := parseInvoiceRequestID(c)
	if !ok {
		return
	}
	if !setting.InvoiceFileEnabled {
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileDisabled)
		return
	}
	request, err := model.GetInvoiceRequestById(requestId)
	if err != nil {
		handleInvoiceError(c, err, i18n.MsgInvoiceUploadFailed)
		return
	}
	if request.Status != model.InvoiceStatusPending {
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileMutationFinal)
		return
	}
	if request.RedactedTime != 0 {
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileMutationArchived)
		return
	}

	maxBody := setting.InvoiceFileMaxSize + setting.InvoiceFileMaxSize/10
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileMissing)
		return
	}
	if fileHeader.Size <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileEmpty)
		return
	}
	if fileHeader.Size > setting.InvoiceFileMaxSize {
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileTooLarge)
		return
	}

	source, err := fileHeader.Open()
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceUploadFailed, err)
		return
	}
	defer source.Close()
	record, err := service.StoreInvoiceFile(c.Request.Context(), service.InvoiceFileContent{
		RequestId:    requestId,
		UploaderId:   c.GetInt("id"),
		FileName:     fileHeader.Filename,
		DeclaredMime: fileHeader.Header.Get("Content-Type"),
		Size:         fileHeader.Size,
		Body:         source,
	})
	if err != nil {
		handleInvoiceFileValidationError(c, err)
		return
	}
	common.ApiSuccess(c, invoiceFileUploadResponse{
		Id:          record.Id,
		FileName:    record.FileName,
		MimeType:    record.MimeType,
		Size:        record.Size,
		Sha256:      record.Sha256,
		StorageType: record.StorageType,
		Previewable: isInvoiceFilePreviewable(record.MimeType),
		CreatedTime: record.CreatedTime,
	})
}

// ReceiveInvoiceWechatFile takes an invoice PDF that the finance group bot saw
// in the configured WeChat group and issues the matching pending request. The
// bot authenticates with its own token, which grants nothing else.
func ReceiveInvoiceWechatFile(c *gin.Context) {
	token, bearer := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !bearer || !setting.MatchInvoiceWechatInboundToken(token) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgTokenInvalid)})
		return
	}
	if !service.InvoiceWechatAutoIssueReady() {
		common.ApiErrorI18n(c, i18n.MsgInvoiceWechatAutoIssueOff)
		return
	}
	maxBody := setting.InvoiceFileMaxSize + setting.InvoiceFileMaxSize/10
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileMissing)
		return
	}
	if strings.TrimSpace(c.PostForm("group_id")) != strings.TrimSpace(setting.InvoiceWechatGroupId) {
		common.ApiErrorI18n(c, i18n.MsgInvoiceWechatGroupMismatch)
		return
	}
	if fileHeader.Size <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileEmpty)
		return
	}
	if fileHeader.Size > setting.InvoiceFileMaxSize {
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileTooLarge)
		return
	}
	source, err := fileHeader.Open()
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceUploadFailed, err)
		return
	}
	defer source.Close()
	content, err := io.ReadAll(io.LimitReader(source, setting.InvoiceFileMaxSize))
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceUploadFailed, err)
		return
	}
	// The multipart file name can lose non-ASCII characters on the way, so
	// the bot also sends the WeChat file name as a plain field.
	fileName := strings.TrimSpace(c.PostForm("file_name"))
	if fileName == "" {
		fileName = fileHeader.Filename
	}
	result, err := service.IssueInvoiceFromWechatFile(c.Request.Context(), fileName, content)
	if err != nil {
		handleInvoiceFileValidationError(c, err)
		return
	}
	common.SysLog(fmt.Sprintf("invoice wechat file %q (message %q): %s request=%d",
		fileName, c.PostForm("message_id"), result.Outcome, result.RequestId))
	common.ApiSuccess(c, result)
}

func DeleteInvoiceFile(c *gin.Context) {
	requestId, ok := parseInvoiceRequestID(c)
	if !ok {
		return
	}
	fileId, err := strconv.Atoi(c.Param("file_id"))
	if err != nil || fileId <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	cleanup, err := model.QueueInvoiceFileDeletion(requestId, fileId)
	if err != nil {
		handleInvoiceFileMutationError(c, err, i18n.MsgInvoiceFileDeleteFailed)
		return
	}
	service.ScheduleInvoiceFileCleanup(cleanup)
	common.ApiSuccess(c, gin.H{"id": fileId})
}

func DownloadInvoiceFile(c *gin.Context) {
	requestId, ok := parseInvoiceRequestID(c)
	if !ok {
		return
	}
	fileId, err := strconv.Atoi(c.Param("file_id"))
	if err != nil || fileId <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	request, err := model.GetInvoiceRequestById(requestId)
	if err != nil {
		handleInvoiceError(c, err)
		return
	}
	if c.GetInt("role") < common.RoleAdminUser && request.UserId != c.GetInt("id") {
		common.ApiErrorI18n(c, i18n.MsgInvoiceNotFound)
		return
	}
	if c.GetInt("role") < common.RoleAdminUser && request.Status != model.InvoiceStatusIssued {
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileNotFound)
		return
	}
	file, err := model.GetInvoiceFileById(fileId)
	if err != nil || file.InvoiceRequestId != requestId {
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileNotFound)
		return
	}
	storage, err := invoicefile.ForProfile(file.StorageProfileId, file.StorageType)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceDownloadFailed, err)
		return
	}

	disposition := "attachment"
	if c.Query("inline") == "1" && isInvoiceFilePreviewable(file.MimeType) {
		disposition = "inline"
	}
	dispositionHeader := invoicefile.ResponseContentDisposition(file.FileName, disposition == "inline")

	// Stream every backend through this response instead of redirecting to a
	// signed object-storage URL. The console downloads with an authenticated
	// XHR, and a cross-origin redirect would require CORS on the bucket.
	reader, err := storage.Get(c.Request.Context(), file.StorageKey)
	if err != nil {
		if errors.Is(err, invoicefile.ErrNotFound) {
			common.ApiErrorI18n(c, i18n.MsgInvoiceFileContentMissing)
			return
		}
		respondInvoiceInternalError(c, i18n.MsgInvoiceDownloadFailed, err)
		return
	}
	defer reader.Close()
	c.Writer.Header().Set("Content-Type", file.MimeType)
	c.Writer.Header().Set("Content-Disposition", dispositionHeader)
	c.Writer.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
	// Files are authorization-gated and may be replaced or purged, so no
	// browser or intermediary may keep a copy.
	c.Writer.Header().Set("Cache-Control", "private, no-store")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, reader); err != nil {
		common.SysLog("invoice file download failed: " + err.Error())
	}
}

func handleInvoiceFileValidationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, invoicefile.ErrFileDisabled):
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileDisabled)
	case errors.Is(err, invoicefile.ErrFileSizeExceeded):
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileTooLarge)
	case errors.Is(err, invoicefile.ErrFileEmpty):
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileEmpty)
	case errors.Is(err, invoicefile.ErrFileName):
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileNameInvalid)
	case errors.Is(err, invoicefile.ErrFileSVGForbidden):
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileSVGForbidden)
	case errors.Is(err, invoicefile.ErrFileExtNotAllowed), errors.Is(err, invoicefile.ErrFileMimeNotAllowed):
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileTypeNotAllowed)
	case errors.Is(err, invoicefile.ErrFileMimeMismatch):
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileTypeMismatch)
	default:
		handleInvoiceFileMutationError(c, err, i18n.MsgInvoiceUploadFailed)
	}
}

func handleInvoiceFileMutationError(c *gin.Context, err error, fallbackKeys ...string) {
	switch {
	case errors.Is(err, model.ErrInvoiceFileLimit):
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileLimit)
	case errors.Is(err, model.ErrInvoiceFileMutationRejected):
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileMutationRejected)
	case errors.Is(err, model.ErrInvoiceFileMutationArchived):
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileMutationArchived)
	case errors.Is(err, model.ErrInvoiceFileMutationFinal):
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileMutationFinal)
	case errors.Is(err, model.ErrInvoiceIssuedFileRequired):
		common.ApiErrorI18n(c, i18n.MsgInvoiceIssuedFileRequired)
	case errors.Is(err, model.ErrInvoiceFileNotFound):
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileNotFound)
	default:
		handleInvoiceError(c, err, fallbackKeys...)
	}
}

func isInvoiceFilePreviewable(mime string) bool {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if idx := strings.Index(mime, ";"); idx >= 0 {
		mime = strings.TrimSpace(mime[:idx])
	}
	return strings.HasPrefix(mime, "image/") && mime != "image/svg+xml"
}

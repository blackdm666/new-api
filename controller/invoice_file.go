package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/invoicefile"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	head, combined, err := invoicefile.ReadSniff(source)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceUploadFailed, err)
		return
	}
	validation, err := invoicefile.Validate(
		fileHeader.Filename,
		fileHeader.Header.Get("Content-Type"),
		fileHeader.Size,
		head,
	)
	if err != nil {
		handleInvoiceFileValidationError(c, err)
		return
	}
	profile, err := invoicefile.EnsureCurrentProfile()
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceUploadFailed, err)
		return
	}
	storage, err := invoicefile.ForProfile(profile.Id, profile.StorageType)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceUploadFailed, err)
		return
	}

	storedName := uuid.NewString()
	if validation.Ext != "" {
		storedName += "." + validation.Ext
	}
	now := time.Now().UTC()
	storageKey := path.Join(
		strconv.Itoa(requestId),
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", now.Month()),
		storedName,
	)
	uploadId := uuid.NewString()
	staged := &model.InvoiceFileUpload{
		Id:               uploadId,
		InvoiceRequestId: requestId,
		UploaderId:       c.GetInt("id"),
		StorageProfileId: profile.Id,
		StorageType:      storage.Kind(),
		StorageKey:       storageKey,
		FileName:         validation.SafeName,
		StoredName:       storedName,
		MimeType:         validation.DetectedMime,
		Size:             fileHeader.Size,
	}
	if err := model.CreateInvoiceFileUpload(staged, setting.InvoiceFileMaxCount); err != nil {
		handleInvoiceFileMutationError(c, err, i18n.MsgInvoiceUploadFailed)
		return
	}
	hasher := sha256.New()
	reader := io.TeeReader(combined, hasher)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	if err := storage.Put(ctx, storageKey, reader, fileHeader.Size, validation.DetectedMime); err != nil {
		cleanup, abandonErr := model.AbandonInvoiceFileUpload(uploadId, err.Error())
		if abandonErr == nil {
			service.ScheduleInvoiceFileCleanup(cleanup)
		}
		respondInvoiceInternalError(c, i18n.MsgInvoiceUploadFailed, err)
		return
	}
	record, err := model.FinalizeInvoiceFileUpload(uploadId, hex.EncodeToString(hasher.Sum(nil)))
	if err != nil {
		cleanup, abandonErr := model.AbandonInvoiceFileUpload(uploadId, err.Error())
		if abandonErr == nil {
			service.ScheduleInvoiceFileCleanup(cleanup)
		}
		handleInvoiceFileMutationError(c, err, i18n.MsgInvoiceUploadFailed)
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
	ttl := time.Duration(setting.InvoiceFileSignedURLTTL) * time.Second
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if file.StorageType != "local" {
		signedURL, signErr := storage.SignedURL(c.Request.Context(), file.StorageKey, ttl, file.FileName, disposition == "inline")
		if signErr == nil && signedURL != "" {
			c.Redirect(http.StatusFound, signedURL)
			return
		}
	}

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
	c.Writer.Header().Set("Cache-Control", "private, max-age=604800, immutable")
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
		respondInvoiceInternalError(c, i18n.MsgInvoiceUploadFailed, err)
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

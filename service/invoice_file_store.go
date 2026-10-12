package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicefile"
	"github.com/QuantumNous/new-api/setting"
	"github.com/google/uuid"
)

// InvoiceFileContent is a file to attach to a pending invoice request.
type InvoiceFileContent struct {
	RequestId    int
	UploaderId   int
	FileName     string
	DeclaredMime string
	Size         int64
	Body         io.Reader
}

// StoreInvoiceFile validates the file, writes it to the current invoice
// storage and records it on the request. A file whose upload or record fails
// is queued for cleanup. Validation errors come from invoicefile and record
// errors from model.
func StoreInvoiceFile(ctx context.Context, content InvoiceFileContent) (*model.InvoiceFile, error) {
	head, combined, err := invoicefile.ReadSniff(content.Body)
	if err != nil {
		return nil, err
	}
	validation, err := invoicefile.Validate(content.FileName, content.DeclaredMime, content.Size, head)
	if err != nil {
		return nil, err
	}
	profile, err := invoicefile.EnsureCurrentProfile()
	if err != nil {
		return nil, err
	}
	storage, err := invoicefile.ForProfile(profile.Id, profile.StorageType)
	if err != nil {
		return nil, err
	}

	storedName := uuid.NewString()
	if validation.Ext != "" {
		storedName += "." + validation.Ext
	}
	storageKey := model.InvoiceFileStorageKey(content.RequestId, time.Now(), storedName)
	uploadId := uuid.NewString()
	staged := &model.InvoiceFileUpload{
		Id:               uploadId,
		InvoiceRequestId: content.RequestId,
		UploaderId:       content.UploaderId,
		StorageProfileId: profile.Id,
		StorageType:      storage.Kind(),
		StorageKey:       storageKey,
		FileName:         validation.SafeName,
		StoredName:       storedName,
		MimeType:         validation.DetectedMime,
		Size:             content.Size,
	}
	if err := model.CreateInvoiceFileUpload(staged, setting.InvoiceFileMaxCount); err != nil {
		return nil, err
	}
	hasher := sha256.New()
	putCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := storage.Put(putCtx, storageKey, io.TeeReader(combined, hasher), content.Size, validation.DetectedMime); err != nil {
		abandonInvoiceFileUpload(uploadId, err)
		return nil, err
	}
	record, err := model.FinalizeInvoiceFileUpload(uploadId, hex.EncodeToString(hasher.Sum(nil)))
	if err != nil {
		abandonInvoiceFileUpload(uploadId, err)
		return nil, err
	}
	return record, nil
}

func abandonInvoiceFileUpload(uploadId string, cause error) {
	cleanup, err := model.AbandonInvoiceFileUpload(uploadId, cause.Error())
	if err == nil {
		ScheduleInvoiceFileCleanup(cleanup)
	}
}

package service

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicefile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconcileInvoiceStorageReportsOnlyUnreferencedInvoiceKeys(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(
		&model.InvoiceStorageProfile{},
		&model.InvoiceFile{},
		&model.InvoiceFileUpload{},
		&model.InvoiceFileCleanup{},
	))
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM invoice_files")
		model.DB.Exec("DELETE FROM invoice_file_uploads")
		model.DB.Exec("DELETE FROM invoice_file_cleanups")
		model.DB.Exec("DELETE FROM invoice_storage_profiles")
	})

	profile, err := invoicefile.EnsureProfile(invoicefile.Config{StorageType: "local", LocalPath: t.TempDir()})
	require.NoError(t, err)
	storage, err := invoicefile.ForProfile(profile.Id, profile.StorageType)
	require.NoError(t, err)

	uploadedAt := time.Date(2026, time.October, 12, 8, 0, 0, 0, time.UTC)
	orphanKey := model.InvoiceFileStorageKey(7, uploadedAt, "0f8fad5b-d9cb-469f-a165-70867728950e.pdf")
	referencedKey := model.InvoiceFileStorageKey(7, uploadedAt, "7c9e6679-7425-40de-944b-e07fc1f90ae7.pdf")
	ctx := context.Background()
	for _, key := range []string{orphanKey, referencedKey, "1-readme.txt", "avatars/7.png", "_health/check.txt"} {
		body := []byte(key)
		require.NoError(t, storage.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "application/octet-stream"))
	}
	// The referencing row lives under another profile that shares the bucket;
	// it must still protect the object.
	require.NoError(t, model.DB.Create(&model.InvoiceFile{
		InvoiceRequestId: 7, UploaderId: 1, FileName: "invoice.pdf", StoredName: "invoice.pdf",
		MimeType: "application/pdf", Size: 1, StorageProfileId: profile.Id + 1, StorageType: "local",
		StorageKey: referencedKey,
	}).Error)

	report, err := ReconcileInvoiceStorage(ctx, 2)
	require.NoError(t, err)
	require.Len(t, report.Profiles, 1)
	assert.Empty(t, report.Profiles[0].Error)
	assert.Equal(t, 3, report.Profiles[0].ObjectsScanned, "only the request-id key space is listed")
	assert.Equal(t, []model.InvoiceStorageKey{{
		StorageProfileId: profile.Id,
		StorageType:      "local",
		StorageKey:       orphanKey,
	}}, report.OrphanKeys)
}

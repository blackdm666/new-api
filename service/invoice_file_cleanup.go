package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicefile"
	"github.com/bytedance/gopkg/util/gopool"
)

const invoiceFileCleanupInterval = 5 * time.Minute
const invoiceFileCleanupLease = 3 * time.Minute
const invoiceFileUploadStaleAfter = 15 * time.Minute

func StartInvoiceFileCleanup() {
	if !common.IsMasterNode {
		return
	}
	gopool.Go(func() {
		cleanupPendingInvoiceFiles()
		ticker := time.NewTicker(invoiceFileCleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			cleanupPendingInvoiceFiles()
		}
	})
}

func ScheduleInvoiceFileCleanup(cleanup *model.InvoiceFileCleanup) {
	if cleanup == nil {
		return
	}
	gopool.Go(func() {
		processInvoiceFileCleanup(cleanup)
	})
}

func cleanupPendingInvoiceFiles() {
	cleanupStaleInvoiceFileUploads()
	cleanups, err := model.ListPendingInvoiceFileCleanups(100, common.GetTimestamp())
	if err != nil {
		common.SysError("failed to list pending invoice file cleanups: " + err.Error())
		return
	}
	for _, cleanup := range cleanups {
		processInvoiceFileCleanup(cleanup)
	}
}

func processInvoiceFileCleanup(cleanup *model.InvoiceFileCleanup) {
	if cleanup == nil {
		return
	}
	now := common.GetTimestamp()
	claimed, err := model.ClaimInvoiceFileCleanup(cleanup.Id, now, time.Now().Add(invoiceFileCleanupLease).Unix())
	if err != nil || !claimed {
		if err != nil {
			common.SysError(fmt.Sprintf("failed to claim invoice file cleanup %d: %s", cleanup.Id, err.Error()))
		}
		return
	}
	storage, err := invoicefile.ForProfile(cleanup.StorageProfileId, cleanup.StorageType)
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		err = storage.Delete(ctx, cleanup.StorageKey)
		cancel()
	}
	if err == nil || errors.Is(err, invoicefile.ErrNotFound) {
		if completeErr := model.CompleteInvoiceFileCleanup(cleanup.Id); completeErr != nil {
			common.SysError(fmt.Sprintf("failed to complete invoice file cleanup %d: %s", cleanup.Id, completeErr.Error()))
		}
		return
	}
	delay := invoiceFileCleanupInterval
	for attempt := 0; attempt < cleanup.Attempts && delay < 24*time.Hour; attempt++ {
		delay *= 2
	}
	if delay > 24*time.Hour {
		delay = 24 * time.Hour
	}
	nextAttempt := time.Now().Add(delay).Unix()
	if recordErr := model.RecordInvoiceFileCleanupFailure(cleanup.Id, err.Error(), nextAttempt); recordErr != nil {
		common.SysError(fmt.Sprintf("failed to record invoice file cleanup %d: %s", cleanup.Id, recordErr.Error()))
	}
}

func cleanupStaleInvoiceFileUploads() {
	cutoff := time.Now().Add(-invoiceFileUploadStaleAfter).Unix()
	uploads, err := model.ListStaleInvoiceFileUploads(cutoff, 100)
	if err != nil {
		common.SysError("failed to list stale invoice file uploads: " + err.Error())
		return
	}
	for _, upload := range uploads {
		cleanup, abandonErr := model.AbandonInvoiceFileUpload(upload.Id, "stale upload staging record")
		if abandonErr != nil {
			common.SysError(fmt.Sprintf("failed to abandon stale invoice upload %s: %s", upload.Id, abandonErr.Error()))
			continue
		}
		if cleanup != nil {
			processInvoiceFileCleanup(cleanup)
		}
	}
}

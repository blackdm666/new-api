package controller

import (
	"context"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type cleanupOrphanInvoiceFilesPayload struct {
	Keys []service.InvoiceStorageKey `json:"keys"`
}

func GetInvoiceMaintenance(c *gin.Context) {
	cleanups, err := model.ListInvoiceFileCleanups(200)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceMaintenanceFailed, err)
		return
	}
	uploads, err := model.ListInvoiceFileUploads(200)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceMaintenanceFailed, err)
		return
	}
	profiles, err := model.ListInvoiceStorageProfiles()
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceMaintenanceFailed, err)
		return
	}
	notifications, err := model.ListPendingInvoiceNotifications(200)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceMaintenanceFailed, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"cleanups":      cleanups,
		"uploads":       uploads,
		"profiles":      profiles,
		"notifications": notifications,
	})
}

func RetryInvoiceFileCleanup(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("cleanup_id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	cleanup, err := model.RetryInvoiceFileCleanup(id)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceMaintenanceFailed, err)
		return
	}
	service.ScheduleInvoiceFileCleanup(cleanup)
	recordManageAudit(c, "invoice.cleanup.retry", map[string]interface{}{"cleanup_id": id})
	common.ApiSuccess(c, cleanup)
}

func RetryInvoiceNotification(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("delivery_id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	delivery, err := model.RetryInvoiceNotification(id)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceMaintenanceFailed, err)
		return
	}
	service.ScheduleInvoiceNotification(delivery)
	recordManageAudit(c, "invoice.notification.retry", map[string]interface{}{"delivery_id": id})
	common.ApiSuccess(c, delivery)
}

func ReconcileInvoiceStorage(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Minute)
	defer cancel()
	report, err := service.ReconcileInvoiceStorage(ctx, 2000)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceMaintenanceFailed, err)
		return
	}
	common.ApiSuccess(c, report)
}

func CleanupInvoiceOrphans(c *gin.Context) {
	var payload cleanupOrphanInvoiceFilesPayload
	if err := common.DecodeJson(c.Request.Body, &payload); err != nil || len(payload.Keys) == 0 || len(payload.Keys) > 2000 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	cleanups, err := service.QueueInvoiceOrphanCleanups(payload.Keys)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceMaintenanceFailed, err)
		return
	}
	for _, cleanup := range cleanups {
		service.ScheduleInvoiceFileCleanup(cleanup)
	}
	recordManageAudit(c, "invoice.orphans.cleanup", map[string]interface{}{"count": len(cleanups)})
	common.ApiSuccess(c, gin.H{"queued": len(cleanups)})
}

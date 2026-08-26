package service

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/bytedance/gopkg/util/gopool"
)

const invoiceRetentionInterval = time.Hour

func StartInvoiceDataRetention() {
	if !common.IsMasterNode {
		return
	}
	gopool.Go(func() {
		redactExpiredInvoiceData()
		ticker := time.NewTicker(invoiceRetentionInterval)
		defer ticker.Stop()
		for range ticker.C {
			redactExpiredInvoiceData()
		}
	})
}

func redactExpiredInvoiceData() {
	warnPendingInvoiceExpiry()
	expirePendingInvoiceRequests()
	days := setting.InvoiceDataRetentionDays
	if days <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	for {
		count, err := model.RedactExpiredInvoiceRequests(cutoff, 100)
		if err != nil {
			common.SysError("failed to redact expired invoice data: " + err.Error())
			return
		}
		if count > 0 {
			common.SysLog(fmt.Sprintf("redacted %d expired invoice applications", count))
		}
		if count < 100 {
			return
		}
	}
}

func warnPendingInvoiceExpiry() {
	days := setting.InvoicePendingExpiryDays
	if days <= 0 {
		return
	}
	now := time.Now()
	warningCutoff := now.Add(-time.Duration(days-1) * 24 * time.Hour).Unix()
	expiryCutoff := now.Add(-time.Duration(days) * 24 * time.Hour).Unix()
	for {
		count, err := model.QueuePendingInvoiceExpiryWarnings(warningCutoff, expiryCutoff, 100, BuildInvoiceExpiryWarningNotifications)
		if err != nil {
			common.SysError("failed to queue pending invoice expiry warnings: " + err.Error())
			return
		}
		if count > 0 {
			common.SysLog(fmt.Sprintf("queued %d pending invoice expiry warnings", count))
		}
		if count < 100 {
			return
		}
	}
}

func expirePendingInvoiceRequests() {
	days := setting.InvoicePendingExpiryDays
	if days <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	for {
		count, err := model.ExpirePendingInvoiceRequests(cutoff, 100)
		if err != nil {
			common.SysError("failed to expire pending invoice applications: " + err.Error())
			return
		}
		if count > 0 {
			common.SysLog(fmt.Sprintf("expired %d stale invoice applications", count))
		}
		if count < 100 {
			return
		}
	}
}

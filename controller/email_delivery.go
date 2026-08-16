package controller

import (
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ListEmailDeliveries exposes delivery metadata to Root. Subject, body and
// unsanitized recipient addresses are deliberately excluded by the model.
func ListEmailDeliveries(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	rows, total, err := model.ListEmailDeliveries(model.EmailDeliveryQueryOptions{
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		Status:   strings.TrimSpace(c.Query("status")),
		Category: strings.TrimSpace(c.Query("category")),
	}, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(rows)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func GetEmailDeliveryStats(c *gin.Context) {
	now := time.Now()
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	localNow := now.In(location)
	dayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location).Unix()
	stats, err := model.GetEmailDeliveryStats(now.Unix(), dayStart)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	circuit, err := model.GetMarketingCircuitState()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	categories, err := model.ListEmailDeliveryCategories()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"queue":                     stats,
		"categories":                categories,
		"smtp_configured":           strings.TrimSpace(common.SMTPServer) != "" && (strings.TrimSpace(common.SMTPFrom) != "" || strings.TrimSpace(common.SMTPAccount) != ""),
		"marketing_daily_limit":     model.MarketingDailyLimit,
		"marketing_daily_remaining": max(0, model.MarketingDailyLimit-int(stats.MarketingSentToday)),
		"marketing_circuit_breaker": circuit,
	})
}

func RetryFailedEmailDelivery(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrEmailDeliveryIdInvalid)
		return
	}
	if err := model.RetryFailedEmailDelivery(id); err != nil {
		common.ApiError(c, err)
		return
	}
	delivery, err := model.GetEmailDeliveryById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if delivery.InvoiceDeliveryId > 0 {
		_ = model.SyncInvoiceNotificationFromEmailDelivery(delivery)
	}
	_ = model.RetryMarketingRecipientByEmailDeliveryId(delivery.Id)
	service.ScheduleSystemEmail(delivery)
	common.ApiSuccess(c, gin.H{"retried": true})
}

type retryEmailDeliveriesRequest struct {
	Ids []int `json:"ids"`
}

func RetryFailedEmailDeliveries(c *gin.Context) {
	var req retryEmailDeliveriesRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 || len(req.Ids) > 100 {
		common.ApiError(c, model.ErrEmailDeliveryIdInvalid)
		return
	}
	count, err := model.RetryEmailDeliveries(req.Ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for _, id := range req.Ids {
		delivery, getErr := model.GetEmailDeliveryById(id)
		if getErr != nil || delivery.DeliveredTime != 0 || delivery.DeadLetterTime != 0 || delivery.ExpiredTime != 0 {
			continue
		}
		if delivery.InvoiceDeliveryId > 0 {
			_ = model.SyncInvoiceNotificationFromEmailDelivery(delivery)
		}
		_ = model.RetryMarketingRecipientByEmailDeliveryId(delivery.Id)
		service.ScheduleSystemEmail(delivery)
	}
	common.ApiSuccess(c, gin.H{"retried": count})
}

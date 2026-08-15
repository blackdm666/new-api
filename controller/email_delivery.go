package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ListFailedEmailDeliveries exposes retryable dead letters to Root without
// mixing them into business-specific invoice attachment deliveries.
func ListFailedEmailDeliveries(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	rows, total, err := model.ListEmailDeliveries(model.EmailDeliveryQueryOptions{
		Keyword:    strings.TrimSpace(c.Query("keyword")),
		FailedOnly: true,
	}, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(rows)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func RetryFailedEmailDelivery(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrEmailDeliveryIdInvalid)
		return
	}
	if err := model.RetryEmailDelivery(id); err != nil {
		common.ApiError(c, err)
		return
	}
	delivery, err := model.GetEmailDeliveryById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	service.ScheduleSystemEmail(delivery)
	common.ApiSuccess(c, gin.H{"retried": true})
}

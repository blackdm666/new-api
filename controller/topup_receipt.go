package controller

// 88api topup receipt: download a top-up's PDF receipt, or have it emailed
// again.

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// DownloadTopUpReceipt returns the PDF receipt of a successful top-up. Users
// get their own orders; administrators get any order. Another user's order
// is reported as missing.
func DownloadTopUpReceipt(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	topUp := model.GetTopUpById(id)
	if topUp == nil || (topUp.UserId != c.GetInt("id") && c.GetInt("role") < common.RoleAdminUser) {
		common.ApiErrorT(c, "Top-up order not found")
		return
	}
	owner, err := model.GetUserById(topUp.UserId, false)
	if err != nil {
		common.ApiErrorT(c, "Top-up order not found")
		return
	}
	content, err := service.RenderTopUpReceipt(topUp, owner, owner.GetSetting().Language)
	if err != nil {
		common.ApiErrorT(c, "Only paid and completed top-up orders have a receipt")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, service.TopUpReceiptFileName(topUp)))
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/pdf", content)
}

type sendTopUpReceiptRequest struct {
	TradeNo string `json:"trade_no"`
}

// AdminSendTopUpReceipt emails the PDF receipt of a successful top-up to its
// user again, whether or not automatic receipts are enabled.
func AdminSendTopUpReceipt(c *gin.Context) {
	var req sendTopUpReceiptRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	topUp := model.GetTopUpByTradeNo(req.TradeNo)
	if topUp == nil {
		common.ApiErrorT(c, "Top-up order not found")
		return
	}
	key := fmt.Sprintf("topup_receipt:%d:resend:%d:%d", topUp.Id, c.GetInt("id"), time.Now().UnixNano())
	delivery, err := service.QueueTopUpReceiptEmail(topUp, key)
	if err != nil {
		if err == service.ErrTopUpReceiptUnavailable {
			common.ApiErrorT(c, "Only paid and completed top-up orders have a receipt")
			return
		}
		common.ApiError(c, err)
		return
	}
	if delivery == nil {
		common.ApiErrorT(c, "The user has no email address")
		return
	}
	common.ApiSuccess(c, gin.H{"email_delivery_id": delivery.Id})
}

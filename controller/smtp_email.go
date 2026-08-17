package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type smtpTestRequest struct {
	Email   string `json:"email"`
	Channel string `json:"channel"`
}

func TestSMTPEmail(c *gin.Context) {
	var request smtpTestRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	recipient, result, err := service.SendSMTPTestEmail(c.GetInt("id"), request.Email, request.Channel)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSMTPTestRecipientRequired):
			common.ApiErrorI18n(c, i18n.MsgUserEmailEmpty)
		case errors.Is(err, service.ErrSMTPTestRecipientInvalid):
			common.ApiErrorI18n(c, i18n.MsgSettingEmailInvalid)
		case errors.Is(err, service.ErrSMTPTestChannelInvalid):
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "无效的 SMTP 通道",
			})
		default:
			common.ApiError(c, err)
		}
		return
	}

	common.ApiSuccess(c, gin.H{
		"recipient": recipient,
		"channel":   result.Channel,
	})
}

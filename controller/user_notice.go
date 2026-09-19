package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func SearchUserNoticeRecipients(c *gin.Context) {
	rows, err := model.SearchUserNoticeRecipients(c.Query("q"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func ListUserNotices(c *gin.Context) {
	rows, err := model.ListUserNotices(0)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func SubmitUserNotice(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
	var input model.UserNoticeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiError(c, model.ErrUserNoticeInvalid)
		return
	}
	row, err := model.SubmitUserNotice(input, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, row)
}

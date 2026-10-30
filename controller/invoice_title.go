package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type InvoiceTitlePayload struct {
	Region         string `json:"region"`
	CompanyName    string `json:"company_name"`
	TaxNumber      string `json:"tax_number"`
	BankName       string `json:"bank_name"`
	BankAccount    string `json:"bank_account"`
	CompanyAddress string `json:"company_address"`
	CompanyPhone   string `json:"company_phone"`
	IsDefault      bool   `json:"is_default"`
}

func (payload InvoiceTitlePayload) details() model.InvoiceTitleDetails {
	return model.InvoiceTitleDetails{
		Region:         payload.Region,
		CompanyName:    payload.CompanyName,
		TaxNumber:      payload.TaxNumber,
		BankName:       payload.BankName,
		BankAccount:    payload.BankAccount,
		CompanyAddress: payload.CompanyAddress,
		CompanyPhone:   payload.CompanyPhone,
	}
}

func bindInvoiceTitlePayload(c *gin.Context) (InvoiceTitlePayload, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	var payload InvoiceTitlePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return payload, false
	}
	return payload, true
}

func ListInvoiceTitles(c *gin.Context) {
	titles, err := model.ListInvoiceTitles(c.GetInt("id"))
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceLoadFailed, err)
		return
	}
	common.ApiSuccess(c, titles)
}

func CreateInvoiceTitle(c *gin.Context) {
	payload, ok := bindInvoiceTitlePayload(c)
	if !ok {
		return
	}
	title, err := model.CreateInvoiceTitle(c.GetInt("id"), payload.details(), payload.IsDefault)
	if err != nil {
		handleInvoiceError(c, err)
		return
	}
	common.ApiSuccess(c, title)
}

func UpdateInvoiceTitle(c *gin.Context) {
	id, ok := parseInvoiceRequestID(c)
	if !ok {
		return
	}
	payload, ok := bindInvoiceTitlePayload(c)
	if !ok {
		return
	}
	title, err := model.UpdateInvoiceTitle(id, c.GetInt("id"), payload.details(), payload.IsDefault)
	if err != nil {
		handleInvoiceError(c, err)
		return
	}
	common.ApiSuccess(c, title)
}

func DeleteInvoiceTitle(c *gin.Context) {
	id, ok := parseInvoiceRequestID(c)
	if !ok {
		return
	}
	if err := model.DeleteInvoiceTitle(id, c.GetInt("id")); err != nil {
		handleInvoiceError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

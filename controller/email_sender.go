package controller

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

type emailSenderAccountRequest struct {
	Name               string `json:"name"`
	Provider           string `json:"provider"`
	Server             string `json:"server"`
	Port               int    `json:"port"`
	Account            string `json:"account"`
	From               string `json:"from"`
	Token              string `json:"token"`
	SSLEnabled         bool   `json:"ssl_enabled"`
	StartTLSEnabled    bool   `json:"starttls_enabled"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
	ForceAuthLogin     bool   `json:"force_auth_login"`
	Weight             int    `json:"weight"`
	RateLimitPerMinute int    `json:"rate_limit_per_minute"`
}

type emailSenderAccountTestRequest struct {
	Email string `json:"email"`
}

type emailSenderAccountEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

type emailReceiptEndpointRequest struct {
	Enabled bool `json:"enabled"`
}

func ListMarketingEmailSenderAccounts(c *gin.Context) {
	rows, err := model.ListMarketingEmailSenderAccounts()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func CreateMarketingEmailSenderAccount(c *gin.Context) {
	request := emailSenderAccountRequest{}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	account := emailSenderAccountFromRequest(request)
	if strings.TrimSpace(request.Token) == "" {
		common.ApiError(c, model.ErrEmailSenderAccountInvalid)
		return
	}
	if err := account.SetToken(request.Token); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.CreateEmailSenderAccount(account); err != nil {
		common.ApiError(c, err)
		return
	}
	created, err := model.GetEmailSenderAccount(account.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, created)
}

func UpdateMarketingEmailSenderAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrEmailSenderAccountInvalid)
		return
	}
	existing, err := model.GetEmailSenderAccount(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	request := emailSenderAccountRequest{}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	account := emailSenderAccountFromRequest(request)
	account.Id = id
	account.TokenEncrypted = existing.TokenEncrypted
	if strings.TrimSpace(request.Token) != "" {
		if err := account.SetToken(request.Token); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if err := account.Normalize(); err != nil {
		common.ApiError(c, err)
		return
	}
	configChanged := existing.ConfigHash != account.ConfigHash || strings.TrimSpace(request.Token) != ""
	if err := model.UpdateEmailSenderAccount(account, configChanged); err != nil {
		common.ApiError(c, err)
		return
	}
	updated, err := model.GetEmailSenderAccount(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, updated)
}

func DeleteMarketingEmailSenderAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrEmailSenderAccountInvalid)
		return
	}
	if err := model.DeleteEmailSenderAccount(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func SetMarketingEmailSenderAccountEnabled(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrEmailSenderAccountInvalid)
		return
	}
	request := emailSenderAccountEnabledRequest{}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.SetEmailSenderAccountEnabled(id, request.Enabled); err != nil {
		common.ApiError(c, err)
		return
	}
	account, err := model.GetEmailSenderAccount(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, account)
}

func TestMarketingEmailSenderAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrEmailSenderAccountInvalid)
		return
	}
	request := emailSenderAccountTestRequest{}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	recipient, attempt, err := service.SendMarketingAccountTest(c.GetInt("id"), id, request.Email)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"recipient": recipient, "attempt_id": attempt.Id, "status": attempt.Status,
		"message": "SMTP accepted; waiting for EventBridge receipt",
	})
}

func GetEmailReceiptEndpoint(c *gin.Context) {
	endpoint, err := model.GetEmailReceiptEndpoint()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"provider": endpoint.Provider, "enabled": endpoint.Enabled,
		"token_configured": endpoint.TokenConfigured, "last_event_time": endpoint.LastEventTime,
		"last_verified_time": endpoint.LastVerifiedTime, "last_error": endpoint.LastError,
		"callback_url": emailReceiptCallbackURL(c),
	})
}

func RotateEmailReceiptEndpointToken(c *gin.Context) {
	token, err := model.RotateEmailReceiptEndpointToken()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"token": token, "callback_url": emailReceiptCallbackURL(c)})
}

func UpdateEmailReceiptEndpoint(c *gin.Context) {
	request := emailReceiptEndpointRequest{}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateEmailReceiptEndpointEnabled(request.Enabled); err != nil {
		common.ApiError(c, err)
		return
	}
	GetEmailReceiptEndpoint(c)
}

func AliyunEmailEventBridgeReceipt(c *gin.Context) {
	token := strings.TrimSpace(c.GetHeader("x-eventbridge-signature-token"))
	if !model.VerifyEmailReceiptEndpointToken(token) {
		c.Status(http.StatusForbidden)
		return
	}
	reader := http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024)
	body, err := io.ReadAll(reader)
	if err != nil {
		_ = model.UpdateEmailReceiptEndpointActivity(false, err.Error())
		c.Status(http.StatusRequestEntityTooLarge)
		return
	}
	if err := service.ProcessAliyunEventBridgeReceipt(body); err != nil {
		_ = model.UpdateEmailReceiptEndpointActivity(false, err.Error())
		c.Status(http.StatusInternalServerError)
		return
	}
	_ = model.UpdateEmailReceiptEndpointActivity(true, "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func emailSenderAccountFromRequest(request emailSenderAccountRequest) *model.EmailSenderAccount {
	return &model.EmailSenderAccount{
		Name: request.Name, Profile: model.EmailSenderProfileMarketing, Provider: request.Provider,
		Server: request.Server, Port: request.Port, Account: request.Account, From: request.From,
		SSLEnabled: request.SSLEnabled, StartTLSEnabled: request.StartTLSEnabled,
		InsecureSkipVerify: request.InsecureSkipVerify, ForceAuthLogin: request.ForceAuthLogin,
		Weight: request.Weight, RateLimitPerMinute: request.RateLimitPerMinute,
	}
}

func emailReceiptCallbackURL(c *gin.Context) string {
	base := strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/")
	if base == "" {
		scheme := "https"
		if c.Request.TLS == nil {
			scheme = "http"
		}
		base = scheme + "://" + c.Request.Host
	}
	return base + "/api/email/receipts/aliyun/eventbridge"
}

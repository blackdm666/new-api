package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type CreateInvoiceRequestPayload struct {
	CompanyName    string `json:"company_name"`
	TaxNumber      string `json:"tax_number"`
	BankName       string `json:"bank_name"`
	BankAccount    string `json:"bank_account"`
	CompanyAddress string `json:"company_address"`
	CompanyPhone   string `json:"company_phone"`
	Remark         string `json:"remark"`
	TopUpOrderIds  []int  `json:"topup_order_ids"`
}

type UpdateInvoiceRequestStatusPayload struct {
	Status          int    `json:"status"`
	RejectionReason string `json:"rejection_reason"`
}

type InvoiceRequestDetailResponse struct {
	Invoice       *model.InvoiceRequest                `json:"invoice"`
	Orders        []*model.TopUp                       `json:"orders"`
	Files         []*model.InvoiceFile                 `json:"files"`
	Events        []*InvoiceRequestEventResponse       `json:"events"`
	Notifications []*model.InvoiceNotificationDelivery `json:"notifications,omitempty"`
}

type InvoiceRequestEventResponse struct {
	Id          int    `json:"id"`
	FromStatus  int    `json:"from_status"`
	ToStatus    int    `json:"to_status"`
	OperatorId  int    `json:"operator_id,omitempty"`
	Reason      string `json:"reason"`
	CreatedTime int64  `json:"created_time"`
}

func GetInvoiceConfig(c *gin.Context) {
	quota, err := model.GetUserQuota(c.GetInt("id"), true)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceLoadFailed, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"minimum_amount_cents":    setting.InvoiceMinimumAmountCents,
		"tax_rate_basis_points":   setting.InvoiceTaxRateBasisPoints,
		"available_balance_cents": model.InvoiceQuotaToCNYCents(quota),
		"issue_day":               10,
	})
}

func parseInvoiceRequestID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return 0, false
	}
	return id, true
}

func respondInvoiceInternalError(c *gin.Context, key string, err error) {
	if err != nil {
		common.SysLog(fmt.Sprintf(
			"invoice request failed: method=%s path=%s user_id=%d error=%s",
			c.Request.Method,
			c.Request.URL.Path,
			c.GetInt("id"),
			err.Error(),
		))
	}
	if key == "" {
		key = i18n.MsgInvoiceOperationFailed
	}
	common.ApiErrorI18n(c, key)
}

func handleInvoiceError(c *gin.Context, err error, fallbackKeys ...string) {
	switch {
	case errors.Is(err, model.ErrInvoiceRequestNotFound), errors.Is(err, model.ErrInvoiceRequestForbidden):
		common.ApiErrorI18n(c, i18n.MsgInvoiceNotFound)
	case errors.Is(err, model.ErrInvoiceStatusInvalid):
		common.ApiErrorI18n(c, i18n.MsgInvoiceStatusInvalid)
	case errors.Is(err, model.ErrInvoiceStatusTransition):
		common.ApiErrorI18n(c, i18n.MsgInvoiceStatusTransition)
	case errors.Is(err, model.ErrInvoiceRejectionReasonRequired):
		common.ApiErrorI18n(c, i18n.MsgInvoiceRejectionReasonRequired)
	case errors.Is(err, model.ErrInvoiceRejectionReasonInvalid):
		common.ApiErrorI18n(c, i18n.MsgInvoiceRejectionReasonInvalid)
	case errors.Is(err, model.ErrInvoiceOrderEmpty):
		common.ApiErrorI18n(c, i18n.MsgInvoiceOrderEmpty)
	case errors.Is(err, model.ErrInvoiceOrderInvalid):
		common.ApiErrorI18n(c, i18n.MsgInvoiceOrderInvalid)
	case errors.Is(err, model.ErrInvoiceOrderDuplicate):
		common.ApiErrorI18n(c, i18n.MsgInvoiceOrderDuplicate)
	case errors.Is(err, model.ErrInvoiceAmountTooSmall):
		common.ApiErrorI18n(c, i18n.MsgInvoiceAmountTooSmall, map[string]any{
			"Amount": fmt.Sprintf("%.2f", float64(setting.InvoiceMinimumAmountCents)/100),
		})
	case errors.Is(err, model.ErrInvoiceTaxFeeInsufficient):
		fee := "0.00"
		balance := "0.00"
		var insufficient *model.InvoiceTaxFeeInsufficientError
		if errors.As(err, &insufficient) {
			fee = fmt.Sprintf("%.2f", float64(insufficient.FeeCents)/100)
			balance = fmt.Sprintf("%.2f", float64(insufficient.AvailableCents)/100)
		}
		common.ApiErrorI18n(c, i18n.MsgInvoiceTaxFeeInsufficient, map[string]any{
			"Fee":     fee,
			"Balance": balance,
		})
	case errors.Is(err, model.ErrInvoiceFileRequired):
		common.ApiErrorI18n(c, i18n.MsgInvoiceFileRequired)
	case errors.Is(err, model.ErrInvoiceCompanyEmpty):
		common.ApiErrorI18n(c, i18n.MsgInvoiceCompanyEmpty)
	case errors.Is(err, model.ErrInvoiceCompanyInvalid):
		common.ApiErrorI18n(c, i18n.MsgInvoiceCompanyInvalid)
	case errors.Is(err, model.ErrInvoiceTaxNumberEmpty):
		common.ApiErrorI18n(c, i18n.MsgInvoiceTaxNumberEmpty)
	case errors.Is(err, model.ErrInvoiceTaxNumberInvalid):
		common.ApiErrorI18n(c, i18n.MsgInvoiceTaxNumberInvalid)
	case errors.Is(err, model.ErrInvoiceEmailEmpty):
		common.ApiErrorI18n(c, i18n.MsgInvoiceEmailEmpty)
	case errors.Is(err, model.ErrInvoiceEmailInvalid):
		common.ApiErrorI18n(c, i18n.MsgInvoiceEmailInvalid)
	case errors.Is(err, model.ErrInvoiceOptionalFieldInvalid):
		common.ApiErrorI18n(c, i18n.MsgInvoiceOptionalFieldInvalid)
	case errors.Is(err, model.ErrInvoiceWithdrawForbidden):
		common.ApiErrorI18n(c, i18n.MsgInvoiceWithdrawForbidden)
	case errors.Is(err, model.ErrInvoicePurgeForbidden):
		common.ApiErrorI18n(c, i18n.MsgInvoicePurgeForbidden)
	default:
		fallbackKey := i18n.MsgInvoiceOperationFailed
		if len(fallbackKeys) > 0 && fallbackKeys[0] != "" {
			fallbackKey = fallbackKeys[0]
		}
		respondInvoiceInternalError(c, fallbackKey, err)
	}
}

func buildInvoiceRequestDetail(request *model.InvoiceRequest, admin bool) (*InvoiceRequestDetailResponse, error) {
	orders := []*model.TopUp{}
	var err error
	if request.RedactedTime == 0 {
		orders, err = model.GetInvoiceRequestOrders(request)
		if err != nil {
			return nil, err
		}
	}
	files := []*model.InvoiceFile{}
	if admin || request.Status == model.InvoiceStatusIssued {
		files, err = model.ListInvoiceFiles(request.Id)
		if err != nil {
			return nil, err
		}
	}
	events, err := model.ListInvoiceRequestEvents(request.Id)
	if err != nil {
		return nil, err
	}
	eventResponses := make([]*InvoiceRequestEventResponse, 0, len(events))
	for _, event := range events {
		operatorId := 0
		if admin {
			operatorId = event.OperatorId
		}
		eventResponses = append(eventResponses, &InvoiceRequestEventResponse{
			Id:          event.Id,
			FromStatus:  event.FromStatus,
			ToStatus:    event.ToStatus,
			OperatorId:  operatorId,
			Reason:      event.Reason,
			CreatedTime: event.CreatedTime,
		})
	}
	response := &InvoiceRequestDetailResponse{
		Invoice: request,
		Orders:  orders,
		Files:   files,
		Events:  eventResponses,
	}
	if admin {
		notifications, notifyErr := model.ListInvoiceNotificationsForRequest(request.Id)
		if notifyErr != nil {
			return nil, notifyErr
		}
		response.Notifications = notifications
	}
	return response, nil
}

func ListEligibleInvoiceOrders(c *gin.Context) {
	orders, err := model.ListEligibleInvoiceOrders(c.GetInt("id"))
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceLoadFailed, err)
		return
	}
	common.ApiSuccess(c, orders)
}

func CreateInvoiceRequest(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	var payload CreateInvoiceRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceSubmitFailed, err)
		return
	}
	request, orders, err := model.CreateInvoiceRequestWithNotifications(model.CreateInvoiceRequestParams{
		UserId:         user.Id,
		Username:       user.Username,
		CompanyName:    payload.CompanyName,
		TaxNumber:      payload.TaxNumber,
		BankName:       payload.BankName,
		BankAccount:    payload.BankAccount,
		CompanyAddress: payload.CompanyAddress,
		CompanyPhone:   payload.CompanyPhone,
		Email:          user.Email,
		Remark:         payload.Remark,
		TopUpOrderIds:  payload.TopUpOrderIds,
	}, service.BuildInvoiceRequestCreatedNotifications)
	if err != nil {
		handleInvoiceError(c, err, i18n.MsgInvoiceSubmitFailed)
		return
	}
	detail, err := buildInvoiceRequestDetail(request, false)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceSubmitFailed, err)
		return
	}
	detail.Orders = orders
	common.ApiSuccess(c, detail)
}

func ListUserInvoiceRequests(c *gin.Context) {
	status, _ := strconv.Atoi(c.Query("status"))
	pageInfo := common.GetPageQuery(c)
	requests, total, err := model.ListInvoiceRequests(model.InvoiceRequestQueryOptions{
		UserId: c.GetInt("id"),
		Status: status,
	}, pageInfo)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceLoadFailed, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(requests)
	common.ApiSuccess(c, pageInfo)
}

func GetUserInvoiceRequest(c *gin.Context) {
	id, ok := parseInvoiceRequestID(c)
	if !ok {
		return
	}
	request, err := model.GetUserInvoiceRequestById(id, c.GetInt("id"))
	if err != nil {
		handleInvoiceError(c, err)
		return
	}
	detail, err := buildInvoiceRequestDetail(request, false)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceLoadFailed, err)
		return
	}
	common.ApiSuccess(c, detail)
}

func ListAdminInvoiceRequests(c *gin.Context) {
	status, _ := strconv.Atoi(c.Query("status"))
	pageInfo := common.GetPageQuery(c)
	requests, total, err := model.ListInvoiceRequests(model.InvoiceRequestQueryOptions{
		Status:             status,
		Keyword:            c.Query("keyword"),
		PrioritizeExpiring: true,
	}, pageInfo)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceLoadFailed, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(requests)
	common.ApiSuccess(c, pageInfo)
}

func GetAdminInvoiceRequest(c *gin.Context) {
	id, ok := parseInvoiceRequestID(c)
	if !ok {
		return
	}
	request, err := model.GetInvoiceRequestById(id)
	if err != nil {
		handleInvoiceError(c, err)
		return
	}
	detail, err := buildInvoiceRequestDetail(request, true)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceLoadFailed, err)
		return
	}
	common.ApiSuccess(c, detail)
}

func UpdateInvoiceRequestStatus(c *gin.Context) {
	id, ok := parseInvoiceRequestID(c)
	if !ok {
		return
	}
	var payload UpdateInvoiceRequestStatusPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	previous, err := model.GetInvoiceRequestById(id)
	if err != nil {
		handleInvoiceError(c, err)
		return
	}
	var notificationFactory model.InvoiceNotificationFactory
	if common.InvoiceIssuedNotifyUserEnabled && payload.Status == model.InvoiceStatusIssued {
		user, userErr := model.GetUserById(previous.UserId, false)
		if userErr != nil {
			respondInvoiceInternalError(c, i18n.MsgInvoiceUpdateFailed, userErr)
			return
		}
		notificationFactory = func(updated *model.InvoiceRequest) ([]*model.InvoiceNotificationDelivery, error) {
			return service.BuildInvoiceIssuedNotifications(updated, previous.Status, user)
		}
	}
	updated, err := model.UpdateInvoiceRequestStatusWithNotifications(id, payload.Status, c.GetInt("id"), payload.RejectionReason, notificationFactory)
	if err != nil {
		handleInvoiceError(c, err, i18n.MsgInvoiceUpdateFailed)
		return
	}
	common.ApiSuccess(c, updated)
}

func ResendIssuedInvoiceNotification(c *gin.Context) {
	id, ok := parseInvoiceRequestID(c)
	if !ok {
		return
	}
	request, err := model.GetInvoiceRequestById(id)
	if err != nil {
		handleInvoiceError(c, err)
		return
	}
	user, err := model.GetUserById(request.UserId, false)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceUpdateFailed, err)
		return
	}
	delivery, err := service.BuildInvoiceIssuedNotificationResend(request, user, c.GetInt("id"))
	if err != nil {
		handleInvoiceError(c, err, i18n.MsgInvoiceUpdateFailed)
		return
	}
	if delivery == nil {
		common.ApiErrorI18n(c, i18n.MsgInvoiceOperationFailed)
		return
	}
	created, _, err := model.EnqueueInvoiceNotification(delivery)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceUpdateFailed, err)
		return
	}
	service.ScheduleInvoiceNotification(created)
	recordManageAudit(c, "invoice.notification.resend", map[string]interface{}{"invoice_id": id})
	common.ApiSuccess(c, created)
}

func WithdrawInvoiceRequest(c *gin.Context) {
	id, ok := parseInvoiceRequestID(c)
	if !ok {
		return
	}
	request, err := model.WithdrawInvoiceRequest(id, c.GetInt("id"))
	if err != nil {
		handleInvoiceError(c, err, i18n.MsgInvoiceUpdateFailed)
		return
	}
	common.ApiSuccess(c, request)
}

func PurgeInvoiceRequest(c *gin.Context) {
	id, ok := parseInvoiceRequestID(c)
	if !ok {
		return
	}
	if err := model.PurgeInvoiceRequest(id); err != nil {
		handleInvoiceError(c, err)
		return
	}
	recordManageAudit(c, "invoice.request.purge", map[string]interface{}{"invoice_id": id})
	common.ApiSuccess(c, gin.H{"id": id})
}

type InvoiceUserProfileResponse struct {
	UserId       int                `json:"user_id"`
	Username     string             `json:"username"`
	DisplayName  string             `json:"display_name"`
	Email        string             `json:"email"`
	Role         int                `json:"role"`
	Status       int                `json:"status"`
	Group        string             `json:"group"`
	CreatedTime  int64              `json:"created_time"`
	Quota        int                `json:"quota"`
	UsedQuota    int                `json:"used_quota"`
	RequestCount int                `json:"request_count"`
	RecentLogs   []*model.Log       `json:"recent_logs"`
	ModelUsage   []*model.QuotaData `json:"model_usage"`
}

func GetInvoiceUserProfile(c *gin.Context) {
	id, ok := parseInvoiceRequestID(c)
	if !ok {
		return
	}
	request, err := model.GetInvoiceRequestById(id)
	if err != nil {
		handleInvoiceError(c, err)
		return
	}
	user, err := model.GetUserById(request.UserId, false)
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceLoadFailed, err)
		return
	}
	response := InvoiceUserProfileResponse{
		UserId:       user.Id,
		Username:     user.Username,
		DisplayName:  user.DisplayName,
		Email:        user.Email,
		Role:         user.Role,
		Status:       user.Status,
		Group:        user.Group,
		CreatedTime:  user.CreatedAt,
		Quota:        user.Quota,
		UsedQuota:    user.UsedQuota,
		RequestCount: user.RequestCount,
	}
	recentLogs, _, logErr := model.GetUserLogs(user.Id, model.LogTypeConsume, 0, 0, "", "", 0, 15, "", "", "")
	if logErr != nil {
		common.SysLog(fmt.Sprintf("invoice user profile: failed to fetch logs for user %d: %s", user.Id, logErr.Error()))
	} else {
		response.RecentLogs = recentLogs
	}
	since := common.GetTimestamp() - 30*24*3600
	usage, usageErr := model.GetUserModelUsageTopN(user.Id, since, 8)
	if usageErr != nil {
		common.SysLog(fmt.Sprintf("invoice user profile: failed to aggregate model usage for user %d: %s", user.Id, usageErr.Error()))
	} else {
		response.ModelUsage = usage
	}
	common.ApiSuccess(c, response)
}

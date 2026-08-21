package controller

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	antommodel "github.com/alipay/global-open-sdk-go/com/alipay/api/model"
	antomnotify "github.com/alipay/global-open-sdk-go/com/alipay/api/request/notify"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

const (
	antomNotifyPaymentResult  = "PAYMENT_RESULT"
	antomNotifyPaymentPending = "PAYMENT_PENDING"
)

var newAntomGateway = service.NewAntomGateway

type AntomPayRequest struct {
	Amount int64 `json:"amount"`
}

type AntomQueryRequest struct {
	TradeNo string `json:"trade_no"`
}

func RequestAntomPay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	if !isAntomTopUpEnabled() {
		common.ApiErrorMsg(c, "Antom 支付未配置或当前计价币种不受支持")
		return
	}

	var req AntomPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if req.Amount < getMinTopup() {
		common.ApiErrorMsg(c, fmt.Sprintf("充值数量不能小于 %d", getMinTopup()))
		return
	}
	userID := c.GetInt("id")
	if rejectInvalidTopUpQuota(c, userID, req.Amount) {
		return
	}

	group, err := model.GetUserGroup(userID, true)
	if err != nil {
		common.ApiErrorMsg(c, "获取用户分组失败")
		return
	}
	payMoney := getPayMoneyDecimal(req.Amount, group).Round(2)
	amountMinor, err := antomMoneyToMinorUnits(payMoney)
	if err != nil || amountMinor <= 0 {
		common.ApiErrorMsg(c, "充值金额无效")
		return
	}
	creditedQuota, err := getTopUpQuota(req.Amount)
	if err != nil || creditedQuota <= 0 {
		common.ApiErrorMsg(c, "充值额度无效")
		return
	}
	currency, err := service.AntomOrderCurrency()
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	tradeNo := fmt.Sprintf("ANTOMUSR%dNO%s%d", userID, common.GetRandomString(6), time.Now().Unix())
	notifyURL, err := service.ResolveAntomNotifyURL()
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	redirectURL, err := service.ResolveAntomRedirectURL(tradeNo)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	gateway, err := newAntomGateway()
	if err != nil {
		common.ApiErrorMsg(c, "Antom 支付配置无效")
		return
	}

	topUp := &model.TopUp{
		UserId:          userID,
		Amount:          req.Amount,
		Money:           payMoney.InexactFloat64(),
		MoneyMinor:      amountMinor,
		CreditedQuota:   creditedQuota,
		Currency:        currency,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodAntom,
		PaymentProvider: model.PaymentProviderAntom,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Antom 创建本地充值订单失败 user_id=%d trade_no=%s error=%q", userID, tradeNo, err.Error()))
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}

	session, err := gateway.CreatePaymentSession(service.AntomPaymentSessionInput{
		PaymentRequestID: tradeNo,
		AmountMinor:      amountMinor,
		Currency:         currency,
		NotifyURL:        notifyURL,
		RedirectURL:      redirectURL,
		BuyerReferenceID: strconv.Itoa(userID),
		ClientIP:         c.ClientIP(),
		UserAgent:        c.Request.UserAgent(),
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Antom 创建支付会话失败 user_id=%d trade_no=%s error=%q", userID, tradeNo, err.Error()))
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Antom 充值订单创建成功 user_id=%d trade_no=%s amount=%d money=%.2f currency=%s", userID, tradeNo, req.Amount, topUp.Money, currency))
	common.ApiSuccess(c, gin.H{
		"normal_url": session.NormalURL,
		"trade_no":   tradeNo,
		"expires_at": session.ExpiryTime,
		"session_id": session.SessionID,
	})
}

func AntomNotify(c *gin.Context) {
	if !isAntomWebhookEnabled() {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeAntomReceipt(c, http.StatusServiceUnavailable, false)
		return
	}
	gateway, err := newAntomGateway()
	if err != nil {
		writeAntomReceipt(c, http.StatusServiceUnavailable, false)
		return
	}
	if err := gateway.VerifyWebhook(
		c.Request.URL.Path,
		c.GetHeader("Client-Id"),
		c.GetHeader("Request-Time"),
		string(payload),
		c.GetHeader("Signature"),
	); err != nil {
		logger.LogWarn(c.Request.Context(), "Antom webhook 验签失败")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	var notification antomnotify.AlipayPayResultNotify
	if err := common.Unmarshal(payload, &notification); err != nil {
		logger.LogWarn(c.Request.Context(), "Antom webhook 报文解析失败")
		writeAntomReceipt(c, http.StatusBadRequest, false)
		return
	}

	if notification.NotifyType != antomNotifyPaymentPending && notification.NotifyType != antomNotifyPaymentResult {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Antom webhook 通知类型无效 trade_no=%s notify_type=%s", notification.PaymentRequestId, notification.NotifyType))
		writeAntomReceipt(c, http.StatusBadRequest, false)
		return
	}
	if notification.PaymentRequestId == "" {
		writeAntomReceipt(c, http.StatusBadRequest, false)
		return
	}
	amountMinor, currency, amountErr := antomNotificationAmount(notification.PaymentAmount)
	if amountErr != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Antom webhook 金额无效 trade_no=%s error=%q", notification.PaymentRequestId, amountErr.Error()))
		writeAntomReceipt(c, http.StatusBadRequest, false)
		return
	}
	if validationErr := model.ValidateAntomTopUpPayment(notification.PaymentRequestId, amountMinor, currency); validationErr != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Antom webhook 订单校验失败 trade_no=%s amount=%d currency=%s error=%q", notification.PaymentRequestId, amountMinor, currency, validationErr.Error()))
		writeAntomReceipt(c, http.StatusBadRequest, false)
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Antom webhook 已验证 trade_no=%s status=%s amount=%d currency=%s payment_method=%s", notification.PaymentRequestId, notification.Result.ResultStatus, amountMinor, currency, notification.PaymentMethodType))
	if notification.NotifyType == antomNotifyPaymentPending {
		if notification.Result.ResultStatus != "S" {
			writeAntomReceipt(c, http.StatusServiceUnavailable, false)
			return
		}
		writeAntomReceipt(c, http.StatusOK, true)
		return
	}

	switch notification.Result.ResultStatus {
	case "S":
		LockOrder(notification.PaymentRequestId)
		alreadyDone, rechargeErr := model.RechargeAntom(notification.PaymentRequestId, notification.PaymentMethodType, amountMinor, currency, c.ClientIP())
		UnlockOrder(notification.PaymentRequestId)
		if rechargeErr != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Antom webhook 入账失败 trade_no=%s error=%q", notification.PaymentRequestId, rechargeErr.Error()))
			writeAntomReceipt(c, http.StatusInternalServerError, false)
			return
		}
		if alreadyDone {
			logger.LogInfo(c.Request.Context(), fmt.Sprintf("Antom webhook 重复通知幂等忽略 trade_no=%s", notification.PaymentRequestId))
		}
	case "F":
		err := model.UpdatePendingTopUpStatus(notification.PaymentRequestId, model.PaymentProviderAntom, common.TopUpStatusFailed)
		if err != nil && !errors.Is(err, model.ErrTopUpStatusInvalid) {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Antom webhook 标记失败订单失败 trade_no=%s error=%q", notification.PaymentRequestId, err.Error()))
			writeAntomReceipt(c, http.StatusInternalServerError, false)
			return
		}
	default:
		writeAntomReceipt(c, http.StatusServiceUnavailable, false)
		return
	}
	writeAntomReceipt(c, http.StatusOK, true)
}

func RequestAntomQuery(c *gin.Context) {
	var req AntomQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.TradeNo) == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	topUp := model.GetTopUpByTradeNo(req.TradeNo)
	if topUp == nil || topUp.UserId != c.GetInt("id") || topUp.PaymentProvider != model.PaymentProviderAntom {
		common.ApiErrorMsg(c, "充值订单不存在")
		return
	}
	if topUp.Status != common.TopUpStatusPending {
		common.ApiSuccess(c, antomQueryResponse(topUp))
		return
	}

	gateway, err := newAntomGateway()
	if err != nil {
		common.ApiErrorMsg(c, "Antom 支付配置无效")
		return
	}
	result, err := gateway.InquiryPayment(topUp.TradeNo)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Antom 查询支付失败 trade_no=%s error=%q", topUp.TradeNo, err.Error()))
		common.ApiErrorMsg(c, "支付结果查询失败，请稍后重试")
		return
	}
	if result.PaymentRequestID != "" && result.PaymentRequestID != topUp.TradeNo {
		common.ApiErrorMsg(c, "支付结果订单号不匹配")
		return
	}

	switch result.PaymentStatus {
	case "SUCCESS":
		LockOrder(topUp.TradeNo)
		_, err = model.RechargeAntom(topUp.TradeNo, result.PaymentMethod, result.AmountMinor, result.Currency, c.ClientIP())
		UnlockOrder(topUp.TradeNo)
	case "FAIL", "CANCELLED":
		err = model.UpdatePendingTopUpStatus(topUp.TradeNo, model.PaymentProviderAntom, common.TopUpStatusFailed)
		if errors.Is(err, model.ErrTopUpStatusInvalid) {
			err = nil
		}
	}
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Antom 同步支付结果失败 trade_no=%s status=%s error=%q", topUp.TradeNo, result.PaymentStatus, err.Error()))
		common.ApiErrorMsg(c, "同步支付结果失败，请稍后重试")
		return
	}

	topUp = model.GetTopUpByTradeNo(req.TradeNo)
	common.ApiSuccess(c, antomQueryResponse(topUp))
}

func antomMoneyToMinorUnits(value decimal.Decimal) (int64, error) {
	rounded := value.Round(2)
	if rounded.LessThanOrEqual(decimal.Zero) {
		return 0, errors.New("amount must be positive")
	}
	minor := rounded.Mul(decimal.NewFromInt(100))
	if minor.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return 0, errors.New("amount is too large")
	}
	return minor.IntPart(), nil
}

func antomNotificationAmount(amount *antommodel.Amount) (int64, string, error) {
	if amount == nil {
		return 0, "", errors.New("missing payment amount")
	}
	value, err := strconv.ParseInt(strings.TrimSpace(amount.Value), 10, 64)
	if err != nil || value <= 0 {
		return 0, "", errors.New("invalid payment amount")
	}
	currency := strings.ToUpper(strings.TrimSpace(amount.Currency))
	if currency == "" {
		return 0, "", errors.New("missing payment currency")
	}
	return value, currency, nil
}

func antomQueryResponse(topUp *model.TopUp) gin.H {
	if topUp == nil {
		return gin.H{"status": common.TopUpStatusFailed}
	}
	return gin.H{
		"trade_no":       topUp.TradeNo,
		"status":         topUp.Status,
		"payment_method": topUp.PaymentMethod,
	}
}

func writeAntomReceipt(c *gin.Context, status int, success bool) {
	resultCode := "SYSTEM_ERROR"
	resultStatus := "U"
	resultMessage := "system error"
	if success {
		resultCode = "SUCCESS"
		resultStatus = "S"
		resultMessage = "success"
	}
	c.JSON(status, gin.H{"result": gin.H{
		"resultCode":    resultCode,
		"resultStatus":  resultStatus,
		"resultMessage": resultMessage,
	}})
}

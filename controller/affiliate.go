package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type affiliateCommissionActionPayload struct {
	Reason string `json:"reason"`
}

type affiliateTransferPayload struct {
	AmountCents int64  `json:"amount_cents" binding:"required"`
	RequestId   string `json:"request_id"`
}

func TransferAffiliateCommission(c *gin.Context) {
	payload := affiliateTransferPayload{}
	if err := common.DecodeJson(c.Request.Body, &payload); err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(payload.RequestId) == "" {
		payload.RequestId = common.NewRequestId()
	}
	user, err := model.GetUserById(c.GetInt("id"), true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	transfer, err := user.TransferAffiliateCentsToQuotaWithRequestId(payload.AmountCents, payload.RequestId)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserTransferFailed, map[string]any{"Error": err.Error()})
		return
	}
	common.ApiSuccessI18n(c, i18n.MsgUserTransferSuccess, transfer)
}

type affiliateSettingsPayload struct {
	Enabled                              bool           `json:"enabled"`
	AutoApprove                          bool           `json:"auto_approve"`
	DefaultRateBasisPoints               int            `json:"default_rate_basis_points"`
	GroupRates                           map[string]int `json:"group_rates"`
	UpgradeInviteesThreshold             int            `json:"upgrade_invitees_threshold"`
	GoldUpgradeInviteesThreshold         int            `json:"gold_upgrade_invitees_threshold"`
	UpgradeTopUpAmountThresholdCents     int64          `json:"upgrade_top_up_amount_threshold_cents"`
	GoldUpgradeTopUpAmountThresholdCents int64          `json:"gold_upgrade_top_up_amount_threshold_cents"`
}

type affiliateUpgradeApprovalPayload struct {
	NextGroup string `json:"next_group"`
}

type affiliatePayoutCreatePayload struct {
	RequestId     string `json:"request_id"`
	AmountCents   int64  `json:"amount_cents"`
	PaymentMethod string `json:"payment_method"`
	AccountName   string `json:"account_name"`
	Account       string `json:"account"`
}

type affiliatePayoutRejectPayload struct {
	Reason string `json:"reason"`
}

type affiliateAlipayPayoutSettingsPayload struct {
	Enabled                 bool   `json:"enabled"`
	AppId                   string `json:"app_id"`
	PrivateKey              string `json:"private_key"`
	AppCertificate          string `json:"app_certificate"`
	AlipayPublicCertificate string `json:"alipay_public_certificate"`
	AlipayRootCertificate   string `json:"alipay_root_certificate"`
	TransferTitle           string `json:"transfer_title"`
	ClearKeys               bool   `json:"clear_keys"`
}

func GetAffiliateSummary(c *gin.Context) {
	summary, err := model.GetAffiliateSummary(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

func GetAffiliateCommissions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	status, _ := strconv.Atoi(c.Query("status"))
	records, total, err := model.ListAffiliateCommissions(model.AffiliateCommissionQueryOptions{
		InviterId: c.GetInt("id"),
		Status:    status,
	}, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(records)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func GetAffiliateInviteeStats(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	rows, total, err := model.ListAffiliateInviteeStats(c.GetInt("id"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(rows)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func GetAffiliateTransfers(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	rows, total, err := model.ListAffiliateTransfers(model.AffiliateTransferQueryOptions{
		UserId: c.GetInt("id"),
	}, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(rows)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func GetAffiliatePayoutSummary(c *gin.Context) {
	summary, err := model.GetAffiliatePayoutSummary(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

func GetAffiliatePayouts(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	status, _ := strconv.Atoi(c.Query("status"))
	rows, total, err := model.ListAffiliatePayouts(model.AffiliatePayoutQueryOptions{
		UserId: c.GetInt("id"),
		Status: status,
	}, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for _, payout := range rows {
		// Provider diagnostics are restricted to administrators. Users only need
		// the normalized payout status and their own rejection reason.
		payout.ProviderErrorCode = ""
		payout.ProviderErrorMessage = ""
	}
	pageInfo.SetItems(rows)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func CreateAffiliatePayout(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	payload := affiliatePayoutCreatePayload{}
	if err := common.DecodeJson(c.Request.Body, &payload); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	payout, err := model.CreateAffiliatePayout(model.CreateAffiliatePayoutParams{
		UserId:        c.GetInt("id"),
		RequestId:     payload.RequestId,
		AmountCents:   payload.AmountCents,
		PaymentMethod: payload.PaymentMethod,
		AccountName:   payload.AccountName,
		Account:       payload.Account,
	})
	if affiliatePayoutError(c, err) {
		return
	}
	common.ApiSuccess(c, payout)
}

func CancelAffiliatePayout(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	if affiliatePayoutError(c, model.CancelAffiliatePayout(id, c.GetInt("id"))) {
		return
	}
	common.ApiSuccess(c, gin.H{"updated": true})
}

func GetAdminAffiliateSummary(c *gin.Context) {
	summary, err := model.GetAffiliateAdminSummary()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

func GetAdminAffiliateCommissions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	status, _ := strconv.Atoi(c.Query("status"))
	keyword := strings.TrimSpace(c.Query("keyword"))
	records, total, err := model.ListAffiliateCommissions(model.AffiliateCommissionQueryOptions{
		Status:  status,
		Keyword: keyword,
	}, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(records)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func GetAdminAffiliateTransfers(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	rows, total, err := model.ListAffiliateTransfers(model.AffiliateTransferQueryOptions{
		Keyword: strings.TrimSpace(c.Query("keyword")),
	}, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(rows)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func GetAdminAffiliatePayoutSummary(c *gin.Context) {
	summary, err := model.GetAffiliatePayoutAdminSummary()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

func GetAdminAffiliatePayouts(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	status, _ := strconv.Atoi(c.Query("status"))
	rows, total, err := model.ListAffiliatePayouts(model.AffiliatePayoutQueryOptions{
		Status:  status,
		Keyword: c.Query("keyword"),
	}, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(rows)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func ApproveAffiliatePayout(c *gin.Context) {
	reviewAffiliatePayout(c, true)
}

func RejectAffiliatePayout(c *gin.Context) {
	reviewAffiliatePayout(c, false)
}

func reviewAffiliatePayout(c *gin.Context, approve bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	payload := affiliatePayoutRejectPayload{}
	if !approve {
		_ = c.ShouldBindJSON(&payload)
	}
	if affiliatePayoutError(c, model.ReviewAffiliatePayout(id, c.GetInt("id"), approve, payload.Reason)) {
		return
	}
	recordManageAudit(c, "affiliate.payout.review", map[string]interface{}{
		"payout_id": id,
		"approved":  approve,
		"reason":    strings.TrimSpace(payload.Reason),
	})
	if approve {
		service.QueueAffiliatePayoutResult(id, "approved")
	} else {
		service.QueueAffiliatePayoutResult(id, "rejected")
	}
	common.ApiSuccess(c, gin.H{"updated": true})
}

func MarkAffiliatePayoutPaid(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	if affiliatePayoutError(c, model.MarkAffiliatePayoutPaid(id, c.GetInt("id"))) {
		return
	}
	recordManageAudit(c, "affiliate.payout.paid", map[string]interface{}{
		"payout_id":         id,
		"disbursement_mode": model.AffiliatePayoutDisbursementManual,
	})
	service.QueueAffiliatePayoutResult(id, "paid")
	common.ApiSuccess(c, gin.H{"updated": true})
}

func PayAffiliatePayoutWithAlipay(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	payout, err := service.ExecuteAffiliateAlipayPayout(c.Request.Context(), id, c.GetInt("id"))
	if affiliateAlipayPayoutError(c, err) {
		return
	}
	recordManageAudit(c, "affiliate.payout.alipay", map[string]interface{}{
		"payout_id":         id,
		"disbursement_mode": model.AffiliatePayoutDisbursementAlipayDirect,
		"status":            payout.Status,
	})
	if payout.Status == model.AffiliatePayoutStatusPaid {
		service.QueueAffiliatePayoutResult(id, "paid")
	}
	common.ApiSuccess(c, payout)
}

func RefreshAffiliatePayoutAlipayStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	payout, err := service.RefreshAffiliateAlipayPayout(c.Request.Context(), id, c.GetInt("id"))
	if affiliateAlipayPayoutError(c, err) {
		return
	}
	recordManageAudit(c, "affiliate.payout.alipay.refresh", map[string]interface{}{
		"payout_id": id,
		"status":    payout.Status,
	})
	if payout.Status == model.AffiliatePayoutStatusPaid {
		service.QueueAffiliatePayoutResult(id, "paid")
	}
	common.ApiSuccess(c, payout)
}

func GetAdminAffiliateAlipayPayoutStatus(c *gin.Context) {
	settings, err := model.GetAffiliateAlipayPayoutSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"enabled":    settings.Enabled,
		"configured": settings.Configured,
	})
}

func GetAffiliateAlipayPayoutSettings(c *gin.Context) {
	settings, err := model.GetAffiliateAlipayPayoutSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, settings)
}

func UpdateAffiliateAlipayPayoutSettings(c *gin.Context) {
	payload := affiliateAlipayPayoutSettingsPayload{}
	if err := common.DecodeJson(c.Request.Body, &payload); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := service.ValidateAffiliateAlipayPayoutKeyMaterial(payload.PrivateKey, payload.AppCertificate, payload.AlipayPublicCertificate, payload.AlipayRootCertificate); err != nil {
		common.ApiErrorI18n(c, i18n.MsgAffiliateAlipayConfigInvalid)
		return
	}
	err := model.UpdateAffiliateAlipayPayoutSettings(model.UpdateAffiliateAlipayPayoutSettingsParams{
		Enabled:                 payload.Enabled,
		AppId:                   payload.AppId,
		PrivateKey:              payload.PrivateKey,
		AppCertificate:          payload.AppCertificate,
		AlipayPublicCertificate: payload.AlipayPublicCertificate,
		AlipayRootCertificate:   payload.AlipayRootCertificate,
		TransferTitle:           payload.TransferTitle,
		ClearKeys:               payload.ClearKeys,
	})
	if errors.Is(err, model.ErrAffiliateAlipayNotConfigured) {
		common.ApiErrorI18n(c, i18n.MsgAffiliateAlipayNotConfigured)
		return
	}
	if errors.Is(err, model.ErrAffiliateAlipayConfigInvalid) {
		common.ApiErrorI18n(c, i18n.MsgAffiliateAlipayConfigInvalid)
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "affiliate.payout.alipay.settings", map[string]interface{}{
		"enabled":    payload.Enabled,
		"mode":       "certificate",
		"app_id":     strings.TrimSpace(payload.AppId),
		"clear_keys": payload.ClearKeys,
	})
	common.ApiSuccess(c, gin.H{"updated": true})
}

func TestAffiliateAlipayPayoutSettings(c *gin.Context) {
	if err := service.TestAffiliateAlipayPayoutConfig(c.Request.Context()); err != nil {
		if affiliateAlipayPayoutError(c, err) {
			return
		}
		return
	}
	recordManageAudit(c, "affiliate.payout.alipay.test", nil)
	common.ApiSuccess(c, gin.H{"verified": true})
}

func affiliateAlipayPayoutError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, model.ErrAffiliateAlipayNotConfigured) {
		common.ApiErrorI18n(c, i18n.MsgAffiliateAlipayNotConfigured)
		return true
	}
	apiErr := &service.AffiliateAlipayAPIError{}
	if errors.As(err, &apiErr) {
		switch strings.ToUpper(apiErr.DiagnosticCode()) {
		case "RESPONSE_SIGNATURE_MISSING", "RESPONSE_SIGNATURE_INVALID", "RESPONSE_SIGNATURE_VERIFICATION_FAILED":
			common.ApiErrorI18n(c, i18n.MsgAffiliateAlipayResponseSignatureInvalid)
			return true
		case "ISV.ILLEGAL-CLIENT-IP":
			common.ApiErrorI18n(c, i18n.MsgAffiliateAlipayClientIPNotTrusted)
			return true
		}
		common.ApiErrorI18n(c, i18n.MsgAffiliateAlipayPayoutFailed, map[string]any{"Error": apiErr.Error()})
		return true
	}
	return affiliatePayoutError(c, err)
}

func affiliatePayoutError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	key := ""
	switch {
	case errors.Is(err, model.ErrAffiliatePayoutNotFound):
		key = i18n.MsgAffiliatePayoutNotFound
	case errors.Is(err, model.ErrAffiliatePayoutForbidden):
		key = i18n.MsgAffiliatePayoutForbidden
	case errors.Is(err, model.ErrAffiliatePayoutStatusInvalid):
		key = i18n.MsgAffiliatePayoutStatusInvalid
	case errors.Is(err, model.ErrAffiliatePayoutAmountTooSmall):
		key = i18n.MsgAffiliatePayoutAmountTooSmall
	case errors.Is(err, model.ErrAffiliatePayoutInsufficientBalance):
		key = i18n.MsgAffiliatePayoutInsufficientBalance
	case errors.Is(err, model.ErrAffiliatePayoutAccountInvalid):
		key = i18n.MsgAffiliatePayoutAccountInvalid
	case errors.Is(err, model.ErrAffiliatePayoutRequestIdInvalid):
		key = i18n.MsgAffiliatePayoutRequestIdInvalid
	case errors.Is(err, model.ErrAffiliatePayoutRequestConflict):
		key = i18n.MsgAffiliatePayoutRequestConflict
	case errors.Is(err, model.ErrAffiliatePayoutRejectionReasonRequired):
		key = i18n.MsgAffiliatePayoutRejectionReasonRequired
	case errors.Is(err, model.ErrAffiliatePayoutSettlementNotDue):
		key = i18n.MsgAffiliatePayoutSettlementNotDue
	}
	if key != "" {
		common.ApiErrorI18n(c, key)
	} else {
		common.ApiError(c, err)
	}
	return true
}

func UpdateAffiliateSettings(c *gin.Context) {
	payload := affiliateSettingsPayload{}
	if err := common.DecodeJson(c.Request.Body, &payload); err != nil {
		common.ApiError(c, err)
		return
	}
	if payload.Enabled && (common.QuotaForInviter != 0 || common.QuotaForInvitee != 0) {
		common.ApiErrorI18n(c, i18n.MsgAffiliateLegacyRewardEnabled)
		return
	}
	juniorRate, ok := payload.GroupRates[model.AffiliatePromoterGroupDefault]
	if !ok {
		juniorRate, ok = payload.GroupRates[model.AffiliatePromoterGroupLegacyJunior]
	}
	if !ok {
		common.ApiError(c, errors.New("affiliate group rate for default is required"))
		return
	}
	payload.GroupRates[model.AffiliatePromoterGroupDefault] = juniorRate
	delete(payload.GroupRates, model.AffiliatePromoterGroupLegacyJunior)
	payload.DefaultRateBasisPoints = juniorRate
	groupRates, err := common.Marshal(payload.GroupRates)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	values := map[string]string{
		model.AffiliateCommissionEnabledOptionKey:               strconv.FormatBool(payload.Enabled),
		model.AffiliateCommissionAutoApproveOptionKey:           strconv.FormatBool(payload.AutoApprove),
		model.AffiliateCommissionDefaultRateOptionKey:           strconv.Itoa(payload.DefaultRateBasisPoints),
		model.AffiliateCommissionGroupRatesOptionKey:            string(groupRates),
		model.AffiliateUpgradeInviteesThresholdOptionKey:        strconv.Itoa(payload.UpgradeInviteesThreshold),
		model.AffiliateGoldUpgradeInviteesThresholdOptionKey:    strconv.Itoa(payload.GoldUpgradeInviteesThreshold),
		model.AffiliateUpgradeTopUpAmountThresholdOptionKey:     strconv.FormatInt(payload.UpgradeTopUpAmountThresholdCents, 10),
		model.AffiliateGoldUpgradeTopUpAmountThresholdOptionKey: strconv.FormatInt(payload.GoldUpgradeTopUpAmountThresholdCents, 10),
	}
	currentEnabled, activatedAt, _, _ := model.GetAffiliatePolicyState()
	if payload.Enabled && (!currentEnabled || activatedAt <= 0) {
		activatedAt = time.Now().Unix()
	}
	values[model.AffiliateCommissionActivatedAtOptionKey] = strconv.FormatInt(activatedAt, 10)
	for key, value := range values {
		if err := model.ValidateAffiliateOptionValue(key, value); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if err := model.ValidateAffiliateUpgradeThresholds(payload.UpgradeInviteesThreshold, payload.GoldUpgradeInviteesThreshold, payload.UpgradeTopUpAmountThresholdCents, payload.GoldUpgradeTopUpAmountThresholdCents); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateOptionsBulk(values); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "affiliate.settings.update", map[string]interface{}{
		"enabled":                                    payload.Enabled,
		"auto_approve":                               payload.AutoApprove,
		"default_rate_basis_points":                  payload.DefaultRateBasisPoints,
		"group_rates":                                payload.GroupRates,
		"upgrade_invitees_threshold":                 payload.UpgradeInviteesThreshold,
		"gold_upgrade_invitees_threshold":            payload.GoldUpgradeInviteesThreshold,
		"upgrade_top_up_amount_threshold_cents":      payload.UpgradeTopUpAmountThresholdCents,
		"gold_upgrade_top_up_amount_threshold_cents": payload.GoldUpgradeTopUpAmountThresholdCents,
		"activated_at":                               activatedAt,
	})
	common.ApiSuccess(c, gin.H{"updated": true})
}

func ApproveAffiliateCommission(c *gin.Context) {
	completeAffiliateCommission(c, true)
}

func RejectAffiliateCommission(c *gin.Context) {
	completeAffiliateCommission(c, false)
}

func completeAffiliateCommission(c *gin.Context, approve bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	payload := affiliateCommissionActionPayload{}
	_ = c.ShouldBindJSON(&payload)
	err = model.CompleteAffiliateCommission(id, c.GetInt("id"), approve, payload.Reason)
	if errors.Is(err, model.ErrAffiliateCommissionNotFound) {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": i18n.T(c, i18n.MsgAffiliateCommissionNotFound)})
		return
	}
	if errors.Is(err, model.ErrAffiliateCommissionStatusInvalid) {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": i18n.T(c, i18n.MsgAffiliateCommissionStatusInvalid)})
		return
	}
	if errors.Is(err, model.ErrAffiliateTopUpInvalid) {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": i18n.T(c, i18n.MsgAffiliateTopUpInvalid)})
		return
	}
	if errors.Is(err, model.ErrAffiliateRejectReasonRequired) {
		common.ApiErrorI18n(c, i18n.MsgAffiliateRejectReasonRequired)
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "affiliate.commission.review", map[string]interface{}{
		"commission_id": id,
		"approved":      approve,
	})
	service.QueueAffiliateCommissionResult(id)
	common.ApiSuccess(c, nil)
}

func GetAdminAffiliateUpgradeCandidates(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	rows, total, err := model.ListAffiliateUpgradeCandidates(pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(rows)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func ApproveAdminAffiliateUpgrade(c *gin.Context) {
	inviterId, err := strconv.Atoi(c.Param("id"))
	if err != nil || inviterId <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	payload := affiliateUpgradeApprovalPayload{}
	if err := common.DecodeJson(c.Request.Body, &payload); err != nil {
		common.ApiError(c, err)
		return
	}
	approvedGroup, err := model.ApproveAffiliateUpgrade(inviterId, payload.NextGroup)
	if err != nil {
		if errors.Is(err, model.ErrAffiliateUpgradeNotEligible) {
			common.ApiErrorI18n(c, i18n.MsgAffiliateUpgradeNotEligible)
			return
		}
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "affiliate.promoter.upgrade", map[string]interface{}{"inviter_id": inviterId, "group": approvedGroup})
	service.QueueAffiliateUpgradeApproved(inviterId, approvedGroup)
	common.ApiSuccess(c, gin.H{"updated": true, "group": approvedGroup})
}

func GetFailedAffiliateUpgradeNotices(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	rows, total, err := model.ListFailedAffiliateUpgradeNotices(pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(rows)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func RetryAffiliateUpgradeNotice(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	if err := model.RetryAffiliateUpgradeNotice(id); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "affiliate.notification.retry", map[string]interface{}{"notice_id": id})
	common.ApiSuccess(c, gin.H{"updated": true})
}

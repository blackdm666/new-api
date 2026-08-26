package service

import (
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/bytedance/gopkg/util/gopool"
)

const (
	affiliateUpgradeNotificationInterval = 30 * time.Second
	affiliateUpgradeNotificationLease    = 5 * time.Minute
)

func StartAffiliateUpgradeNotificationDelivery() {
	if !common.IsMasterNode {
		return
	}
	gopool.Go(func() {
		deliverPendingAffiliateUpgradeNotifications()
		ticker := time.NewTicker(affiliateUpgradeNotificationInterval)
		defer ticker.Stop()
		for range ticker.C {
			deliverPendingAffiliateUpgradeNotifications()
		}
	})
}

func deliverPendingAffiliateUpgradeNotifications() {
	if err := model.EnsureAffiliateUpgradeNoticesForEligibleInviters(); err != nil {
		common.SysError("failed to backfill affiliate upgrade notifications: " + err.Error())
		return
	}
	retryBefore := time.Now().Add(-affiliateUpgradeNotificationLease).Unix()
	notices, err := model.ListDueAffiliateUpgradeNotices(20, retryBefore)
	if err != nil {
		common.SysError("failed to list affiliate upgrade notifications: " + err.Error())
		return
	}
	for _, notice := range notices {
		deliverAffiliateUpgradeNotification(notice, retryBefore)
	}
}

func deliverAffiliateUpgradeNotification(notice *model.AffiliateUpgradeNotice, retryBefore int64) {
	if notice == nil || notice.Id <= 0 || notice.SentTime != 0 {
		return
	}
	claimed, err := model.ClaimAffiliateUpgradeNotice(notice.Id, common.GetTimestamp(), retryBefore)
	if err != nil || !claimed {
		if err != nil {
			common.SysError(fmt.Sprintf("failed to claim affiliate upgrade notification %d: %s", notice.Id, err.Error()))
		}
		return
	}

	inviter, err := model.GetUserById(notice.InviterId, false)
	if err != nil {
		recordAffiliateUpgradeNotificationFailure(notice.Id, err)
		return
	}
	enabled, _, _, _ := model.GetAffiliatePolicyState()
	if !enabled {
		_ = model.PostponeAffiliateUpgradeNotice(notice.Id, "affiliate commission is disabled", 6*time.Hour)
		return
	}
	if inviter.Status != common.UserStatusEnabled {
		_ = model.PostponeAffiliateUpgradeNotice(notice.Id, "affiliate promoter is disabled", 6*time.Hour)
		return
	}
	target, eligible := model.GetAffiliateUpgradeTarget(inviter.Group)
	if !eligible || notice.Threshold != target.Threshold || (notice.TopUpAmountThresholdCents > 0 && notice.TopUpAmountThresholdCents != target.TopUpAmountThresholdCents) {
		_ = model.CompleteAffiliateUpgradeNotice(notice.Id)
		return
	}
	metrics, metricsErr := model.GetAffiliateUpgradeMetrics(inviter.Id)
	if metricsErr != nil {
		recordAffiliateUpgradeNotificationFailure(notice.Id, metricsErr)
		return
	}
	if metrics.EffectiveInviteeCount < int64(target.Threshold) && metrics.EffectiveTopUpAmountCents < target.TopUpAmountThresholdCents {
		_ = model.PostponeAffiliateUpgradeNotice(notice.Id, "effective invitee count and top-up amount are below thresholds", time.Hour)
		return
	}
	root := model.GetRootUser()
	if root == nil {
		recordAffiliateUpgradeNotificationFailure(notice.Id, fmt.Errorf("root administrator is not configured"))
		return
	}
	receiver := strings.TrimSpace(root.GetSetting().NotificationEmail)
	if receiver == "" {
		receiver = strings.TrimSpace(root.Email)
	}
	if receiver == "" {
		recordAffiliateUpgradeNotificationFailure(notice.Id, fmt.Errorf("root administrator notification email is empty"))
		return
	}

	lang := root.GetSetting().Language
	if lang == "" {
		lang = appI18n.LangZhCN
	}
	displayName := strings.TrimSpace(inviter.DisplayName)
	if displayName == "" {
		displayName = inviter.Username
	}
	rateLabel := formatAffiliateRate(target.RateBasisPoints)
	rows := common.RenderInfoTableHTML([]common.EmailTemplateRow{
		{Label: affiliateEmailText(lang, "affiliate.email.label.promoter"), Value: html.EscapeString(displayName)},
		{Label: affiliateEmailText(lang, "affiliate.email.label.username"), Value: html.EscapeString(inviter.Username)},
		{Label: affiliateEmailText(lang, "affiliate.email.label.uid"), Value: fmt.Sprintf("%d", inviter.Id)},
		{Label: affiliateEmailText(lang, "affiliate.email.label.current_group"), Value: html.EscapeString(affiliateTierLabel(inviter.Group, lang))},
		{Label: affiliateEmailText(lang, "affiliate.email.label.effective_invitees"), Value: fmt.Sprintf("%d / %d", metrics.EffectiveInviteeCount, target.Threshold)},
		{Label: affiliateEmailText(lang, "affiliate.email.label.effective_topup_amount"), Value: fmt.Sprintf("%s / %s", formatAffiliateMoney(metrics.EffectiveTopUpAmountCents), formatAffiliateMoney(target.TopUpAmountThresholdCents))},
		{Label: affiliateEmailText(lang, "affiliate.email.label.next_group"), Value: html.EscapeString(affiliateTierLabel(target.Group, lang))},
		{Label: affiliateEmailText(lang, "affiliate.email.label.rate"), Value: html.EscapeString(rateLabel)},
	})
	actionURL := ""
	if baseURL := strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/"); baseURL != "" {
		actionURL = fmt.Sprintf("%s/admin-affiliates?keyword=%s", baseURL, url.QueryEscape(inviter.Username))
	}
	vars := map[string]string{
		"email_subject":          html.EscapeString(affiliateEmailText(lang, "affiliate.email.upgrade.subject")),
		"system_name":            html.EscapeString(common.SystemNameOrDefault()),
		"server_address":         html.EscapeString(strings.TrimRight(system_setting.ServerAddress, "/")),
		"heading":                html.EscapeString(affiliateEmailText(lang, "affiliate.email.upgrade.heading")),
		"intro":                  html.EscapeString(affiliateEmailText(lang, "affiliate.email.upgrade.intro", map[string]any{"Username": displayName, "NextGroup": affiliateTierLabel(target.Group, lang), "Rate": rateLabel})),
		"username":               html.EscapeString(inviter.Username),
		"effective_invitees":     fmt.Sprintf("%d", metrics.EffectiveInviteeCount),
		"upgrade_threshold":      fmt.Sprintf("%d", target.Threshold),
		"effective_topup_amount": formatAffiliateMoney(metrics.EffectiveTopUpAmountCents),
		"topup_amount_threshold": formatAffiliateMoney(target.TopUpAmountThresholdCents),
		"next_group":             html.EscapeString(affiliateTierLabel(target.Group, lang)),
		"next_rate":              html.EscapeString(rateLabel),
		"info_table":             rows,
		"action_url":             html.EscapeString(actionURL),
		"action_label":           html.EscapeString(affiliateEmailText(lang, "affiliate.email.upgrade.action")),
	}
	subject, content := RenderEmailByKeyForLang(constant.EmailTemplateKeyAffiliateUpgradeAdmin, lang, vars)
	if _, err = QueueSystemEmail(
		fmt.Sprintf("affiliate-upgrade-admin:%d", notice.Id),
		"affiliate_upgrade_admin",
		notice.Id,
		inviter.Id,
		receiver,
		subject,
		content,
		0,
	); err != nil {
		recordAffiliateUpgradeNotificationFailure(notice.Id, err)
		return
	}
	if err = model.CompleteAffiliateUpgradeNotice(notice.Id); err != nil {
		common.SysError(fmt.Sprintf("failed to complete affiliate upgrade notification %d: %s", notice.Id, err.Error()))
	}
}

func affiliateEmailText(lang string, key string, data ...map[string]any) string {
	_ = appI18n.Init()
	return appI18n.Translate(lang, key, data...)
}

func affiliateTierLabel(group string, lang string) string {
	key := "affiliate.email.tier.default"
	switch strings.TrimSpace(group) {
	case model.AffiliatePromoterGroupDefault, model.AffiliatePromoterGroupLegacyJunior:
		key = "affiliate.email.tier.junior"
	case model.AffiliatePromoterGroupAdvanced:
		key = "affiliate.email.tier.advanced"
	case model.AffiliatePromoterGroupGold:
		key = "affiliate.email.tier.gold"
	}
	return affiliateEmailText(lang, key)
}

func formatAffiliateRate(rateBasisPoints int) string {
	value := float64(rateBasisPoints) / 100
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".") + "%"
}

func formatAffiliateMoney(cents int64) string {
	return fmt.Sprintf("¥%d.%02d", cents/100, cents%100)
}

func QueueAffiliateCommissionResult(commissionId int) {
	record, err := model.GetAffiliateCommissionById(commissionId)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to load affiliate commission notification %d: %s", commissionId, err.Error()))
		return
	}
	user, err := model.GetUserById(record.InviterId, false)
	if err != nil || strings.TrimSpace(user.Email) == "" {
		return
	}
	lang := user.GetSetting().Language
	if lang == "" {
		lang = appI18n.LangZhCN
	}
	statusKey := "affiliate.email.status.approved"
	mode := "approved"
	if record.Status == model.AffiliateCommissionStatusRejected {
		statusKey = "affiliate.email.status.rejected"
		mode = "rejected"
	}
	statusLabel := affiliateEmailText(lang, statusKey)
	rows := []common.EmailTemplateRow{
		{Label: affiliateEmailText(lang, "affiliate.email.label.order"), Value: html.EscapeString(record.TradeNo)},
		{Label: affiliateEmailText(lang, "affiliate.email.label.topup_amount"), Value: fmt.Sprintf("%.2f", float64(record.TopUpAmountCents)/100)},
		{Label: affiliateEmailText(lang, "affiliate.email.label.commission"), Value: fmt.Sprintf("%.2f", float64(record.CommissionCents)/100)},
		{Label: affiliateEmailText(lang, "affiliate.email.label.status"), Value: html.EscapeString(statusLabel)},
	}
	if mode == "rejected" && record.RejectReason != "" {
		rows = append(rows, common.EmailTemplateRow{Label: affiliateEmailText(lang, "affiliate.email.label.reason"), Value: html.EscapeString(record.RejectReason)})
	}
	actionURL := strings.TrimRight(system_setting.ServerAddress, "/") + "/referral"
	vars := map[string]string{
		"email_subject":     html.EscapeString(affiliateEmailText(lang, "affiliate.email.commission."+mode+".subject")),
		"system_name":       html.EscapeString(common.SystemNameOrDefault()),
		"server_address":    html.EscapeString(strings.TrimRight(system_setting.ServerAddress, "/")),
		"heading":           html.EscapeString(affiliateEmailText(lang, "affiliate.email.commission."+mode+".heading")),
		"intro":             html.EscapeString(affiliateEmailText(lang, "affiliate.email.commission."+mode+".intro")),
		"order_number":      html.EscapeString(record.TradeNo),
		"topup_amount":      fmt.Sprintf("%.2f", float64(record.TopUpAmountCents)/100),
		"commission_amount": fmt.Sprintf("%.2f", float64(record.CommissionCents)/100),
		"commission_status": html.EscapeString(statusLabel),
		"reject_reason":     html.EscapeString(record.RejectReason),
		"info_table":        common.RenderInfoTableHTML(rows),
		"action_url":        html.EscapeString(actionURL),
		"action_label":      html.EscapeString(affiliateEmailText(lang, "affiliate.email.commission.action")),
	}
	subject, body := RenderEmailByKeyForLang(constant.EmailTemplateKeyAffiliateCommissionUser, lang, vars)
	if _, err := QueueSystemEmail(
		fmt.Sprintf("affiliate-commission:%d:%s", commissionId, mode),
		"affiliate_commission_user",
		commissionId,
		user.Id,
		strings.TrimSpace(user.Email),
		subject,
		body,
		0,
	); err != nil {
		common.SysError(fmt.Sprintf("failed to queue affiliate commission notification %d: %s", commissionId, err.Error()))
	}
}

func QueueAffiliateUpgradeApproved(userId int, group string) {
	user, err := model.GetUserById(userId, false)
	if err != nil || strings.TrimSpace(user.Email) == "" {
		return
	}
	lang := user.GetSetting().Language
	if lang == "" {
		lang = appI18n.LangZhCN
	}
	groupLabel := affiliateTierLabel(group, lang)
	rateLabel := formatAffiliateRate(model.AffiliateRateBasisPointsForGroup(group))
	rows := common.RenderInfoTableHTML([]common.EmailTemplateRow{
		{Label: affiliateEmailText(lang, "affiliate.email.label.username"), Value: html.EscapeString(user.Username)},
		{Label: affiliateEmailText(lang, "affiliate.email.label.new_group"), Value: html.EscapeString(groupLabel)},
		{Label: affiliateEmailText(lang, "affiliate.email.label.rate"), Value: html.EscapeString(rateLabel)},
	})
	actionURL := strings.TrimRight(system_setting.ServerAddress, "/") + "/referral"
	vars := map[string]string{
		"email_subject":  html.EscapeString(affiliateEmailText(lang, "affiliate.email.upgrade_approved.subject")),
		"system_name":    html.EscapeString(common.SystemNameOrDefault()),
		"server_address": html.EscapeString(strings.TrimRight(system_setting.ServerAddress, "/")),
		"heading":        html.EscapeString(affiliateEmailText(lang, "affiliate.email.upgrade_approved.heading")),
		"intro":          html.EscapeString(affiliateEmailText(lang, "affiliate.email.upgrade_approved.intro", map[string]any{"Group": groupLabel, "Rate": rateLabel})),
		"username":       html.EscapeString(user.Username),
		"new_group":      html.EscapeString(groupLabel),
		"new_rate":       html.EscapeString(rateLabel),
		"info_table":     rows,
		"action_url":     html.EscapeString(actionURL),
		"action_label":   html.EscapeString(affiliateEmailText(lang, "affiliate.email.upgrade_approved.action")),
	}
	subject, body := RenderEmailByKeyForLang(constant.EmailTemplateKeyAffiliateUpgradeUser, lang, vars)
	if _, err := QueueSystemEmail(
		fmt.Sprintf("affiliate-upgrade-user:%d:%s", userId, group),
		"affiliate_upgrade_user",
		userId,
		userId,
		strings.TrimSpace(user.Email),
		subject,
		body,
		0,
	); err != nil {
		common.SysError(fmt.Sprintf("failed to queue affiliate upgrade notification for user %d: %s", userId, err.Error()))
	}
}

func QueueAffiliatePayoutResult(payoutId int, event string) {
	payout, err := model.GetAffiliatePayoutById(payoutId)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to load affiliate payout notification %d: %s", payoutId, err.Error()))
		return
	}
	user, err := model.GetUserById(payout.UserId, false)
	if err != nil || strings.TrimSpace(user.Email) == "" {
		return
	}
	lang := user.GetSetting().Language
	if lang == "" {
		lang = appI18n.LangZhCN
	}
	statusKey := "affiliate.email.payout.status." + event
	statusLabel := affiliateEmailText(lang, statusKey)
	rows := []common.EmailTemplateRow{
		{Label: affiliateEmailText(lang, "affiliate.email.label.payout_id"), Value: fmt.Sprintf("#%d", payout.Id)},
		{Label: affiliateEmailText(lang, "affiliate.email.label.amount"), Value: formatAffiliateMoney(payout.AmountCents)},
		{Label: affiliateEmailText(lang, "affiliate.email.label.status"), Value: html.EscapeString(statusLabel)},
		{Label: affiliateEmailText(lang, "affiliate.email.label.alipay_recipient"), Value: html.EscapeString(payout.AccountName)},
		{Label: affiliateEmailText(lang, "affiliate.email.label.alipay_account"), Value: html.EscapeString("••••" + payout.AccountLast4)},
	}
	if event == "approved" {
		rows = append(rows, common.EmailTemplateRow{
			Label: affiliateEmailText(lang, "affiliate.email.label.settlement_time"),
			Value: time.Unix(payout.EligibleSettlementTime, 0).In(affiliatePayoutEmailLocation()).Format("2006-01-02"),
		})
	}
	if event == "rejected" && strings.TrimSpace(payout.RejectReason) != "" {
		rows = append(rows, common.EmailTemplateRow{Label: affiliateEmailText(lang, "affiliate.email.label.reason"), Value: html.EscapeString(payout.RejectReason)})
	}
	actionURL := strings.TrimRight(system_setting.ServerAddress, "/") + "/referral?tab=payouts"
	vars := map[string]string{
		"email_subject":   html.EscapeString(affiliateEmailText(lang, "affiliate.email.payout."+event+".subject")),
		"system_name":     html.EscapeString(common.SystemNameOrDefault()),
		"server_address":  html.EscapeString(strings.TrimRight(system_setting.ServerAddress, "/")),
		"heading":         html.EscapeString(affiliateEmailText(lang, "affiliate.email.payout."+event+".heading")),
		"intro":           html.EscapeString(affiliateEmailText(lang, "affiliate.email.payout."+event+".intro")),
		"payout_id":       fmt.Sprintf("%d", payout.Id),
		"amount":          formatAffiliateMoney(payout.AmountCents),
		"payout_status":   html.EscapeString(statusLabel),
		"account_name":    html.EscapeString(payout.AccountName),
		"account_last4":   html.EscapeString(payout.AccountLast4),
		"settlement_time": time.Unix(payout.EligibleSettlementTime, 0).In(affiliatePayoutEmailLocation()).Format("2006-01-02"),
		"reject_reason":   html.EscapeString(payout.RejectReason),
		"info_table":      common.RenderInfoTableHTML(rows),
		"action_url":      html.EscapeString(actionURL),
		"action_label":    html.EscapeString(affiliateEmailText(lang, "affiliate.email.payout.action")),
	}
	subject, body := RenderEmailByKeyForLang(constant.EmailTemplateKeyAffiliatePayoutUser, lang, vars)
	if _, err := QueueSystemEmail(
		fmt.Sprintf("affiliate-payout:%d:%s", payout.Id, event),
		"affiliate_payout_user",
		payout.Id,
		user.Id,
		strings.TrimSpace(user.Email),
		subject,
		body,
		0,
	); err != nil {
		common.SysError(fmt.Sprintf("failed to queue affiliate payout notification %d: %s", payout.Id, err.Error()))
	}
}

func affiliatePayoutEmailLocation() *time.Location {
	return time.FixedZone("Asia/Shanghai", 8*60*60)
}

func recordAffiliateUpgradeNotificationFailure(id int, err error) {
	if recordErr := model.RecordAffiliateUpgradeNoticeFailure(id, err.Error()); recordErr != nil {
		common.SysError(fmt.Sprintf("failed to record affiliate upgrade notification %d error: %s", id, recordErr.Error()))
	}
}

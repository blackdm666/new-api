package service

import (
	"encoding/hex"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

func invoiceEmailText(lang string, key string, data ...map[string]any) string {
	_ = appI18n.Init()
	return appI18n.Translate(lang, key, data...)
}

func invoiceRequestStatusLabel(status int, lang string) string {
	key := "invoice.email.status.unknown"
	switch status {
	case model.InvoiceStatusPending:
		key = "invoice.email.status.pending"
	case model.InvoiceStatusIssued:
		key = "invoice.email.status.issued"
	case model.InvoiceStatusRejected:
		key = "invoice.email.status.rejected"
	case model.InvoiceStatusWithdrawn:
		key = "invoice.email.status.withdrawn"
	case model.InvoiceStatusExpired:
		key = "invoice.email.status.expired"
	}
	return invoiceEmailText(lang, key)
}

func invoiceRequestLink(id int, admin bool) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	if base == "" {
		return ""
	}
	if admin {
		return fmt.Sprintf("%s/admin-invoices/%d", base, id)
	}
	return fmt.Sprintf("%s/invoices/%d", base, id)
}

func buildInvoiceRequestVars(request *model.InvoiceRequest, admin bool, previousStatus int, lang string, expiresAt int64) map[string]string {
	statusLabel := invoiceRequestStatusLabel(request.Status, lang)
	createdAt := time.Unix(request.CreatedTime, 0).Format("2006-01-02 15:04:05")
	rows := []common.EmailTemplateRow{
		{Label: invoiceEmailText(lang, "invoice.email.label.request_id"), Value: fmt.Sprintf("#%d", request.Id)},
		{Label: invoiceEmailText(lang, "invoice.email.label.username"), Value: html.EscapeString(request.Username)},
		{Label: invoiceEmailText(lang, "invoice.email.label.company_name"), Value: html.EscapeString(request.CompanyName)},
		{Label: invoiceEmailText(lang, "invoice.email.label.tax_number"), Value: html.EscapeString(request.TaxNumber)},
		{Label: invoiceEmailText(lang, "invoice.email.label.amount"), Value: fmt.Sprintf("CNY %.2f", request.TotalMoney)},
		{Label: invoiceEmailText(lang, "invoice.email.label.status"), Value: html.EscapeString(statusLabel)},
		{Label: invoiceEmailText(lang, "invoice.email.label.created_at"), Value: createdAt},
		{Label: invoiceEmailText(lang, "invoice.email.label.orders"), Value: common.EscapeAndBreak(request.OrderNumbers)},
	}
	mode := "user.issued"
	if admin {
		mode = "admin.created"
	}
	if expiresAt > 0 {
		mode = "admin.expiry"
		expiresAtText := time.Unix(expiresAt, 0).Format("2006-01-02 15:04:05")
		rows = append(rows, common.EmailTemplateRow{
			Label: invoiceEmailText(lang, "invoice.email.label.expires_at"),
			Value: expiresAtText,
		})
	}
	data := map[string]any{
		"ID":          request.Id,
		"Username":    request.Username,
		"CompanyName": request.CompanyName,
	}
	expiresAtText := ""
	if expiresAt > 0 {
		expiresAtText = time.Unix(expiresAt, 0).Format("2006-01-02 15:04:05")
	}
	return map[string]string{
		"email_subject":   html.EscapeString(invoiceEmailText(lang, "invoice.email."+mode+".subject", data)),
		"system_name":     html.EscapeString(common.SystemNameOrDefault()),
		"server_address":  html.EscapeString(strings.TrimRight(system_setting.ServerAddress, "/")),
		"heading":         html.EscapeString(invoiceEmailText(lang, "invoice.email."+mode+".heading")),
		"intro":           html.EscapeString(invoiceEmailText(lang, "invoice.email."+mode+".intro", data)),
		"invoice_id":      fmt.Sprintf("%d", request.Id),
		"username":        html.EscapeString(request.Username),
		"company_name":    html.EscapeString(request.CompanyName),
		"tax_number":      html.EscapeString(request.TaxNumber),
		"total_money":     fmt.Sprintf("%.2f", request.TotalMoney),
		"invoice_status":  html.EscapeString(statusLabel),
		"previous_status": html.EscapeString(invoiceRequestStatusLabel(previousStatus, lang)),
		"created_at":      html.EscapeString(createdAt),
		"expires_at":      html.EscapeString(expiresAtText),
		"order_numbers":   html.EscapeString(request.OrderNumbers),
		"info_table":      common.RenderInfoTableHTML(rows),
		"action_url":      html.EscapeString(invoiceRequestLink(request.Id, admin)),
		"action_label":    html.EscapeString(invoiceEmailText(lang, "invoice.email."+mode+".action")),
	}
}

func BuildInvoiceRequestCreatedNotifications(request *model.InvoiceRequest) ([]*model.InvoiceNotificationDelivery, error) {
	if request == nil || !common.InvoiceApplicationNotifyAdminEnabled {
		return nil, nil
	}
	recipients := parseNotificationEmails(common.InvoiceAdminEmail)
	vars := buildInvoiceRequestVars(request, true, 0, appI18n.LangZhCN, 0)
	subject, body := RenderEmailByKeyForLang(constant.EmailTemplateKeyInvoiceRequestAdmin, appI18n.LangZhCN, vars)
	deliveries := make([]*model.InvoiceNotificationDelivery, 0, len(recipients))
	for _, recipient := range recipients {
		seed := fmt.Sprintf("invoice:%d:%d:admin:%s", request.Id, request.CreatedTime, strings.ToLower(recipient))
		deliveries = append(deliveries, &model.InvoiceNotificationDelivery{
			DeliveryKey:      hex.EncodeToString(common.Sha256Raw([]byte(seed))),
			InvoiceRequestId: request.Id,
			Kind:             model.InvoiceNotificationKindAdminEmail,
			Recipient:        recipient,
			Subject:          subject,
			Body:             body,
		})
	}
	return deliveries, nil
}

func BuildInvoiceExpiryWarningNotifications(request *model.InvoiceRequest) ([]*model.InvoiceNotificationDelivery, error) {
	if request == nil {
		return nil, nil
	}
	expiresAt := time.Unix(request.CreatedTime, 0).Add(time.Duration(setting.InvoicePendingExpiryDays) * 24 * time.Hour).Unix()
	recipients := parseNotificationEmails(common.InvoiceAdminEmail)
	vars := buildInvoiceRequestVars(request, true, 0, appI18n.LangZhCN, expiresAt)
	subject, body := RenderEmailByKeyForLang(constant.EmailTemplateKeyInvoiceExpiryAdmin, appI18n.LangZhCN, vars)
	deliveries := make([]*model.InvoiceNotificationDelivery, 0, len(recipients))
	for _, recipient := range recipients {
		seed := fmt.Sprintf("invoice:%d:expires:%d:admin:%s", request.Id, expiresAt, strings.ToLower(recipient))
		deliveries = append(deliveries, &model.InvoiceNotificationDelivery{
			DeliveryKey:      hex.EncodeToString(common.Sha256Raw([]byte(seed))),
			InvoiceRequestId: request.Id,
			Kind:             model.InvoiceNotificationKindAdminEmail,
			Recipient:        recipient,
			Subject:          subject,
			Body:             body,
		})
	}
	return deliveries, nil
}

func shouldNotifyInvoiceIssued(request *model.InvoiceRequest, previousStatus int) bool {
	return request != nil &&
		common.InvoiceIssuedNotifyUserEnabled &&
		request.Status == model.InvoiceStatusIssued &&
		previousStatus != model.InvoiceStatusIssued
}

func BuildInvoiceIssuedNotifications(request *model.InvoiceRequest, previousStatus int, user *model.User) ([]*model.InvoiceNotificationDelivery, error) {
	if user == nil || !shouldNotifyInvoiceIssued(request, previousStatus) {
		return nil, nil
	}
	email := strings.TrimSpace(user.Email)
	if email == "" {
		return nil, nil
	}
	lang := user.GetSetting().Language
	vars := buildInvoiceRequestVars(request, false, previousStatus, lang, 0)
	subject, body := RenderEmailByKeyForLang(constant.EmailTemplateKeyInvoiceIssuedUser, lang, vars)
	seed := fmt.Sprintf("invoice:%d:issued:user_email", request.Id)
	return []*model.InvoiceNotificationDelivery{{
		DeliveryKey:      hex.EncodeToString(common.Sha256Raw([]byte(seed))),
		InvoiceRequestId: request.Id,
		Kind:             model.InvoiceNotificationKindUserEmail,
		UserId:           user.Id,
		Recipient:        email,
		Subject:          subject,
		Body:             body,
	}}, nil
}

func BuildInvoiceIssuedNotificationResend(request *model.InvoiceRequest, user *model.User, operatorId int) (*model.InvoiceNotificationDelivery, error) {
	if request == nil || user == nil || request.Status != model.InvoiceStatusIssued {
		return nil, model.ErrInvoiceStatusTransition
	}
	files, err := model.CountInvoiceFiles(request.Id)
	if err != nil {
		return nil, err
	}
	if files == 0 {
		return nil, model.ErrInvoiceFileRequired
	}
	deliveries, err := BuildInvoiceIssuedNotifications(request, model.InvoiceStatusPending, user)
	if err != nil {
		return nil, err
	}
	if len(deliveries) == 0 {
		return nil, nil
	}
	seed := fmt.Sprintf("invoice:%d:issued:user_email:resend:%d:%d", request.Id, time.Now().UnixNano(), operatorId)
	deliveries[0].DeliveryKey = hex.EncodeToString(common.Sha256Raw([]byte(seed)))
	return deliveries[0], nil
}

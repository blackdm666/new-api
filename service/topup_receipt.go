package service

// 88api topup receipt: a PDF receipt for every successful top-up. It is
// emailed to the user shortly after payment when payment_setting enables it,
// and the user (or an administrator) can download it from the billing history.

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicebill"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/shopspring/decimal"
)

const (
	TopUpReceiptEmailCategory = "topup_receipt_user"

	topUpReceiptScanInterval = time.Minute
	// Only top-ups completed this recently are emailed, so turning the
	// setting on does not mail receipts for old orders.
	topUpReceiptLookback  = 2 * time.Hour
	topUpReceiptScanLimit = 500
	topUpReceiptEmailTTL  = 3 * 24 * time.Hour
)

var (
	ErrTopUpReceiptUnavailable = errors.New("top-up receipt is only available for a successful paid order")

	// Receipts are dated in Beijing / Hong Kong time.
	topUpReceiptLocation = time.FixedZone("UTC+8", 8*60*60)
)

// TopUpReceiptNumber is the receipt number printed on the PDF:
// 88API-RC-<payment date>-<top-up id>.
func TopUpReceiptNumber(topUp *model.TopUp) string {
	return fmt.Sprintf("88API-RC-%s-%06d", topUpReceiptPaidAt(topUp).Format("20060102"), topUp.Id)
}

// TopUpReceiptFileName is the attachment and download name of the receipt.
func TopUpReceiptFileName(topUp *model.TopUp) string {
	return TopUpReceiptNumber(topUp) + ".pdf"
}

func topUpReceiptPaidAt(topUp *model.TopUp) time.Time {
	paidAt := topUp.CompleteTime
	if paidAt == 0 {
		paidAt = topUp.CreateTime
	}
	return time.Unix(paidAt, 0).In(topUpReceiptLocation)
}

func topUpReceiptCurrency(topUp *model.TopUp) string {
	if currency := strings.ToUpper(strings.TrimSpace(topUp.Currency)); currency != "" {
		return currency
	}
	return "CNY"
}

// topUpReceiptAvailable reports whether an order is a real, completed payment.
// Balance conversions are not payments and get no receipt.
func topUpReceiptAvailable(topUp *model.TopUp) bool {
	return topUp != nil && topUp.Id > 0 && topUp.Status == common.TopUpStatusSuccess && topUp.Money > 0 &&
		topUp.PaymentProvider != model.PaymentProviderBalance && topUp.PaymentMethod != model.PaymentMethodBalance
}

// topUpPaymentMethodName is the payment method as the top-up page shows it,
// e.g. "Stripe" for 88fkstripe.
func topUpPaymentMethodName(method string) string {
	for _, payMethod := range operation_setting.PayMethods {
		if payMethod["type"] == method && strings.TrimSpace(payMethod["name"]) != "" {
			return strings.TrimSpace(payMethod["name"])
		}
	}
	return method
}

func topUpReceiptText(lang string, key string, data ...map[string]any) string {
	_ = appI18n.Init()
	return appI18n.Translate(lang, key, data...)
}

// RenderTopUpReceipt lays out the receipt of a successful top-up in lang:
// the Hong Kong entity as issuer, the account as payer and the paid amount.
func RenderTopUpReceipt(topUp *model.TopUp, user *model.User, lang string) ([]byte, error) {
	if !topUpReceiptAvailable(topUp) || user == nil {
		return nil, ErrTopUpReceiptUnavailable
	}
	text := func(key string, data ...map[string]any) string {
		return topUpReceiptText(lang, key, data...)
	}
	var issuer []invoicebill.Field
	for _, field := range []invoicebill.Field{
		{Label: text(appI18n.MsgTopUpReceiptPdfIssuerName), Value: setting.InvoiceHongKongIssuerName},
		{Label: text(appI18n.MsgTopUpReceiptPdfIssuerRegistration), Value: setting.InvoiceHongKongIssuerRegistrationNo},
		{Label: text(appI18n.MsgTopUpReceiptPdfIssuerAddress), Value: setting.InvoiceHongKongIssuerAddress},
	} {
		field.Value = strings.TrimSpace(field.Value)
		if field.Value != "" {
			issuer = append(issuer, field)
		}
	}
	var payer []invoicebill.Field
	for _, field := range []invoicebill.Field{
		{Label: text(appI18n.MsgTopUpReceiptPdfPayerUsername), Value: user.Username},
		{Label: text(appI18n.MsgTopUpReceiptPdfPayerEmail), Value: user.Email},
		{Label: text(appI18n.MsgTopUpReceiptPdfPayerUserId), Value: fmt.Sprintf("%d", user.Id)},
	} {
		field.Value = strings.TrimSpace(field.Value)
		if field.Value != "" {
			payer = append(payer, field)
		}
	}
	cents := decimal.NewFromFloat(topUp.Money).Shift(2).Round(0).IntPart()
	paidAt := topUpReceiptPaidAt(topUp)
	return invoicebill.Render(invoicebill.Bill{
		Labels: invoicebill.Labels{
			Title:            text(appI18n.MsgTopUpReceiptPdfTitle),
			Subtitle:         text(appI18n.MsgTopUpReceiptPdfSubtitle),
			NumberLabel:      text(appI18n.MsgTopUpReceiptPdfNumber),
			RecipientHeading: text(appI18n.MsgTopUpReceiptPdfPayer),
			Amount:           text(appI18n.MsgTopUpReceiptPdfAmountPaid),
			Subtotal:         text(appI18n.MsgTopUpReceiptPdfSubtotal),
			Total:            text(appI18n.MsgTopUpReceiptPdfTotal),
			Description:      text(appI18n.MsgTopUpReceiptPdfDescription),
			Quantity:         text(appI18n.MsgTopUpReceiptPdfQuantity),
			UnitPrice:        text(appI18n.MsgTopUpReceiptPdfUnitPrice),
			LineAmount:       text(appI18n.MsgTopUpReceiptPdfLineAmount),
		},
		HeaderFields: []invoicebill.Field{
			{Label: text(appI18n.MsgTopUpReceiptPdfPaidAt), Value: paidAt.Format("2006-01-02 15:04")},
			{Label: text(appI18n.MsgTopUpReceiptPdfPaymentMethod), Value: topUpPaymentMethodName(topUp.PaymentMethod)},
		},
		Number:        TopUpReceiptNumber(topUp),
		Currency:      topUpReceiptCurrency(topUp),
		IssuerHeading: "88API",
		Issuer:        issuer,
		Recipient:     payer,
		Items: []invoicebill.Item{{
			Description: text(appI18n.MsgTopUpReceiptPdfItem),
			Detail:      text(appI18n.MsgTopUpReceiptPdfItemDetail, map[string]any{"TradeNo": topUp.TradeNo}),
			Quantity:    1,
			UnitCents:   cents,
			AmountCents: cents,
		}},
		TotalCents: cents,
		Footer:     text(appI18n.MsgTopUpReceiptPdfFooter),
		CreatedAt:  paidAt,
	})
}

func topUpReceiptAmount(topUp *model.TopUp) string {
	return topUpReceiptCurrency(topUp) + " " + decimal.NewFromFloat(topUp.Money).StringFixed(2)
}

// topUpReceiptEmailVars fills the topup_receipt_user email template.
func topUpReceiptEmailVars(topUp *model.TopUp, user *model.User, lang string) map[string]string {
	text := func(key string, data ...map[string]any) string {
		return topUpReceiptText(lang, key, data...)
	}
	amount := topUpReceiptAmount(topUp)
	paidAt := topUpReceiptPaidAt(topUp).Format("2006-01-02 15:04")
	method := topUpPaymentMethodName(topUp.PaymentMethod)
	rows := []common.EmailTemplateRow{
		{Label: text(appI18n.MsgTopUpReceiptEmailLabelReceiptNo), Value: html.EscapeString(TopUpReceiptNumber(topUp))},
		{Label: text(appI18n.MsgTopUpReceiptEmailLabelTradeNo), Value: html.EscapeString(topUp.TradeNo)},
		{Label: text(appI18n.MsgTopUpReceiptEmailLabelAmount), Value: html.EscapeString(amount)},
		{Label: text(appI18n.MsgTopUpReceiptEmailLabelPaidAt), Value: html.EscapeString(paidAt)},
		{Label: text(appI18n.MsgTopUpReceiptEmailLabelMethod), Value: html.EscapeString(method)},
	}
	return map[string]string{
		"system_name":    html.EscapeString(common.SystemNameOrDefault()),
		"server_address": html.EscapeString(systemEmailSiteURL()),
		"email_subject":  html.EscapeString(text(appI18n.MsgTopUpReceiptEmailSubject, map[string]any{"Amount": amount, "TradeNo": topUp.TradeNo})),
		"heading":        html.EscapeString(text(appI18n.MsgTopUpReceiptEmailHeading)),
		"intro":          html.EscapeString(text(appI18n.MsgTopUpReceiptEmailIntro)),
		"username":       html.EscapeString(user.Username),
		"receipt_number": html.EscapeString(TopUpReceiptNumber(topUp)),
		"trade_no":       html.EscapeString(topUp.TradeNo),
		"amount":         html.EscapeString(amount),
		"paid_at":        html.EscapeString(paidAt),
		"payment_method": html.EscapeString(method),
		"info_table":     common.RenderInfoTableHTML(rows),
		"action_url":     html.EscapeString(systemEmailSiteURL() + "/wallet"),
		"action_label":   html.EscapeString(text(appI18n.MsgTopUpReceiptEmailAction)),
	}
}

// QueueTopUpReceiptEmail puts the receipt email of a successful top-up in the
// outbox under deliveryKey. It returns nil without queuing anything when the
// account has no email address. The PDF is rendered when the email is sent.
func QueueTopUpReceiptEmail(topUp *model.TopUp, deliveryKey string) (*model.EmailDelivery, error) {
	if !topUpReceiptAvailable(topUp) {
		return nil, ErrTopUpReceiptUnavailable
	}
	user, err := model.GetUserById(topUp.UserId, false)
	if err != nil {
		return nil, err
	}
	email := strings.TrimSpace(user.Email)
	if email == "" {
		return nil, nil
	}
	lang := user.GetSetting().Language
	subject, body := RenderEmailByKeyForLang(constant.EmailTemplateKeyTopUpReceiptUser, lang, topUpReceiptEmailVars(topUp, user, lang))
	if strings.TrimSpace(subject) == "" || strings.TrimSpace(body) == "" {
		return nil, errors.New("top-up receipt email template is empty")
	}
	// The outbox poller on the master node sends it within half a minute.
	delivery, _, err := model.EnqueueEmailDelivery(&model.EmailDelivery{
		DeliveryKey: deliveryKey,
		Category:    TopUpReceiptEmailCategory,
		SMTPProfile: smtpProfileForCategory(TopUpReceiptEmailCategory),
		RelatedId:   topUp.Id,
		UserId:      user.Id,
		Recipient:   email,
		Subject:     subject,
		Body:        body,
		Priority:    model.EmailPriorityBusiness,
		ExpiresTime: time.Now().Add(topUpReceiptEmailTTL).Unix(),
	})
	if err != nil {
		return nil, err
	}
	return delivery, nil
}

// sendTopUpReceiptEmailDelivery sends a queued receipt email with the PDF
// rendered from the current order.
func sendTopUpReceiptEmailDelivery(delivery *model.EmailDelivery) (common.SMTPDeliveryResult, error) {
	topUp := model.GetTopUpById(delivery.RelatedId)
	if !topUpReceiptAvailable(topUp) || topUp.UserId != delivery.UserId {
		return common.SMTPDeliveryResult{}, ErrTopUpReceiptUnavailable
	}
	user, err := model.GetUserById(topUp.UserId, false)
	if err != nil {
		return common.SMTPDeliveryResult{}, err
	}
	content, err := RenderTopUpReceipt(topUp, user, user.GetSetting().Language)
	if err != nil {
		return common.SMTPDeliveryResult{}, err
	}
	return common.SendEmailWithAttachmentsProfileResult(delivery.SMTPProfile, delivery.Subject, delivery.Recipient, delivery.Body, []common.EmailAttachment{{
		Filename:    TopUpReceiptFileName(topUp),
		ContentType: "application/pdf",
		Reader:      bytes.NewReader(content),
	}})
}

// StartTopUpReceiptEmail runs on the master node and queues a receipt email
// for each top-up that completed in the last couple of hours. The outbox's
// unique delivery key keeps every order to one automatic email.
func StartTopUpReceiptEmail() {
	if !common.IsMasterNode {
		return
	}
	gopool.Go(func() {
		ticker := time.NewTicker(topUpReceiptScanInterval)
		defer ticker.Stop()
		for range ticker.C {
			queueRecentTopUpReceiptEmails(time.Now())
		}
	})
}

func queueRecentTopUpReceiptEmails(now time.Time) {
	paymentSetting := operation_setting.GetPaymentSetting()
	if !paymentSetting.TopUpReceiptEmailEnabled {
		return
	}
	topUps, err := model.ListTopUpsCompletedSince(now.Add(-topUpReceiptLookback).Unix(), topUpReceiptScanLimit)
	if err != nil {
		common.SysError("failed to list completed top-ups for receipts: " + err.Error())
		return
	}
	ids := make([]int, 0, len(topUps))
	for _, topUp := range topUps {
		ids = append(ids, topUp.Id)
	}
	queued, err := model.EmailDeliveryRelatedIds(TopUpReceiptEmailCategory, ids)
	if err != nil {
		common.SysError("failed to list queued top-up receipts: " + err.Error())
		return
	}
	for _, topUp := range topUps {
		if queued[topUp.Id] || !topUpReceiptAvailable(topUp) {
			continue
		}
		if methods := paymentSetting.TopUpReceiptPaymentMethods; len(methods) > 0 && !slices.Contains(methods, topUp.PaymentMethod) {
			continue
		}
		if _, err := QueueTopUpReceiptEmail(topUp, fmt.Sprintf("topup_receipt:%d", topUp.Id)); err != nil {
			common.SysError(fmt.Sprintf("failed to queue receipt of top-up %d: %s", topUp.Id, err.Error()))
		}
	}
}

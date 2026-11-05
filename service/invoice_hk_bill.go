package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicebill"
	"github.com/QuantumNous/new-api/setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// invoiceHongKongBillLocation dates the bill in Hong Kong time.
var invoiceHongKongBillLocation = time.FixedZone("HKT", 8*60*60)

// InvoiceHongKongAutoBillReady reports whether a Hong Kong request is issued
// at once with a generated bill instead of waiting for finance.
func InvoiceHongKongAutoBillReady() bool {
	return setting.InvoiceHongKongAutoBillEnabled && setting.InvoiceFileEnabled &&
		strings.TrimSpace(setting.InvoiceHongKongIssuerName) != ""
}

// InvoiceHongKongBillNumber is the bill number printed on the PDF:
// 88API-HK-<issue date>-<request id>.
func InvoiceHongKongBillNumber(request *model.InvoiceRequest, issuedAt time.Time) string {
	return fmt.Sprintf("88API-HK-%s-%06d", issuedAt.In(invoiceHongKongBillLocation).Format("20060102"), request.Id)
}

// RenderInvoiceHongKongBill lays out the bill for a Hong Kong request: the
// issuer from the settings, the applicant's name as recipient, one line per
// top-up order, in CNY and without tax.
func RenderInvoiceHongKongBill(request *model.InvoiceRequest, orders []*model.TopUp, issuedAt time.Time) ([]byte, string, error) {
	if request == nil || request.Region != model.InvoiceRegionHongKong {
		return nil, "", errors.New("not a Hong Kong invoice request")
	}
	number := InvoiceHongKongBillNumber(request, issuedAt)
	date := issuedAt.In(invoiceHongKongBillLocation).Format("2006-01-02")
	var issuer []invoicebill.Field
	for _, field := range []invoicebill.Field{
		{Label: "名稱", Value: setting.InvoiceHongKongIssuerName},
		{Label: "商業登記證號碼", Value: setting.InvoiceHongKongIssuerRegistrationNo},
		{Label: "地址", Value: setting.InvoiceHongKongIssuerAddress},
	} {
		field.Value = strings.TrimSpace(field.Value)
		if field.Value != "" {
			issuer = append(issuer, field)
		}
	}
	items := make([]invoicebill.Item, 0, len(orders))
	for _, order := range orders {
		cents := decimal.NewFromFloat(order.Money).Shift(2).Round(0).IntPart()
		paidAt := order.CompleteTime
		if paidAt == 0 {
			paidAt = order.CreateTime
		}
		items = append(items, invoicebill.Item{
			Description: "88API 平台 API 服務費（帳戶預充值）",
			Detail: fmt.Sprintf("訂單 %s · %s", order.TradeNo,
				time.Unix(paidAt, 0).In(invoiceHongKongBillLocation).Format("2006-01-02")),
			Quantity:    1,
			UnitCents:   cents,
			AmountCents: cents,
		})
	}
	content, err := invoicebill.Render(invoicebill.Bill{
		Number:        number,
		IssueDate:     date,
		DueDate:       date,
		Currency:      "CNY",
		IssuerHeading: "88API",
		Issuer:        issuer,
		Recipient:     []invoicebill.Field{{Label: "名稱", Value: request.CompanyName}},
		Items:         items,
		TotalCents:    invoiceTotalCents(request),
		Footer:        "本帳單由 88API 系統生成",
		CreatedAt:     issuedAt,
	})
	if err != nil {
		return nil, "", err
	}
	return content, number, nil
}

// IssueInvoiceHongKongBill generates the bill of a pending Hong Kong request,
// attaches it and marks the request issued: the applicant is emailed as for a
// manual issue and the finance group is told the bill went out on its own.
func IssueInvoiceHongKongBill(ctx context.Context, request *model.InvoiceRequest, orders []*model.TopUp) (*model.InvoiceRequest, error) {
	issuedAt := time.Now()
	content, number, err := RenderInvoiceHongKongBill(request, orders, issuedAt)
	if err != nil {
		return nil, err
	}
	fileName := fmt.Sprintf("帳單-%s.pdf", number)
	if _, err := StoreInvoiceFile(ctx, InvoiceFileContent{
		RequestId:    request.Id,
		FileName:     fileName,
		DeclaredMime: "application/pdf",
		Size:         int64(len(content)),
		Body:         bytes.NewReader(content),
	}); err != nil {
		return nil, err
	}
	// An applicant whose account is gone cannot be emailed, but the bill is
	// still issued.
	var user *model.User
	if common.InvoiceIssuedNotifyUserEnabled {
		user, err = model.GetUserById(request.UserId, false)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	issued, err := model.UpdateInvoiceRequestStatusWithNotifications(request.Id, model.InvoiceStatusIssued, 0, "",
		func(updated *model.InvoiceRequest) ([]*model.InvoiceNotificationDelivery, error) {
			deliveries, err := BuildInvoiceIssuedNotifications(updated, model.InvoiceStatusPending, user)
			if err != nil {
				return nil, err
			}
			if !invoiceWechatNotifyReady() {
				return deliveries, nil
			}
			return append(deliveries, newInvoiceWechatDelivery(
				updated, "issued", fmt.Sprintf("帳單已自动开具 #%d", updated.Id), buildInvoiceWechatAutoBilledText(updated, fileName),
			)), nil
		})
	if err != nil {
		return nil, err
	}
	WakeInvoiceWechatDelivery()
	return issued, nil
}

// QueueInvoiceRequestCreatedNotifications sends the usual new-request
// notifications for a Hong Kong request whose bill could not be generated,
// so finance issues it by hand.
func QueueInvoiceRequestCreatedNotifications(request *model.InvoiceRequest) error {
	deliveries, err := BuildInvoiceRequestCreatedNotifications(request)
	if err != nil {
		return err
	}
	for _, delivery := range deliveries {
		if delivery.Kind == model.InvoiceNotificationKindWechatGroup {
			delivery.Body += "\n系统自动生成帳單失败，请人工开具。"
		}
		if _, _, err := model.EnqueueInvoiceNotification(delivery); err != nil {
			return err
		}
	}
	WakeInvoiceWechatDelivery()
	return nil
}

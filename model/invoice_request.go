package model

import (
	"errors"
	"net/mail"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	InvoiceStatusPending   = 1
	InvoiceStatusIssued    = 2
	InvoiceStatusRejected  = 3
	InvoiceStatusWithdrawn = 4
	InvoiceStatusExpired   = 5

	InvoiceCompanyNameMaxLength    = 255
	InvoiceTaxNumberMaxLength      = 64
	InvoiceBankNameMaxLength       = 255
	InvoiceBankAccountMaxLength    = 128
	InvoiceCompanyAddressMaxLength = 512
	InvoiceCompanyPhoneMaxLength   = 32
	InvoiceEmailMaxLength          = 128
	InvoiceRemarkMaxLength         = 2000
)

var (
	ErrInvoiceRequestNotFound         = errors.New("invoice request not found")
	ErrInvoiceRequestForbidden        = errors.New("invoice request forbidden")
	ErrInvoiceStatusInvalid           = errors.New("invoice status invalid")
	ErrInvoiceStatusTransition        = errors.New("invoice status transition is not allowed")
	ErrInvoiceRejectionReasonRequired = errors.New("invoice rejection reason is required")
	ErrInvoiceRejectionReasonInvalid  = errors.New("invoice rejection reason is invalid")
	ErrInvoiceOrderEmpty              = errors.New("invoice order empty")
	ErrInvoiceOrderInvalid            = errors.New("invoice order invalid")
	ErrInvoiceOrderDuplicate          = errors.New("invoice order duplicate")
	ErrInvoiceAmountTooSmall          = errors.New("invoice amount too small")
	ErrInvoiceCompanyEmpty            = errors.New("invoice company empty")
	ErrInvoiceCompanyInvalid          = errors.New("invoice company invalid")
	ErrInvoiceTaxNumberEmpty          = errors.New("invoice tax number empty")
	ErrInvoiceTaxNumberInvalid        = errors.New("invoice tax number invalid")
	ErrInvoiceEmailEmpty              = errors.New("invoice email empty")
	ErrInvoiceEmailInvalid            = errors.New("invoice email invalid")
	ErrInvoiceOptionalFieldInvalid    = errors.New("invoice optional field invalid")
	ErrInvoiceWithdrawForbidden       = errors.New("invoice withdrawal is not allowed")
	ErrInvoicePurgeForbidden          = errors.New("invoice purge is not allowed")
	ErrInvoiceFileRequired            = errors.New("invoice file required")
)

type InvoiceRequest struct {
	Id                int     `json:"id"`
	UserId            int     `json:"user_id" gorm:"index;not null"`
	Username          string  `json:"username" gorm:"type:varchar(64);index"`
	CompanyName       string  `json:"company_name" gorm:"type:varchar(255);not null"`
	TaxNumber         string  `json:"tax_number" gorm:"type:varchar(64);not null;index"`
	BankName          string  `json:"bank_name" gorm:"type:varchar(255)"`
	BankAccount       string  `json:"bank_account" gorm:"type:varchar(128)"`
	CompanyAddress    string  `json:"company_address" gorm:"type:varchar(512)"`
	CompanyPhone      string  `json:"company_phone" gorm:"type:varchar(32)"`
	Email             string  `json:"-" gorm:"type:varchar(128);not null"`
	Remark            string  `json:"remark" gorm:"type:text"`
	TopUpOrderIds     string  `json:"topup_order_ids" gorm:"type:text;not null"`
	OrderNumbers      string  `json:"-" gorm:"type:text"`
	TotalMoney        float64 `json:"total_money"`
	TotalMoneyCents   int64   `json:"total_money_cents" gorm:"bigint;default:0"`
	Status            int     `json:"status" gorm:"type:int;index;default:1"`
	RejectionReason   string  `json:"rejection_reason" gorm:"type:varchar(500)"`
	IssuedTime        int64   `json:"issued_time" gorm:"bigint;default:0"`
	RedactedTime      int64   `json:"redacted_time" gorm:"bigint;index;default:0"`
	ExpiryWarningTime int64   `json:"expiry_warning_time" gorm:"bigint;index;default:0"`
	ExpiresAt         int64   `json:"expires_at" gorm:"-"`
	CreatedTime       int64   `json:"created_time" gorm:"bigint;index"`
	UpdatedTime       int64   `json:"updated_time" gorm:"bigint;index"`
}

type InvoiceOrderClaim struct {
	TopUpId          int   `json:"topup_id" gorm:"primaryKey;autoIncrement:false"`
	InvoiceRequestId int   `json:"invoice_request_id" gorm:"index;not null"`
	UserId           int   `json:"user_id" gorm:"index;not null"`
	CreatedTime      int64 `json:"created_time" gorm:"bigint"`
}

type InvoiceRequestEvent struct {
	Id               int    `json:"id"`
	InvoiceRequestId int    `json:"invoice_request_id" gorm:"index;not null"`
	FromStatus       int    `json:"from_status" gorm:"type:int;not null"`
	ToStatus         int    `json:"to_status" gorm:"type:int;not null"`
	OperatorId       int    `json:"operator_id" gorm:"index;not null"`
	Reason           string `json:"reason" gorm:"type:varchar(500)"`
	CreatedTime      int64  `json:"created_time" gorm:"bigint;index"`
}

const (
	invoiceSearchKindUsername = "username"
	invoiceSearchKindCompany  = "company"
	invoiceSearchKindTax      = "tax"
	invoiceSearchKindOrder    = "order"
)

// InvoiceSearchTerm keeps the administrator search path independent from the
// invoice payload. Values are normalized once and prefix-matched through an
// index instead of scanning multiple invoice columns with %keyword%.
type InvoiceSearchTerm struct {
	Id               int    `json:"id"`
	InvoiceRequestId int    `json:"invoice_request_id" gorm:"not null;index;uniqueIndex:idx_invoice_search_term"`
	Kind             string `json:"kind" gorm:"type:varchar(16);not null;uniqueIndex:idx_invoice_search_term"`
	Value            string `json:"value" gorm:"type:varchar(255);not null;index;uniqueIndex:idx_invoice_search_term"`
}

type InvoiceRequestQueryOptions struct {
	UserId             int
	Status             int
	Keyword            string
	PrioritizeExpiring bool
}

type CreateInvoiceRequestParams struct {
	UserId         int
	Username       string
	CompanyName    string
	TaxNumber      string
	BankName       string
	BankAccount    string
	CompanyAddress string
	CompanyPhone   string
	Email          string
	Remark         string
	TopUpOrderIds  []int
}

type InvoiceNotificationFactory func(request *InvoiceRequest) ([]*InvoiceNotificationDelivery, error)

func (request *InvoiceRequest) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if request.CreatedTime == 0 {
		request.CreatedTime = now
	}
	if request.UpdatedTime == 0 {
		request.UpdatedTime = now
	}
	return nil
}

func IsValidInvoiceStatus(status int) bool {
	switch status {
	case InvoiceStatusPending, InvoiceStatusIssued, InvoiceStatusRejected, InvoiceStatusWithdrawn, InvoiceStatusExpired:
		return true
	default:
		return false
	}
}

func normalizeInvoiceOrderIDs(orderIds []int) ([]int, error) {
	if len(orderIds) == 0 {
		return nil, ErrInvoiceOrderEmpty
	}
	if len(orderIds) > 100 {
		return nil, ErrInvoiceOrderInvalid
	}
	seen := make(map[int]struct{}, len(orderIds))
	normalized := make([]int, 0, len(orderIds))
	for _, orderId := range orderIds {
		if orderId <= 0 {
			return nil, ErrInvoiceOrderInvalid
		}
		if _, exists := seen[orderId]; exists {
			continue
		}
		seen[orderId] = struct{}{}
		normalized = append(normalized, orderId)
	}
	return normalized, nil
}

func (request *InvoiceRequest) SetTopUpOrderIDs(orderIds []int) error {
	normalized, err := normalizeInvoiceOrderIDs(orderIds)
	if err != nil {
		return err
	}
	raw, err := common.Marshal(normalized)
	if err != nil {
		return err
	}
	request.TopUpOrderIds = string(raw)
	return nil
}

func (request *InvoiceRequest) GetTopUpOrderIDs() ([]int, error) {
	if strings.TrimSpace(request.TopUpOrderIds) == "" {
		return []int{}, nil
	}
	var orderIds []int
	if err := common.UnmarshalJsonStr(request.TopUpOrderIds, &orderIds); err != nil {
		return nil, err
	}
	return normalizeInvoiceOrderIDs(orderIds)
}

func fetchInvoiceOrders(tx *gorm.DB, userId int, orderIds []int) ([]*TopUp, error) {
	var orders []*TopUp
	err := tx.Where("user_id = ? AND status = ?", userId, common.TopUpStatusSuccess).
		Where("id IN ?", orderIds).
		Find(&orders).Error
	return orders, err
}

func orderInvoiceTopUps(orderIds []int, orders []*TopUp) []*TopUp {
	orderIndex := make(map[int]*TopUp, len(orders))
	for _, order := range orders {
		orderIndex[order.Id] = order
	}
	result := make([]*TopUp, 0, len(orderIds))
	for _, orderId := range orderIds {
		if order := orderIndex[orderId]; order != nil {
			result = append(result, order)
		}
	}
	return result
}

func invoiceOrderNumbers(orders []*TopUp) string {
	values := make([]string, 0, len(orders))
	for _, order := range orders {
		if tradeNo := strings.TrimSpace(order.TradeNo); tradeNo != "" {
			values = append(values, tradeNo)
		}
	}
	return strings.Join(values, "\n")
}

func normalizedInvoiceSearchValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func buildInvoiceSearchTerms(request *InvoiceRequest, orders []*TopUp) []*InvoiceSearchTerm {
	values := []struct {
		kind  string
		value string
	}{
		{kind: invoiceSearchKindUsername, value: request.Username},
		{kind: invoiceSearchKindCompany, value: request.CompanyName},
		{kind: invoiceSearchKindTax, value: request.TaxNumber},
	}
	for _, order := range orders {
		values = append(values, struct {
			kind  string
			value string
		}{kind: invoiceSearchKindOrder, value: order.TradeNo})
	}

	terms := make([]*InvoiceSearchTerm, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		value := normalizedInvoiceSearchValue(item.value)
		if value == "" || len([]rune(value)) > 255 {
			continue
		}
		key := item.kind + "\x00" + value
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		terms = append(terms, &InvoiceSearchTerm{
			InvoiceRequestId: request.Id,
			Kind:             item.kind,
			Value:            value,
		})
	}
	return terms
}

func createInvoiceSearchTerms(tx *gorm.DB, request *InvoiceRequest, orders []*TopUp) error {
	terms := buildInvoiceSearchTerms(request, orders)
	if len(terms) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&terms).Error
}

// BackfillInvoiceSearchTerms upgrades existing non-redacted invoice requests.
// It is safe to rerun: only requests without search terms are selected and the
// unique key makes retries idempotent.
func BackfillInvoiceSearchTerms(batchSize int) (int, error) {
	if batchSize <= 0 || batchSize > 500 {
		batchSize = 100
	}
	backfilled := 0
	lastID := 0
	for {
		var requests []*InvoiceRequest
		err := DB.Where("id > ? AND redacted_time = 0 AND NOT EXISTS (SELECT 1 FROM invoice_search_terms WHERE invoice_search_terms.invoice_request_id = invoice_requests.id)", lastID).
			Order("id ASC").Limit(batchSize).Find(&requests).Error
		if err != nil {
			return backfilled, err
		}
		if len(requests) == 0 {
			return backfilled, nil
		}
		for _, request := range requests {
			lastID = request.Id
			orderIds, parseErr := request.GetTopUpOrderIDs()
			if parseErr != nil {
				common.SysLog("invoice search backfill skipped malformed order IDs for request " + strconv.Itoa(request.Id))
				orderIds = nil
			}
			orders, fetchErr := fetchInvoiceOrders(DB, request.UserId, orderIds)
			if fetchErr != nil {
				return backfilled, fetchErr
			}
			if err := createInvoiceSearchTerms(DB, request, orders); err != nil {
				return backfilled, err
			}
			backfilled++
		}
	}
}

func escapeInvoiceSearchPrefix(value string) string {
	replacer := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
	return replacer.Replace(value) + "%"
}

func createIndependentInvoiceOrderClaims(tx *gorm.DB, request *InvoiceRequest, orderIds []int) error {
	claims := make([]*InvoiceOrderClaim, 0, len(orderIds))
	now := common.GetTimestamp()
	for _, orderId := range orderIds {
		claims = append(claims, &InvoiceOrderClaim{
			TopUpId:          orderId,
			InvoiceRequestId: request.Id,
			UserId:           request.UserId,
			CreatedTime:      now,
		})
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&claims)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != int64(len(claims)) {
		return ErrInvoiceOrderDuplicate
	}
	return nil
}

func CreateInvoiceRequest(params CreateInvoiceRequestParams) (*InvoiceRequest, []*TopUp, error) {
	return CreateInvoiceRequestWithNotifications(params, nil)
}

func CreateInvoiceRequestWithNotifications(params CreateInvoiceRequestParams, notificationFactory InvoiceNotificationFactory) (*InvoiceRequest, []*TopUp, error) {
	companyName := strings.TrimSpace(params.CompanyName)
	if companyName == "" {
		return nil, nil, ErrInvoiceCompanyEmpty
	}
	if len([]rune(companyName)) > InvoiceCompanyNameMaxLength {
		return nil, nil, ErrInvoiceCompanyInvalid
	}
	taxNumber := strings.TrimSpace(params.TaxNumber)
	if taxNumber == "" {
		return nil, nil, ErrInvoiceTaxNumberEmpty
	}
	if len([]rune(taxNumber)) > InvoiceTaxNumberMaxLength {
		return nil, nil, ErrInvoiceTaxNumberInvalid
	}
	email := strings.TrimSpace(params.Email)
	parsedEmail, parseErr := mail.ParseAddress(email)
	if email == "" {
		return nil, nil, ErrInvoiceEmailEmpty
	}
	if parseErr != nil || !strings.EqualFold(parsedEmail.Address, email) || len([]rune(email)) > InvoiceEmailMaxLength {
		return nil, nil, ErrInvoiceEmailInvalid
	}
	if len([]rune(strings.TrimSpace(params.BankName))) > InvoiceBankNameMaxLength ||
		len([]rune(strings.TrimSpace(params.BankAccount))) > InvoiceBankAccountMaxLength ||
		len([]rune(strings.TrimSpace(params.CompanyAddress))) > InvoiceCompanyAddressMaxLength ||
		len([]rune(strings.TrimSpace(params.CompanyPhone))) > InvoiceCompanyPhoneMaxLength ||
		len([]rune(strings.TrimSpace(params.Remark))) > InvoiceRemarkMaxLength {
		return nil, nil, ErrInvoiceOptionalFieldInvalid
	}
	orderIds, err := normalizeInvoiceOrderIDs(params.TopUpOrderIds)
	if err != nil {
		return nil, nil, err
	}

	var request *InvoiceRequest
	var orderedTopUps []*TopUp
	err = DB.Transaction(func(tx *gorm.DB) error {
		orders, err := fetchInvoiceOrders(tx, params.UserId, orderIds)
		if err != nil {
			return err
		}
		if len(orders) != len(orderIds) {
			return ErrInvoiceOrderInvalid
		}
		orderedTopUps = orderInvoiceTopUps(orderIds, orders)
		total := decimal.Zero
		for _, order := range orderedTopUps {
			total = total.Add(decimal.NewFromFloat(order.Money))
		}
		totalCents := total.Mul(decimal.NewFromInt(100)).Round(0).IntPart()
		if totalCents < setting.InvoiceMinimumAmountCents {
			return ErrInvoiceAmountTooSmall
		}

		now := common.GetTimestamp()
		request = &InvoiceRequest{
			UserId:          params.UserId,
			Username:        strings.TrimSpace(params.Username),
			CompanyName:     companyName,
			TaxNumber:       taxNumber,
			BankName:        strings.TrimSpace(params.BankName),
			BankAccount:     strings.TrimSpace(params.BankAccount),
			CompanyAddress:  strings.TrimSpace(params.CompanyAddress),
			CompanyPhone:    strings.TrimSpace(params.CompanyPhone),
			Email:           email,
			Remark:          strings.TrimSpace(params.Remark),
			OrderNumbers:    invoiceOrderNumbers(orderedTopUps),
			TotalMoney:      decimal.NewFromInt(totalCents).Div(decimal.NewFromInt(100)).InexactFloat64(),
			TotalMoneyCents: totalCents,
			Status:          InvoiceStatusPending,
			CreatedTime:     now,
			UpdatedTime:     now,
		}
		if err := request.SetTopUpOrderIDs(orderIds); err != nil {
			return err
		}
		if err := tx.Create(request).Error; err != nil {
			return err
		}
		if err := createIndependentInvoiceOrderClaims(tx, request, orderIds); err != nil {
			return err
		}
		if err := createInvoiceSearchTerms(tx, request, orderedTopUps); err != nil {
			return err
		}
		if err := tx.Create(&InvoiceRequestEvent{
			InvoiceRequestId: request.Id,
			FromStatus:       0,
			ToStatus:         InvoiceStatusPending,
			OperatorId:       params.UserId,
			CreatedTime:      now,
		}).Error; err != nil {
			return err
		}
		if notificationFactory != nil {
			deliveries, err := notificationFactory(request)
			if err != nil {
				return err
			}
			for _, delivery := range deliveries {
				if _, _, err := EnqueueInvoiceNotificationTx(tx, delivery); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return request, orderedTopUps, nil
}

func ListInvoiceRequests(options InvoiceRequestQueryOptions, pageInfo *common.PageInfo) ([]*InvoiceRequest, int64, error) {
	query := DB.Model(&InvoiceRequest{})
	if options.UserId > 0 {
		query = query.Where("user_id = ?", options.UserId)
	}
	if options.Status > 0 {
		query = query.Where("status = ?", options.Status)
	}
	if keyword := strings.TrimSpace(options.Keyword); keyword != "" {
		normalizedKeyword := normalizedInvoiceSearchValue(keyword)
		conditions := "EXISTS (SELECT 1 FROM invoice_search_terms WHERE invoice_search_terms.invoice_request_id = invoice_requests.id AND invoice_search_terms.value LIKE ? ESCAPE '!')"
		args := []interface{}{escapeInvoiceSearchPrefix(normalizedKeyword)}
		if id, err := strconv.Atoi(keyword); err == nil && id > 0 {
			conditions = "id = ? OR " + conditions
			args = append([]interface{}{id}, args...)
		}
		query = query.Where("("+conditions+")", args...)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var requests []*InvoiceRequest
	order := "updated_time DESC, id DESC"
	if options.PrioritizeExpiring {
		order = "CASE WHEN status = 1 AND expiry_warning_time > 0 THEN 0 ELSE 1 END ASC, CASE WHEN status = 1 AND expiry_warning_time > 0 THEN created_time ELSE updated_time END ASC, id DESC"
	}
	err := query.Order(order).
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&requests).Error
	enrichInvoiceRequestExpiry(requests)
	return requests, total, err
}

func enrichInvoiceRequestExpiry(requests []*InvoiceRequest) {
	if setting.InvoicePendingExpiryDays <= 0 {
		return
	}
	ttl := int64(setting.InvoicePendingExpiryDays) * 24 * 3600
	for _, request := range requests {
		if request != nil && request.Status == InvoiceStatusPending && request.CreatedTime > 0 {
			request.ExpiresAt = request.CreatedTime + ttl
		}
	}
}

func GetInvoiceRequestById(id int) (*InvoiceRequest, error) {
	var request InvoiceRequest
	if err := DB.First(&request, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceRequestNotFound
		}
		return nil, err
	}
	return &request, nil
}

func GetUserInvoiceRequestById(id int, userId int) (*InvoiceRequest, error) {
	request, err := GetInvoiceRequestById(id)
	if err != nil {
		return nil, err
	}
	if request.UserId != userId {
		return nil, ErrInvoiceRequestForbidden
	}
	return request, nil
}

func GetInvoiceRequestOrders(request *InvoiceRequest) ([]*TopUp, error) {
	orderIds, err := request.GetTopUpOrderIDs()
	if err != nil {
		return nil, err
	}
	orders, err := fetchInvoiceOrders(DB, request.UserId, orderIds)
	if err != nil {
		return nil, err
	}
	return orderInvoiceTopUps(orderIds, orders), nil
}

func ListEligibleInvoiceOrders(userId int) ([]*TopUp, error) {
	var claimedIds []int
	if err := DB.Model(&InvoiceOrderClaim{}).Where("user_id = ?", userId).Pluck("top_up_id", &claimedIds).Error; err != nil {
		return nil, err
	}
	query := DB.Where("user_id = ? AND status = ?", userId, common.TopUpStatusSuccess)
	if len(claimedIds) > 0 {
		query = query.Not("id IN ?", claimedIds)
	}
	var orders []*TopUp
	err := query.Order("complete_time DESC, id DESC").Find(&orders).Error
	return orders, err
}

func UpdateInvoiceRequestStatus(id int, status int, operatorId int, reason string) (*InvoiceRequest, error) {
	return UpdateInvoiceRequestStatusWithNotifications(id, status, operatorId, reason, nil)
}

func UpdateInvoiceRequestStatusWithNotifications(id int, status int, operatorId int, reason string, notificationFactory InvoiceNotificationFactory) (*InvoiceRequest, error) {
	if !IsValidInvoiceStatus(status) {
		return nil, ErrInvoiceStatusInvalid
	}
	reason = strings.TrimSpace(reason)
	if status == InvoiceStatusRejected {
		if reason == "" {
			return nil, ErrInvoiceRejectionReasonRequired
		}
		if len([]rune(reason)) > 500 {
			return nil, ErrInvoiceRejectionReasonInvalid
		}
	}
	var request InvoiceRequest
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&request, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceRequestNotFound
			}
			return err
		}
		if request.Status == status {
			return nil
		}
		if request.Status != InvoiceStatusPending || (status != InvoiceStatusIssued && status != InvoiceStatusRejected) {
			return ErrInvoiceStatusTransition
		}
		if status == InvoiceStatusIssued {
			var fileCount int64
			if err := tx.Model(&InvoiceFile{}).Where("invoice_request_id = ?", request.Id).Count(&fileCount).Error; err != nil {
				return err
			}
			if fileCount == 0 {
				return ErrInvoiceFileRequired
			}
		}
		now := common.GetTimestamp()
		updates := map[string]interface{}{
			"status":           status,
			"updated_time":     now,
			"issued_time":      int64(0),
			"rejection_reason": "",
		}
		if status == InvoiceStatusIssued {
			updates["issued_time"] = now
		}
		if status == InvoiceStatusRejected {
			updates["rejection_reason"] = reason
			if err := tx.Where("invoice_request_id = ?", request.Id).Delete(&InvoiceOrderClaim{}).Error; err != nil {
				return err
			}
			if err := queueAndDeleteInvoiceFilesTx(tx, request.Id); err != nil {
				return err
			}
			if err := queueAndDeleteInvoiceUploadsTx(tx, request.Id); err != nil {
				return err
			}
		}
		if err := tx.Model(&InvoiceRequest{}).Where("id = ?", request.Id).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Create(&InvoiceRequestEvent{
			InvoiceRequestId: request.Id,
			FromStatus:       request.Status,
			ToStatus:         status,
			OperatorId:       operatorId,
			Reason:           reason,
			CreatedTime:      now,
		}).Error; err != nil {
			return err
		}
		request.Status = status
		request.RejectionReason = updates["rejection_reason"].(string)
		request.UpdatedTime = now
		request.IssuedTime = updates["issued_time"].(int64)
		if notificationFactory != nil {
			deliveries, err := notificationFactory(&request)
			if err != nil {
				return err
			}
			for _, delivery := range deliveries {
				if _, _, err := EnqueueInvoiceNotificationTx(tx, delivery); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	enrichInvoiceRequestExpiry([]*InvoiceRequest{&request})
	return &request, nil
}

func queueAndDeleteInvoiceFilesTx(tx *gorm.DB, requestId int) error {
	var files []*InvoiceFile
	if err := tx.Where("invoice_request_id = ?", requestId).Find(&files).Error; err != nil {
		return err
	}
	now := common.GetTimestamp()
	for _, file := range files {
		cleanup := &InvoiceFileCleanup{
			StorageProfileId: file.StorageProfileId,
			StorageType:      file.StorageType,
			StorageKey:       file.StorageKey,
			NextAttemptTime:  now,
			CreatedTime:      now,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(cleanup).Error; err != nil {
			return err
		}
	}
	return tx.Where("invoice_request_id = ?", requestId).Delete(&InvoiceFile{}).Error
}

func queueAndDeleteInvoiceUploadsTx(tx *gorm.DB, requestId int) error {
	var uploads []*InvoiceFileUpload
	if err := tx.Where("invoice_request_id = ?", requestId).Find(&uploads).Error; err != nil {
		return err
	}
	now := common.GetTimestamp()
	for _, upload := range uploads {
		cleanup := &InvoiceFileCleanup{
			StorageProfileId: upload.StorageProfileId,
			StorageType:      upload.StorageType,
			StorageKey:       upload.StorageKey,
			NextAttemptTime:  now,
			CreatedTime:      now,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(cleanup).Error; err != nil {
			return err
		}
	}
	return tx.Where("invoice_request_id = ?", requestId).Delete(&InvoiceFileUpload{}).Error
}

func WithdrawInvoiceRequest(id int, userId int) (*InvoiceRequest, error) {
	var request InvoiceRequest
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&request, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceRequestNotFound
			}
			return err
		}
		if request.UserId != userId {
			return ErrInvoiceRequestForbidden
		}
		if request.Status != InvoiceStatusPending {
			return ErrInvoiceWithdrawForbidden
		}
		return closePendingInvoiceRequestTx(tx, &request, InvoiceStatusWithdrawn, userId)
	})
	return &request, err
}

func closePendingInvoiceRequestTx(tx *gorm.DB, request *InvoiceRequest, status int, operatorId int) error {
	if request == nil || request.Status != InvoiceStatusPending || (status != InvoiceStatusWithdrawn && status != InvoiceStatusExpired) {
		return ErrInvoiceStatusTransition
	}
	now := common.GetTimestamp()
	if err := tx.Model(&InvoiceRequest{}).Where("id = ?", request.Id).Updates(map[string]interface{}{
		"status":           status,
		"updated_time":     now,
		"rejection_reason": "",
	}).Error; err != nil {
		return err
	}
	if err := tx.Where("invoice_request_id = ?", request.Id).Delete(&InvoiceOrderClaim{}).Error; err != nil {
		return err
	}
	if err := queueAndDeleteInvoiceFilesTx(tx, request.Id); err != nil {
		return err
	}
	if err := queueAndDeleteInvoiceUploadsTx(tx, request.Id); err != nil {
		return err
	}
	if err := tx.Create(&InvoiceRequestEvent{
		InvoiceRequestId: request.Id,
		FromStatus:       request.Status,
		ToStatus:         status,
		OperatorId:       operatorId,
		CreatedTime:      now,
	}).Error; err != nil {
		return err
	}
	request.Status = status
	request.UpdatedTime = now
	return nil
}

func ExpirePendingInvoiceRequests(cutoff int64, limit int) (int, error) {
	if cutoff <= 0 {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var ids []int
	if err := DB.Model(&InvoiceRequest{}).
		Where("status = ? AND created_time > 0 AND created_time <= ?", InvoiceStatusPending, cutoff).
		Order("id ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	expired := 0
	for _, id := range ids {
		err := DB.Transaction(func(tx *gorm.DB) error {
			var request InvoiceRequest
			if err := lockForUpdate(tx).First(&request, "id = ?", id).Error; err != nil {
				return err
			}
			if request.Status != InvoiceStatusPending || request.CreatedTime > cutoff {
				return nil
			}
			if err := closePendingInvoiceRequestTx(tx, &request, InvoiceStatusExpired, 0); err != nil {
				return err
			}
			expired++
			return nil
		})
		if err != nil {
			return expired, err
		}
	}
	return expired, nil
}

func QueuePendingInvoiceExpiryWarnings(warningCutoff int64, expiryCutoff int64, limit int, notificationFactory InvoiceNotificationFactory) (int, error) {
	if warningCutoff <= 0 || limit <= 0 {
		return 0, nil
	}
	if limit > 100 {
		limit = 100
	}
	var ids []int
	query := DB.Model(&InvoiceRequest{}).
		Where("status = ? AND expiry_warning_time = 0 AND created_time > 0 AND created_time <= ?", InvoiceStatusPending, warningCutoff)
	if expiryCutoff > 0 {
		query = query.Where("created_time > ?", expiryCutoff)
	}
	if err := query.Order("id ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	queued := 0
	for _, id := range ids {
		err := DB.Transaction(func(tx *gorm.DB) error {
			var request InvoiceRequest
			if err := lockForUpdate(tx).First(&request, "id = ?", id).Error; err != nil {
				return err
			}
			if request.Status != InvoiceStatusPending || request.ExpiryWarningTime != 0 || request.CreatedTime > warningCutoff || (expiryCutoff > 0 && request.CreatedTime <= expiryCutoff) {
				return nil
			}
			if notificationFactory != nil {
				deliveries, err := notificationFactory(&request)
				if err != nil {
					return err
				}
				if len(deliveries) == 0 {
					return nil
				}
				for _, delivery := range deliveries {
					if _, _, err := EnqueueInvoiceNotificationTx(tx, delivery); err != nil {
						return err
					}
				}
			}
			now := common.GetTimestamp()
			if err := tx.Model(&request).Update("expiry_warning_time", now).Error; err != nil {
				return err
			}
			queued++
			return nil
		})
		if err != nil {
			return queued, err
		}
	}
	return queued, nil
}

func PurgeInvoiceRequest(id int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var request InvoiceRequest
		if err := lockForUpdate(tx).First(&request, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceRequestNotFound
			}
			return err
		}
		if request.Status != InvoiceStatusRejected && request.Status != InvoiceStatusWithdrawn && request.Status != InvoiceStatusExpired {
			return ErrInvoicePurgeForbidden
		}
		if err := queueAndDeleteInvoiceFilesTx(tx, request.Id); err != nil {
			return err
		}
		if err := queueAndDeleteInvoiceUploadsTx(tx, request.Id); err != nil {
			return err
		}
		if err := tx.Where("invoice_request_id = ?", request.Id).Delete(&InvoiceOrderClaim{}).Error; err != nil {
			return err
		}
		if err := tx.Where("invoice_request_id = ?", request.Id).Delete(&InvoiceSearchTerm{}).Error; err != nil {
			return err
		}
		if err := tx.Where("invoice_request_id = ?", request.Id).Delete(&InvoiceRequestEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Where("invoice_request_id = ?", request.Id).Delete(&InvoiceNotificationDelivery{}).Error; err != nil {
			return err
		}
		return tx.Delete(&request).Error
	})
}

func ListInvoiceRequestEvents(requestId int) ([]*InvoiceRequestEvent, error) {
	var events []*InvoiceRequestEvent
	err := DB.Where("invoice_request_id = ?", requestId).Order("id ASC").Find(&events).Error
	return events, err
}

func RedactExpiredInvoiceRequests(cutoff int64, limit int) (int, error) {
	if cutoff <= 0 {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var ids []int
	err := DB.Model(&InvoiceRequest{}).
		Where("status IN ? AND redacted_time = 0 AND updated_time > 0 AND updated_time <= ?", []int{InvoiceStatusIssued, InvoiceStatusRejected, InvoiceStatusWithdrawn, InvoiceStatusExpired}, cutoff).
		Order("id ASC").Limit(limit).Pluck("id", &ids).Error
	if err != nil {
		return 0, err
	}
	redacted := 0
	for _, id := range ids {
		err = DB.Transaction(func(tx *gorm.DB) error {
			var request InvoiceRequest
			if err := lockForUpdate(tx).First(&request, "id = ?", id).Error; err != nil {
				return err
			}
			if request.RedactedTime != 0 || (request.Status != InvoiceStatusIssued && request.Status != InvoiceStatusRejected && request.Status != InvoiceStatusWithdrawn && request.Status != InvoiceStatusExpired) || request.UpdatedTime > cutoff {
				return nil
			}
			var files []*InvoiceFile
			if err := tx.Where("invoice_request_id = ?", request.Id).Find(&files).Error; err != nil {
				return err
			}
			now := common.GetTimestamp()
			for _, file := range files {
				cleanup := &InvoiceFileCleanup{
					StorageProfileId: file.StorageProfileId,
					StorageType:      file.StorageType,
					StorageKey:       file.StorageKey,
					NextAttemptTime:  now,
					CreatedTime:      now,
				}
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(cleanup).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("invoice_request_id = ?", request.Id).Delete(&InvoiceFile{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&InvoiceRequestEvent{}).Where("invoice_request_id = ?", request.Id).Update("reason", "").Error; err != nil {
				return err
			}
			if err := tx.Where("invoice_request_id = ?", request.Id).Delete(&InvoiceSearchTerm{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&InvoiceNotificationDelivery{}).Where("invoice_request_id = ? AND delivered_time = 0", request.Id).Updates(map[string]interface{}{
				"recipient":      "",
				"subject":        "",
				"body":           "",
				"last_error":     "",
				"locked_until":   int64(0),
				"delivered_time": now,
				"updated_time":   now,
			}).Error; err != nil {
				return err
			}
			result := tx.Model(&InvoiceRequest{}).Where("id = ?", request.Id).Updates(map[string]interface{}{
				"username":         "",
				"company_name":     "",
				"tax_number":       "",
				"bank_name":        "",
				"bank_account":     "",
				"company_address":  "",
				"company_phone":    "",
				"email":            "",
				"remark":           "",
				"order_numbers":    "",
				"rejection_reason": "",
				"redacted_time":    now,
			}).Error
			if result == nil {
				redacted++
			}
			return result
		})
		if err != nil {
			return redacted, err
		}
	}
	return redacted, nil
}

func TouchInvoiceRequest(id int) error {
	return DB.Model(&InvoiceRequest{}).Where("id = ?", id).
		Update("updated_time", common.GetTimestamp()).Error
}

package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	AffiliatePayoutStatusPending    = 1
	AffiliatePayoutStatusApproved   = 2
	AffiliatePayoutStatusPaid       = 3
	AffiliatePayoutStatusRejected   = 4
	AffiliatePayoutStatusCancelled  = 5
	AffiliatePayoutStatusProcessing = 6

	AffiliatePayoutMinimumCents  = int64(10000)
	AffiliatePayoutSettlementDay = 10

	AffiliatePayoutMethodAlipay = "alipay"
	AffiliatePayoutMethodBank   = "bank"

	AffiliatePayoutDisbursementManual       = "manual"
	AffiliatePayoutDisbursementAlipayDirect = "alipay_direct"
)

const affiliatePayoutAccountPurpose = "affiliate-payout-account"

var (
	ErrAffiliatePayoutNotFound                = errors.New("affiliate payout not found")
	ErrAffiliatePayoutForbidden               = errors.New("affiliate payout forbidden")
	ErrAffiliatePayoutStatusInvalid           = errors.New("affiliate payout status invalid")
	ErrAffiliatePayoutAmountTooSmall          = errors.New("affiliate payout amount too small")
	ErrAffiliatePayoutInsufficientBalance     = errors.New("affiliate payout balance is insufficient")
	ErrAffiliatePayoutAccountInvalid          = errors.New("affiliate payout account is invalid")
	ErrAffiliatePayoutRequestIdInvalid        = errors.New("affiliate payout request id is invalid")
	ErrAffiliatePayoutRequestConflict         = errors.New("affiliate payout request id conflicts with another payout")
	ErrAffiliatePayoutRejectionReasonRequired = errors.New("affiliate payout rejection reason is required")
	ErrAffiliatePayoutSettlementNotDue        = errors.New("affiliate payout is not due for settlement")
)

var affiliatePayoutNow = time.Now

type AffiliatePayout struct {
	Id                     int    `json:"id"`
	UserId                 int    `json:"user_id" gorm:"not null;index;index:idx_affiliate_payout_user_status,priority:1"`
	RequestId              string `json:"request_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	AmountCents            int64  `json:"amount_cents" gorm:"bigint;not null"`
	AmountQuota            int    `json:"amount_quota" gorm:"not null"`
	PaymentMethod          string `json:"payment_method" gorm:"type:varchar(16);not null"`
	AccountName            string `json:"account_name" gorm:"type:varchar(128);not null;index"`
	Account                string `json:"account" gorm:"-"`
	AccountEncrypted       string `json:"-" gorm:"column:account_encrypted;type:text;not null"`
	AccountLast4           string `json:"account_last4" gorm:"type:varchar(8);not null"`
	Status                 int    `json:"status" gorm:"not null;default:1;index;index:idx_affiliate_payout_user_status,priority:2"`
	EligibleSettlementTime int64  `json:"eligible_settlement_time" gorm:"bigint;not null;index"`
	RejectReason           string `json:"reject_reason" gorm:"type:varchar(500)"`
	PaymentReference       string `json:"payment_reference" gorm:"type:varchar(255);index"`
	DisbursementMode       string `json:"disbursement_mode" gorm:"type:varchar(24);index"`
	ProviderOrderId        string `json:"provider_order_id" gorm:"type:varchar(128);index"`
	ProviderFundOrderId    string `json:"provider_fund_order_id" gorm:"type:varchar(128);index"`
	ProviderStatus         string `json:"provider_status" gorm:"type:varchar(32);index"`
	ProviderErrorCode      string `json:"provider_error_code,omitempty" gorm:"type:varchar(128)"`
	ProviderErrorMessage   string `json:"provider_error_message,omitempty" gorm:"type:varchar(500)"`
	PaymentAttempt         int    `json:"payment_attempt" gorm:"default:0"`
	ProcessingTime         int64  `json:"processing_time" gorm:"bigint;default:0"`
	OperatorId             int    `json:"operator_id" gorm:"default:0"`
	ReviewedTime           int64  `json:"reviewed_time" gorm:"bigint;default:0"`
	PaidTime               int64  `json:"paid_time" gorm:"bigint;default:0"`
	CancelledTime          int64  `json:"cancelled_time" gorm:"bigint;default:0"`
	CreatedTime            int64  `json:"created_time" gorm:"bigint;autoCreateTime;index"`
	UpdatedTime            int64  `json:"updated_time" gorm:"bigint;autoUpdateTime"`
	Username               string `json:"username" gorm:"->;-:migration"`
	DisplayName            string `json:"display_name" gorm:"->;-:migration"`
}

func (payout *AffiliatePayout) AfterFind(_ *gorm.DB) error {
	if payout.AccountEncrypted == "" {
		return nil
	}
	plain, err := common.DecryptSensitiveValue(affiliatePayoutAccountPurpose, payout.AccountEncrypted)
	if err != nil {
		return fmt.Errorf("decrypt affiliate payout account: %w", err)
	}
	payout.Account = string(plain)
	return nil
}

type CreateAffiliatePayoutParams struct {
	UserId        int
	RequestId     string
	AmountCents   int64
	PaymentMethod string
	AccountName   string
	Account       string
}

type AffiliatePayoutQueryOptions struct {
	UserId  int
	Status  int
	Keyword string
}

type AffiliatePayoutSummary struct {
	AvailableQuota     int   `json:"available_quota"`
	AvailableCents     int64 `json:"available_cents"`
	FrozenQuota        int   `json:"frozen_quota"`
	FrozenCents        int64 `json:"frozen_cents"`
	MinimumCents       int64 `json:"minimum_cents"`
	SettlementDay      int   `json:"settlement_day"`
	NextSettlementTime int64 `json:"next_settlement_time"`
	IsSettlementDay    bool  `json:"is_settlement_day"`
}

type AffiliatePayoutAdminSummary struct {
	PendingCount    int64 `json:"pending_count"`
	ApprovedCount   int64 `json:"approved_count"`
	ProcessingCount int64 `json:"processing_count"`
	PendingCents    int64 `json:"pending_cents"`
	ApprovedCents   int64 `json:"approved_cents"`
	ProcessingCents int64 `json:"processing_cents"`
	PaidCents       int64 `json:"paid_cents"`
	SettlementDay   int   `json:"settlement_day"`
	IsSettlementDay bool  `json:"is_settlement_day"`
}

func CreateAffiliatePayout(params CreateAffiliatePayoutParams) (*AffiliatePayout, error) {
	params.RequestId = strings.TrimSpace(params.RequestId)
	params.PaymentMethod = strings.ToLower(strings.TrimSpace(params.PaymentMethod))
	params.AccountName = strings.TrimSpace(params.AccountName)
	params.Account = strings.TrimSpace(params.Account)
	if params.UserId <= 0 || params.RequestId == "" || len(params.RequestId) > 64 {
		return nil, ErrAffiliatePayoutRequestIdInvalid
	}
	if params.AmountCents < AffiliatePayoutMinimumCents {
		return nil, ErrAffiliatePayoutAmountTooSmall
	}
	if !validAffiliatePayoutAccount(params.PaymentMethod, params.AccountName, params.Account) {
		return nil, ErrAffiliatePayoutAccountInvalid
	}
	amountQuota, err := affiliateCentsToQuota(params.AmountCents)
	if err != nil || amountQuota <= 0 {
		return nil, ErrAffiliatePayoutAmountTooSmall
	}
	encrypted, err := common.EncryptSensitiveValue(affiliatePayoutAccountPurpose, []byte(params.Account))
	if err != nil {
		return nil, err
	}

	payout := &AffiliatePayout{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		existing := &AffiliatePayout{}
		existingResult := tx.Where("request_id = ?", params.RequestId).Limit(1).Find(existing)
		if existingResult.Error != nil {
			return existingResult.Error
		}
		if existingResult.RowsAffected == 1 {
			if existing.UserId != params.UserId {
				return ErrAffiliatePayoutForbidden
			}
			if existing.AmountCents != params.AmountCents || existing.PaymentMethod != params.PaymentMethod || existing.AccountName != params.AccountName || existing.Account != params.Account {
				return ErrAffiliatePayoutRequestConflict
			}
			*payout = *existing
			return nil
		}

		if err := lockForUpdate(tx).Select("id").Where("id = ?", params.UserId).First(&User{}).Error; err != nil {
			return err
		}
		account, err := ensureAffiliateAccountTx(tx, params.UserId)
		if err != nil {
			return err
		}
		if account.AvailableCents < params.AmountCents {
			return ErrAffiliatePayoutInsufficientBalance
		}

		now := affiliatePayoutNow()
		*payout = AffiliatePayout{
			UserId:                 params.UserId,
			RequestId:              params.RequestId,
			AmountCents:            params.AmountCents,
			AmountQuota:            amountQuota,
			PaymentMethod:          params.PaymentMethod,
			AccountName:            params.AccountName,
			Account:                params.Account,
			AccountEncrypted:       encrypted,
			AccountLast4:           affiliatePayoutLast4(params.Account),
			Status:                 AffiliatePayoutStatusPending,
			EligibleSettlementTime: nextAffiliatePayoutSettlementTime(now),
		}
		if err := tx.Create(payout).Error; err != nil {
			return err
		}
		return tx.Model(account).Updates(map[string]any{
			"available_cents": gorm.Expr("available_cents - ?", params.AmountCents),
			"frozen_cents":    gorm.Expr("frozen_cents + ?", params.AmountCents),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return payout, nil
}

func CancelAffiliatePayout(id int, userId int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		payout, err := lockAffiliatePayout(tx, id)
		if err != nil {
			return err
		}
		if payout.UserId != userId {
			return ErrAffiliatePayoutForbidden
		}
		if payout.Status != AffiliatePayoutStatusPending {
			return ErrAffiliatePayoutStatusInvalid
		}
		if err := releaseAffiliatePayoutFrozenTx(tx, payout, true); err != nil {
			return err
		}
		return tx.Model(payout).Updates(map[string]interface{}{
			"status":         AffiliatePayoutStatusCancelled,
			"cancelled_time": common.GetTimestamp(),
		}).Error
	})
}

func ReviewAffiliatePayout(id int, operatorId int, approve bool, reason string) error {
	reason = strings.TrimSpace(reason)
	if !approve && reason == "" {
		return ErrAffiliatePayoutRejectionReasonRequired
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		payout, err := lockAffiliatePayout(tx, id)
		if err != nil {
			return err
		}
		if payout.Status != AffiliatePayoutStatusPending {
			return ErrAffiliatePayoutStatusInvalid
		}
		updates := map[string]interface{}{
			"operator_id":   operatorId,
			"reviewed_time": common.GetTimestamp(),
		}
		if approve {
			updates["status"] = AffiliatePayoutStatusApproved
			return tx.Model(payout).Updates(updates).Error
		}
		if err := releaseAffiliatePayoutFrozenTx(tx, payout, true); err != nil {
			return err
		}
		updates["status"] = AffiliatePayoutStatusRejected
		updates["reject_reason"] = reason
		return tx.Model(payout).Updates(updates).Error
	})
}

func MarkAffiliatePayoutPaid(id int, operatorId int) error {
	now := affiliatePayoutNow()
	return DB.Transaction(func(tx *gorm.DB) error {
		payout, err := lockAffiliatePayout(tx, id)
		if err != nil {
			return err
		}
		if payout.Status != AffiliatePayoutStatusApproved {
			return ErrAffiliatePayoutStatusInvalid
		}
		if !affiliatePayoutSettlementDue(payout, now) {
			return ErrAffiliatePayoutSettlementNotDue
		}
		if err := releaseAffiliatePayoutFrozenTx(tx, payout, false); err != nil {
			return err
		}
		paymentReference := fmt.Sprintf("MANUAL-%d-%d", payout.Id, now.Unix())
		return tx.Model(payout).Updates(map[string]interface{}{
			"status":                 AffiliatePayoutStatusPaid,
			"operator_id":            operatorId,
			"payment_reference":      paymentReference,
			"disbursement_mode":      AffiliatePayoutDisbursementManual,
			"provider_status":        "MANUAL_CONFIRMED",
			"provider_error_code":    "",
			"provider_error_message": "",
			"paid_time":              now.Unix(),
		}).Error
	})
}

// BeginAffiliatePayoutDisbursement reserves an approved payout for a single
// provider attempt. A processing payout reuses the same merchant order number
// so an ambiguous network response is queried instead of paid twice.
func BeginAffiliatePayoutDisbursement(id int, operatorId int) (*AffiliatePayout, bool, error) {
	payout := &AffiliatePayout{}
	started := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		current, err := lockAffiliatePayout(tx, id)
		if err != nil {
			return err
		}
		if current.Status == AffiliatePayoutStatusPaid && current.DisbursementMode == AffiliatePayoutDisbursementAlipayDirect {
			*payout = *current
			return nil
		}
		if current.Status == AffiliatePayoutStatusProcessing && current.DisbursementMode == AffiliatePayoutDisbursementAlipayDirect {
			*payout = *current
			return nil
		}
		if current.Status != AffiliatePayoutStatusApproved {
			return ErrAffiliatePayoutStatusInvalid
		}
		if !affiliatePayoutSettlementDue(current, affiliatePayoutNow()) {
			return ErrAffiliatePayoutSettlementNotDue
		}
		attempt := current.PaymentAttempt + 1
		reference := affiliatePayoutProviderReference(current.Id, current.RequestId, attempt)
		now := affiliatePayoutNow().Unix()
		if err := tx.Model(current).Updates(map[string]interface{}{
			"status":                 AffiliatePayoutStatusProcessing,
			"operator_id":            operatorId,
			"payment_reference":      reference,
			"disbursement_mode":      AffiliatePayoutDisbursementAlipayDirect,
			"provider_status":        "SUBMITTING",
			"provider_error_code":    "",
			"provider_error_message": "",
			"payment_attempt":        attempt,
			"processing_time":        now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", current.Id).First(current).Error; err != nil {
			return err
		}
		*payout = *current
		started = true
		return nil
	})
	return payout, started, err
}

func CompleteAffiliatePayoutDisbursement(id int, operatorId int, reference string, providerOrderId string, providerFundOrderId string, providerStatus string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		payout, err := lockAffiliatePayout(tx, id)
		if err != nil {
			return err
		}
		if payout.Status == AffiliatePayoutStatusPaid && payout.PaymentReference == reference {
			return nil
		}
		if payout.Status != AffiliatePayoutStatusProcessing || payout.PaymentReference != reference || payout.DisbursementMode != AffiliatePayoutDisbursementAlipayDirect {
			return ErrAffiliatePayoutStatusInvalid
		}
		if err := releaseAffiliatePayoutFrozenTx(tx, payout, false); err != nil {
			return err
		}
		return tx.Model(payout).Updates(map[string]interface{}{
			"status":                 AffiliatePayoutStatusPaid,
			"operator_id":            operatorId,
			"provider_order_id":      strings.TrimSpace(providerOrderId),
			"provider_fund_order_id": strings.TrimSpace(providerFundOrderId),
			"provider_status":        strings.TrimSpace(providerStatus),
			"provider_error_code":    "",
			"provider_error_message": "",
			"paid_time":              affiliatePayoutNow().Unix(),
		}).Error
	})
}

func UpdateAffiliatePayoutDisbursementProcessing(id int, reference string, providerStatus string, providerOrderId string, errorCode string, errorMessage string) error {
	result := DB.Model(&AffiliatePayout{}).
		Where("id = ? AND status = ? AND payment_reference = ?", id, AffiliatePayoutStatusProcessing, reference).
		Updates(map[string]interface{}{
			"provider_status":        strings.TrimSpace(providerStatus),
			"provider_order_id":      strings.TrimSpace(providerOrderId),
			"provider_error_code":    strings.TrimSpace(errorCode),
			"provider_error_message": strings.TrimSpace(errorMessage),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAffiliatePayoutStatusInvalid
	}
	return nil
}

func FailAffiliatePayoutDisbursement(id int, reference string, errorCode string, errorMessage string) error {
	result := DB.Model(&AffiliatePayout{}).
		Where("id = ? AND status = ? AND payment_reference = ?", id, AffiliatePayoutStatusProcessing, reference).
		Updates(map[string]interface{}{
			"status":                 AffiliatePayoutStatusApproved,
			"provider_status":        "FAILED",
			"provider_error_code":    strings.TrimSpace(errorCode),
			"provider_error_message": strings.TrimSpace(errorMessage),
			"processing_time":        0,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAffiliatePayoutStatusInvalid
	}
	return nil
}

func GetAffiliatePayoutById(id int) (*AffiliatePayout, error) {
	payout := &AffiliatePayout{}
	if err := DB.Where("id = ?", id).First(payout).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAffiliatePayoutNotFound
		}
		return nil, err
	}
	return payout, nil
}

func affiliatePayoutProviderReference(id int, requestId string, attempt int) string {
	digest := sha256.Sum256([]byte(requestId))
	return fmt.Sprintf("AFFP-%d-%d-%x", id, attempt, digest[:8])
}

func ListAffiliatePayouts(options AffiliatePayoutQueryOptions, pageInfo *common.PageInfo) ([]*AffiliatePayout, int64, error) {
	query := DB.Model(&AffiliatePayout{}).
		Select("affiliate_payouts.*, users.username, users.display_name").
		Joins("LEFT JOIN users ON users.id = affiliate_payouts.user_id")
	if options.UserId > 0 {
		query = query.Where("affiliate_payouts.user_id = ?", options.UserId)
	}
	if options.Status > 0 {
		query = query.Where("affiliate_payouts.status = ?", options.Status)
	}
	if keyword := strings.TrimSpace(options.Keyword); keyword != "" {
		pattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		if id, err := strconv.Atoi(keyword); err == nil && id > 0 {
			query = query.Where("affiliate_payouts.id = ? OR affiliate_payouts.user_id = ?", id, id)
		} else {
			query = query.Where("(users.username LIKE ? ESCAPE '!' OR users.display_name LIKE ? ESCAPE '!' OR affiliate_payouts.account_name LIKE ? ESCAPE '!' OR affiliate_payouts.payment_reference LIKE ? ESCAPE '!')", pattern, pattern, pattern, pattern)
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []*AffiliatePayout{}
	if err := query.Order("affiliate_payouts.id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func GetAffiliatePayoutSummary(userId int) (*AffiliatePayoutSummary, error) {
	if _, err := GetUserById(userId, false); err != nil {
		return nil, err
	}
	account, err := GetAffiliateAccount(userId)
	if err != nil {
		return nil, err
	}
	availableQuota, err := affiliateCentsToQuota(account.AvailableCents)
	if err != nil {
		return nil, err
	}
	frozenQuota, err := affiliateCentsToQuota(account.FrozenCents)
	if err != nil {
		return nil, err
	}
	now := affiliatePayoutNow()
	return &AffiliatePayoutSummary{
		AvailableQuota:     availableQuota,
		AvailableCents:     account.AvailableCents,
		FrozenQuota:        frozenQuota,
		FrozenCents:        account.FrozenCents,
		MinimumCents:       AffiliatePayoutMinimumCents,
		SettlementDay:      AffiliatePayoutSettlementDay,
		NextSettlementTime: nextAffiliatePayoutSettlementTime(now),
		IsSettlementDay:    IsAffiliatePayoutSettlementDay(now),
	}, nil
}

func GetAffiliatePayoutAdminSummary() (*AffiliatePayoutAdminSummary, error) {
	type sums struct {
		PendingCount    int64
		ApprovedCount   int64
		ProcessingCount int64
		PendingCents    int64
		ApprovedCents   int64
		ProcessingCents int64
		PaidCents       int64
	}
	row := sums{}
	if err := DB.Model(&AffiliatePayout{}).Select(
		"COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS pending_count, COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS approved_count, COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS processing_count, COALESCE(SUM(CASE WHEN status = ? THEN amount_cents ELSE 0 END), 0) AS pending_cents, COALESCE(SUM(CASE WHEN status = ? THEN amount_cents ELSE 0 END), 0) AS approved_cents, COALESCE(SUM(CASE WHEN status = ? THEN amount_cents ELSE 0 END), 0) AS processing_cents, COALESCE(SUM(CASE WHEN status = ? THEN amount_cents ELSE 0 END), 0) AS paid_cents",
		AffiliatePayoutStatusPending,
		AffiliatePayoutStatusApproved,
		AffiliatePayoutStatusProcessing,
		AffiliatePayoutStatusPending,
		AffiliatePayoutStatusApproved,
		AffiliatePayoutStatusProcessing,
		AffiliatePayoutStatusPaid,
	).Scan(&row).Error; err != nil {
		return nil, err
	}
	return &AffiliatePayoutAdminSummary{
		PendingCount:    row.PendingCount,
		ApprovedCount:   row.ApprovedCount,
		ProcessingCount: row.ProcessingCount,
		PendingCents:    row.PendingCents,
		ApprovedCents:   row.ApprovedCents,
		ProcessingCents: row.ProcessingCents,
		PaidCents:       row.PaidCents,
		SettlementDay:   AffiliatePayoutSettlementDay,
		IsSettlementDay: IsAffiliatePayoutSettlementDay(affiliatePayoutNow()),
	}, nil
}

func lockAffiliatePayout(tx *gorm.DB, id int) (*AffiliatePayout, error) {
	payout := &AffiliatePayout{}
	if err := lockForUpdate(tx).Where("id = ?", id).First(payout).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAffiliatePayoutNotFound
		}
		return nil, err
	}
	return payout, nil
}

func releaseAffiliatePayoutFrozenTx(tx *gorm.DB, payout *AffiliatePayout, restoreAvailable bool) error {
	account, err := ensureAffiliateAccountTx(tx, payout.UserId)
	if err != nil {
		return err
	}
	if account.FrozenCents < payout.AmountCents {
		return ErrAffiliatePayoutStatusInvalid
	}
	updates := map[string]any{
		"frozen_cents": gorm.Expr("frozen_cents - ?", payout.AmountCents),
	}
	if restoreAvailable {
		updates["available_cents"] = gorm.Expr("available_cents + ?", payout.AmountCents)
	}
	return tx.Model(account).Updates(updates).Error
}

func validAffiliatePayoutAccount(method string, name string, account string) bool {
	if method != AffiliatePayoutMethodAlipay {
		return false
	}
	return name != "" && len(name) <= 128 && account != "" && len(account) <= 255
}

func affiliatePayoutLast4(account string) string {
	runes := []rune(account)
	if len(runes) <= 4 {
		return string(runes)
	}
	return string(runes[len(runes)-4:])
}

func affiliatePayoutLocation() *time.Location {
	return time.FixedZone("Asia/Shanghai", 8*60*60)
}

func IsAffiliatePayoutSettlementDay(now time.Time) bool {
	return now.In(affiliatePayoutLocation()).Day() == AffiliatePayoutSettlementDay
}

func affiliatePayoutSettlementDue(payout *AffiliatePayout, now time.Time) bool {
	return payout != nil &&
		IsAffiliatePayoutSettlementDay(now) &&
		now.Unix() >= payout.EligibleSettlementTime
}

func nextAffiliatePayoutSettlementTime(now time.Time) int64 {
	local := now.In(affiliatePayoutLocation())
	year, month := local.Year(), local.Month()
	if local.Day() > AffiliatePayoutSettlementDay {
		month++
		if month > time.December {
			year++
			month = time.January
		}
	}
	return time.Date(year, month, AffiliatePayoutSettlementDay, 0, 0, 0, 0, affiliatePayoutLocation()).Unix()
}

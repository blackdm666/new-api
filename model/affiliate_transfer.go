package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	AffiliateTransferStatusPending   = 1
	AffiliateTransferStatusApproved  = 2
	AffiliateTransferStatusRejected  = 3
	AffiliateTransferStatusCancelled = 4

	// AffiliateTransferAutoApproveOptionKey decides whether converting
	// commission to API balance is credited at once or waits for an
	// administrator. Commission itself is always credited automatically.
	AffiliateTransferAutoApproveOptionKey = "AffiliateTransferAutoApprove"
)

var (
	ErrAffiliateTransferAmountTooSmall          = errors.New("affiliate transfer amount is too small")
	ErrAffiliateTransferInsufficientBalance     = errors.New("affiliate transfer balance is insufficient")
	ErrAffiliateTransferRequestIdInvalid        = errors.New("affiliate transfer request id is invalid")
	ErrAffiliateTransferRequestConflict         = errors.New("affiliate transfer request id conflicts with another transfer")
	ErrAffiliateTransferNotFound                = errors.New("affiliate transfer not found")
	ErrAffiliateTransferStatusInvalid           = errors.New("affiliate transfer status invalid")
	ErrAffiliateTransferForbidden               = errors.New("affiliate transfer does not belong to the user")
	ErrAffiliateTransferRejectionReasonRequired = errors.New("affiliate transfer rejection reason is required")
)

// AffiliateTransfer is the audit record for commission converted to API
// balance. It is intentionally separate from top-up orders because this
// operation must never generate another referral commission. A pending
// transfer keeps its amount in AffiliateAccount.FrozenCents until review;
// QuotaBefore/QuotaAfter are filled when the API balance is actually credited.
type AffiliateTransfer struct {
	Id                 int                  `json:"id"`
	UserId             int                  `json:"user_id" gorm:"not null;index;index:idx_affiliate_transfer_user_created,priority:1"`
	RequestId          string               `json:"request_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	AmountCents        int64                `json:"amount_cents" gorm:"bigint;not null;default:0"`
	AmountQuota        int                  `json:"amount_quota" gorm:"not null"`
	BalanceCentsBefore int64                `json:"balance_cents_before" gorm:"bigint;not null;default:0"`
	BalanceCentsAfter  int64                `json:"balance_cents_after" gorm:"bigint;not null;default:0"`
	QuotaBefore        int                  `json:"quota_before" gorm:"not null"`
	QuotaAfter         int                  `json:"quota_after" gorm:"not null"`
	Status             int                  `json:"status" gorm:"not null;default:2;index"`
	OperatorId         int                  `json:"operator_id" gorm:"not null;default:0"`
	ReviewedTime       int64                `json:"reviewed_time" gorm:"bigint;not null;default:0"`
	RejectReason       string               `json:"reject_reason" gorm:"type:varchar(255);default:''"`
	RiskScore          int                  `json:"risk_score" gorm:"not null;default:0"`
	RiskDetail         string               `json:"-" gorm:"type:text"`
	RiskEvaluatedTime  int64                `json:"risk_evaluated_time" gorm:"bigint;not null;default:0"`
	CreatedTime        int64                `json:"created_time" gorm:"bigint;autoCreateTime;index;index:idx_affiliate_transfer_user_created,priority:2"`
	Username           string               `json:"username" gorm:"->;-:migration"`
	DisplayName        string               `json:"display_name" gorm:"->;-:migration"`
	Remark             string               `json:"remark,omitempty" gorm:"->;-:migration"`
	Risk               *AffiliateRiskDetail `json:"risk,omitempty" gorm:"-"`
}

type AffiliateTransferQueryOptions struct {
	IncludeAdminRemarks bool
	UserId              int
	Status              int
	Keyword             string
}

func affiliateTransferAutoApprove() bool {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[AffiliateTransferAutoApproveOptionKey]
	common.OptionMapRWMutex.RUnlock()
	return raw != "false"
}

// TransferLegacyAffQuotaToQuota preserves NewAPI's original fixed invitation
// reward behavior. Legacy aff_quota remains API credit and is deliberately
// excluded from the withdrawable cash commission ledger.
func TransferLegacyAffQuotaToQuota(userId int, quota int) error {
	if userId <= 0 || quota <= 0 {
		return ErrAffiliateTransferAmountTooSmall
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		user := &User{}
		if err := lockForUpdate(tx).Where("id = ?", userId).First(user).Error; err != nil {
			return err
		}
		if user.AffQuota < quota {
			return ErrAffiliateTransferInsufficientBalance
		}
		if err := increaseUserQuotaTx(tx, userId, quota); err != nil {
			return err
		}
		return tx.Model(user).Update("aff_quota", user.AffQuota-quota).Error
	})
}

func (user *User) TransferAffQuotaToQuota(quota int) error {
	_, err := user.TransferAffQuotaToQuotaWithRequestId(quota, common.NewRequestId())
	return err
}

func (user *User) TransferAffQuotaToQuotaWithRequestId(quota int, requestId string) (*AffiliateTransfer, error) {
	amountCents := affiliateQuotaToCents(quota)
	normalizedQuota, conversionErr := affiliateCentsToQuota(amountCents)
	if conversionErr != nil || amountCents < 100 || normalizedQuota != quota {
		return nil, ErrAffiliateTransferAmountTooSmall
	}
	return user.TransferAffiliateCentsToQuotaWithRequestId(amountCents, requestId)
}

func (user *User) TransferAffiliateCentsToQuotaWithRequestId(amountCents int64, requestId string) (*AffiliateTransfer, error) {
	requestId = strings.TrimSpace(requestId)
	if user == nil || user.Id <= 0 || requestId == "" || len(requestId) > 64 {
		return nil, ErrAffiliateTransferRequestIdInvalid
	}
	if amountCents < 100 {
		return nil, ErrAffiliateTransferAmountTooSmall
	}
	quota, conversionErr := affiliateCentsToQuota(amountCents)
	if conversionErr != nil || quota <= 0 {
		return nil, ErrAffiliateTransferAmountTooSmall
	}

	autoApprove := affiliateTransferAutoApprove()
	record := &AffiliateTransfer{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		lockedUser := &User{}
		if err := lockForUpdate(tx).Where("id = ?", user.Id).First(lockedUser).Error; err != nil {
			return err
		}
		account, err := ensureAffiliateAccountTx(tx, user.Id)
		if err != nil {
			return err
		}

		existing := &AffiliateTransfer{}
		result := tx.Where("request_id = ?", requestId).Limit(1).Find(existing)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			if existing.UserId != user.Id || existing.AmountCents != amountCents || existing.AmountQuota != quota {
				return ErrAffiliateTransferRequestConflict
			}
			*record = *existing
			*user = *lockedUser
			return nil
		}

		if account.AvailableCents < amountCents {
			return ErrAffiliateTransferInsufficientBalance
		}
		if lockedUser.Quota > common.MaxWalletQuota-quota {
			return ErrWalletQuotaLimitExceeded
		}
		*record = AffiliateTransfer{
			UserId:             lockedUser.Id,
			RequestId:          requestId,
			AmountCents:        amountCents,
			AmountQuota:        quota,
			BalanceCentsBefore: account.AvailableCents,
			BalanceCentsAfter:  account.AvailableCents - amountCents,
			QuotaBefore:        lockedUser.Quota,
			QuotaAfter:         lockedUser.Quota,
			Status:             AffiliateTransferStatusPending,
		}
		if !autoApprove {
			if err := tx.Model(account).Updates(map[string]any{
				"available_cents": gorm.Expr("available_cents - ?", amountCents),
				"frozen_cents":    gorm.Expr("frozen_cents + ?", amountCents),
			}).Error; err != nil {
				return err
			}
			if err := tx.Create(record).Error; err != nil {
				return err
			}
			*user = *lockedUser
			return nil
		}
		record.Status = AffiliateTransferStatusApproved
		record.ReviewedTime = common.GetTimestamp()
		record.QuotaAfter = lockedUser.Quota + quota
		if err := tx.Model(account).Update("available_cents", record.BalanceCentsAfter).Error; err != nil {
			return err
		}
		if err := increaseUserQuotaTx(tx, lockedUser.Id, quota); err != nil {
			return err
		}
		if err := tx.Create(record).Error; err != nil {
			return err
		}
		lockedUser.Quota = record.QuotaAfter
		*user = *lockedUser
		return nil
	})
	if err != nil {
		return nil, err
	}
	return record, nil
}

func ListAffiliateTransfers(options AffiliateTransferQueryOptions, pageInfo *common.PageInfo) ([]*AffiliateTransfer, int64, error) {
	query := DB.Model(&AffiliateTransfer{}).
		Select("affiliate_transfers.*, users.username, users.display_name").
		Joins("LEFT JOIN users ON users.id = affiliate_transfers.user_id")
	if options.IncludeAdminRemarks {
		query = query.Select("affiliate_transfers.*, users.username, users.display_name, users.remark")
	}
	if options.UserId > 0 {
		query = query.Where("affiliate_transfers.user_id = ?", options.UserId)
	}
	if options.Status > 0 {
		query = query.Where("affiliate_transfers.status = ?", options.Status)
	}
	if keyword := strings.TrimSpace(options.Keyword); keyword != "" {
		pattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		if id, err := strconv.Atoi(keyword); err == nil && id > 0 {
			query = query.Where(
				"(affiliate_transfers.request_id LIKE ? ESCAPE '!' OR users.username LIKE ? ESCAPE '!' OR users.display_name LIKE ? ESCAPE '!' OR affiliate_transfers.id = ? OR affiliate_transfers.user_id = ?)",
				pattern, pattern, pattern, id, id,
			)
		} else {
			query = query.Where(
				"(affiliate_transfers.request_id LIKE ? ESCAPE '!' OR users.username LIKE ? ESCAPE '!' OR users.display_name LIKE ? ESCAPE '!')",
				pattern, pattern, pattern,
			)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	records := []*AffiliateTransfer{}
	if err := query.Order("affiliate_transfers.id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	for _, record := range records {
		// Risk evidence names other accounts and IPs, so only reviewers get it.
		if !options.IncludeAdminRemarks {
			record.RiskScore = 0
			record.RiskEvaluatedTime = 0
			continue
		}
		if record.RiskDetail == "" {
			continue
		}
		detail := &AffiliateRiskDetail{}
		if err := common.UnmarshalJsonStr(record.RiskDetail, detail); err == nil {
			record.Risk = detail
		}
	}
	return records, total, nil
}

func lockAffiliateTransfer(tx *gorm.DB, id int) (*AffiliateTransfer, error) {
	transfer := &AffiliateTransfer{}
	if err := lockForUpdate(tx).Where("id = ?", id).First(transfer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAffiliateTransferNotFound
		}
		return nil, err
	}
	return transfer, nil
}

// releaseAffiliateTransferFrozenTx takes a pending transfer's amount out of
// the frozen balance. restoreAvailable returns it to the available balance
// (reject / cancel); otherwise it leaves the commission ledger (approve).
func releaseAffiliateTransferFrozenTx(tx *gorm.DB, transfer *AffiliateTransfer, restoreAvailable bool) error {
	account, err := ensureAffiliateAccountTx(tx, transfer.UserId)
	if err != nil {
		return err
	}
	if account.FrozenCents < transfer.AmountCents {
		return ErrAffiliateTransferStatusInvalid
	}
	updates := map[string]any{
		"frozen_cents": gorm.Expr("frozen_cents - ?", transfer.AmountCents),
	}
	if restoreAvailable {
		updates["available_cents"] = gorm.Expr("available_cents + ?", transfer.AmountCents)
	}
	return tx.Model(account).Updates(updates).Error
}

// ReviewAffiliateTransfer settles a pending transfer. Approval credits the
// API balance with the quota fixed at request time; rejection returns the
// frozen commission to the available balance.
func ReviewAffiliateTransfer(id int, operatorId int, approve bool, reason string) (*AffiliateTransfer, error) {
	reason = strings.TrimSpace(reason)
	if !approve && reason == "" {
		return nil, ErrAffiliateTransferRejectionReasonRequired
	}
	transfer := &AffiliateTransfer{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		locked, err := lockAffiliateTransfer(tx, id)
		if err != nil {
			return err
		}
		if locked.Status != AffiliateTransferStatusPending {
			return ErrAffiliateTransferStatusInvalid
		}
		updates := map[string]any{
			"operator_id":   operatorId,
			"reviewed_time": common.GetTimestamp(),
		}
		if !approve {
			if err := releaseAffiliateTransferFrozenTx(tx, locked, true); err != nil {
				return err
			}
			updates["status"] = AffiliateTransferStatusRejected
			updates["reject_reason"] = reason
			return tx.Model(locked).Updates(updates).Error
		}
		user := &User{}
		if err := lockForUpdate(tx).Where("id = ?", locked.UserId).First(user).Error; err != nil {
			return err
		}
		if user.Quota > common.MaxWalletQuota-locked.AmountQuota {
			return ErrWalletQuotaLimitExceeded
		}
		if err := releaseAffiliateTransferFrozenTx(tx, locked, false); err != nil {
			return err
		}
		if err := increaseUserQuotaTx(tx, user.Id, locked.AmountQuota); err != nil {
			return err
		}
		updates["status"] = AffiliateTransferStatusApproved
		updates["quota_before"] = user.Quota
		updates["quota_after"] = user.Quota + locked.AmountQuota
		return tx.Model(locked).Updates(updates).Error
	})
	if err != nil {
		return nil, err
	}
	if err := DB.First(transfer, id).Error; err != nil {
		return nil, err
	}
	if approve {
		if err := cacheIncrUserQuota(transfer.UserId, int64(transfer.AmountQuota)); err != nil {
			_ = invalidateUserCache(transfer.UserId)
		}
	}
	return transfer, nil
}

// CancelAffiliateTransfer lets the requester withdraw a transfer that is still
// waiting for review.
func CancelAffiliateTransfer(id int, userId int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		transfer, err := lockAffiliateTransfer(tx, id)
		if err != nil {
			return err
		}
		if transfer.UserId != userId {
			return ErrAffiliateTransferForbidden
		}
		if transfer.Status != AffiliateTransferStatusPending {
			return ErrAffiliateTransferStatusInvalid
		}
		if err := releaseAffiliateTransferFrozenTx(tx, transfer, true); err != nil {
			return err
		}
		return tx.Model(transfer).Updates(map[string]any{
			"status":        AffiliateTransferStatusCancelled,
			"reviewed_time": common.GetTimestamp(),
		}).Error
	})
}

type AffiliateTransferPendingSummary struct {
	Count int64 `json:"count"`
	Cents int64 `json:"cents"`
}

func GetAffiliateTransferPendingSummary(userId int) (AffiliateTransferPendingSummary, error) {
	summary := AffiliateTransferPendingSummary{}
	query := DB.Model(&AffiliateTransfer{}).Where("status = ?", AffiliateTransferStatusPending)
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	err := query.Select("COUNT(*) AS count, COALESCE(SUM(amount_cents), 0) AS cents").Scan(&summary).Error
	return summary, err
}

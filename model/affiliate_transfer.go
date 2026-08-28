package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var (
	ErrAffiliateTransferAmountTooSmall      = errors.New("affiliate transfer amount is too small")
	ErrAffiliateTransferInsufficientBalance = errors.New("affiliate transfer balance is insufficient")
	ErrAffiliateTransferRequestIdInvalid    = errors.New("affiliate transfer request id is invalid")
	ErrAffiliateTransferRequestConflict     = errors.New("affiliate transfer request id conflicts with another transfer")
)

// AffiliateTransfer is the immutable audit record for commission converted to
// API balance. It is intentionally separate from top-up orders because this
// operation must never generate another referral commission.
type AffiliateTransfer struct {
	Id                 int    `json:"id"`
	UserId             int    `json:"user_id" gorm:"not null;index;index:idx_affiliate_transfer_user_created,priority:1"`
	RequestId          string `json:"request_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	AmountCents        int64  `json:"amount_cents" gorm:"bigint;not null;default:0"`
	AmountQuota        int    `json:"amount_quota" gorm:"not null"`
	BalanceCentsBefore int64  `json:"balance_cents_before" gorm:"bigint;not null;default:0"`
	BalanceCentsAfter  int64  `json:"balance_cents_after" gorm:"bigint;not null;default:0"`
	QuotaBefore        int    `json:"quota_before" gorm:"not null"`
	QuotaAfter         int    `json:"quota_after" gorm:"not null"`
	CreatedTime        int64  `json:"created_time" gorm:"bigint;autoCreateTime;index;index:idx_affiliate_transfer_user_created,priority:2"`
	Username           string `json:"username" gorm:"->;-:migration"`
	DisplayName        string `json:"display_name" gorm:"->;-:migration"`
}

type AffiliateTransferQueryOptions struct {
	UserId  int
	Keyword string
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
			QuotaAfter:         lockedUser.Quota + quota,
		}
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
	if options.UserId > 0 {
		query = query.Where("affiliate_transfers.user_id = ?", options.UserId)
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
	return records, total, nil
}

package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AffiliateAccount is the authoritative CNY cash ledger for the referral
// program. It must stay separate from users.aff_quota, which belongs to the
// legacy fixed-invitation reward feature and represents API quota rather than
// withdrawable cash.
type AffiliateAccount struct {
	UserId              int   `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	AvailableCents      int64 `json:"available_cents" gorm:"bigint;not null;default:0"`
	FrozenCents         int64 `json:"frozen_cents" gorm:"bigint;not null;default:0"`
	LifetimeEarnedCents int64 `json:"lifetime_earned_cents" gorm:"bigint;not null;default:0"`
	CreatedTime         int64 `json:"created_time" gorm:"bigint;autoCreateTime"`
	UpdatedTime         int64 `json:"updated_time" gorm:"bigint;autoUpdateTime"`
}

func affiliateCentsToQuota(cents int64) (int, error) {
	if cents <= 0 || common.QuotaPerUnit <= 0 {
		return 0, nil
	}
	return common.QuotaFromDecimalStrict(
		decimal.NewFromInt(cents).
			Div(decimal.NewFromInt(100)).
			Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
	)
}

func affiliateQuotaToCents(quota int) int64 {
	if quota <= 0 || common.QuotaPerUnit <= 0 {
		return 0
	}
	return decimal.NewFromInt(int64(quota)).
		Div(decimal.NewFromFloat(common.QuotaPerUnit)).
		Mul(decimal.NewFromInt(100)).
		Round(0).
		IntPart()
}

func ensureAffiliateAccountTx(tx *gorm.DB, userId int) (*AffiliateAccount, error) {
	if tx == nil || userId <= 0 {
		return nil, gorm.ErrInvalidData
	}
	seed := &AffiliateAccount{UserId: userId}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(seed).Error; err != nil {
		return nil, err
	}
	account := &AffiliateAccount{}
	if err := lockForUpdate(tx).Where("user_id = ?", userId).First(account).Error; err != nil {
		return nil, err
	}
	return account, nil
}

func creditAffiliateAccountTx(tx *gorm.DB, userId int, cents int64) error {
	if cents <= 0 {
		return nil
	}
	account, err := ensureAffiliateAccountTx(tx, userId)
	if err != nil {
		return err
	}
	return tx.Model(account).Updates(map[string]any{
		"available_cents":       gorm.Expr("available_cents + ?", cents),
		"lifetime_earned_cents": gorm.Expr("lifetime_earned_cents + ?", cents),
	}).Error
}

func GetAffiliateAccount(userId int) (*AffiliateAccount, error) {
	if userId <= 0 {
		return nil, gorm.ErrInvalidData
	}
	account := &AffiliateAccount{}
	result := DB.Where("user_id = ?", userId).Limit(1).Find(account)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return &AffiliateAccount{UserId: userId}, nil
	}
	return account, nil
}

// populateAffiliateLifetimeEarningsTx attaches the authoritative cumulative
// cash commission to a page of users with one query. Legacy users.aff_history
// is API quota and must never be presented as referral cash commission.
func populateAffiliateLifetimeEarningsTx(tx *gorm.DB, users []*User) error {
	if tx == nil || len(users) == 0 {
		return nil
	}

	userIds := make([]int, 0, len(users))
	for _, user := range users {
		if user != nil && user.Id > 0 {
			userIds = append(userIds, user.Id)
		}
	}
	if len(userIds) == 0 {
		return nil
	}

	accounts := make([]AffiliateAccount, 0, len(userIds))
	if err := tx.Select("user_id", "lifetime_earned_cents").
		Where("user_id IN ?", userIds).
		Find(&accounts).Error; err != nil {
		return err
	}

	lifetimeByUser := make(map[int]int64, len(accounts))
	for _, account := range accounts {
		lifetimeByUser[account.UserId] = account.LifetimeEarnedCents
	}
	for _, user := range users {
		if user != nil {
			user.AffEarnedCents = lifetimeByUser[user.Id]
		}
	}
	return nil
}

// backfillAffiliateAccounts migrates preview/early-build referral data into
// the independent cash ledger. It never imports users.aff_quota, because that
// column contains legacy API rewards and is intentionally outside this ledger.
func backfillAffiliateAccounts() error {
	if !DB.Migrator().HasTable(&AffiliateCommission{}) {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if tx.Migrator().HasTable(&AffiliateTransfer{}) && tx.Migrator().HasColumn(&AffiliateTransfer{}, "AmountCents") {
			var transfers []*AffiliateTransfer
			if err := tx.Where("amount_cents = 0 AND amount_quota > 0").Find(&transfers).Error; err != nil {
				return err
			}
			for _, transfer := range transfers {
				if err := tx.Model(transfer).Update("amount_cents", affiliateQuotaToCents(transfer.AmountQuota)).Error; err != nil {
					return err
				}
			}
		}

		userIds := []int{}
		if err := tx.Model(&AffiliateCommission{}).
			Where("status = ?", AffiliateCommissionStatusApproved).
			Distinct("inviter_id").
			Pluck("inviter_id", &userIds).Error; err != nil {
			return err
		}
		for _, userId := range userIds {
			var existing int64
			if err := tx.Model(&AffiliateAccount{}).Where("user_id = ?", userId).Count(&existing).Error; err != nil {
				return err
			}
			if existing > 0 {
				continue
			}
			var lifetime int64
			if err := tx.Model(&AffiliateCommission{}).
				Where("inviter_id = ? AND status = ?", userId, AffiliateCommissionStatusApproved).
				Select("COALESCE(SUM(commission_cents), 0)").
				Scan(&lifetime).Error; err != nil {
				return err
			}
			var transferred int64
			if tx.Migrator().HasTable(&AffiliateTransfer{}) {
				if err := tx.Model(&AffiliateTransfer{}).
					Where("user_id = ?", userId).
					Select("COALESCE(SUM(amount_cents), 0)").
					Scan(&transferred).Error; err != nil {
					return err
				}
			}
			var paidOrReserved int64
			var frozen int64
			if tx.Migrator().HasTable(&AffiliatePayout{}) {
				if err := tx.Model(&AffiliatePayout{}).
					Where("user_id = ? AND status IN (?, ?, ?, ?)", userId, AffiliatePayoutStatusPending, AffiliatePayoutStatusApproved, AffiliatePayoutStatusPaid, AffiliatePayoutStatusProcessing).
					Select("COALESCE(SUM(amount_cents), 0)").
					Scan(&paidOrReserved).Error; err != nil {
					return err
				}
				if err := tx.Model(&AffiliatePayout{}).
					Where("user_id = ? AND status IN (?, ?, ?)", userId, AffiliatePayoutStatusPending, AffiliatePayoutStatusApproved, AffiliatePayoutStatusProcessing).
					Select("COALESCE(SUM(amount_cents), 0)").
					Scan(&frozen).Error; err != nil {
					return err
				}
			}
			available := lifetime - transferred - paidOrReserved
			if available < 0 {
				return fmt.Errorf("affiliate account backfill is inconsistent for user %d", userId)
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&AffiliateAccount{
				UserId:              userId,
				AvailableCents:      available,
				FrozenCents:         frozen,
				LifetimeEarnedCents: lifetime,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	QuotaNotificationSourceWallet       = "wallet"
	QuotaNotificationSourceSubscription = "subscription"
)

// QuotaNotificationState records whether one funding source is currently
// below its warning threshold. A notification is claimed only on the
// above-to-below transition; sustained low balance does not create more mail.
type QuotaNotificationState struct {
	UserId         int    `gorm:"primaryKey;autoIncrement:false"`
	Source         string `gorm:"type:varchar(24);primaryKey"`
	SourceId       int64  `gorm:"primaryKey;autoIncrement:false"`
	BelowThreshold bool
	Version        int64 `gorm:"not null"`
	Threshold      int64 `gorm:"not null"`
	LastRemaining  int64 `gorm:"not null"`
	NotifiedTime   int64 `gorm:"not null"`
	UpdatedTime    int64 `gorm:"not null;index"`
}

func ClaimQuotaWarning(userId int, source string, sourceId int64, threshold int64, remaining int64) (bool, int64, error) {
	if userId <= 0 || source == "" || sourceId < 0 || threshold <= 0 {
		return false, 0, fmt.Errorf("invalid quota notification state")
	}
	now := common.GetTimestamp()
	claimed := false
	version := int64(0)
	err := DB.Transaction(func(tx *gorm.DB) error {
		seed := &QuotaNotificationState{
			UserId:        userId,
			Source:        source,
			SourceId:      sourceId,
			Threshold:     threshold,
			LastRemaining: remaining,
			UpdatedTime:   now,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(seed).Error; err != nil {
			return err
		}

		state := &QuotaNotificationState{}
		if err := lockForUpdate(tx).Where("user_id = ? AND source = ? AND source_id = ?", userId, source, sourceId).First(state).Error; err != nil {
			return err
		}
		if remaining >= threshold {
			return tx.Model(state).Updates(map[string]any{
				"below_threshold": false,
				"threshold":       threshold,
				"last_remaining":  remaining,
				"updated_time":    now,
			}).Error
		}
		if state.BelowThreshold {
			return tx.Model(state).Updates(map[string]any{
				"threshold":      threshold,
				"last_remaining": remaining,
				"updated_time":   now,
			}).Error
		}

		version = state.Version + 1
		claimed = true
		return tx.Model(state).Updates(map[string]any{
			"below_threshold": true,
			"version":         version,
			"threshold":       threshold,
			"last_remaining":  remaining,
			"notified_time":   now,
			"updated_time":    now,
		}).Error
	})
	return claimed, version, err
}

func QuotaWarningDeliveryKey(userId int, source string, sourceId int64, version int64) string {
	return fmt.Sprintf("quota-warning:%d:%s:%d:%d", userId, source, sourceId, version)
}

func ReleaseQuotaWarningClaim(userId int, source string, sourceId int64, version int64) error {
	return DB.Model(&QuotaNotificationState{}).
		Where("user_id = ? AND source = ? AND source_id = ? AND version = ?", userId, source, sourceId, version).
		Updates(map[string]any{"below_threshold": false, "updated_time": common.GetTimestamp()}).Error
}

package model

// 88api topup receipt: queries behind the emailed top-up receipts.

import "github.com/QuantumNous/new-api/common"

// ListTopUpsCompletedSince returns successful top-ups completed at or after
// since, oldest first.
func ListTopUpsCompletedSince(since int64, limit int) ([]*TopUp, error) {
	var topUps []*TopUp
	err := DB.Where("status = ? AND complete_time >= ?", common.TopUpStatusSuccess, since).
		Order("complete_time asc").Order("id asc").Limit(limit).Find(&topUps).Error
	return topUps, err
}

// EmailDeliveryRelatedIds reports which of relatedIds already have an email
// of category in the outbox.
func EmailDeliveryRelatedIds(category string, relatedIds []int) (map[int]bool, error) {
	found := make(map[int]bool, len(relatedIds))
	if len(relatedIds) == 0 {
		return found, nil
	}
	var ids []int
	if err := DB.Model(&EmailDelivery{}).Where("category = ? AND related_id IN ?", category, relatedIds).
		Distinct("related_id").Pluck("related_id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		found[id] = true
	}
	return found, nil
}

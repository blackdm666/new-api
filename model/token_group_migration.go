package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrTokenGroupMigrationConflict = errors.New("token group references changed; preview again")

type TokenGroupMigrationItem struct {
	ID         int      `json:"id"`
	UserID     int      `json:"user_id"`
	Name       string   `json:"name"`
	Group      string   `json:"group"`
	AutoGroups []string `json:"auto_groups"`
}

type TokenGroupMigrationPlan struct {
	From    string                    `json:"from"`
	To      string                    `json:"to"`
	Version string                    `json:"version"`
	Items   []TokenGroupMigrationItem `json:"items"`
}

func PreviewTokenGroupMigration(from, to string) (*TokenGroupMigrationPlan, error) {
	return tokenGroupMigrationPlan(DB, from, to, false)
}

func tokenGroupMigrationPlan(db *gorm.DB, from, to string, locked bool) (*TokenGroupMigrationPlan, error) {
	if strings.TrimSpace(from) != from || strings.TrimSpace(to) != to || from == "" || to == "" ||
		from == to || from == "auto" || to == "auto" || len(from) > 128 || len(to) > 128 {
		return nil, errors.New("invalid source or destination group")
	}
	query := db.Select("id", "user_id", "name", "group", "auto_groups").
		Where(clause.Or(clause.Eq{Column: "group", Value: from}, clause.Neq{Column: "auto_groups", Value: ""})).Order("id")
	if locked {
		query = lockForUpdate(query)
	}
	var tokens []Token
	if err := query.Find(&tokens).Error; err != nil {
		return nil, err
	}
	plan := &TokenGroupMigrationPlan{From: from, To: to, Items: []TokenGroupMigrationItem{}}
	for _, token := range tokens {
		groups, err := token.GetAutoGroups()
		if err != nil {
			return nil, fmt.Errorf("invalid auto groups for token %d", token.Id)
		}
		if token.Group != from && !slices.Contains(groups, from) {
			continue
		}
		plan.Items = append(plan.Items, TokenGroupMigrationItem{token.Id, token.UserId, token.Name, token.Group, groups})
	}
	encoded, err := common.Marshal(plan)
	if err != nil {
		return nil, err
	}
	plan.Version = fmt.Sprintf("%x", sha256.Sum256(encoded))
	return plan, nil
}

// MigrateTokenGroup changes only group references. Concurrent quota consumption,
// credentials, expiry, status and user-selected limits are never overwritten.
func MigrateTokenGroup(from, to, version string, authorize func(int) error) (*TokenGroupMigrationPlan, error) {
	if version == "" || authorize == nil {
		return nil, errors.New("preview version and destination authorization required")
	}
	var applied *TokenGroupMigrationPlan
	err := DB.Transaction(func(tx *gorm.DB) error {
		plan, err := tokenGroupMigrationPlan(tx, from, to, true)
		if err != nil {
			return err
		}
		if plan.Version != version {
			return ErrTokenGroupMigrationConflict
		}
		for _, item := range plan.Items {
			if err := authorize(item.UserID); err != nil {
				return err
			}
		}
		for _, item := range plan.Items {
			var token Token
			if err := lockForUpdate(tx).First(&token, item.ID).Error; err != nil {
				return err
			}
			// Refuse the mutation if the cache fence cannot be established.
			if err := invalidateTokenCacheForMutation(token.Key); err != nil {
				return errors.New("token cache invalidation failed; no changes committed")
			}
			updates := map[string]any{}
			if item.Group == from {
				updates["group"] = to
			}
			if slices.Contains(item.AutoGroups, from) {
				groups := make([]string, 0, len(item.AutoGroups))
				for _, group := range item.AutoGroups {
					if group == from {
						group = to
					}
					if !slices.Contains(groups, group) {
						groups = append(groups, group)
					}
				}
				if err := token.SetAutoGroups(groups); err != nil {
					return err
				}
				updates["auto_groups"] = token.AutoGroups
			}
			if err := tx.Model(&Token{}).Where("id = ?", item.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		applied = plan
		return nil
	})
	return applied, err
}

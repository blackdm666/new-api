package controller

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// MigrateTokenGroupReferences is root-only and requires an unchanged preview
// before atomically moving existing token references to an authorized group.
func MigrateTokenGroupReferences(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "root access required"})
		return
	}
	var request struct {
		From            string `json:"from"`
		To              string `json:"to"`
		Apply           bool   `json:"apply"`
		ExpectedVersion string `json:"expected_version"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	if !ratio_setting.ContainsGroupRatio(request.To) {
		common.ApiErrorMsg(c, "destination group does not exist")
		return
	}
	authorize := func(userID int) error {
		var user model.User
		if err := model.DB.Select("id", "group").First(&user, userID).Error; err != nil {
			return err
		}
		if !service.IsUserSelectableGroup(user.Group, request.To) {
			return fmt.Errorf("destination group is unavailable to user %d", userID)
		}
		return nil
	}
	var plan *model.TokenGroupMigrationPlan
	var err error
	if request.Apply {
		plan, err = model.MigrateTokenGroup(request.From, request.To, request.ExpectedVersion, authorize)
	} else {
		plan, err = model.PreviewTokenGroupMigration(request.From, request.To)
		if err == nil {
			for _, item := range plan.Items {
				if err = authorize(item.UserID); err != nil {
					break
				}
			}
		}
	}
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrTokenGroupMigrationConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	if request.Apply {
		ids := make([]int, 0, len(plan.Items))
		for _, item := range plan.Items {
			ids = append(ids, item.ID)
		}
		recordManageAudit(c, "token.group.migrate", map[string]any{
			"from": request.From, "to": request.To, "token_ids": ids, "count": len(ids),
		})
	}
	common.ApiSuccess(c, plan)
}

package controller

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestTokenGroupMigrationDatabaseMatrix(t *testing.T) {
	for _, dialect := range []common.DatabaseType{common.DatabaseTypeSQLite, common.DatabaseTypeMySQL, common.DatabaseTypePostgreSQL} {
		t.Run(string(dialect), func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case common.DatabaseTypeSQLite:
				driver = sqlite.Open(fmt.Sprintf("file:migrate-groups-%d?mode=memory&cache=shared", time.Now().UnixNano()))
			case common.DatabaseTypeMySQL:
				if os.Getenv("TEST_MYSQL_DSN") == "" {
					t.Skip("TEST_MYSQL_DSN missing")
				}
				driver = mysql.Open(os.Getenv("TEST_MYSQL_DSN"))
			case common.DatabaseTypePostgreSQL:
				if os.Getenv("TEST_POSTGRES_DSN") == "" {
					t.Skip("TEST_POSTGRES_DSN missing")
				}
				driver = postgres.Open(os.Getenv("TEST_POSTGRES_DSN"))
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			priorDB, priorLog, priorRedis := model.DB, model.LOG_DB, common.RedisEnabled
			priorType, priorLogType := common.MainDatabaseType(), common.LogDatabaseType()
			model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
			common.SetDatabaseTypes(dialect, dialect)
			t.Cleanup(func() {
				model.DB, model.LOG_DB, common.RedisEnabled = priorDB, priorLog, priorRedis
				common.SetDatabaseTypes(priorType, priorLogType)
				conn, err := db.DB()
				require.NoError(t, err)
				require.NoError(t, conn.Close())
			})
			require.NoError(t, db.AutoMigrate(&model.Token{}, &model.User{}))
			var version string
			query := "SELECT version()"
			if dialect == common.DatabaseTypeSQLite {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Log("database", version)
			from, to := fmt.Sprintf("retired-%d", time.Now().UnixNano()), "destination-group"
			configureTokenGroupAuthorizationTest(t, `{"destination-group":"Destination"}`, `{"destination-group":0.5}`, nil)
			owner := model.User{Username: from, Password: "unused-test-password", Group: "default", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
			require.NoError(t, db.Create(&owner).Error)
			t.Cleanup(func() { require.NoError(t, db.Unscoped().Delete(&model.User{}, owner.Id).Error) })
			ip := "192.0.2.1"
			fixed := model.Token{UserId: 42, Name: "fixed", Key: from + "-fixed-secret", Group: from, Status: 2,
				RemainQuota: 500, UsedQuota: 35, ExpiredTime: 12345, ModelLimitsEnabled: true, ModelLimits: "qwen-test", AllowIps: &ip}
			auto := model.Token{UserId: 43, Name: "auto", Key: from + "-auto-secret", Group: "auto", CrossGroupRetry: true, Status: 1}
			require.NoError(t, auto.SetAutoGroups([]string{"first", from, to, "last"}))
			deleted := model.Token{UserId: 44, Name: "deleted", Key: from + "-deleted-secret", Group: from, Status: 1}
			for _, token := range []*model.Token{&fixed, &auto, &deleted} {
				require.NoError(t, db.Create(token).Error)
			}
			t.Cleanup(func() {
				require.NoError(t, db.Unscoped().Delete(&model.Token{}, []int{fixed.Id, auto.Id, deleted.Id}).Error)
			})
			require.NoError(t, db.Delete(&deleted).Error)
			plan, err := model.PreviewTokenGroupMigration(from, to)
			require.NoError(t, err)
			require.Len(t, plan.Items, 2)
			encoded, err := common.Marshal(plan)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "secret")
			for _, role := range []int{0, common.RoleCommonUser, common.RoleAdminUser} {
				ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/group/migrate", map[string]any{"from": from, "to": to, "apply": true, "expected_version": plan.Version}, 1)
				ctx.Set("role", role)
				MigrateTokenGroupReferences(ctx)
				assert.Equal(t, http.StatusForbidden, recorder.Code)
			}
			// A real root preview validates each owner's available groups without
			// returning usable credentials. Missing owners must be rejected.
			ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/group/migrate", map[string]any{"from": from, "to": to}, 1)
			ctx.Set("role", common.RoleRootUser)
			MigrateTokenGroupReferences(ctx)
			assert.False(t, decodeAPIResponse(t, recorder).Success)
			require.NoError(t, db.Model(&model.Token{}).Where("id IN ?", []int{fixed.Id, auto.Id}).Update("user_id", owner.Id).Error)
			fixed.UserId, auto.UserId = owner.Id, owner.Id
			ctx, recorder = newAuthenticatedContext(t, http.MethodPost, "/api/token/group/migrate", map[string]any{"from": from, "to": to}, 1)
			ctx.Set("role", common.RoleRootUser)
			MigrateTokenGroupReferences(ctx)
			response := decodeAPIResponse(t, recorder)
			require.True(t, response.Success, recorder.Body.String())
			require.NoError(t, common.Unmarshal(response.Data, &plan))
			assert.NotContains(t, recorder.Body.String(), "secret")
			_, err = model.MigrateTokenGroup(from, to, "", func(int) error { return nil })
			require.Error(t, err)
			_, err = model.MigrateTokenGroup(from, to, "stale", func(int) error { return nil })
			require.ErrorIs(t, err, model.ErrTokenGroupMigrationConflict)
			_, err = model.MigrateTokenGroup(from, to, plan.Version, func(id int) error {
				if id == owner.Id {
					return fmt.Errorf("unavailable destination")
				}
				return nil
			})
			require.Error(t, err)
			var unchanged model.Token
			require.NoError(t, db.First(&unchanged, fixed.Id).Error)
			assert.Equal(t, from, unchanged.Group)
			// Quota consumption after preview must survive the metadata migration.
			require.NoError(t, db.Model(&model.Token{}).Where("id = ?", fixed.Id).Updates(map[string]any{"remain_quota": 450, "used_quota": 85}).Error)
			applied, err := model.MigrateTokenGroup(from, to, plan.Version, func(int) error { return nil })
			require.NoError(t, err)
			require.Len(t, applied.Items, 2)
			var got model.Token
			require.NoError(t, db.First(&got, fixed.Id).Error)
			fixed.Group, fixed.RemainQuota, fixed.UsedQuota = to, 450, 85
			assert.Equal(t, fixed, got)
			require.NoError(t, db.First(&got, fixed.Id).Error)
			var autoAfter model.Token
			require.NoError(t, db.First(&autoAfter, auto.Id).Error)
			require.NoError(t, auto.SetAutoGroups([]string{"first", to, "last"}))
			assert.Equal(t, auto, autoAfter)
			var deletedAfter model.Token
			require.NoError(t, db.Unscoped().First(&deletedAfter, deleted.Id).Error)
			assert.Equal(t, from, deletedAfter.Group)
			_, err = model.MigrateTokenGroup(from, to, plan.Version, func(int) error { return nil })
			require.ErrorIs(t, err, model.ErrTokenGroupMigrationConflict)
			empty, err := model.PreviewTokenGroupMigration(from, to)
			require.NoError(t, err)
			assert.Empty(t, empty.Items)
			_, err = model.PreviewTokenGroupMigration("auto", to)
			require.Error(t, err)
			_, err = model.PreviewTokenGroupMigration(from, from)
			require.Error(t, err)
		})
	}
}

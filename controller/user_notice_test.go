package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUserNoticeDatabaseMatrix(t *testing.T) {
	for _, dialect := range []common.DatabaseType{common.DatabaseTypeSQLite, common.DatabaseTypeMySQL, common.DatabaseTypePostgreSQL} {
		t.Run(string(dialect), func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case common.DatabaseTypeSQLite:
				driver = sqlite.Open(filepath.Join(t.TempDir(), "notices.db"))
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
			oldDB, oldLogDB, oldRedis := model.DB, model.LOG_DB, common.RedisEnabled
			oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
			model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
			common.SetDatabaseTypes(dialect, dialect)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() {
				model.DB, model.LOG_DB, common.RedisEnabled = oldDB, oldLogDB, oldRedis
				common.SetDatabaseTypes(oldMain, oldLog)
				require.NoError(t, sqlDB.Close())
			})
			var version string
			query := "SELECT version()"
			if dialect == common.DatabaseTypeSQLite {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Log("database:", version)
			// Existing RC38 table shapes first, then install the new models twice.
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.AuditLog{}, &model.EmailDelivery{}, &model.MarketingSuppression{}, &model.EmailDeliveryMinuteQuota{}))
			rootToken, userToken := "notice.root.fixture", "notice.user.fixture"
			users := []model.User{
				{Username: "notice-root", Password: "unused", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AccessToken: &rootToken, AffCode: "notice-root"},
				{Username: "notice-normal", Password: "unused", Email: "normal@example.invalid", Status: common.UserStatusEnabled, AccessToken: &userToken, AffCode: "notice-normal"},
				{Username: "notice-disabled", Password: "unused", Email: "disabled@example.invalid", Status: common.UserStatusDisabled, AffCode: "notice-disabled"},
				{Username: "notice-missing", Password: "unused", AffCode: "notice-missing"},
				{Username: "notice-blocked", Password: "unused", Email: "blocked@example.invalid", AffCode: "notice-blocked"},
			}
			require.NoError(t, db.Create(&users).Error)
			oldDelivery, _, err := model.EnqueueEmailDelivery(&model.EmailDelivery{DeliveryKey: "notice-test-existing", Category: "email_verification", Recipient: "old@example.invalid", Subject: "Existing code", Body: "Existing content", SMTPProfile: common.SMTPProfileSecurity})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.UserNotice{}, &model.UserNoticeRequest{}))
			t.Cleanup(func() {
				db.Where("category = ? OR delivery_key = ?", model.UserNoticeCategory, "notice-test-existing").Delete(&model.EmailDelivery{})
				db.Where("request_key <> ?", "").Delete(&model.UserNoticeRequest{})
				db.Where("id > ?", 0).Delete(&model.UserNotice{})
				db.Where("email_hash <> ?", "").Delete(&model.MarketingSuppression{})
				db.Unscoped().Where("username LIKE ?", "notice-%").Delete(&model.User{})
			})
			require.NoError(t, model.CreateMarketingSuppression(users[4].Id, users[4].Email, "permanent SMTP rejection", users[0].Id))
			router := gin.New()
			group := router.Group("/api/marketing", middleware.RootAuth())
			group.GET("/user-notices/users", SearchUserNoticeRecipients)
			group.GET("/user-notices", ListUserNotices)
			group.POST("/user-notices", SubmitUserNotice)
			request := func(method, path, token string, payload any) *httptest.ResponseRecorder {
				raw, err := common.Marshal(payload)
				require.NoError(t, err)
				req := httptest.NewRequest(method, path, bytes.NewReader(raw))
				req.Header.Set("Content-Type", "application/json")
				if token != "" {
					req.Header.Set("Authorization", "Bearer "+token)
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				return rec
			}
			const path = "/api/marketing/user-notices"
			input := model.UserNoticeInput{Kind: "usage", Subject: "Review API usage", Body: "<script>alert(1)</script>\nPlease review retries.",
				UserIDs: []int{users[1].Id, users[2].Id, users[3].Id, users[4].Id, users[1].Id}, Action: "draft", RequestKey: "notice-draft-1"}
			t.Run("root_only", func(t *testing.T) {
				for _, route := range []struct{ method, path string }{{"GET", path}, {"GET", path + "/users"}, {"POST", path}} {
					assert.Equal(t, http.StatusUnauthorized, request(route.method, route.path, "", input).Code)
					assert.Equal(t, http.StatusForbidden, request(route.method, route.path, userToken, input).Code)
				}
			})
			t.Run("recipient_search_never_leaks_credentials_or_email", func(t *testing.T) {
				rec := request("GET", path+"/users?q=disabled@example.invalid", rootToken, nil)
				assert.Equal(t, http.StatusOK, rec.Code)
				var result struct {
					Success bool
					Data    []model.UserNoticeRecipient
				}
				require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &result))
				require.True(t, result.Success)
				require.Len(t, result.Data, 1)
				assert.True(t, result.Data[0].Disabled)
				assert.NotContains(t, rec.Body.String(), users[2].Email)
				assert.NotContains(t, rec.Body.String(), `"password"`)
				assert.NotContains(t, rec.Body.String(), `"access_token"`)
			})
			var draft model.UserNotice
			t.Run("draft_and_idempotent_send", func(t *testing.T) {
				rec := request("POST", path, rootToken, input)
				var result struct {
					Success bool
					Message string
					Data    model.UserNotice
				}
				require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &result))
				require.True(t, result.Success, result.Message)
				draft = result.Data
				assert.Equal(t, "draft", draft.Status)
				require.Len(t, draft.Recipients, 4)
				var count int64
				require.NoError(t, db.Model(&model.EmailDelivery{}).Where("category = ?", model.UserNoticeCategory).Count(&count).Error)
				assert.Zero(t, count)
				input.Id, input.Action, input.RequestKey = draft.Id, "send", "notice-send-1"
				for range 2 {
					rec = request("POST", path, rootToken, input)
					require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &result))
					require.True(t, result.Success, result.Message)
					assert.Equal(t, draft.Id, result.Data.Id)
					assert.Equal(t, "queued", result.Data.Status)
				}
				var deliveries []model.EmailDelivery
				require.NoError(t, db.Where("category = ?", model.UserNoticeCategory).Order("user_id").Find(&deliveries).Error)
				require.Len(t, deliveries, 2)
				assert.Equal(t, users[1].Id, deliveries[0].UserId)
				assert.Equal(t, users[2].Id, deliveries[1].UserId)
				for _, delivery := range deliveries {
					assert.Equal(t, common.SMTPProfileSecurity, delivery.SMTPProfile)
					assert.Less(t, delivery.Priority, model.EmailPriorityCritical)
					assert.Equal(t, model.EmailDeliveryStatusQueued, delivery.State)
					assert.Contains(t, delivery.Body, "&lt;script&gt;")
					assert.NotContains(t, delivery.Body, "<script>")
				}
				input.Subject = "Different payload"
				rec = request("POST", path, rootToken, input)
				require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &result))
				assert.False(t, result.Success)
				input.RequestKey = "notice-resend-rejected"
				rec = request("POST", path, rootToken, input)
				require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &result))
				assert.False(t, result.Success)
				// Real completion metadata is visible, without claiming inbox delivery.
				require.NoError(t, model.CompleteEmailDelivery(deliveries[0].Id, common.SMTPProfileSecurity, common.SMTPChannelSecurity, "fixture-message-id"))
				rows, err := model.ListUserNotices(draft.Id)
				require.NoError(t, err)
				assert.Equal(t, common.SMTPChannelSecurity, rows[0].Recipients[0].SMTPChannel)
				assert.Equal(t, model.EmailDeliveryStatusAcceptedUntracked, rows[0].Recipients[0].State)
				// Changed mailbox cannot receive an already-queued notice.
				require.NoError(t, db.Model(&users[2]).Update("email", "changed@example.invalid").Error)
				eligible, err := model.UserNoticeDeliveryEligible(&deliveries[1])
				require.NoError(t, err)
				assert.False(t, eligible)
			})
			t.Run("invalid_recipient_and_content_fail_atomically", func(t *testing.T) {
				for n, ids := range [][]int{nil, {0}, {99999999}, {users[3].Id}, {users[4].Id}, {users[1].Id, 99999999}} {
					bad := model.UserNoticeInput{Kind: "service", Subject: "Notice", Body: "Content", Action: "send", RequestKey: fmt.Sprintf("notice-invalid-%d", n), UserIDs: ids}
					_, err := model.SubmitUserNotice(bad, users[0].Id)
					assert.Error(t, err)
				}
				for _, subject := range []string{" ", "a\r\nBcc: injected@example.invalid", strings.Repeat("字", 121)} {
					bad := model.UserNoticeInput{Kind: "service", Subject: subject, Body: "Content", Action: "send", RequestKey: "invalid-subject", UserIDs: []int{users[1].Id}}
					_, err := model.SubmitUserNotice(bad, users[0].Id)
					assert.ErrorIs(t, err, model.ErrUserNoticeInvalid)
				}
				var count int64
				require.NoError(t, db.Model(&model.UserNotice{}).Count(&count).Error)
				assert.EqualValues(t, 1, count)
				require.NoError(t, db.Model(&model.UserNoticeRequest{}).Count(&count).Error)
				assert.EqualValues(t, 2, count)
			})
			t.Run("repeated_upgrade_preserves_data_and_unique_delivery_keys", func(t *testing.T) {
				for range 2 {
					require.NoError(t, db.AutoMigrate(&model.UserNotice{}, &model.UserNoticeRequest{}))
				}
				var retained model.UserNotice
				require.NoError(t, db.First(&retained, draft.Id).Error)
				assert.Equal(t, "queued", retained.Status)
				var existing model.EmailDelivery
				require.NoError(t, db.First(&existing, oldDelivery.Id).Error)
				assert.Equal(t, "Existing content", existing.Body)
				assert.True(t, db.Migrator().HasIndex(&model.EmailDelivery{}, "idx_email_deliveries_delivery_key"))
				assert.True(t, db.Migrator().HasIndex(&model.UserNoticeRequest{}, "idx_user_notice_requests_notice_id"))
			})
		})
	}
}

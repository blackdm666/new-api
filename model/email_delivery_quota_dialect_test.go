package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func testEmailDeliveryMinuteQuotaDialect(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&EmailDeliveryMinuteQuota{}))
	scope := fmt.Sprintf("email-quota-dialect-%d", time.Now().UnixNano())
	windowStart := time.Now().Unix()/60*60 - 60
	t.Cleanup(func() {
		_ = db.Where("quota_key LIKE ?", scope+"%").Delete(&EmailDeliveryMinuteQuota{}).Error
	})

	reserved, err := reserveEmailDeliveryMinuteQuota(db, scope, windowStart, 2)
	require.NoError(t, err)
	assert.True(t, reserved)
	reserved, err = reserveEmailDeliveryMinuteQuota(db, scope, windowStart, 2)
	require.NoError(t, err)
	assert.True(t, reserved)
	reserved, err = reserveEmailDeliveryMinuteQuota(db, scope, windowStart, 2)
	require.NoError(t, err)
	assert.False(t, reserved)
}

func TestEmailDeliveryMinuteQuotaSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	closeEmailQuotaTestDB(t, db)
	testEmailDeliveryMinuteQuotaDialect(t, db)
}

func TestEmailDeliveryMinuteQuotaMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	closeEmailQuotaTestDB(t, db)
	testEmailDeliveryMinuteQuotaDialect(t, db)
}

func TestEmailDeliveryMinuteQuotaPostgreSQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not configured")
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: dsn, PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	closeEmailQuotaTestDB(t, db)
	testEmailDeliveryMinuteQuotaDialect(t, db)
}

func closeEmailQuotaTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
}

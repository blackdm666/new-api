package model

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDynamicPluginDatabaseMatrix(t *testing.T) {
	for _, kind := range []common.DatabaseType{common.DatabaseTypeMySQL, common.DatabaseTypePostgreSQL} {
		t.Run(string(kind), func(t *testing.T) {
			var driver gorm.Dialector
			if kind == common.DatabaseTypeMySQL {
				if os.Getenv("TEST_MYSQL_DSN") == "" {
					t.Skip("TEST_MYSQL_DSN missing")
				}
				driver = mysql.Open(os.Getenv("TEST_MYSQL_DSN"))
			} else {
				if os.Getenv("TEST_POSTGRES_DSN") == "" {
					t.Skip("TEST_POSTGRES_DSN missing")
				}
				driver = postgres.Open(os.Getenv("TEST_POSTGRES_DSN"))
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			priorDB, priorLog := DB, LOG_DB
			priorMain, priorLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(kind, kind)
			initCol()
			t.Cleanup(func() {
				DB, LOG_DB = priorDB, priorLog
				common.SetDatabaseTypes(priorMain, priorLogType)
				initCol()
				conn, _ := db.DB()
				conn.Close()
			})
			require.NoError(t, db.AutoMigrate(&Task{}, &Channel{}, &Ability{}, &Option{}, &Model{}, &Vendor{}))
			var version string
			require.NoError(t, db.Raw("SELECT version()").Scan(&version).Error)
			t.Log(version)
			t.Run("candidates", TestDynamicTaskCandidatesFollowEnabledChannelBindings)
			t.Run("pricing", TestDynamicChannelPricingUsesProviderSchema)
		})
	}
}

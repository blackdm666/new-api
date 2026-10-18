package model

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// The distributor tells "the token group does not list the model" from "its
// channels are disabled" with this lookup, so it must read disabled abilities
// and the routing-normalized model name on every supported database.
func TestModelGroupAvailabilityDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not configured")
				}
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			previousDB, previousType := DB, common.MainDatabaseType()
			DB = db
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			initCol()
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&Ability{}))
				DB = previousDB
				common.SetMainDatabaseType(previousType)
				initCol()
			})
			require.NoError(t, db.Migrator().DropTable(&Ability{}))
			require.NoError(t, db.AutoMigrate(&Ability{}))
			require.NoError(t, db.Create([]Ability{
				{Group: "video", Model: "video-model", ChannelId: 1, Enabled: true},
				{Group: "video", Model: "video-model", ChannelId: 2, Enabled: false},
				{Group: "paused", Model: "video-model", ChannelId: 3, Enabled: false},
				{Group: "text", Model: "text-model", ChannelId: 4, Enabled: true},
			}).Error)

			availability, err := GetModelGroupAvailability("video-model")
			require.NoError(t, err)
			assert.Equal(t, map[string]bool{"video": true, "paused": false}, availability)

			availability, err = GetModelGroupAvailability("missing-model")
			require.NoError(t, err)
			assert.Empty(t, availability)
		})
	}
}

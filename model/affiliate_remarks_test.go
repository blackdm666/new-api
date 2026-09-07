package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAffiliateAdminRemarksDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			kind := common.DatabaseTypeSQLite
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(filepath.Join(t.TempDir(), "remarks.db"))
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN not configured")
				}
				driver = mysql.Open(dsn)
				kind = common.DatabaseTypeMySQL
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not configured")
				}
				driver = postgres.Open(dsn)
				kind = common.DatabaseTypePostgreSQL
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			oldDB, oldLog := DB, LOG_DB
			oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(kind, kind)
			initCol()
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() {
				DB, LOG_DB = oldDB, oldLog
				common.SetDatabaseTypes(oldMain, oldLogType)
				initCol()
				_ = sqlDB.Close()
			})
			for i := 0; i < 2; i++ {
				require.NoError(t, db.AutoMigrate(&User{}, &AffiliateAccount{}, &AffiliateCommission{}, &AffiliatePayout{}, &AffiliateUpgradeNotice{}, &AffiliateTransfer{}))
			}
			assert.False(t, db.Migrator().HasColumn(&User{}, "inviter_remark"))
			assert.False(t, db.Migrator().HasColumn(&AffiliateCommission{}, "inviter_remark"))
			assert.False(t, db.Migrator().HasColumn(&AffiliatePayout{}, "remark"))
			owner := &User{Username: "remarks-owner", AffCode: "remarks-owner-code", Remark: "推广备注 <VIP>", Group: "default"}
			require.NoError(t, db.Create(owner).Error)
			buyer := &User{Username: "remarks-buyer", AffCode: "remarks-buyer-code", Remark: "受邀备注", InviterId: owner.Id}
			require.NoError(t, db.Create(buyer).Error)
			missing := &User{Username: "remarks-missing", AffCode: "remarks-missing-code", InviterId: 99999}
			require.NoError(t, db.Create(missing).Error)
			require.NoError(t, db.Create(&AffiliateCommission{InviterId: owner.Id, InviteeId: buyer.Id, TopUpId: 1, TradeNo: "remarks-trade", Status: AffiliateCommissionStatusApproved, TopUpAmountCents: 10000}).Error)
			page := &common.PageInfo{Page: 1, PageSize: 10}
			records, total, err := ListAffiliateCommissions(AffiliateCommissionQueryOptions{IncludeAdminRemarks: true}, page)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, records, 1)
			assert.Equal(t, owner.Remark, records[0].InviterRemark)
			assert.Equal(t, buyer.Remark, records[0].InviteeRemark)
			publicRecords, _, err := ListAffiliateCommissions(AffiliateCommissionQueryOptions{InviterId: owner.Id}, page)
			require.NoError(t, err)
			require.Len(t, publicRecords, 1)
			raw, err := common.Marshal(publicRecords)
			require.NoError(t, err)
			assert.NotContains(t, string(raw), "inviter_remark")
			assert.NotContains(t, string(raw), "invitee_remark")
			require.NoError(t, db.Create(&AffiliatePayout{UserId: owner.Id, RequestId: "remarks-payout", Status: AffiliatePayoutStatusPending, AccountEncrypted: "", AccountName: "test"}).Error)
			payouts, _, err := ListAffiliatePayouts(AffiliatePayoutQueryOptions{IncludeAdminRemarks: true}, page)
			require.NoError(t, err)
			require.Len(t, payouts, 1)
			assert.Equal(t, owner.Remark, payouts[0].Remark)
			payouts, _, err = ListAffiliatePayouts(AffiliatePayoutQueryOptions{UserId: owner.Id}, page)
			require.NoError(t, err)
			require.Len(t, payouts, 1)
			assert.Empty(t, payouts[0].Remark)
			require.NoError(t, db.Create(&AffiliateTransfer{UserId: owner.Id, RequestId: "remarks-transfer", AmountCents: 100}).Error)
			transfers, _, err := ListAffiliateTransfers(AffiliateTransferQueryOptions{IncludeAdminRemarks: true}, page)
			require.NoError(t, err)
			require.Len(t, transfers, 1)
			assert.Equal(t, owner.Remark, transfers[0].Remark)
			transfers, _, err = ListAffiliateTransfers(AffiliateTransferQueryOptions{UserId: owner.Id}, page)
			require.NoError(t, err)
			assert.Empty(t, transfers[0].Remark)
			users, _, err := GetAllUsers(page)
			require.NoError(t, err)
			require.Len(t, users, 3)
			for _, u := range users {
				if u.Id == buyer.Id {
					assert.Equal(t, owner.Remark, u.InviterRemark)
				} else {
					assert.Empty(t, u.InviterRemark)
				}
			}
			users, _, err = SearchUsers("remarks-buyer", "", nil, nil, 0, 10)
			require.NoError(t, err)
			require.Len(t, users, 1)
			assert.Equal(t, owner.Remark, users[0].InviterRemark)
			enableAffiliateForTest(t)
			setAffiliateOptionForTest(t, AffiliateUpgradeInviteesThresholdOptionKey, "1")
			candidates, _, err := ListAffiliateUpgradeCandidates(page)
			require.NoError(t, err)
			require.Len(t, candidates, 1)
			assert.Equal(t, owner.Remark, candidates[0].Remark)
			require.NoError(t, db.Create(&AffiliateUpgradeNotice{InviterId: owner.Id, Threshold: 1, LastError: "test notification failure"}).Error)
			notices, _, err := ListFailedAffiliateUpgradeNotices(page)
			require.NoError(t, err)
			require.Len(t, notices, 1)
			assert.Equal(t, owner.Remark, notices[0].InviterRemark)
			require.NoError(t, db.Model(owner).Update("remark", "更新备注").Error)
			records, _, err = ListAffiliateCommissions(AffiliateCommissionQueryOptions{IncludeAdminRemarks: true}, page)
			require.NoError(t, err)
			assert.Equal(t, "更新备注", records[0].InviterRemark)
			require.NoError(t, db.Delete(owner).Error)
			users, _, err = SearchUsers("remarks-buyer", "", nil, nil, 0, 10)
			require.NoError(t, err)
			require.Len(t, users, 1)
			assert.Equal(t, "更新备注", users[0].InviterRemark)
		})
	}
}

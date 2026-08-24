package controller

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLowBalanceAlertNotifiesOnceAndRearmsAfterRecovery(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:channel-balance-alert-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.EmailDelivery{}))

	root := &model.User{Username: "root", Password: "unused", Email: "root@example.com", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(root).Error)
	channel := &model.Channel{
		Name:   "alert-channel",
		Status: common.ChannelStatusEnabled,
		BalanceInfo: &model.ChannelBalanceInfo{
			Remaining:   "5",
			Unit:        model.ChannelBalanceUnitMoney,
			Currency:    "USD",
			DisplayUnit: "$",
			MetricKind:  dto.ChannelBalanceMetricWallet,
			UpdatedAt:   common.GetTimestamp(),
		},
	}
	require.NoError(t, db.Create(channel).Error)
	config := &dto.ChannelBalanceQueryConfig{
		AutoRefresh:         true,
		RefreshMinutes:      15,
		LowBalanceAlert:     true,
		LowBalanceThreshold: "10",
	}

	notified, err := updateChannelLowBalanceAlert(channel, config, channel.BalanceInfo)
	require.NoError(t, err)
	assert.True(t, notified)
	assert.True(t, channel.BalanceInfo.LowBalanceAlertActive)
	assert.EqualValues(t, 1, channel.BalanceInfo.LowBalanceAlertVersion)

	notified, err = updateChannelLowBalanceAlert(channel, config, channel.BalanceInfo)
	require.NoError(t, err)
	assert.False(t, notified)
	var deliveryCount int64
	require.NoError(t, db.Model(&model.EmailDelivery{}).Count(&deliveryCount).Error)
	assert.EqualValues(t, 1, deliveryCount)

	recovered := *channel.BalanceInfo
	recovered.Remaining = "20"
	require.NoError(t, channel.UpdateBalanceInfo(recovered, nil))
	notified, err = updateChannelLowBalanceAlert(channel, config, channel.BalanceInfo)
	require.NoError(t, err)
	assert.False(t, notified)
	assert.False(t, channel.BalanceInfo.LowBalanceAlertActive)

	belowAgain := *channel.BalanceInfo
	belowAgain.Remaining = "4"
	require.NoError(t, channel.UpdateBalanceInfo(belowAgain, nil))
	notified, err = updateChannelLowBalanceAlert(channel, config, channel.BalanceInfo)
	require.NoError(t, err)
	assert.True(t, notified)
	assert.EqualValues(t, 2, channel.BalanceInfo.LowBalanceAlertVersion)
	require.NoError(t, db.Model(&model.EmailDelivery{}).Count(&deliveryCount).Error)
	assert.EqualValues(t, 2, deliveryCount)
}

package controller

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTasksToDtoAddsAdminChannelName(t *testing.T) {
	previousDB := model.DB
	previousRedisEnabled := common.RedisEnabled
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedisEnabled
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})

	dsn := fmt.Sprintf("file:task-dto-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}))
	model.DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false

	require.NoError(t, db.Create(&model.User{Id: 7, Username: "task-user"}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 71, Name: "Vertex video channel"}).Error)

	adminResult := tasksToDto([]*model.Task{{UserId: 7, ChannelId: 71}}, true)
	require.Len(t, adminResult, 1)
	assert.Equal(t, "task-user", adminResult[0].Username)
	assert.Equal(t, "Vertex video channel", adminResult[0].ChannelName)

	userResult := tasksToDto([]*model.Task{{UserId: 7, ChannelId: 71}}, false)
	require.Len(t, userResult, 1)
	assert.Empty(t, userResult[0].Username)
	assert.Empty(t, userResult[0].ChannelName)
}

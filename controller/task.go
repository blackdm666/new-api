package controller

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func GetAllTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 解析其他查询参数
	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ChannelID:      c.Query("channel_id"),
	}

	items := model.TaskGetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllTasks(queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, true))
	common.ApiSuccess(c, pageInfo)
}

func GetUserTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	userId := c.GetInt("id")

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	}

	items := model.TaskGetAllUserTask(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllUserTask(userId, queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, false))
	common.ApiSuccess(c, pageInfo)
}

func GetTaskPreviewURL(c *gin.Context) {
	taskId := strings.TrimSpace(c.Param("task_id"))
	if taskId == "" {
		common.ApiError(c, errors.New("task_id is required"))
		return
	}

	var task *model.Task
	var exists bool
	var err error
	if c.GetInt("role") >= common.RoleAdminUser {
		task, exists, err = model.GetTaskByTaskId(taskId)
	} else {
		task, exists, err = model.GetByTaskId(c.GetInt("id"), taskId)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !exists || task == nil {
		common.ApiError(c, errors.New("task not found"))
		return
	}
	if task.Status != model.TaskStatusSuccess {
		common.ApiError(c, errors.New("task is not completed"))
		return
	}

	previewURL, cached, err := service.GetTaskVideoPreviewURL(c.Request.Context(), task)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if cached {
		common.ApiSuccess(c, gin.H{
			"url":        previewURL,
			"expires_in": int64(service.TaskVideoPreviewURLTTL().Seconds()),
		})
		return
	}

	resultURL := strings.TrimSpace(task.GetResultURL())
	parsed, parseErr := url.Parse(resultURL)
	if parseErr == nil && parsed != nil && (parsed.Scheme == "https" || parsed.Scheme == "http") &&
		!strings.Contains(parsed.Path, "/v1/videos/"+task.TaskID+"/content") {
		common.ApiSuccess(c, gin.H{"url": resultURL, "expires_in": 0})
		return
	}
	common.ApiError(c, errors.New("direct video preview is unavailable"))
}

func tasksToDto(tasks []*model.Task, fillUser bool) []*dto.TaskDto {
	var userIdMap map[int]*model.UserBase
	var channelNameMap map[int]string
	if fillUser {
		userIdMap = make(map[int]*model.UserBase)
		channelNameMap = make(map[int]string)
		userIds := types.NewSet[int]()
		channelIds := types.NewSet[int]()
		for _, task := range tasks {
			userIds.Add(task.UserId)
			channelIds.Add(task.ChannelId)
		}
		for _, userId := range userIds.Items() {
			cacheUser, err := model.GetUserCache(userId)
			if err == nil {
				userIdMap[userId] = cacheUser
			}
		}
		for _, channelId := range channelIds.Items() {
			cacheChannel, err := model.CacheGetChannel(channelId)
			if err == nil {
				channelNameMap[channelId] = cacheChannel.Name
			}
		}
	}
	result := make([]*dto.TaskDto, len(tasks))
	for i, task := range tasks {
		if fillUser {
			if user, ok := userIdMap[task.UserId]; ok {
				task.Username = user.Username
			}
			task.ChannelName = channelNameMap[task.ChannelId]
		}
		result[i] = relay.TaskModel2Dto(task)
	}
	return result
}

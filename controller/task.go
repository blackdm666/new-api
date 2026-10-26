package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type taskArtifactResponse struct {
	Key        string `json:"key"`
	Type       string `json:"type"`
	MimeType   string `json:"mime_type,omitempty"`
	ContentURL string `json:"content_url"`
}

var (
	taskArtifactKeyPattern           = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~-]{0,127}$`)
	errTaskArtifactPluginUnavailable = errors.New("task artifact plugin unavailable")
	errTaskArtifactPlugin            = errors.New("task artifact plugin error")
	getTaskVideoPreviewURL           = service.GetTaskVideoPreviewURL
	prepareTaskVideoPreviewURL       = service.PrepareTaskVideoPreviewURL
)

func GetTask(c *gin.Context) {
	task, exists, err := model.GetByTaskId(c.GetInt("id"), c.Param("key"))
	if err != nil {
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to query task")
		return
	}
	if !exists {
		videoProxyError(c, http.StatusNotFound, "invalid_request_error", "Task not found")
		return
	}
	createdAt := task.CreatedAt
	if createdAt == 0 {
		createdAt = task.SubmitTime
	}
	failReason := task.FailReason
	if task.Status == model.TaskStatusSuccess && taskFailReasonIsLegacyResultURL(task.FailReason) {
		failReason = ""
	}
	response := gin.H{
		"task_id":     task.TaskID,
		"status":      task.Status,
		"progress":    task.Progress,
		"fail_reason": failReason,
		"created_at":  createdAt,
		"finished_at": task.FinishTime,
	}
	// Plugin keys name the serving provider; only Suno clients use the platform.
	if task.Platform == constant.TaskPlatformSuno {
		response["platform"] = task.Platform
	}
	if officialID := relay.PublicOfficialTaskID(task); officialID != "" {
		response["official_task_id"] = officialID
	}
	c.JSON(http.StatusOK, response)
}

func GetTaskArtifacts(c *gin.Context) {
	task, exists, err := model.GetByTaskId(c.GetInt("id"), c.Param("key"))
	if err != nil {
		writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_internal_error", "Failed to query task")
		return
	}
	if !exists || task == nil {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	writeTaskArtifacts(c, task, false)
}

func GetDashboardTaskArtifacts(c *gin.Context) {
	task, exists, err := getTaskForArtifactRequest(c, c.Param("task_id"))
	if err != nil {
		writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_internal_error", "Failed to query task")
		return
	}
	if !exists || task == nil {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	writeTaskArtifacts(c, task, true)
}

func writeTaskArtifacts(c *gin.Context, task *model.Task, dashboard bool) {
	c.Header("Cache-Control", "private, no-store")
	artifacts, err := projectTaskArtifacts(task)
	if err != nil {
		writeTaskArtifactProjectionError(c, err)
		return
	}
	items := make([]taskArtifactResponse, 0, len(artifacts))
	deliveryContext, cancelDelivery := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancelDelivery()
	var deliveryProvider relaychannel.TaskContentRequestProvider
	if service.TaskMediaPublicEnabled() && len(artifacts) > 0 {
		if adaptor, initErr := initTaskArtifactAdaptor(task); initErr == nil {
			deliveryProvider, _ = adaptor.(relaychannel.TaskContentRequestProvider)
		}
	}
	for _, artifact := range artifacts {
		contentURL, buildErr := service.BuildTaskArtifactContentURL(task.TaskID, artifact.Key)
		if buildErr != nil {
			writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_url_error", "Failed to build artifact content URL")
			return
		}
		contentURL = resolveTaskArtifactDelivery(deliveryContext, task, artifacts, artifact, deliveryProvider, contentURL)
		items = append(items, taskArtifactResponse{
			Key:        artifact.Key,
			Type:       artifact.Type,
			MimeType:   artifact.MimeType,
			ContentURL: contentURL,
		})
	}
	response := gin.H{"task_id": task.TaskID, "artifacts": items}
	if dashboard && task.Status == model.TaskStatusSuccess && task.Platform == constant.TaskPlatformSuno {
		response["legacy_audio_clips"] = legacySunoAudioClips(task.Data)
	}
	if legacyVideoAvailable(task) {
		legacyContentURL, buildErr := buildLegacyTaskContentURL(c, task)
		if buildErr != nil {
			writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_url_error", "Failed to build artifact content URL")
			return
		}
		response["legacy_content_url"] = legacyContentURL
	}
	if dashboard {
		common.ApiSuccess(c, response)
		return
	}
	c.JSON(http.StatusOK, response)
}

// buildLegacyTaskContentURL shares public delivery with API/plugin outputs.
// With public delivery disabled the original capability policy is preserved.
func buildLegacyTaskContentURL(c *gin.Context, task *model.Task) (string, error) {
	if service.TaskMediaPublicEnabled() {
		return service.TaskVideoDeliveryURL(c.Request.Context(), task), nil
	}
	capabilityURL, err := service.BuildTaskArtifactContentURL(task.TaskID, "video")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(task.PrivateData.ResultStorageKey) != "" {
		return capabilityURL, nil
	}

	previewURL, _, prepareErr := prepareTaskVideoPreviewURL(c.Request.Context(), task)
	if prepareErr != nil || strings.TrimSpace(task.PrivateData.ResultStorageKey) != "" {
		return capabilityURL, nil
	}
	if isSafeDirectTaskVideoURL(previewURL) {
		return strings.TrimSpace(previewURL), nil
	}
	return capabilityURL, nil
}

func isSafeDirectTaskVideoURL(rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || len(rawURL) > 64<<10 || strings.ContainsAny(rawURL, "\r\n\\") {
		return false
	}
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed != nil && parsed.Scheme == "https" &&
		parsed.Host != "" && parsed.Hostname() != "" && parsed.User == nil && parsed.Fragment == ""
}

// legacySunoAudioClips projects a pre-plugin Suno task's persisted snapshot
// into the clips the dashboard audio preview renders. Task lists no longer
// carry the snapshot, so the dashboard reads it here on demand. The snapshot
// is either a clip array or a JSON string holding one; only clips with an
// audio URL are kept and only preview fields are exposed.
func legacySunoAudioClips(data json.RawMessage) []map[string]any {
	clips := make([]map[string]any, 0)
	if len(data) == 0 {
		return clips
	}
	var items []map[string]any
	if err := common.Unmarshal(data, &items); err != nil {
		var encoded string
		if common.Unmarshal(data, &encoded) != nil || common.UnmarshalJsonStr(encoded, &items) != nil {
			return clips
		}
	}
	for _, item := range items {
		audioURL, _ := item["audio_url"].(string)
		if strings.TrimSpace(audioURL) == "" {
			continue
		}
		clip := map[string]any{"audio_url": audioURL}
		for _, key := range []string{"clip_id", "id", "title", "tags", "duration", "image_url", "image_large_url", "metadata"} {
			if value, ok := item[key]; ok {
				clip[key] = value
			}
		}
		clips = append(clips, clip)
	}
	return clips
}

func projectTaskArtifacts(task *model.Task) ([]relaychannel.TaskArtifact, error) {
	if task == nil || task.Status != model.TaskStatusSuccess || !taskHasPluginExecution(task) || !task.ResultRetrievable() {
		return []relaychannel.TaskArtifact{}, nil
	}
	adaptor := relay.GetTaskAdaptor(task.Platform)
	if adaptor == nil {
		return nil, errTaskArtifactPluginUnavailable
	}
	provider, ok := adaptor.(relaychannel.TaskArtifactProvider)
	if !ok {
		return []relaychannel.TaskArtifact{}, nil
	}
	artifacts, err := provider.ListArtifacts(task)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errTaskArtifactPlugin, err)
	}
	return validateProjectedTaskArtifacts(artifacts)
}

func validateProjectedTaskArtifacts(artifacts []relaychannel.TaskArtifact) ([]relaychannel.TaskArtifact, error) {
	if len(artifacts) > 64 {
		return nil, fmt.Errorf("%w: too many artifacts", errTaskArtifactPlugin)
	}
	seen := make(map[string]struct{}, len(artifacts))
	for i := range artifacts {
		if artifacts[i].Key != strings.TrimSpace(artifacts[i].Key) ||
			artifacts[i].Type != strings.TrimSpace(artifacts[i].Type) {
			return nil, fmt.Errorf("%w: invalid artifact identity", errTaskArtifactPlugin)
		}
		if !taskArtifactKeyPattern.MatchString(artifacts[i].Key) {
			return nil, fmt.Errorf("%w: invalid artifact key", errTaskArtifactPlugin)
		}
		if _, exists := seen[artifacts[i].Key]; exists {
			return nil, fmt.Errorf("%w: duplicate artifact key", errTaskArtifactPlugin)
		}
		seen[artifacts[i].Key] = struct{}{}
		switch artifacts[i].Type {
		case "video", "audio", "image", "file":
		default:
			return nil, fmt.Errorf("%w: invalid artifact type", errTaskArtifactPlugin)
		}
		if len(artifacts[i].MimeType) > 255 || strings.ContainsAny(artifacts[i].MimeType, "\r\n") {
			return nil, fmt.Errorf("%w: invalid artifact mime type", errTaskArtifactPlugin)
		}
	}
	return artifacts, nil
}

func initTaskArtifactAdaptor(task *model.Task) (relaychannel.TaskAdaptor, error) {
	if task == nil {
		return nil, errTaskArtifactPluginUnavailable
	}
	return initTaskArtifactAdaptorInstance(task, relay.GetTaskAdaptor(task.Platform))
}

func initTaskArtifactAdaptorInstance(task *model.Task, adaptor relaychannel.TaskAdaptor) (relaychannel.TaskAdaptor, error) {
	if task == nil || !taskHasPluginExecution(task) {
		return nil, errTaskArtifactPluginUnavailable
	}
	channelModel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil {
		return nil, fmt.Errorf("%w: channel unavailable", errTaskArtifactPluginUnavailable)
	}
	if adaptor == nil {
		return nil, errTaskArtifactPluginUnavailable
	}
	pluginKey := task.PrivateData.Key
	if pluginKey == "" {
		pluginKey = channelModel.Key
	}
	baseURL := channelModel.GetBaseURL()
	if baseURL == "" {
		baseURL = constant.GetChannelBaseURL(channelModel.Type)
	}
	adaptor.Init(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:     channelModel.Type,
			ChannelBaseUrl:  baseURL,
			ApiKey:          pluginKey,
			ChannelSetting:  channelModel.GetSetting(),
			HeadersOverride: channelModel.GetHeaderOverride(),
		},
	})
	return adaptor, nil
}

func resolveTaskArtifactDelivery(ctx context.Context, task *model.Task, artifacts []relaychannel.TaskArtifact, artifact relaychannel.TaskArtifact, provider relaychannel.TaskContentRequestProvider, fallback string) string {
	if !service.TaskMediaPublicEnabled() {
		return fallback
	}
	videoCount := 0
	for _, item := range artifacts {
		if item.Type == "video" {
			videoCount++
		}
	}
	var source *service.TaskArtifactDeliverySource
	if provider != nil {
		if descriptor, err := provider.BuildContentRequest(ctx, task, artifact.Key, relaychannel.TaskArtifactClientRequest{Method: http.MethodGet}); err == nil && descriptor != nil {
			source = &service.TaskArtifactDeliverySource{URL: descriptor.URL, Method: descriptor.Method, Anonymous: descriptor.Credentialless && len(descriptor.Headers) == 0 && len(descriptor.Body) == 0}
		}
	}
	return service.TaskArtifactDeliveryURL(ctx, task, artifact, videoCount == 1, source, fallback)
}

func taskHasPluginExecution(task *model.Task) bool {
	return task != nil &&
		task.PrivateData.Execution != nil &&
		task.PrivateData.Execution.TaskPlugin != nil &&
		strings.TrimSpace(task.PrivateData.Execution.TaskPlugin.Key) != ""
}

func legacyVideoAvailable(task *model.Task) bool {
	if task == nil || task.Status != model.TaskStatusSuccess ||
		taskHasPluginExecution(task) || task.Platform == constant.TaskPlatformSuno ||
		strings.TrimSpace(task.GetResultURL()) == "" {
		return false
	}
	switch constant.NormalizeTaskAction(task.Action) {
	case constant.TaskActionImageToVideo,
		constant.TaskActionTextToVideo,
		constant.TaskActionFirstTailToVideo,
		constant.TaskActionReferenceToVideo,
		constant.TaskActionRemix:
		return true
	default:
		return false
	}
}

func getTaskForArtifactRequest(c *gin.Context, taskID string) (*model.Task, bool, error) {
	if middleware.IsTaskArtifactAccess(c) {
		task, exists, err := model.GetUniqueByOnlyTaskId(taskID)
		if err != nil || !exists || task == nil {
			return task, exists, err
		}
		owner, err := model.GetUserCache(task.UserId)
		if err != nil || owner == nil || owner.Status != common.UserStatusEnabled {
			return nil, false, err
		}
		return task, true, nil
	}
	if c.GetInt("token_id") == 0 && c.GetInt("role") >= common.RoleAdminUser {
		return model.GetByOnlyTaskId(taskID)
	}
	return model.GetByTaskId(c.GetInt("id"), taskID)
}

func writeTaskArtifactProjectionError(c *gin.Context, err error) {
	if errors.Is(err, errTaskArtifactPluginUnavailable) {
		writeTaskArtifactError(c, http.StatusServiceUnavailable, "artifact_plugin_unavailable", "Artifact preview plugin is unavailable")
		return
	}
	writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_plugin_error", "Artifact preview plugin failed")
}

func writeTaskArtifactError(c *gin.Context, status int, code, message string) {
	c.Header("Cache-Control", "private, no-store")
	if middleware.IsTaskArtifactAccess(c) {
		status = http.StatusNotFound
		code = "artifact_not_found"
		message = "Task or artifact not found"
	}
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.JSON(status, gin.H{"success": false, "code": code, "message": message})
		return
	}
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    code,
			"code":    code,
		},
	})
}

func TaskArtifactContent(c *gin.Context) {
	task, exists, err := getTaskForArtifactRequest(c, c.Param("key"))
	if err != nil {
		writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_internal_error", "Failed to query task")
		return
	}
	if !exists || task == nil {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	artifactKey := strings.TrimSpace(c.Param("artifact_key"))
	if !taskArtifactKeyPattern.MatchString(artifactKey) || !task.ResultRetrievable() {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	if task.Status != model.TaskStatusSuccess {
		writeTaskArtifactError(c, http.StatusConflict, "artifact_not_ready", "Task artifacts are not ready")
		return
	}
	if !taskHasPluginExecution(task) {
		if artifactKey != "video" || !legacyVideoAvailable(task) {
			writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
			return
		}
		if previewURL, cached, previewErr := getTaskVideoPreviewURL(c.Request.Context(), task); cached {
			if previewErr != nil || strings.TrimSpace(previewURL) == "" {
				writeTaskMediaProxyError(c, &taskMediaProxyError{
					status: http.StatusBadGateway, code: "artifact_upstream_error",
					message: "Failed to open cached artifact content", err: previewErr,
				})
				return
			}
			c.Header("Cache-Control", "private, no-store")
			c.Redirect(http.StatusTemporaryRedirect, previewURL)
			return
		}
		descriptor := &relaychannel.TaskContentRequest{
			URL:            task.GetResultURL(),
			Method:         c.Request.Method,
			Credentialless: true,
		}
		if err := proxyTaskMedia(c, task, descriptor); err != nil {
			writeTaskMediaProxyError(c, err)
		}
		return
	}
	artifacts, err := projectTaskArtifacts(task)
	if err != nil {
		writeTaskArtifactProjectionError(c, err)
		return
	}
	found := false
	for _, artifact := range artifacts {
		if artifact.Key == artifactKey {
			found = true
			break
		}
	}
	if !found {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	artifactStore := service.GetTaskArtifactStore()
	if ref, resolveErr := artifactStore.Resolve(task, artifactKey); resolveErr == nil && ref != nil {
		_ = artifactStore.Serve(c, task, ref)
		return
	}

	adaptor, err := initTaskArtifactAdaptor(task)
	if err != nil {
		writeTaskArtifactProjectionError(c, err)
		return
	}
	provider, ok := adaptor.(relaychannel.TaskContentRequestProvider)
	if !ok {
		writeTaskArtifactError(c, http.StatusServiceUnavailable, "artifact_plugin_unavailable", "Artifact content plugin is unavailable")
		return
	}
	clientRequest := relaychannel.TaskArtifactClientRequest{
		Method:  c.Request.Method,
		Headers: taskArtifactClientHeaders(c.Request.Header),
	}
	descriptor, err := provider.BuildContentRequest(c.Request.Context(), task, artifactKey, clientRequest)
	if err != nil || descriptor == nil {
		if err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("Failed to resolve plugin %s content for task %s: %v", artifactKey, task.TaskID, err))
		}
		writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_plugin_error", "Artifact content plugin failed")
		return
	}
	// Old capability links stay usable, but a positively mapped public artifact
	// redirects to the same object without proxying or renewing it.
	videoCount := 0
	var requestedArtifact relaychannel.TaskArtifact
	for _, artifact := range artifacts {
		if artifact.Type == "video" {
			videoCount++
		}
		if artifact.Key == artifactKey {
			requestedArtifact = artifact
		}
	}
	publicURL := service.TaskArtifactDeliveryURL(c.Request.Context(), task, requestedArtifact, videoCount == 1, &service.TaskArtifactDeliverySource{
		URL: descriptor.URL, Method: descriptor.Method, Anonymous: descriptor.Credentialless && len(descriptor.Headers) == 0 && len(descriptor.Body) == 0,
	}, "")
	if publicURL != "" {
		c.Header("Cache-Control", "private, no-store")
		c.Redirect(http.StatusTemporaryRedirect, publicURL)
		return
	}
	if err := proxyTaskMedia(c, task, descriptor); err != nil {
		writeTaskMediaProxyError(c, err)
	}
}

func taskArtifactClientHeaders(headers http.Header) map[string]string {
	result := make(map[string]string, 4)
	for _, name := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			result[name] = value
		}
	}
	return result
}

/*
	The task list handlers below deliberately do not call projectTaskArtifacts.
	Artifact projection is confined to the explicit endpoints above.
*/

func GetAllTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	queryParams := model.SyncTaskQueryParams{Platform: constant.TaskPlatform(c.Query("platform")), Status: c.Query("status"), Action: c.Query("action"), StartTimestamp: startTimestamp, EndTimestamp: endTimestamp, ChannelID: c.Query("channel_id"), ModelName: strings.TrimSpace(c.Query("model_name"))}
	if username := strings.TrimSpace(c.Query("username")); username != "" {
		userIDs, err := model.FindTaskUserIDsByUsername(username)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if len(userIDs) == 0 {
			pageInfo.SetItems([]*dto.TaskDto{})
			common.ApiSuccess(c, pageInfo)
			return
		}
		queryParams.UserIDs = userIDs
	}
	if !resolveTaskIDSearch(c, &queryParams, 0) {
		pageInfo.SetItems([]*dto.TaskDto{})
		common.ApiSuccess(c, pageInfo)
		return
	}
	items := model.TaskGetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	pageInfo.SetTotal(int(model.TaskCountAllTasks(queryParams)))
	pageInfo.SetItems(tasksToDto(items, true, c.GetInt("role")))
	common.ApiSuccess(c, pageInfo)
}

func GetUserTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userID := c.GetInt("id")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	queryParams := model.SyncTaskQueryParams{Platform: constant.TaskPlatform(c.Query("platform")), Status: c.Query("status"), Action: c.Query("action"), StartTimestamp: startTimestamp, EndTimestamp: endTimestamp, ModelName: strings.TrimSpace(c.Query("model_name"))}
	if !resolveTaskIDSearch(c, &queryParams, userID) {
		pageInfo.SetItems([]*dto.TaskDto{})
		common.ApiSuccess(c, pageInfo)
		return
	}
	items := model.TaskGetAllUserTask(userID, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	pageInfo.SetTotal(int(model.TaskCountAllUserTask(userID, queryParams)))
	pageInfo.SetItems(tasksToDto(items, false, common.RoleCommonUser))
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
		task, exists, err = model.GetByOnlyTaskId(taskId)
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

	previewURL, expiresIn, err := service.PrepareTaskVideoPreviewURL(c.Request.Context(), task)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"url": previewURL, "expires_in": expiresIn})
}

func tasksToDto(tasks []*model.Task, fillUser bool, viewerRole int) []*dto.TaskDto {
	var userIDMap map[int]*model.UserBase
	var channelNameMap map[int]string
	if fillUser {
		userIDMap = make(map[int]*model.UserBase)
		channelNameMap = make(map[int]string)
		userIDs := types.NewSet[int]()
		channelIDs := types.NewSet[int]()
		for _, task := range tasks {
			userIDs.Add(task.UserId)
			channelIDs.Add(task.ChannelId)
		}
		for _, userID := range userIDs.Items() {
			if cacheUser, err := model.GetUserCache(userID); err == nil {
				userIDMap[userID] = cacheUser
			}
		}
		for _, channelId := range channelIDs.Items() {
			cacheChannel, err := model.CacheGetChannel(channelId)
			if err == nil {
				channelNameMap[channelId] = cacheChannel.Name
			}
		}
	}
	// Lists are fetched without snapshots; official-ID plugins read theirs.
	if err := model.HydrateTaskData(tasks, func(task *model.Task) bool {
		return relay.TaskDeclaresOfficialID(task.Platform)
	}); err != nil {
		common.SysError("load task snapshots for official IDs: " + err.Error())
	}
	result := make([]*dto.TaskDto, len(tasks))
	for i, task := range tasks {
		if fillUser {
			if user, ok := userIDMap[task.UserId]; ok {
				task.Username = user.Username
			}
			task.ChannelName = channelNameMap[task.ChannelId]
		}
		item := relay.TaskModel2Dto(task)
		if viewerRole < common.RoleAdminUser {
			// Users see the model they requested, never the plugin key, channel
			// or channel-mapped upstream model that would name the provider.
			relay.HideTaskRouting(item, task)
			// Data is the raw upstream snapshot (provider task IDs, storage URLs)
			// and would identify the upstream. Users only need the Suno preview clips.
			item.Data = nil
			if task.Platform == constant.TaskPlatformSuno && task.Status == model.TaskStatusSuccess {
				if clips, err := common.Marshal(legacySunoAudioClips(task.Data)); err == nil {
					item.Data = clips
				}
			}
		}
		// Plugins of official-relay channels declare the model platform's own
		// task ID public; root also sees the best-effort ID for every task.
		item.OfficialTaskID = relay.PublicOfficialTaskID(task)
		item.LegacyVideoAvailable = legacyVideoAvailable(task)
		item.ResultDiscarded = task.PrivateData.ResultDiscarded
		if task.Status == model.TaskStatusSuccess {
			item.ResultURL = ""
			if taskFailReasonIsLegacyResultURL(task.FailReason) {
				item.FailReason = ""
			}
		}
		if viewerRole >= common.RoleAdminUser {
			adminInfo := &dto.TaskAdminInfo{}
			if execution := task.PrivateData.Execution; execution != nil {
				adminInfo.RequestID = execution.RequestID
				adminInfo.RequestPath = execution.RequestPath
				if snapshot := execution.TaskPlugin; snapshot != nil {
					adminInfo.TaskPlugin = &dto.TaskPluginInfo{
						Key:     snapshot.Key,
						Name:    snapshot.Name,
						Version: snapshot.Version,
					}
					if snapshot.Author != nil {
						adminInfo.TaskPlugin.Author = &dto.TaskPluginAuthorInfo{
							Name: snapshot.Author.Name,
							URL:  snapshot.Author.URL,
						}
					}
				}
			}
			if adminInfo.RequestID != "" || adminInfo.RequestPath != "" || adminInfo.TaskPlugin != nil {
				item.AdminInfo = adminInfo
			}
		}
		if viewerRole >= common.RoleRootUser {
			rootInfo := &dto.TaskRootInfo{
				UpstreamTaskID: task.PrivateData.UpstreamTaskID,
				NodeName:       task.PrivateData.NodeName,
			}
			if item.OfficialTaskID == "" {
				item.OfficialTaskID = relay.RootOfficialTaskID(task)
			}
			if execution := task.PrivateData.Execution; execution != nil {
				if snapshot := execution.TaskPlugin; snapshot != nil {
					rootInfo.TaskPlugin = &dto.TaskPluginRuntimeInfo{
						Key:        snapshot.Key,
						Version:    snapshot.Version,
						APIVersion: snapshot.APIVersion,
						Generation: snapshot.Generation,
					}
				}
			}
			if rootInfo.TaskPlugin != nil || rootInfo.UpstreamTaskID != "" || rootInfo.NodeName != "" {
				item.RootInfo = rootInfo
			}
		}
		result[i] = item
	}
	return result
}

// taskIDSearchCandidateLimit bounds how many payload matches are checked
// against the official task ID while old tasks are still being indexed.
const taskIDSearchCandidateLimit = 200

// resolveTaskIDSearch turns the task_id query into the rows it names: our own
// task ID, or an ID from the task search index that this viewer may see (the
// published official ID for everyone; the fallback official ID and the
// upstream task ID for root). It reports false when nothing matches.
func resolveTaskIDSearch(c *gin.Context, queryParams *model.SyncTaskQueryParams, userID int) bool {
	keyword := strings.TrimSpace(c.Query("task_id"))
	if keyword == "" {
		return true
	}
	role := c.GetInt("role")
	kinds := []string{model.TaskSearchTermKindOfficial}
	if role >= common.RoleRootUser {
		kinds = append(kinds, model.TaskSearchTermKindRootOfficial, model.TaskSearchTermKindUpstream)
	}
	ids, err := model.FindTaskRowIDsByTaskID(keyword, userID)
	if err != nil {
		common.SysError(common.LogText("task id search failed: %s", err.Error()))
		return false
	}
	indexed, err := model.FindTaskIDsBySearchTerm(keyword, kinds)
	if err != nil {
		common.SysError(common.LogText("task id search failed: %s", err.Error()))
		return false
	}
	ids = append(ids, indexed...)
	// Until every older task is indexed, also check payload matches directly.
	if !model.TaskSearchTermsBackfilled() {
		candidates, err := model.FindTaskSearchCandidates(keyword, userID, taskIDSearchCandidateLimit)
		if err != nil {
			common.SysError(common.LogText("task id search failed: %s", err.Error()))
			return false
		}
		for _, task := range candidates {
			for _, term := range relay.TaskSearchTerms(task) {
				if term.Value == keyword && slices.Contains(kinds, term.Kind) {
					ids = append(ids, task.ID)
					break
				}
			}
		}
	}
	slices.Sort(ids)
	queryParams.IDs = slices.Compact(ids)
	return len(queryParams.IDs) > 0
}

func taskFailReasonIsLegacyResultURL(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= len("https://") && strings.EqualFold(value[:len("https://")], "https://") ||
		len(value) >= len("http://") && strings.EqualFold(value[:len("http://")], "http://") ||
		len(value) >= len("data:") && strings.EqualFold(value[:len("data:")], "data:")
}

// CancelVideoTask cancels the caller's unfinished task at the provider
// (DELETE /v1/videos/:task_id and /v1/video/generations/:task_id). Only a
// provider-confirmed cancellation fails the task locally, which refunds the
// reserved quota exactly once through the regular failure settlement.
func CancelVideoTask(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	task, exist, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		writeTaskCancelError(c, http.StatusInternalServerError, "get_task_failed", "查询任务失败，请稍后重试")
		return
	}
	if !exist || task == nil || !task.ResultRetrievable() {
		writeTaskCancelError(c, http.StatusNotFound, "task_not_exist", "任务不存在")
		return
	}
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		writeTaskCancelError(c, http.StatusConflict, "task_not_cancellable", "任务已结束，无法取消")
		return
	}
	adaptor, err := initTaskArtifactAdaptor(task)
	provider, cancellable := adaptor.(relaychannel.TaskCancelProvider)
	pollingAdaptor, pollable := adaptor.(service.TaskPollingAdaptor)
	if err != nil || !cancellable || !pollable {
		writeTaskCancelError(c, http.StatusNotImplemented, "cancel_not_supported", "该模型暂不支持取消任务")
		return
	}
	result, err := provider.CancelTask(c.Request.Context(), task)
	if err != nil {
		common.SysError(fmt.Sprintf("cancel task %s failed: %v", task.TaskID, err))
		writeTaskCancelError(c, http.StatusBadGateway, "cancel_failed", "取消请求失败，请稍后重试或继续查询任务状态")
		return
	}
	if result == nil {
		writeTaskCancelError(c, http.StatusNotImplemented, "cancel_not_supported", "该模型暂不支持取消任务")
		return
	}
	if !result.Cancelled {
		message := result.Message
		if message == "" {
			message = "任务已开始生成或已结束，无法取消"
		}
		writeTaskCancelError(c, http.StatusConflict, "task_not_cancellable", message)
		return
	}
	// The poller may move the task between non-terminal states concurrently;
	// retry the transition from the latest status until it ends.
	for range 3 {
		won, failErr := service.FailTask(c.Request.Context(), pollingAdaptor, task, task.Status, "任务已被用户取消")
		if failErr != nil {
			common.SysError(fmt.Sprintf("fail cancelled task %s: %v", task.TaskID, failErr))
			break
		}
		if won {
			c.JSON(http.StatusOK, gin.H{"id": task.TaskID, "object": "video", "status": "cancelled", "cancelled": true})
			return
		}
		task, exist, err = model.GetByTaskId(c.GetInt("id"), taskID)
		if err != nil || !exist || task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
			break
		}
	}
	// The provider accepted the cancellation but the local task already ended;
	// its own settlement stands and the next poll reconciles any difference.
	writeTaskCancelError(c, http.StatusConflict, "task_not_cancellable", "任务状态已变化，请重新查询任务")
}

func writeTaskCancelError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message, "type": "new_api_error"}})
}

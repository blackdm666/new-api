package globalaiopc

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

const (
	ChannelName           = "globalaiopc"
	ModelSeedance25       = "seedance-2.5"
	ModelSeedance25C2     = "seedance-2.5-c2"
	ModelSeedance20Fast   = "seedance-2.0-fast"
	ModelVideosStable     = "videos_stable"
	ModelVideosStableFast = "videos_stable_fast"
	ModelMiniMaxH3        = "minimax-h3"
	ModelKlingO3          = "KlingO3"
	ModelDigitalHuman     = "digitalHuman"

	defaultBaseURL         = "https://zcbservice.aizfw.cn/kyyReactApiServer"
	modelCenterSubmitPath  = "/v2/model-center/tasks"
	modelCenterQueryPath   = "/v2/model-center/tasks/%s"
	digitalHumanSubmitPath = "/v1/digitalHuman/videos"
	digitalHumanQueryPath  = "/v1/result/%s"
	taskActionDigitalHuman = "digitalHuman"
)

type requestPayload struct {
	Model                  string   `json:"model"`
	Prompt                 string   `json:"prompt"`
	ReferenceImages        []string `json:"reference_images,omitempty"`
	ReferenceVideos        []string `json:"reference_videos,omitempty"`
	ReferenceAudios        []string `json:"reference_audios,omitempty"`
	Duration               int      `json:"duration,omitempty"`
	AspectRatio            string   `json:"aspect_ratio,omitempty"`
	Resolution             string   `json:"resolution,omitempty"`
	Size                   string   `json:"size,omitempty"`
	FirstImage             any      `json:"first_image,omitempty"`
	LastImage              any      `json:"last_image,omitempty"`
	Seed                   *int     `json:"seed,omitempty"`
	GenerateAudio          *bool    `json:"generate_audio,omitempty"`
	Tools                  []any    `json:"tools,omitempty"`
	Watermark              *bool    `json:"watermark,omitempty"`
	ReferenceMode          string   `json:"reference_mode,omitempty"`
	AudioSetting           string   `json:"audio_setting,omitempty"`
	RequiresReferenceVideo string   `json:"requires_reference_video,omitempty"`
	VideoEditBasic         string   `json:"video_edit.basic,omitempty"`
	Async                  string   `json:"async,omitempty"`
	AudioReference         string   `json:"audio_reference,omitempty"`
	EndImageURL            string   `json:"end_image_url,omitempty"`
	StartImageURL          string   `json:"start_image_url,omitempty"`
	VideoReference         string   `json:"video_reference,omitempty"`
}

type digitalHumanPayload struct {
	Model        string `json:"model"`
	AudioURL     string `json:"audioUrl"`
	VideoURL     string `json:"videoUrl"`
	ModelVersion int    `json:"modelVersion,omitempty"`
	SideFace     int    `json:"sideFace,omitempty"`
	TiltedFace   int    `json:"tiltedFace,omitempty"`
}

type responsePayload struct {
	ID             string  `json:"id"`
	Object         string  `json:"object"`
	Created        int64   `json:"created"`
	Model          string  `json:"model"`
	Status         string  `json:"status"`
	Progress       any     `json:"progress,omitempty"`
	ResultURL      string  `json:"result_url,omitempty"`
	VideoURL       string  `json:"video_url,omitempty"`
	Amount         float64 `json:"amount,omitempty"`
	ActualDuration float64 `json:"actualDuration,omitempty"`
	Error          any     `json:"error,omitempty"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.apiKey = info.ApiKey
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	if a.baseURL == "" {
		a.baseURL = defaultBaseURL
	}
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	if err := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate); err != nil {
		return err
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "get_task_request_failed", http.StatusBadRequest)
	}
	if err := validateGlobalAiOpcRequest(req, info); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	modelName := taskcommon.DefaultString(info.UpstreamModelName, req.Model)
	if modelName == ModelDigitalHuman {
		info.Action = taskActionDigitalHuman
		return nil
	}
	if hasReferenceInputs(req) {
		info.Action = constant.TaskActionReferenceGenerate
	} else {
		info.Action = constant.TaskActionTextGenerate
	}
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	modelName := taskcommon.DefaultString(info.UpstreamModelName, req.Model)
	cfg, ok := modelConfigs[modelName]
	if !ok || cfg.perCallBilling {
		return nil
	}
	duration := requestDuration(req)
	if duration <= 0 {
		duration = cfg.defaultDuration
	}
	ratios := map[string]float64{"seconds": float64(duration)}
	if cfg.videoReferenceRatio > 0 {
		body, convertErr := convertToRequestPayload(req, info)
		if convertErr == nil && len(body.ReferenceVideos) > 0 {
			ratios["video_reference"] = cfg.videoReferenceRatio
		}
	}
	return ratios
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if isDigitalHumanModel(taskcommon.DefaultString(info.UpstreamModelName, info.OriginModelName)) {
		return a.baseURL + digitalHumanSubmitPath, nil
	}
	return a.baseURL + modelCenterSubmitPath, nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	body, err := convertRequestBody(req, info)
	if err != nil {
		return nil, err
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}

	var upstream responsePayload
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}
	if upstream.ID == "" {
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("task id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}
	if upstream.Status == "failed" {
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("%s", errorMessage(upstream.Error)), "task_failed", http.StatusBadRequest)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return upstream.ID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	baseUrl = strings.TrimRight(baseUrl, "/")
	if baseUrl == "" {
		baseUrl = defaultBaseURL
	}
	queryPath := modelCenterQueryPath
	if action, _ := body["action"].(string); action == taskActionDigitalHuman {
		queryPath = digitalHumanQueryPath
	}
	req, err := http.NewRequest(http.MethodGet, baseUrl+fmt.Sprintf(queryPath, taskID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var upstream responsePayload
	if err := common.Unmarshal(respBody, &upstream); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskInfo := &relaycommon.TaskInfo{
		TaskID:   upstream.ID,
		Progress: progressString(upstream.Progress),
	}
	switch upstream.Status {
	case "queued":
		taskInfo.Status = model.TaskStatusQueued
		if taskInfo.Progress == "" {
			taskInfo.Progress = taskcommon.ProgressQueued
		}
	case "processing":
		taskInfo.Status = model.TaskStatusInProgress
		if taskInfo.Progress == "" {
			taskInfo.Progress = taskcommon.ProgressInProgress
		}
	case "completed":
		taskInfo.Status = model.TaskStatusSuccess
		taskInfo.Progress = taskcommon.ProgressComplete
		taskInfo.Url = firstNonEmpty(upstream.VideoURL, upstream.ResultURL)
	case "failed":
		taskInfo.Status = model.TaskStatusFailure
		taskInfo.Progress = taskcommon.ProgressComplete
		taskInfo.Reason = errorMessage(upstream.Error)
	default:
		return nil, fmt.Errorf("unknown task status: %s", upstream.Status)
	}
	return taskInfo, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return append([]string(nil), supportedModels...)
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var upstream responsePayload
	if err := common.Unmarshal(originTask.Data, &upstream); err != nil {
		return nil, errors.Wrap(err, "unmarshal globalaiopc task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName
	if upstream.ActualDuration > 0 {
		openAIVideo.Seconds = strconv.FormatFloat(upstream.ActualDuration, 'f', -1, 64)
	}
	if resultURL := firstNonEmpty(upstream.VideoURL, upstream.ResultURL); resultURL != "" {
		openAIVideo.SetMetadata("url", resultURL)
	}
	if upstream.Status == "failed" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: errorMessage(upstream.Error),
		}
	}
	return common.Marshal(openAIVideo)
}

func convertRequestBody(req relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (any, error) {
	modelName := taskcommon.DefaultString(info.UpstreamModelName, req.Model)
	if isDigitalHumanModel(modelName) {
		return convertToDigitalHumanPayload(req, modelName)
	}
	return convertToRequestPayload(req, info)
}

func convertToDigitalHumanPayload(req relaycommon.TaskSubmitReq, modelName string) (*digitalHumanPayload, error) {
	body := digitalHumanPayload{Model: ModelDigitalHuman, ModelVersion: 2}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &body); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	body.Model = ModelDigitalHuman
	if strings.TrimSpace(body.AudioURL) == "" {
		return nil, fmt.Errorf("metadata.audioUrl is required")
	}
	if strings.TrimSpace(body.VideoURL) == "" {
		return nil, fmt.Errorf("metadata.videoUrl is required")
	}
	if body.ModelVersion < 0 || body.ModelVersion > 2 {
		return nil, fmt.Errorf("metadata.modelVersion must be 1 or 2")
	}
	if body.SideFace < 0 || body.SideFace > 1 || body.TiltedFace < 0 || body.TiltedFace > 1 {
		return nil, fmt.Errorf("metadata.sideFace and metadata.tiltedFace must be 0 or 1")
	}
	if duration := requestDuration(req); duration < 1 || duration > relaycommon.MaxTaskDurationSeconds {
		return nil, fmt.Errorf("duration is required for digitalHuman billing and must be between 1 and %d", relaycommon.MaxTaskDurationSeconds)
	}
	return &body, nil
}

func convertToRequestPayload(req relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*requestPayload, error) {
	modelName := taskcommon.DefaultString(info.UpstreamModelName, req.Model)
	cfg, ok := modelConfigs[modelName]
	if !ok || cfg.digitalHuman {
		return nil, fmt.Errorf("model must be one of %s", strings.Join((&TaskAdaptor{}).GetModelList(), ", "))
	}
	body := requestPayload{
		Model:      upstreamModelFor(modelName),
		Prompt:     req.Prompt,
		Duration:   requestDuration(req),
		Resolution: cfg.defaultResolution,
	}
	applyRequestImages(&body, req.Images, cfg.imageInputMode)
	if len(cfg.allowedRatioList) > 0 {
		body.AspectRatio = aspectRatioForModel(modelName, req.Size)
	}
	if isAllowedSize(modelName, req.Size) {
		body.Size = req.Size
	}
	if body.Duration == 0 {
		body.Duration = cfg.defaultDuration
	}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &body); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	// The model always defines the billable upstream family. Resolution is also
	// locked for resolution-priced aliases; only models with resolution-invariant
	// upstream pricing may accept a metadata resolution override.
	body.Model = upstreamModelFor(modelName)
	if cfg.lockResolution || body.Resolution == "" {
		body.Resolution = cfg.defaultResolution
	}
	if body.AspectRatio == "" && len(cfg.allowedRatioList) > 0 {
		body.AspectRatio = cfg.defaultRatio
	}
	return &body, validatePayload(body, cfg)
}

func validateGlobalAiOpcRequest(req relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) error {
	_, err := convertRequestBody(req, info)
	return err
}

func validatePayload(body requestPayload, cfg modelConfig) error {
	if cfg.requirePrompt && strings.TrimSpace(body.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	if cfg.maxPromptLength > 0 && len([]rune(body.Prompt)) > cfg.maxPromptLength {
		return fmt.Errorf("prompt must contain at most %d characters", cfg.maxPromptLength)
	}
	if len(body.ReferenceImages) < cfg.minReferenceImages {
		return fmt.Errorf("reference_images must contain at least %d items", cfg.minReferenceImages)
	}
	if len(body.ReferenceImages) > cfg.maxReferenceImages {
		return fmt.Errorf("reference_images must contain at most %d items", cfg.maxReferenceImages)
	}
	if len(body.ReferenceVideos) < cfg.minReferenceVideos {
		return fmt.Errorf("reference_videos must contain at least %d items", cfg.minReferenceVideos)
	}
	if len(body.ReferenceVideos) > cfg.maxReferenceVideos {
		return fmt.Errorf("reference_videos must contain at most %d items", cfg.maxReferenceVideos)
	}
	if len(body.ReferenceAudios) > cfg.maxReferenceAudios {
		return fmt.Errorf("reference_audios must contain at most %d items", cfg.maxReferenceAudios)
	}
	if cfg.requireVisualWithAudio && len(body.ReferenceAudios) > 0 && len(body.ReferenceImages) == 0 && len(body.ReferenceVideos) == 0 && imageValueCount(body.FirstImage) == 0 && imageValueCount(body.LastImage) == 0 {
		return fmt.Errorf("reference_audios requires at least one image or video reference")
	}
	if body.Duration < cfg.minDuration || body.Duration > cfg.maxDuration {
		return fmt.Errorf("duration must be between %d and %d", cfg.minDuration, cfg.maxDuration)
	}
	if _, ok := cfg.allowedResolutions[body.Resolution]; !ok {
		return fmt.Errorf("resolution must be one of %s", strings.Join(cfg.allowedResolutionList, ", "))
	}
	if len(cfg.allowedRatioList) > 0 {
		if _, ok := cfg.allowedRatios[body.AspectRatio]; !ok {
			return fmt.Errorf("aspect_ratio must be one of %s", strings.Join(cfg.allowedRatioList, ", "))
		}
	}
	if body.Size != "" {
		if _, ok := cfg.allowedSizes[body.Size]; !ok {
			return fmt.Errorf("size must be one of %s", strings.Join(cfg.allowedSizeList, ", "))
		}
	}
	if cfg.requireFirstImage && imageValueCount(body.FirstImage) != 1 {
		return fmt.Errorf("first_image must contain exactly one item")
	}
	if cfg.requireLastImage && imageValueCount(body.LastImage) != 1 {
		return fmt.Errorf("last_image must contain exactly one item")
	}
	if imageValueCount(body.FirstImage) > 1 || imageValueCount(body.LastImage) > 1 {
		return fmt.Errorf("first_image and last_image must contain at most one item")
	}
	if cfg.mutuallyExclusiveMedia && (imageValueCount(body.FirstImage) > 0 || imageValueCount(body.LastImage) > 0) && (len(body.ReferenceImages) > 0 || len(body.ReferenceVideos) > 0 || len(body.ReferenceAudios) > 0) {
		return fmt.Errorf("first/last image mode cannot be mixed with reference_images, reference_videos, or reference_audios")
	}
	return nil
}

func applyRequestImages(body *requestPayload, images []string, mode imageInputMode) {
	if len(images) == 0 {
		return
	}
	switch mode {
	case imageInputFirst:
		body.FirstImage = []string{images[0]}
	case imageInputFirstLast:
		body.FirstImage = []string{images[0]}
		if len(images) > 1 {
			body.LastImage = []string{images[1]}
		}
	default:
		body.ReferenceImages = images
	}
}

func imageValueCount(value any) int {
	switch v := value.(type) {
	case nil:
		return 0
	case string:
		if strings.TrimSpace(v) == "" {
			return 0
		}
		return 1
	case []string:
		return len(v)
	case []any:
		return len(v)
	default:
		return 1
	}
}

func requestDuration(req relaycommon.TaskSubmitReq) int {
	if req.Duration > 0 {
		return req.Duration
	}
	if req.Seconds != "" {
		if seconds, err := strconv.Atoi(req.Seconds); err == nil {
			return seconds
		}
	}
	return 0
}

func aspectRatioForModel(modelName, size string) string {
	cfg := configForModel(modelName)
	if _, ok := cfg.allowedRatios[size]; ok {
		return size
	}
	switch size {
	case "1280x720", "1920x1080", "2560x1440":
		return "16:9"
	case "720x1280", "1080x1920", "1440x2560":
		return "9:16"
	case "1024x1024", "512x512", "1440x1440":
		return "1:1"
	case "1920x1440":
		return "4:3"
	case "1440x1920":
		return "3:4"
	case "3360x1440":
		return "21:9"
	default:
		return cfg.defaultRatio
	}
}

func isAllowedSize(modelName, size string) bool {
	if size == "" {
		return false
	}
	_, ok := configForModel(modelName).allowedSizes[size]
	return ok
}

func hasReferenceInputs(req relaycommon.TaskSubmitReq) bool {
	if len(req.Images) > 0 {
		return true
	}
	body := requestPayload{}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &body); err != nil {
		return false
	}
	return len(body.ReferenceImages) > 0 || len(body.ReferenceVideos) > 0 || len(body.ReferenceAudios) > 0 || imageValueCount(body.FirstImage) > 0 || imageValueCount(body.LastImage) > 0
}

type imageInputMode int

const (
	imageInputReferences imageInputMode = iota
	imageInputFirst
	imageInputFirstLast
)

type modelConfig struct {
	upstreamModel          string
	digitalHuman           bool
	perCallBilling         bool
	videoReferenceRatio    float64
	defaultDuration        int
	minDuration            int
	maxDuration            int
	defaultRatio           string
	defaultResolution      string
	lockResolution         bool
	requirePrompt          bool
	maxPromptLength        int
	minReferenceImages     int
	maxReferenceImages     int
	minReferenceVideos     int
	maxReferenceVideos     int
	maxReferenceAudios     int
	requireVisualWithAudio bool
	requireFirstImage      bool
	requireLastImage       bool
	mutuallyExclusiveMedia bool
	imageInputMode         imageInputMode
	allowedRatios          map[string]struct{}
	allowedRatioList       []string
	allowedResolutions     map[string]struct{}
	allowedResolutionList  []string
	allowedSizes           map[string]struct{}
	allowedSizeList        []string
}

var (
	supportedModels, modelConfigs = buildModelConfigs()
)

func buildModelConfigs() ([]string, map[string]modelConfig) {
	models := make([]string, 0, 40)
	configs := make(map[string]modelConfig)
	add := func(name string, cfg modelConfig) {
		cfg.allowedRatios = stringSet(cfg.allowedRatioList)
		cfg.allowedResolutions = stringSet(cfg.allowedResolutionList)
		cfg.allowedSizes = stringSet(cfg.allowedSizeList)
		models = append(models, name)
		configs[name] = cfg
	}

	add(ModelSeedance25, modelConfig{
		upstreamModel: ModelSeedance25, defaultDuration: 4, minDuration: 4, maxDuration: 30,
		defaultRatio: "9:16", defaultResolution: "720p", lockResolution: true,
		requirePrompt: true, maxPromptLength: 2000, maxReferenceImages: 30, maxReferenceAudios: 10,
		allowedRatioList: []string{"16:9", "9:16", "1:1"}, allowedResolutionList: []string{"720p"},
	})
	add(ModelSeedance25C2, modelConfig{
		upstreamModel: ModelSeedance25C2, defaultDuration: 5, minDuration: 4, maxDuration: 29,
		defaultRatio: "16:9", defaultResolution: "720p", lockResolution: true,
		requirePrompt: true, maxPromptLength: 2000, maxReferenceImages: 30, maxReferenceVideos: 10, maxReferenceAudios: 10,
		allowedRatioList: []string{"16:9", "9:16", "1:1"}, allowedResolutionList: []string{"720p"},
	})
	add(ModelMiniMaxH3, modelConfig{
		upstreamModel: ModelMiniMaxH3, perCallBilling: true, defaultDuration: 5, minDuration: 5, maxDuration: 15,
		defaultRatio: "16:9", defaultResolution: "2k", lockResolution: true,
		requirePrompt: true, maxPromptLength: 2000, maxReferenceImages: 5, maxReferenceAudios: 1, requireVisualWithAudio: true,
		allowedRatioList: []string{"16:9", "9:16", "1:1", "4:3", "3:4", "21:9"}, allowedResolutionList: []string{"2k"},
		allowedSizeList: []string{"2560x1440", "1440x2560", "1440x1440", "1920x1440", "1440x1920", "3360x1440"},
	})

	for _, item := range []struct {
		name, resolution string
		videoRatio       float64
	}{
		{"seedance-2.0-720p", "720p", 1.27 / 0.58},
		{"seedance-2.0-1080p", "1080p", 1.30 / 0.60},
		{"seedance-2.0-2k", "2k", 1.33 / 0.70},
		{"seedance-2.0-4k", "4k", 1.35 / 0.70},
	} {
		add(item.name, modelConfig{
			upstreamModel: "sd_2.0_special", videoReferenceRatio: item.videoRatio,
			defaultDuration: 5, minDuration: 4, maxDuration: 15, defaultRatio: "16:9",
			defaultResolution: item.resolution, lockResolution: true, requirePrompt: true, maxPromptLength: 5000,
			maxReferenceImages: 9, maxReferenceVideos: 3, maxReferenceAudios: 3, requireVisualWithAudio: true, mutuallyExclusiveMedia: true,
			allowedRatioList: []string{"16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "adaptive"}, allowedResolutionList: []string{item.resolution},
		})
	}
	add(ModelSeedance20Fast, modelConfig{
		upstreamModel: "sd_2.0_fast_special", videoReferenceRatio: 1.05 / 0.50,
		defaultDuration: 5, minDuration: 4, maxDuration: 15, defaultRatio: "16:9", defaultResolution: "720p", lockResolution: true,
		requirePrompt: true, maxPromptLength: 5000, maxReferenceImages: 9, maxReferenceVideos: 3, maxReferenceAudios: 3,
		requireVisualWithAudio: true, mutuallyExclusiveMedia: true,
		allowedRatioList: []string{"16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "adaptive"}, allowedResolutionList: []string{"720p"},
	})

	for _, name := range []string{ModelVideosStable, ModelVideosStableFast} {
		add(name, modelConfig{
			upstreamModel: name, perCallBilling: true, defaultDuration: 4, minDuration: 4, maxDuration: 15,
			defaultRatio: "16:9", defaultResolution: "720p", lockResolution: true,
			requirePrompt: true, maxPromptLength: 5000, maxReferenceImages: 4, maxReferenceVideos: 3, maxReferenceAudios: 1,
			allowedRatioList: []string{"9:16", "16:9", "1:1"}, allowedResolutionList: []string{"720p"},
		})
	}

	happyHorseFamilies := []struct {
		name              string
		minImages         int
		maxImages         int
		mode              imageInputMode
		requireFirstImage bool
		ratios            []string
	}{
		{"happyhorse-1.0-t2v", 0, 0, imageInputReferences, false, []string{"16:9", "9:16", "1:1", "4:3", "3:4", "4:5", "5:4", "9:21", "21:9"}},
		{"happyhorse-1.0-i2v", 0, 0, imageInputFirst, true, nil},
		{"happyhorse-1.0-r2v", 1, 9, imageInputReferences, false, []string{"16:9", "9:16", "3:4", "4:3", "4:5", "5:4", "1:1", "9:21", "21:9"}},
		{"happyhorse-1.1-t2v", 0, 0, imageInputReferences, false, []string{"16:9", "9:16", "1:1", "4:3", "3:4", "4:5", "5:4", "9:21", "21:9"}},
		{"happyhorse-1.1-i2v", 0, 0, imageInputFirst, true, nil},
		{"happyhorse-1.1-r2v", 1, 9, imageInputReferences, false, []string{"16:9", "9:16", "3:4", "4:3", "4:5", "5:4", "1:1", "9:21", "21:9"}},
		{"hh-1.1-t2v-o", 0, 0, imageInputReferences, false, []string{"16:9", "9:16", "3:4", "4:3", "4:5", "5:4", "1:1", "9:21", "21:9"}},
		{"hh-1.1-i2v-o", 0, 0, imageInputFirst, true, nil},
		{"hh-1.1-r2v-o", 1, 9, imageInputReferences, false, []string{"16:9", "9:16", "3:4", "4:3", "4:5", "5:4", "1:1", "9:21", "21:9"}},
	}
	for _, family := range happyHorseFamilies {
		for _, resolution := range []string{"720P", "1080P"} {
			publicName := family.name + "-" + strings.ToLower(resolution)
			add(publicName, modelConfig{
				upstreamModel: family.name, defaultDuration: 5, minDuration: 3, maxDuration: 15,
				defaultRatio: "16:9", defaultResolution: resolution, lockResolution: true,
				requirePrompt: true, maxPromptLength: 5000, minReferenceImages: family.minImages, maxReferenceImages: family.maxImages,
				requireFirstImage: family.requireFirstImage, imageInputMode: family.mode,
				allowedRatioList: family.ratios, allowedResolutionList: []string{resolution},
			})
		}
	}

	wanRatios := []string{"16:9", "9:16", "1:1", "4:3", "3:4"}
	add("wan2.7-t2v", modelConfig{upstreamModel: "wan2.7-t2v", defaultDuration: 5, minDuration: 4, maxDuration: 15, defaultRatio: "16:9", defaultResolution: "1080P", lockResolution: true, requirePrompt: true, maxPromptLength: 5000, allowedRatioList: wanRatios, allowedResolutionList: []string{"1080P"}})
	add("wan2.7-i2v", modelConfig{upstreamModel: "wan2.7-i2v", defaultDuration: 5, minDuration: 4, maxDuration: 15, defaultResolution: "1080P", lockResolution: true, requirePrompt: true, maxPromptLength: 5000, requireFirstImage: true, requireLastImage: true, imageInputMode: imageInputFirstLast, allowedResolutionList: []string{"1080P"}})
	add("wan2.7-r2v", modelConfig{upstreamModel: "wan2.7-r2v", defaultDuration: 5, minDuration: 4, maxDuration: 15, defaultRatio: "16:9", defaultResolution: "1080P", lockResolution: true, requirePrompt: true, maxPromptLength: 5000, minReferenceImages: 1, maxReferenceImages: 3, allowedRatioList: wanRatios, allowedResolutionList: []string{"1080P"}})
	add("wan2.7-videoedit", modelConfig{upstreamModel: "wan2.7-videoedit", defaultDuration: 5, minDuration: 1, maxDuration: 3600, defaultRatio: "16:9", defaultResolution: "1080P", lockResolution: true, requirePrompt: true, maxPromptLength: 5000, maxReferenceImages: 4, minReferenceVideos: 1, maxReferenceVideos: 1, allowedRatioList: wanRatios, allowedResolutionList: []string{"1080P"}})

	add(ModelKlingO3, modelConfig{
		upstreamModel: ModelKlingO3, perCallBilling: true, defaultDuration: 6, minDuration: 3, maxDuration: 15,
		defaultRatio: "16:9", defaultResolution: "720p", requirePrompt: true, maxPromptLength: 5000, maxReferenceImages: 3,
		allowedRatioList: []string{"16:9", "9:16", "1:1"}, allowedResolutionList: []string{"720p", "1080p"},
	})
	add(ModelDigitalHuman, modelConfig{upstreamModel: ModelDigitalHuman, digitalHuman: true, defaultDuration: 1, minDuration: 1, maxDuration: 3600, requirePrompt: true})

	return models, configs
}

func configForModel(modelName string) modelConfig {
	if cfg, ok := modelConfigs[modelName]; ok {
		return cfg
	}
	return modelConfigs[ModelSeedance25]
}

func upstreamModelFor(modelName string) string {
	if cfg, ok := modelConfigs[modelName]; ok && cfg.upstreamModel != "" {
		return cfg.upstreamModel
	}
	return modelName
}

func isDigitalHumanModel(modelName string) bool {
	return modelName == ModelDigitalHuman
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func progressString(progress any) string {
	switch v := progress.(type) {
	case float64:
		return fmt.Sprintf("%.0f%%", v)
	case string:
		if strings.HasSuffix(v, "%") {
			return v
		}
		if v != "" {
			return v + "%"
		}
	}
	return ""
}

func errorMessage(v any) string {
	switch errValue := v.(type) {
	case nil:
		return "task failed"
	case string:
		if errValue != "" {
			return errValue
		}
	case map[string]any:
		if msg, ok := errValue["message"].(string); ok && msg != "" {
			return msg
		}
		if code, ok := errValue["code"].(string); ok && code != "" {
			return code
		}
	}
	return fmt.Sprintf("%v", v)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

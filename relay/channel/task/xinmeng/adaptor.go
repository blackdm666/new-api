package xinmeng

import (
	"bytes"
	"encoding/json"
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
	ChannelName = "XinMeng Video"

	ModelDVCSeedance25        = "dvc-seedance-2.5"
	ModelDVCSeedance20        = "dvc-seedance-2.0"
	ModelMiniMaxH3768P        = "minimax-h3-768p"
	ModelDoubaoSeedance25     = "doubao-seedance-2-5-720p"
	ModelDoubaoSeedance20     = "doubao-seedance-2-0-720p"
	ModelDoubaoSeedance20Fast = "doubao-seedance-2-0-fast-720p"
	ModelSeedance20Mini480P   = "seedance-2.0-mini-480p"
	ModelSeedance20Mini720P   = "seedance-2.0-mini-720p"
	ModelWan30Video720P       = "wan3.0-video-720p"
	ModelWan30Video1080P      = "wan3.0-video-1080p"
	ModelKling30Turbo         = "kling-3.0-turbo"
	ModelKling30Turbo720P     = "kling-3.0-turbo-720p"
	ModelKling30Turbo1080P    = "kling-3.0-turbo-1080p"
	ModelKling30Turbo2K       = "kling-3.0-turbo-2k"
	ModelKling30Turbo4K       = "kling-3.0-turbo-4k"

	defaultBaseURL = "https://www.jimengvip.online"
	submitPath     = "/v1/videos/generations"
	queryPath      = "/v1/tasks/%s"
)

// ModelList seeds channel creation and Playground metadata. Request-time
// support is not limited to this list; unknown names are checked against the
// authenticated XinMeng /v1/models catalog.
var ModelList = []string{
	ModelDVCSeedance25,
	ModelDVCSeedance20,
	ModelMiniMaxH3768P,
	ModelDoubaoSeedance25,
	ModelDoubaoSeedance20,
	ModelDoubaoSeedance20Fast,
	ModelSeedance20Mini480P,
	ModelSeedance20Mini720P,
	ModelWan30Video720P,
	ModelWan30Video1080P,
	ModelKling30Turbo720P,
	ModelKling30Turbo1080P,
	ModelKling30Turbo2K,
	ModelKling30Turbo4K,
}

type modelConfig struct {
	upstreamModel          string
	defaultDuration        int
	minDuration            int
	maxDuration            int
	defaultRatio           string
	resolution             string
	allowedDurations       map[int]struct{}
	maxPromptLength        int
	maxReferenceImages     int
	maxReferenceVideos     int
	maxReferenceAudios     int
	maxReferenceMedia      int
	requireVisualWithAudio bool
	framesExclusive        bool
	allowPromptlessMedia   bool
	useNativeMediaFields   bool
	supportsGenerateAudio  bool
	allowedRatios          map[string]struct{}
}

var modelConfigs = map[string]modelConfig{
	ModelDVCSeedance25: {
		upstreamModel: ModelDVCSeedance25, defaultDuration: 5, minDuration: 4, maxDuration: 30,
		defaultRatio: "auto", resolution: "720p", maxPromptLength: 5000,
		maxReferenceImages: 30, maxReferenceVideos: 10, maxReferenceAudios: 10,
		framesExclusive: true, supportsGenerateAudio: true,
		allowedRatios: stringSet("auto", "1:1", "21:9", "16:9", "9:16", "3:4", "4:3"),
	},
	ModelDVCSeedance20: {
		upstreamModel: ModelDVCSeedance20, defaultDuration: 5, minDuration: 4, maxDuration: 15,
		defaultRatio: "16:9", resolution: "720p", maxPromptLength: 5000,
		maxReferenceImages: 9, maxReferenceVideos: 3, maxReferenceAudios: 3, maxReferenceMedia: 12,
		framesExclusive: true,
		allowedRatios:   stringSet("1:1", "21:9", "16:9", "9:16", "3:4", "4:3"),
	},
	ModelMiniMaxH3768P: {
		upstreamModel: ModelMiniMaxH3768P, defaultDuration: 4, minDuration: 4, maxDuration: 15,
		defaultRatio: "16:9", resolution: "768p", maxPromptLength: 2500,
		maxReferenceImages: 10, maxReferenceVideos: 5, maxReferenceAudios: 5, requireVisualWithAudio: true,
		allowedRatios: stringSet("1:1", "16:9", "9:16"),
	},
	ModelDoubaoSeedance25: {
		upstreamModel: ModelDoubaoSeedance25, defaultDuration: 4, minDuration: 4, maxDuration: 30,
		defaultRatio: "16:9", resolution: "720p", maxPromptLength: 5000,
		maxReferenceImages: 30, maxReferenceVideos: 10, maxReferenceAudios: 10,
		framesExclusive: true,
		allowedRatios:   stringSet("16:9", "9:16", "1:1", "4:3", "3:4", "21:9"),
	},
	ModelDoubaoSeedance20: {
		upstreamModel: ModelDoubaoSeedance20, defaultDuration: 4, minDuration: 4, maxDuration: 15,
		defaultRatio: "16:9", resolution: "720p", maxPromptLength: 5000,
		maxReferenceImages: 9, maxReferenceVideos: 3, maxReferenceAudios: 3,
		framesExclusive: true,
		allowedRatios:   stringSet("16:9", "9:16", "1:1", "4:3", "3:4", "21:9"),
	},
	ModelDoubaoSeedance20Fast: {
		upstreamModel: ModelDoubaoSeedance20Fast, defaultDuration: 4, minDuration: 4, maxDuration: 15,
		defaultRatio: "16:9", resolution: "720p", maxPromptLength: 5000,
		maxReferenceImages: 9, maxReferenceVideos: 3, maxReferenceAudios: 3,
		framesExclusive: true,
		allowedRatios:   stringSet("16:9", "9:16", "1:1", "4:3", "3:4", "21:9"),
	},
	ModelSeedance20Mini480P: {
		upstreamModel: ModelSeedance20Mini480P, defaultDuration: 5, minDuration: 4, maxDuration: 15,
		defaultRatio: "16:9", resolution: "480p", maxPromptLength: 5000,
		maxReferenceImages: 9, maxReferenceVideos: 3, maxReferenceAudios: 3,
		allowedRatios: stringSet("16:9", "9:16", "1:1", "4:3", "3:4", "21:9"),
	},
	ModelSeedance20Mini720P: {
		upstreamModel: ModelSeedance20Mini720P, defaultDuration: 5, minDuration: 4, maxDuration: 15,
		defaultRatio: "16:9", resolution: "720p", maxPromptLength: 5000,
		maxReferenceImages: 9, maxReferenceVideos: 3, maxReferenceAudios: 3,
		allowedRatios: stringSet("16:9", "9:16", "1:1", "4:3", "3:4", "21:9"),
	},
	ModelWan30Video720P: {
		upstreamModel: ModelWan30Video720P, defaultDuration: 5, minDuration: 4, maxDuration: 30,
		defaultRatio: "16:9", resolution: "720p", maxPromptLength: 5000,
		maxReferenceImages: 10, maxReferenceVideos: 5, maxReferenceAudios: 5,
		allowPromptlessMedia: true, framesExclusive: true,
		allowedRatios: stringSet("16:9", "9:16", "1:1", "4:3", "3:4"),
	},
	ModelWan30Video1080P: {
		upstreamModel: ModelWan30Video1080P, defaultDuration: 5, minDuration: 4, maxDuration: 30,
		defaultRatio: "16:9", resolution: "1080p", maxPromptLength: 5000,
		maxReferenceImages: 10, maxReferenceVideos: 5, maxReferenceAudios: 5,
		allowPromptlessMedia: true, framesExclusive: true,
		allowedRatios: stringSet("16:9", "9:16", "1:1", "4:3", "3:4"),
	},
	ModelKling30Turbo720P:  kling30TurboConfig("720p"),
	ModelKling30Turbo1080P: kling30TurboConfig("1080p"),
	ModelKling30Turbo2K:    kling30TurboConfig("2k"),
	ModelKling30Turbo4K:    kling30TurboConfig("4k"),
}

func kling30TurboConfig(resolution string) modelConfig {
	return modelConfig{
		upstreamModel: ModelKling30Turbo, defaultDuration: 5, minDuration: 4, maxDuration: 15,
		defaultRatio: "16:9", resolution: resolution, maxPromptLength: 2000,
		maxReferenceImages: 30, maxReferenceVideos: 10,
		useNativeMediaFields: true, supportsGenerateAudio: true,
		allowedRatios: stringSet("16:9", "9:16", "1:1"),
	}
}

type requestPayload struct {
	Model               string                   `json:"model"`
	Prompt              string                   `json:"prompt,omitempty"`
	NegativePrompt      string                   `json:"negative_prompt,omitempty"`
	Ratio               string                   `json:"ratio"`
	Duration            int                      `json:"duration"`
	Resolution          string                   `json:"resolution"`
	ReferenceImages     []string                 `json:"referenceImages,omitempty"`
	ReferenceVideos     []string                 `json:"referenceVideos,omitempty"`
	ReferenceAudios     []string                 `json:"referenceAudios,omitempty"`
	FirstFrame          string                   `json:"firstFrame,omitempty"`
	LastFrame           string                   `json:"lastFrame,omitempty"`
	Media               []map[string]interface{} `json:"media,omitempty"`
	Images              []interface{}            `json:"images,omitempty"`
	Videos              []string                 `json:"videos,omitempty"`
	Seed                *int                     `json:"seed,omitempty"`
	GenerateAudio       *bool                    `json:"generateAudio,omitempty"`
	GenerateAudioNative *bool                    `json:"generate_audio,omitempty"`
	CameraControl       map[string]interface{}   `json:"camera_control,omitempty"`
}

type requestMetadata struct {
	Ratio       string `json:"ratio,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Resolution  string `json:"resolution,omitempty"`
	Quality     string `json:"quality,omitempty"`
	VQuality    string `json:"vquality,omitempty"`

	ReferenceImages      []string      `json:"referenceImages,omitempty"`
	ReferenceImagesSnake []string      `json:"reference_images,omitempty"`
	ImageURLs            []string      `json:"image_urls,omitempty"`
	Images               []interface{} `json:"images,omitempty"`
	Image                string        `json:"image,omitempty"`
	FilePaths            []string      `json:"file_paths,omitempty"`

	ReferenceVideos      []string `json:"referenceVideos,omitempty"`
	ReferenceVideosSnake []string `json:"reference_videos,omitempty"`
	VideoURLs            []string `json:"video_urls,omitempty"`
	Videos               []string `json:"videos,omitempty"`

	ReferenceAudios      []string `json:"referenceAudios,omitempty"`
	ReferenceAudiosSnake []string `json:"reference_audios,omitempty"`
	AudioURLs            []string `json:"audio_urls,omitempty"`
	Audios               []string `json:"audios,omitempty"`

	FirstFrame      string `json:"firstFrame,omitempty"`
	FirstImage      any    `json:"first_image,omitempty"`
	LastFrame       string `json:"lastFrame,omitempty"`
	LastImage       any    `json:"last_image,omitempty"`
	GenerateAudio   *bool  `json:"generateAudio,omitempty"`
	GenerateAudioV1 *bool  `json:"generate_audio,omitempty"`
	Seed            *int   `json:"seed,omitempty"`
	NegativePrompt  string `json:"negative_prompt,omitempty"`

	Media         []map[string]interface{} `json:"media,omitempty"`
	CameraControl map[string]interface{}   `json:"camera_control,omitempty"`
}

type upstreamResponse struct {
	ID             string      `json:"id,omitempty"`
	TaskID         string      `json:"task_id,omitempty"`
	Object         string      `json:"object,omitempty"`
	Model          string      `json:"model,omitempty"`
	Status         string      `json:"status,omitempty"`
	Progress       any         `json:"progress,omitempty"`
	Created        int64       `json:"created,omitempty"`
	Result         string      `json:"result,omitempty"`
	ResultURL      string      `json:"result_url,omitempty"`
	VideoURL       string      `json:"video_url,omitempty"`
	Data           interface{} `json:"data,omitempty"`
	PointsCost     float64     `json:"points_cost,omitempty"`
	ActualDuration float64     `json:"actualDuration,omitempty"`
	Error          any         `json:"error,omitempty"`
	Message        string      `json:"message,omitempty"`
}

type upstreamModelListResponse struct {
	Data []upstreamModelCapability `json:"data"`
}

type upstreamModelCapability struct {
	ID                 string   `json:"id"`
	Code               string   `json:"code"`
	Category           string   `json:"category"`
	SupportedRatios    []string `json:"supportedRatios"`
	SupportedQualities []string `json:"supportedQualities"`
	SupportedDurations []int    `json:"supportedDurations"`
	DefaultRatio       string   `json:"defaultRatio"`
	DefaultQuality     string   `json:"defaultQuality"`
	DefaultDuration    int      `json:"defaultDuration"`
	VideoModes         []string `json:"videoModes"`
	SupportsVideoRef   int      `json:"supportsVideoRef"`
	SupportsAudioRef   int      `json:"supportsAudioRef"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	baseURL       string
	apiKey        string
	proxy         string
	requestConfig *modelConfig
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.baseURL = normalizeBaseURL(info.ChannelBaseUrl)
	a.apiKey = info.ApiKey
	a.proxy = info.ChannelMeta.ChannelSetting.Proxy
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	if err := relaycommon.ValidateTaskRequestAllowMedia(c, info, constant.TaskActionTextToVideo); err != nil {
		return err
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return localTaskError(err)
	}
	cfg, err := a.resolveModelConfig(req, info)
	if err != nil {
		return localTaskError(err)
	}
	a.requestConfig = &cfg
	body, err := convertToRequestPayloadWithConfig(req, cfg)
	if err != nil {
		return localTaskError(err)
	}
	if hasReferenceInputs(body) {
		info.Action = constant.TaskActionReferenceToVideo
	} else {
		info.Action = constant.TaskActionTextToVideo
	}
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	cfg := a.requestConfig
	if cfg == nil {
		if _, configured, ok := configuredModelForRequest(info, req.Model); ok {
			cfg = &configured
		}
	}
	if cfg == nil {
		return nil
	}
	return map[string]float64{"seconds": float64(requestDuration(req, *cfg))}
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + submitPath, nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	cfg := a.requestConfig
	if cfg == nil {
		if _, configured, ok := configuredModelForRequest(info, req.Model); ok {
			cfg = &configured
		}
	}
	if cfg == nil {
		return nil, fmt.Errorf("XinMeng model capabilities were not initialized")
	}
	body, err := convertToRequestPayloadWithConfig(req, *cfg)
	if err != nil {
		return nil, err
	}
	payload, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(payload), nil
}
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) ParseResponse(_ *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*channel.TaskSubmitResponse, *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var upstream upstreamResponse
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		return nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	upstreamID := firstNonEmpty(upstream.ID, upstream.TaskID)
	if upstreamID == "" {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("task id is empty"), "invalid_response", http.StatusInternalServerError)
	}
	status := publicVideoStatus(upstream.Status)
	if status == dto.VideoStatusFailed {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("%s", upstreamErrorMessage(upstream)), "task_failed", http.StatusBadRequest)
	}
	if status == dto.VideoStatusUnknown {
		status = dto.VideoStatusQueued
	}
	createdAt := upstream.Created
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}
	video := dto.NewOpenAIVideo()
	video.ID = info.PublicTaskID
	video.TaskID = info.PublicTaskID
	video.Model = info.OriginModelName
	video.Status = status
	video.Progress = progressPercent(upstream.Progress)
	video.CreatedAt = createdAt
	return &channel.TaskSubmitResponse{UpstreamTaskID: upstreamID, TaskData: responseBody, ClientResponse: video}, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	requestURL := normalizeBaseURL(baseURL) + fmt.Sprintf(queryPath, taskID)
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var upstream upstreamResponse
	if err := common.Unmarshal(respBody, &upstream); err != nil {
		return nil, errors.Wrap(err, "unmarshal XinMeng task result failed")
	}
	result := &relaycommon.TaskInfo{
		TaskID: firstNonEmpty(upstream.ID, upstream.TaskID),
	}
	progress := progressPercent(upstream.Progress)
	switch strings.ToLower(strings.TrimSpace(upstream.Status)) {
	case "pending", "queued", "submitted":
		result.Status = model.TaskStatusQueued
	case "processing", "in_progress", "running":
		result.Status = model.TaskStatusInProgress
	case "completed", "success", "succeeded", "done":
		result.Status = model.TaskStatusSuccess
		result.Url = firstNonEmpty(upstream.Result, upstream.VideoURL, upstream.ResultURL, upstreamDataURL(upstream.Data))
	case "failed", "cancelled", "expired":
		result.Status = model.TaskStatusFailure
		result.Reason = upstreamErrorMessage(upstream)
	default:
		return nil, fmt.Errorf("unknown XinMeng task status: %s", upstream.Status)
	}
	if progress > 0 && progress < 100 {
		result.Progress = fmt.Sprintf("%d%%", progress)
	} else if result.Status == model.TaskStatusSuccess || result.Status == model.TaskStatusFailure {
		result.Progress = taskcommon.ProgressComplete
	}
	return result, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	video := task.ToOpenAIVideo()
	video.TaskID = task.TaskID
	if task.Status == model.TaskStatusFailure {
		video.Error = &dto.OpenAIVideoError{
			Message: firstNonEmpty(task.FailReason, "video generation failed"),
			Code:    "video_generation_failed",
		}
	}
	return common.Marshal(video)
}

func (a *TaskAdaptor) GetModelList() []string {
	return append([]string(nil), ModelList...)
}

func (a *TaskAdaptor) GetChannelName() string { return ChannelName }

func (a *TaskAdaptor) SupportsPromptOnlyVideo(modelName string) bool {
	return strings.TrimSpace(modelName) != ""
}

func (a *TaskAdaptor) SupportsDynamicTaskModels() bool { return true }

func configuredModelForRequest(info *relaycommon.RelayInfo, requestModel string) (string, modelConfig, bool) {
	for _, modelName := range taskcommon.ModelConfigCandidates(info, requestModel) {
		if cfg, ok := modelConfigs[modelName]; ok {
			if info != nil && info.ChannelMeta != nil && info.IsModelMapped {
				if upstreamModel := strings.TrimSpace(info.UpstreamModelName); upstreamModel != "" {
					cfg.upstreamModel = upstreamModel
				}
			}
			return modelName, cfg, true
		}
	}
	return "", modelConfig{}, false
}

func (a *TaskAdaptor) resolveModelConfig(req relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (modelConfig, error) {
	if _, cfg, ok := configuredModelForRequest(info, req.Model); ok {
		return cfg, nil
	}
	upstreamModel := taskcommon.ResolvedModelName(info, req.Model)
	if upstreamModel == "" {
		return modelConfig{}, fmt.Errorf("model is required")
	}
	if upstreamModel == ModelKling30Turbo {
		return modelConfig{}, fmt.Errorf("model %s must use a fixed-resolution sales alias", ModelKling30Turbo)
	}
	capability, err := a.fetchUpstreamModelCapability(upstreamModel)
	if err != nil {
		return modelConfig{}, err
	}
	return dynamicModelConfig(capability)
}

func (a *TaskAdaptor) fetchUpstreamModelCapability(modelName string) (upstreamModelCapability, error) {
	req, err := http.NewRequest(http.MethodGet, a.baseURL+"/v1/models", nil)
	if err != nil {
		return upstreamModelCapability{}, err
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(a.proxy)
	if err != nil {
		return upstreamModelCapability{}, fmt.Errorf("new proxy http client failed: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return upstreamModelCapability{}, fmt.Errorf("fetch XinMeng model catalog failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return upstreamModelCapability{}, fmt.Errorf("fetch XinMeng model catalog failed with status %d", resp.StatusCode)
	}
	var catalog upstreamModelListResponse
	if err := common.DecodeJson(resp.Body, &catalog); err != nil {
		return upstreamModelCapability{}, fmt.Errorf("decode XinMeng model catalog failed: %w", err)
	}
	for _, capability := range catalog.Data {
		candidate := firstNonEmpty(capability.ID, capability.Code)
		if candidate == modelName && (capability.Category == "" || capability.Category == "video") {
			return capability, nil
		}
	}
	return upstreamModelCapability{}, fmt.Errorf("model %s is not present in the XinMeng upstream catalog", modelName)
}

func dynamicModelConfig(capability upstreamModelCapability) (modelConfig, error) {
	modelName := firstNonEmpty(capability.ID, capability.Code)
	if modelName == "" {
		return modelConfig{}, fmt.Errorf("XinMeng model catalog returned an empty model name")
	}
	if len(capability.SupportedQualities) != 1 {
		return modelConfig{}, fmt.Errorf("model %s exposes %d resolutions and requires fixed-resolution sales aliases", modelName, len(capability.SupportedQualities))
	}
	if len(capability.SupportedDurations) == 0 {
		return modelConfig{}, fmt.Errorf("model %s does not advertise supported durations", modelName)
	}
	minDuration := capability.SupportedDurations[0]
	maxDuration := capability.SupportedDurations[0]
	for _, duration := range capability.SupportedDurations[1:] {
		if duration < minDuration {
			minDuration = duration
		}
		if duration > maxDuration {
			maxDuration = duration
		}
	}
	modes := stringSet(capability.VideoModes...)
	cfg := modelConfig{
		upstreamModel: modelName, defaultDuration: capability.DefaultDuration,
		minDuration: minDuration, maxDuration: maxDuration,
		defaultRatio: firstNonEmpty(capability.DefaultRatio, "16:9"),
		resolution:   capability.SupportedQualities[0], maxPromptLength: 5000,
		allowedDurations:     intSet(capability.SupportedDurations...),
		allowedRatios:        stringSet(capability.SupportedRatios...),
		allowPromptlessMedia: true,
	}
	if cfg.defaultDuration == 0 {
		cfg.defaultDuration = minDuration
	}
	if _, ok := modes["reference_image"]; ok {
		cfg.maxReferenceImages = 30
	}
	if _, ok := modes["first_last_frame"]; ok {
		cfg.maxReferenceImages = 30
		cfg.framesExclusive = true
	}
	if _, ok := modes["reference_video"]; ok || capability.SupportsVideoRef > 0 {
		cfg.maxReferenceVideos = 10
	}
	if _, ok := modes["reference_audio"]; ok || capability.SupportsAudioRef > 0 {
		cfg.maxReferenceAudios = 10
	}
	return cfg, nil
}

func convertToRequestPayload(req relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*requestPayload, error) {
	_, cfg, ok := configuredModelForRequest(info, req.Model)
	if !ok {
		return nil, fmt.Errorf("model %s requires live XinMeng capability discovery", taskcommon.ResolvedModelName(info, req.Model))
	}
	return convertToRequestPayloadWithConfig(req, cfg)
}

func convertToRequestPayloadWithConfig(req relaycommon.TaskSubmitReq, cfg modelConfig) (*requestPayload, error) {
	metadata := requestMetadata{}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &metadata); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	body := &requestPayload{
		Model:          cfg.upstreamModel,
		Prompt:         strings.TrimSpace(req.Prompt),
		NegativePrompt: firstNonEmpty(req.NegativePrompt, metadata.NegativePrompt),
		Ratio:          requestRatio(req, metadata, cfg),
		Duration:       requestDuration(req, cfg),
		Resolution:     requestResolution(metadata, cfg),
		Seed:           firstNonNilInt(req.Seed, metadata.Seed),
		Media:          firstNonEmptyMedia(req.Media, metadata.Media),
		CameraControl:  firstNonEmptyMap(req.CameraControl, metadata.CameraControl),
	}
	metadataImages := stringValues(metadata.Images)
	body.ReferenceImages = firstNonEmptySlice(
		requestImages(req), metadata.ReferenceImages, metadata.ReferenceImagesSnake,
		metadata.ImageURLs, metadataImages, singleStringSlice(metadata.Image), metadata.FilePaths,
	)
	body.ReferenceVideos = firstNonEmptySlice(metadata.ReferenceVideos, metadata.ReferenceVideosSnake, metadata.VideoURLs, metadata.Videos)
	body.ReferenceAudios = firstNonEmptySlice(metadata.ReferenceAudios, metadata.ReferenceAudiosSnake, metadata.AudioURLs, metadata.Audios)
	body.FirstFrame = firstNonEmpty(metadata.FirstFrame, firstString(metadata.FirstImage))
	body.LastFrame = firstNonEmpty(metadata.LastFrame, firstString(metadata.LastImage))
	generateAudio := firstNonNilBool(req.GenerateAudio, metadata.GenerateAudio, metadata.GenerateAudioV1)
	if cfg.useNativeMediaFields {
		body.Images = firstNonEmptyInterfaces(metadata.Images, stringsToInterfaces(requestImages(req)))
		if body.FirstFrame != "" {
			body.Images = append(body.Images, map[string]interface{}{"url": body.FirstFrame, "role": "first_frame"})
		}
		if body.LastFrame != "" {
			body.Images = append(body.Images, map[string]interface{}{"url": body.LastFrame, "role": "last_frame"})
		}
		body.Videos = firstNonEmptySlice(req.Videos, singleStringSlice(req.Video), metadata.ReferenceVideos, metadata.ReferenceVideosSnake, metadata.VideoURLs, metadata.Videos)
		body.ReferenceImages = nil
		body.ReferenceVideos = nil
		body.FirstFrame = ""
		body.LastFrame = ""
		body.GenerateAudioNative = generateAudio
	} else {
		body.GenerateAudio = generateAudio
	}
	if err := validatePayload(body, cfg); err != nil {
		return nil, err
	}
	return body, nil
}

func validatePayload(body *requestPayload, cfg modelConfig) error {
	if body.Prompt == "" && (!cfg.allowPromptlessMedia || !hasReferenceInputs(body)) {
		return fmt.Errorf("prompt is required")
	}
	if cfg.maxPromptLength > 0 && len([]rune(body.Prompt)) > cfg.maxPromptLength {
		return fmt.Errorf("prompt must contain at most %d characters", cfg.maxPromptLength)
	}
	if body.Duration < cfg.minDuration || body.Duration > cfg.maxDuration {
		return fmt.Errorf("duration must be between %d and %d", cfg.minDuration, cfg.maxDuration)
	}
	if len(cfg.allowedDurations) > 0 {
		if _, ok := cfg.allowedDurations[body.Duration]; !ok {
			return fmt.Errorf("duration %d is not supported by model %s", body.Duration, body.Model)
		}
	}
	if _, ok := cfg.allowedRatios[body.Ratio]; !ok {
		return fmt.Errorf("ratio %s is not supported by model %s", body.Ratio, body.Model)
	}
	imageCount := len(body.ReferenceImages) + len(body.Images)
	videoCount := len(body.ReferenceVideos) + len(body.Videos)
	if imageCount > cfg.maxReferenceImages {
		return fmt.Errorf("reference images must contain at most %d items", cfg.maxReferenceImages)
	}
	if videoCount > cfg.maxReferenceVideos {
		return fmt.Errorf("reference videos must contain at most %d items", cfg.maxReferenceVideos)
	}
	if len(body.ReferenceAudios) > cfg.maxReferenceAudios {
		return fmt.Errorf("reference audios must contain at most %d items", cfg.maxReferenceAudios)
	}
	if cfg.maxReferenceMedia > 0 && imageCount+videoCount+len(body.ReferenceAudios) > cfg.maxReferenceMedia {
		return fmt.Errorf("reference images, videos, and audios must contain at most %d items in total", cfg.maxReferenceMedia)
	}
	if cfg.requireVisualWithAudio && len(body.ReferenceAudios) > 0 && len(body.ReferenceImages) == 0 && len(body.ReferenceVideos) == 0 && body.FirstFrame == "" && body.LastFrame == "" {
		return fmt.Errorf("reference audios require at least one image or video reference")
	}
	if cfg.framesExclusive && (body.FirstFrame != "" || body.LastFrame != "") && (len(body.ReferenceImages) > 0 || len(body.ReferenceVideos) > 0 || len(body.ReferenceAudios) > 0) {
		return fmt.Errorf("first/last frame mode cannot be mixed with reference images, videos, or audios")
	}
	if (body.GenerateAudio != nil || body.GenerateAudioNative != nil) && !cfg.supportsGenerateAudio {
		return fmt.Errorf("generateAudio is not supported by model %s", body.Model)
	}
	if body.LastFrame != "" && body.FirstFrame == "" {
		return fmt.Errorf("lastFrame requires firstFrame")
	}
	return nil
}

func requestDuration(req relaycommon.TaskSubmitReq, cfg modelConfig) int {
	if req.Duration > 0 {
		return req.Duration
	}
	if seconds, err := strconv.Atoi(strings.TrimSpace(req.Seconds)); err == nil && seconds > 0 {
		return seconds
	}
	return cfg.defaultDuration
}

func requestRatio(req relaycommon.TaskSubmitReq, metadata requestMetadata, cfg modelConfig) string {
	if value := strings.TrimSpace(metadata.Ratio); value != "" {
		return value
	}
	if value := strings.TrimSpace(metadata.AspectRatio); value != "" {
		return value
	}
	value := strings.TrimSpace(req.Size)
	if _, ok := cfg.allowedRatios[value]; ok {
		return value
	}
	switch value {
	case "1280x720", "1920x1080", "2560x1440":
		return "16:9"
	case "720x1280", "1080x1920", "1440x2560":
		return "9:16"
	case "1024x1024", "1440x1440":
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

func requestResolution(metadata requestMetadata, cfg modelConfig) string {
	if cfg.resolution != "" {
		return cfg.resolution
	}
	return firstNonEmpty(metadata.Resolution, metadata.Quality, metadata.VQuality)
}

func requestImages(req relaycommon.TaskSubmitReq) []string {
	if len(req.Images) > 0 {
		return req.Images
	}
	return firstNonEmptySlice(singleStringSlice(req.Image), singleStringSlice(req.InputReference))
}

func hasReferenceInputs(body *requestPayload) bool {
	return len(body.ReferenceImages) > 0 || len(body.ReferenceVideos) > 0 || len(body.ReferenceAudios) > 0 || body.FirstFrame != "" || body.LastFrame != "" || len(body.Media) > 0 || len(body.Images) > 0 || len(body.Videos) > 0
}

func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return defaultBaseURL
	}
	return baseURL
}

func publicVideoStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "queued", "submitted":
		return dto.VideoStatusQueued
	case "processing", "in_progress", "running":
		return dto.VideoStatusInProgress
	case "completed", "success", "succeeded", "done":
		return dto.VideoStatusCompleted
	case "failed", "cancelled", "expired":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusUnknown
	}
}

func progressPercent(value any) int {
	switch typed := value.(type) {
	case float64:
		return clampPercent(int(typed))
	case float32:
		return clampPercent(int(typed))
	case int:
		return clampPercent(typed)
	case int64:
		return clampPercent(int(typed))
	case json.Number:
		parsed, _ := strconv.Atoi(string(typed))
		return clampPercent(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(typed), "%"))
		return clampPercent(parsed)
	default:
		return 0
	}
}

func clampPercent(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func upstreamErrorMessage(upstream upstreamResponse) string {
	return firstNonEmpty(errorMessage(upstream.Error), upstream.Message, "video generation failed")
}

func upstreamDataURL(value interface{}) string {
	switch typed := value.(type) {
	case map[string]interface{}:
		return firstNonEmpty(
			fmt.Sprint(typed["url"]),
			fmt.Sprint(typed["video_url"]),
			fmt.Sprint(typed["result_url"]),
			fmt.Sprint(typed["result"]),
		)
	case []interface{}:
		for _, item := range typed {
			if url := upstreamDataURL(item); url != "" {
				return url
			}
		}
	}
	return ""
}

func errorMessage(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		return firstNonEmpty(fmt.Sprint(typed["message"]), fmt.Sprint(typed["code"]))
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func firstNonEmptySlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return append([]string(nil), value...)
		}
	}
	return nil
}

func stringValues(values []interface{}) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, strings.TrimSpace(text))
		}
	}
	return result
}

func stringsToInterfaces(values []string) []interface{} {
	result := make([]interface{}, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func firstNonEmptyInterfaces(values ...[]interface{}) []interface{} {
	for _, value := range values {
		if len(value) > 0 {
			return append([]interface{}(nil), value...)
		}
	}
	return nil
}

func firstNonEmptyMedia(values ...[]map[string]interface{}) []map[string]interface{} {
	for _, value := range values {
		if len(value) > 0 {
			return append([]map[string]interface{}(nil), value...)
		}
	}
	return nil
}

func firstNonEmptyMap(values ...map[string]interface{}) map[string]interface{} {
	for _, value := range values {
		if len(value) > 0 {
			result := make(map[string]interface{}, len(value))
			for key, item := range value {
				result[key] = item
			}
			return result
		}
	}
	return nil
}

func singleStringSlice(value string) []string {
	if value = strings.TrimSpace(value); value != "" {
		return []string{value}
	}
	return nil
}

func firstString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []string:
		if len(typed) > 0 {
			return strings.TrimSpace(typed[0])
		}
	case []any:
		if len(typed) > 0 {
			return strings.TrimSpace(fmt.Sprint(typed[0]))
		}
	}
	return ""
}

func firstNonNilBool(values ...*bool) *bool {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstNonNilInt(values ...*int) *int {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func stringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func intSet(values ...int) map[int]struct{} {
	set := make(map[int]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func localTaskError(err error) *taskdto.TaskError {
	return &taskdto.TaskError{Code: "invalid_request", Message: err.Error(), StatusCode: http.StatusBadRequest, LocalError: true, Error: err}
}

var _ channel.TaskAdaptor = (*TaskAdaptor)(nil)
var _ channel.OpenAIVideoConverter = (*TaskAdaptor)(nil)
var _ channel.PromptOnlyVideoTester = (*TaskAdaptor)(nil)

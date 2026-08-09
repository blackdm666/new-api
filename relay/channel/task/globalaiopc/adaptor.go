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
	ChannelName       = "globalaiopc"
	ModelSeedance25   = "seedance-2.5"
	defaultBaseURL    = "https://zcbservice.aizfw.cn/kyyReactApiServer"
	defaultDuration   = 4
	minDuration       = 4
	maxDuration       = 30
	defaultRatio      = "9:16"
	defaultResolution = "720p"
)

type requestPayload struct {
	Model           string   `json:"model"`
	Prompt          string   `json:"prompt"`
	ReferenceImages []string `json:"reference_images,omitempty"`
	ReferenceAudios []string `json:"reference_audios,omitempty"`
	Duration        int      `json:"duration,omitempty"`
	AspectRatio     string   `json:"aspect_ratio,omitempty"`
	Resolution      string   `json:"resolution"`
}

type responsePayload struct {
	ID             string      `json:"id"`
	Object         string      `json:"object"`
	Created        int64       `json:"created"`
	Model          string      `json:"model"`
	Status         string      `json:"status"`
	Progress       any         `json:"progress,omitempty"`
	ResultURL      string      `json:"result_url,omitempty"`
	VideoURL       string      `json:"video_url,omitempty"`
	Amount         float64     `json:"amount,omitempty"`
	ActualDuration int         `json:"actualDuration,omitempty"`
	Error          any         `json:"error,omitempty"`
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
	if err := validateSeedanceRequest(req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if len(req.Images) > 0 {
		info.Action = constant.TaskActionReferenceGenerate
	} else {
		info.Action = constant.TaskActionTextGenerate
	}
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	duration := requestDuration(req)
	if duration <= 0 {
		duration = defaultDuration
	}
	return map[string]float64{"seconds": float64(duration)}
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v2/model-center/tasks", a.baseURL), nil
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
	body, err := convertToRequestPayload(req, info)
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
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v2/model-center/tasks/%s", baseUrl, taskID), nil)
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
	return []string{ModelSeedance25}
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
		openAIVideo.Seconds = strconv.Itoa(upstream.ActualDuration)
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

func convertToRequestPayload(req relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*requestPayload, error) {
	body := requestPayload{
		Model:           taskcommon.DefaultString(info.UpstreamModelName, ModelSeedance25),
		Prompt:          req.Prompt,
		ReferenceImages: req.Images,
		Duration:        requestDuration(req),
		AspectRatio:     aspectRatio(req.Size),
		Resolution:      defaultResolution,
	}
	if body.Duration == 0 {
		body.Duration = defaultDuration
	}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &body); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	if body.Model == "" {
		body.Model = ModelSeedance25
	}
	if body.AspectRatio == "" {
		body.AspectRatio = defaultRatio
	}
	if body.Resolution == "" {
		body.Resolution = defaultResolution
	}
	return &body, validatePayload(*body)
}

func validateSeedanceRequest(req relaycommon.TaskSubmitReq) error {
	body, err := convertToRequestPayload(req, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: ModelSeedance25}})
	if err != nil {
		return err
	}
	return validatePayload(*body)
}

func validatePayload(body requestPayload) error {
	if body.Model != ModelSeedance25 {
		return fmt.Errorf("model must be %s", ModelSeedance25)
	}
	if strings.TrimSpace(body.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	if len(body.ReferenceImages) > 30 {
		return fmt.Errorf("reference_images must contain at most 30 items")
	}
	if len(body.ReferenceAudios) > 10 {
		return fmt.Errorf("reference_audios must contain at most 10 items")
	}
	if body.Duration < minDuration || body.Duration > maxDuration {
		return fmt.Errorf("duration must be between %d and %d", minDuration, maxDuration)
	}
	if body.Resolution != defaultResolution {
		return fmt.Errorf("resolution must be %s", defaultResolution)
	}
	switch body.AspectRatio {
	case "16:9", "9:16", "1:1":
		return nil
	default:
		return fmt.Errorf("aspect_ratio must be one of 16:9, 9:16, 1:1")
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

func aspectRatio(size string) string {
	switch size {
	case "16:9", "9:16", "1:1":
		return size
	case "1280x720", "1920x1080":
		return "16:9"
	case "720x1280", "1080x1920":
		return "9:16"
	case "1024x1024", "512x512":
		return "1:1"
	default:
		return defaultRatio
	}
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

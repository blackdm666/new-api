package sub2api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

type modelConfig struct {
	requireImage      bool
	defaultDuration   int
	defaultRatio      string
	defaultResolution string
	resolutions       map[string]struct{}
}

var modelConfigs = map[string]modelConfig{
	ModelGrokImagineVideo: {
		defaultDuration:   8,
		defaultRatio:      "16:9",
		defaultResolution: "720p",
		resolutions:       stringSet("480p", "720p"),
	},
	ModelGrokImagineVideo15: {
		requireImage:      true,
		defaultDuration:   8,
		defaultRatio:      "16:9",
		defaultResolution: "720p",
		resolutions:       stringSet("480p", "720p"),
	},
	ModelGrokImagineVideo151080: {
		requireImage:      true,
		defaultDuration:   8,
		defaultRatio:      "16:9",
		defaultResolution: "1080p",
		resolutions:       stringSet("1080p"),
	},
}

type createRequest struct {
	Model       string            `json:"model"`
	Prompt      string            `json:"prompt"`
	Duration    int               `json:"duration"`
	AspectRatio string            `json:"aspect_ratio,omitempty"`
	Resolution  string            `json:"resolution,omitempty"`
	Image       map[string]string `json:"image,omitempty"`
}

type upstreamVideo struct {
	URL      string `json:"url,omitempty"`
	Duration int    `json:"duration,omitempty"`
}

type upstreamResponse struct {
	RequestID string         `json:"request_id,omitempty"`
	ID        string         `json:"id,omitempty"`
	TaskID    string         `json:"task_id,omitempty"`
	Object    string         `json:"object,omitempty"`
	Model     string         `json:"model,omitempty"`
	Status    string         `json:"status,omitempty"`
	Progress  any            `json:"progress,omitempty"`
	Video     *upstreamVideo `json:"video,omitempty"`
	Error     any            `json:"error,omitempty"`
	Message   string         `json:"message,omitempty"`
}

type publicResponse struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Model       string `json:"model,omitempty"`
	Status      string `json:"status"`
	Progress    int    `json:"progress,omitempty"`
	CreatedAt   int64  `json:"created_at,omitempty"`
	CompletedAt int64  `json:"completed_at,omitempty"`
	Error       any    `json:"error,omitempty"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	baseURL string
	apiKey  string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if taskErr := relaycommon.ValidateMultipartDirect(c, info); taskErr != nil {
		return taskErr
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return localTaskError(err)
	}
	cfg, ok := modelConfigs[strings.TrimSpace(req.Model)]
	if !ok {
		return localTaskError(fmt.Errorf("model must be one of %s", strings.Join(ModelList, ", ")))
	}
	images := requestImages(req)
	if len(images) > 1 {
		return localTaskError(fmt.Errorf("Grok video accepts at most one input image"))
	}
	if cfg.requireImage && len(images) != 1 {
		return localTaskError(fmt.Errorf("model %s requires exactly one input image", req.Model))
	}
	duration := requestDuration(req, cfg)
	if duration < 1 || duration > 15 {
		return localTaskError(fmt.Errorf("duration must be between 1 and 15 seconds"))
	}
	resolution := requestResolution(req, cfg)
	if _, ok := cfg.resolutions[resolution]; !ok {
		return localTaskError(fmt.Errorf("resolution %s is not supported by model %s", resolution, req.Model))
	}
	return nil
}

func localTaskError(err error) *dto.TaskError {
	return &dto.TaskError{Code: "invalid_request", Message: err.Error(), StatusCode: http.StatusBadRequest, LocalError: true, Error: err}
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + "/v1/videos/generations", nil
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
	cfg, ok := modelConfigs[info.OriginModelName]
	if !ok {
		cfg = modelConfigs[req.Model]
	}
	body := createRequest{
		Model:       info.UpstreamModelName,
		Prompt:      req.Prompt,
		Duration:    requestDuration(req, cfg),
		AspectRatio: requestAspectRatio(req, cfg),
		Resolution:  requestResolution(req, cfg),
	}
	if images := requestImages(req); len(images) > 0 {
		body.Image = map[string]string{"url": images[0]}
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

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var upstream upstreamResponse
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	upstreamID := firstNonEmpty(upstream.RequestID, upstream.ID, upstream.TaskID)
	if upstreamID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("request_id is empty"), "invalid_response", http.StatusInternalServerError)
	}

	status := publicVideoStatus(upstream.Status)
	if status == "unknown" {
		status = "queued"
	}
	c.JSON(http.StatusOK, publicResponse{
		ID:       info.PublicTaskID,
		Object:   "video",
		Model:    info.OriginModelName,
		Status:   status,
		Progress: progressPercent(upstream.Progress),
	})
	return upstreamID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	requestURL := strings.TrimRight(baseURL, "/") + "/v1/videos/" + taskID
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
	var result upstreamResponse
	if err := common.Unmarshal(respBody, &result); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}
	task := &relaycommon.TaskInfo{TaskID: firstNonEmpty(result.RequestID, result.ID, result.TaskID)}
	progress := progressPercent(result.Progress)
	switch strings.ToLower(strings.TrimSpace(result.Status)) {
	case "queued":
		task.Status = model.TaskStatusQueued
	case "pending", "processing", "in_progress":
		if progress > 0 {
			task.Status = model.TaskStatusInProgress
		} else {
			task.Status = model.TaskStatusQueued
		}
	case "done", "completed", "succeeded":
		task.Status = model.TaskStatusSuccess
		if result.Video != nil {
			task.Url = a.absoluteUpstreamURL(result.Video.URL)
		}
	case "failed", "cancelled", "expired":
		task.Status = model.TaskStatusFailure
		task.Reason = firstNonEmpty(errorMessage(result.Error), result.Message, "task "+result.Status)
	}
	if progress > 0 && progress < 100 {
		task.Progress = fmt.Sprintf("%d%%", progress)
	}
	return task, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	status := task.Status.ToVideoStatus()
	response := publicResponse{
		ID:        task.TaskID,
		Object:    "video",
		Model:     firstNonEmpty(task.Properties.OriginModelName, task.Properties.UpstreamModelName),
		Status:    status,
		Progress:  progressPercent(task.Progress),
		CreatedAt: task.CreatedAt,
	}
	if task.Status == model.TaskStatusSuccess {
		response.CompletedAt = task.FinishTime
	}
	if task.Status == model.TaskStatusFailure {
		response.Error = map[string]string{"message": firstNonEmpty(task.FailReason, "video generation failed"), "code": "video_generation_failed"}
	}
	return common.Marshal(response)
}

func (a *TaskAdaptor) GetModelList() []string { return ModelList }
func (a *TaskAdaptor) GetChannelName() string { return ChannelName }

func (a *TaskAdaptor) absoluteUpstreamURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "/") {
		return strings.TrimRight(a.baseURL, "/") + raw
	}
	return raw
}

func requestImages(req relaycommon.TaskSubmitReq) []string {
	if len(req.Images) > 0 {
		return req.Images
	}
	if image := strings.TrimSpace(req.Image); image != "" {
		return []string{image}
	}
	if image := strings.TrimSpace(req.InputReference); image != "" {
		return []string{image}
	}
	return nil
}

func requestDuration(req relaycommon.TaskSubmitReq, cfg modelConfig) int {
	if req.Duration > 0 {
		return req.Duration
	}
	if seconds, err := strconv.Atoi(req.Seconds); err == nil && seconds > 0 {
		return seconds
	}
	return cfg.defaultDuration
}

func requestAspectRatio(req relaycommon.TaskSubmitReq, cfg modelConfig) string {
	if value := strings.TrimSpace(req.Size); value != "" && value != "auto" {
		return value
	}
	if value := metadataString(req.Metadata, "aspect_ratio"); value != "" && value != "auto" {
		return value
	}
	return cfg.defaultRatio
}

func requestResolution(req relaycommon.TaskSubmitReq, cfg modelConfig) string {
	if cfg.defaultResolution == "1080p" {
		return cfg.defaultResolution
	}
	value := strings.ToLower(metadataString(req.Metadata, "resolution"))
	if value == "" {
		return cfg.defaultResolution
	}
	if !strings.HasSuffix(value, "p") {
		value += "p"
	}
	return value
}

func metadataString(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func publicVideoStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "pending":
		return "queued"
	case "processing", "in_progress":
		return "in_progress"
	case "done", "completed", "succeeded":
		return "completed"
	case "failed", "cancelled", "expired":
		return "failed"
	default:
		return "unknown"
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

func stringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

var _ channel.TaskAdaptor = (*TaskAdaptor)(nil)
var _ channel.OpenAIVideoConverter = (*TaskAdaptor)(nil)

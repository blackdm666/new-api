package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

const (
	defaultTaskVideoWorkerTimeout = 15 * time.Minute
	minTaskVideoWorkerTimeout     = 30 * time.Second
	maxTaskVideoWorkerTimeout     = 30 * time.Minute
	maxTaskVideoWorkerResponse    = int64(64 * 1024)
	maxTaskVideoWorkerBytes       = int64(5*1024*1024*1024 - 5*1024*1024)
)

type taskVideoWorkerRequest struct {
	Version   int    `json:"version"`
	TaskID    string `json:"task_id"`
	SourceURL string `json:"source_url"`
	KeyPrefix string `json:"key_prefix"`
	MaxBytes  int64  `json:"max_bytes"`
}

type taskVideoWorkerResponse struct {
	Success  bool   `json:"success"`
	Key      string `json:"key"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
	ETag     string `json:"etag"`
	Reused   bool   `json:"reused"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

var taskVideoWorkerDo = func(req *http.Request) (*http.Response, error) {
	return GetHttpClient().Do(req)
}

func TaskVideoWorkerEnabled() bool {
	if !common.GetEnvOrDefaultBool("TASK_VIDEO_WORKER_ENABLED", false) {
		return false
	}
	return taskVideoWorkerURL() != "" && strings.TrimSpace(common.GetEnvOrDefaultString("TASK_VIDEO_WORKER_SECRET", "")) != ""
}

func TaskVideoWorkerTimeout() time.Duration {
	raw := strings.TrimSpace(common.GetEnvOrDefaultString("TASK_VIDEO_WORKER_TIMEOUT_SECONDS", ""))
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return defaultTaskVideoWorkerTimeout
	}
	timeout := time.Duration(seconds) * time.Second
	if timeout < minTaskVideoWorkerTimeout || timeout > maxTaskVideoWorkerTimeout {
		return defaultTaskVideoWorkerTimeout
	}
	return timeout
}

func taskVideoWorkerURL() string {
	raw := strings.TrimSpace(common.GetEnvOrDefaultString("TASK_VIDEO_WORKER_URL", ""))
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Path != "/transfer" {
		return ""
	}
	parsed.Fragment = ""
	return parsed.String()
}

func taskVideoWorkerEligible(task *model.Task, resultURL string) bool {
	if !TaskVideoWorkerEnabled() || task == nil || isTaskVideoProxyURL(resultURL, task.TaskID) {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(resultURL))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return false
	}
	if !isBrowserRoutableVideoHost(parsed.Hostname()) || isCloudflareR2URL(parsed) || taskVideoURLContainsProviderCredential(parsed) {
		return false
	}
	channelType, _ := strconv.Atoi(string(task.Platform))
	switch channelType {
	case constant.ChannelTypeOpenAI,
		constant.ChannelTypeGemini,
		constant.ChannelTypeVertexAi,
		constant.ChannelTypeSora,
		constant.ChannelTypeSub2API:
		return false
	default:
		return true
	}
}

func cacheTaskVideoWithWorker(ctx context.Context, task *model.Task, resultURL string) (bool, error) {
	workerURL := taskVideoWorkerURL()
	secret := strings.TrimSpace(common.GetEnvOrDefaultString("TASK_VIDEO_WORKER_SECRET", ""))
	if workerURL == "" || secret == "" {
		return false, errors.New("Cloudflare video transfer is not configured")
	}
	maxBytes := TaskVideoCacheMaxBytes()
	if maxBytes > maxTaskVideoWorkerBytes {
		maxBytes = maxTaskVideoWorkerBytes
	}
	requestData := taskVideoWorkerRequest{
		Version:   1,
		TaskID:    task.TaskID,
		SourceURL: strings.TrimSpace(resultURL),
		KeyPrefix: taskVideoCacheKeyPrefix(task),
		MaxBytes:  maxBytes,
	}
	payload, err := common.Marshal(requestData)
	if err != nil {
		return false, fmt.Errorf("encode Cloudflare video transfer request: %w", err)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(payload)
	signature := hex.EncodeToString(mac.Sum(nil))

	transferCtx, cancel := context.WithTimeout(ctx, TaskVideoWorkerTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(transferCtx, http.MethodPost, workerURL, bytes.NewReader(payload))
	if err != nil {
		return false, fmt.Errorf("create Cloudflare video transfer request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-NewAPI-Timestamp", timestamp)
	req.Header.Set("X-NewAPI-Signature", signature)
	resp, err := taskVideoWorkerDo(req)
	if err != nil {
		return false, fmt.Errorf("call Cloudflare video transfer Worker: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTaskVideoWorkerResponse+1))
	if err != nil {
		return false, fmt.Errorf("read Cloudflare video transfer response: %w", err)
	}
	if int64(len(body)) > maxTaskVideoWorkerResponse {
		return false, errors.New("Cloudflare video transfer response is too large")
	}
	var result taskVideoWorkerResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("decode Cloudflare video transfer response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || !result.Success {
		code := strings.TrimSpace(result.Code)
		if code == "" {
			code = "worker_error"
		}
		return false, fmt.Errorf("Cloudflare video transfer returned status %d (%s)", resp.StatusCode, code)
	}
	if result.Size <= 0 || result.Size > requestData.MaxBytes || !strings.HasPrefix(result.MimeType, "video/") {
		return false, errors.New("Cloudflare video transfer returned invalid video metadata")
	}
	expectedKeys := map[string]struct{}{
		requestData.KeyPrefix + ".mp4":  {},
		requestData.KeyPrefix + ".webm": {},
		requestData.KeyPrefix + ".mov":  {},
	}
	if _, ok := expectedKeys[result.Key]; !ok {
		return false, errors.New("Cloudflare video transfer returned an unexpected object key")
	}
	storage, err := taskVideoCacheStorageFactory()
	if err != nil {
		return false, err
	}
	if storage.Kind() != taskVideoCacheKind {
		return false, fmt.Errorf("Cloudflare video transfer requires %s storage, got %s", taskVideoCacheKind, storage.Kind())
	}
	exists, err := storage.Exists(ctx, result.Key)
	if err != nil {
		return false, fmt.Errorf("verify Worker-cached video object: %w", err)
	}
	if !exists {
		return false, errors.New("Worker-cached video object is not readable")
	}
	installTaskVideoCacheResult(task, storage.Kind(), result.Key, result.MimeType)
	return true, nil
}

func taskVideoResultHost(resultURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(resultURL))
	if err != nil || parsed == nil {
		return "unknown"
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return "unknown"
	}
	return host
}

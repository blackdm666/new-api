package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// TaskVideoSource is an authenticated provider response ready to be copied to
// private object storage or streamed by the compatibility proxy.
type TaskVideoSource struct {
	Body          io.ReadCloser
	ContentLength int64
	ContentType   string
	Header        http.Header
	StatusCode    int
}

// OpenTaskVideoSourceFunc is injected by main to keep service independent from
// provider adaptors while still supporting provider-specific download auth.
var OpenTaskVideoSourceFunc func(ctx context.Context, task *model.Task, resultURL string) (*TaskVideoSource, error)

type TaskVideoPreparation struct {
	Cached    bool
	DirectURL string
}

var taskVideoDirectProbe = probeTaskVideoDirectAccess

// PrepareTaskVideoResult applies one policy for every video channel: upstream
// R2 URLs and explicitly trusted official media hosts may stay direct, while
// every other result is copied once to the configured private R2/S3 bucket.
func PrepareTaskVideoResult(ctx context.Context, task *model.Task, reportedURL string) (TaskVideoPreparation, error) {
	if task == nil || strings.TrimSpace(task.TaskID) == "" {
		return TaskVideoPreparation{}, errors.New("task id is required")
	}
	if task.PrivateData.ResultStorageKey != "" {
		return TaskVideoPreparation{Cached: true}, nil
	}

	resultURL := ResolveTaskVideoResultURL(task, reportedURL)
	if strings.HasPrefix(resultURL, "data:") {
		if !TaskVideoCacheEnabled() {
			return TaskVideoPreparation{}, nil
		}
		cached, err := cacheTaskVideoDataURL(ctx, task, resultURL)
		return TaskVideoPreparation{Cached: cached}, err
	}

	direct, err := taskVideoURLCanOpenDirectly(ctx, task, resultURL)
	if err != nil {
		return TaskVideoPreparation{}, err
	}
	if direct {
		task.PrivateData.ResultURL = resultURL
		return TaskVideoPreparation{DirectURL: resultURL}, nil
	}
	if !TaskVideoCacheEnabled() {
		return TaskVideoPreparation{}, nil
	}
	cached, err := cacheTaskVideoRemoteSource(ctx, task, resultURL)
	return TaskVideoPreparation{Cached: cached}, err
}

// PrepareTaskVideoPreviewURL repairs historical tasks lazily. Public results
// remain direct; protected results are copied to R2 once and then signed.
func PrepareTaskVideoPreviewURL(ctx context.Context, task *model.Task) (string, int64, error) {
	if task == nil {
		return "", 0, errors.New("task is required")
	}
	if previewURL, cached, err := GetTaskVideoPreviewURL(ctx, task); cached || err != nil {
		if err != nil {
			return "", 0, err
		}
		return previewURL, int64(TaskVideoPreviewURLTTL().Seconds()), nil
	}

	previousURL := task.PrivateData.ResultURL
	previousStorageKey := task.PrivateData.ResultStorageKey
	prepared, err := PrepareTaskVideoResult(ctx, task, task.GetResultURL())
	if err != nil {
		return "", 0, err
	}
	if task.ID > 0 && (task.PrivateData.ResultURL != previousURL || task.PrivateData.ResultStorageKey != previousStorageKey) {
		updated, err := task.UpdateResultLocation()
		if err != nil {
			return "", 0, fmt.Errorf("persist task video result: %w", err)
		}
		if !updated {
			return "", 0, errors.New("task video result changed concurrently")
		}
	}
	if prepared.Cached {
		previewURL, cached, err := GetTaskVideoPreviewURL(ctx, task)
		if err != nil {
			return "", 0, err
		}
		if !cached {
			return "", 0, errors.New("cached task video is unavailable")
		}
		return previewURL, int64(TaskVideoPreviewURLTTL().Seconds()), nil
	}
	if prepared.DirectURL != "" {
		return prepared.DirectURL, 0, nil
	}
	return "", 0, errors.New("direct video preview is unavailable")
}

// ResolveTaskVideoResultURL normalizes common provider response envelopes so
// task logs do not need channel-specific URL extraction.
func ResolveTaskVideoResultURL(task *model.Task, reportedURL string) string {
	reportedURL = strings.TrimSpace(reportedURL)
	if task == nil {
		return reportedURL
	}
	var extracted string
	if len(task.Data) > 0 {
		var payload any
		if err := common.Unmarshal(task.Data, &payload); err == nil {
			extracted = extractTaskVideoResultURL(payload, 0)
			if parsed, err := url.Parse(extracted); err == nil && isCloudflareR2URL(parsed) {
				return extracted
			}
		}
	}
	if reportedURL != "" && !isTaskVideoProxyURL(reportedURL, task.TaskID) {
		return reportedURL
	}
	if extracted != "" && !isTaskVideoProxyURL(extracted, task.TaskID) {
		return extracted
	}
	return reportedURL
}

func extractTaskVideoResultURL(value any, depth int) string {
	if depth > 5 || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"video_url", "result_url", "output_url", "file_url", "url", "uri"} {
			if candidate, ok := typed[key].(string); ok && isTaskVideoURLCandidate(candidate) {
				return strings.TrimSpace(candidate)
			}
		}
		if video, ok := typed["video"].(string); ok && isTaskVideoURLCandidate(video) {
			return strings.TrimSpace(video)
		}
		for _, key := range []string{"video", "videos", "result", "results", "output", "outputs", "data", "response"} {
			if candidate := extractTaskVideoResultURL(typed[key], depth+1); candidate != "" {
				return candidate
			}
		}
	case []any:
		for _, item := range typed {
			if candidate := extractTaskVideoResultURL(item, depth+1); candidate != "" {
				return candidate
			}
		}
	}
	return ""
}

func isTaskVideoURLCandidate(candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	return strings.HasPrefix(candidate, "data:video/") ||
		strings.HasPrefix(candidate, "http://") ||
		strings.HasPrefix(candidate, "https://") ||
		strings.HasPrefix(candidate, "gs://") ||
		strings.HasPrefix(candidate, "/")
}

func taskVideoURLCanOpenDirectly(ctx context.Context, task *model.Task, resultURL string) (bool, error) {
	resultURL = strings.TrimSpace(resultURL)
	if resultURL == "" || isTaskVideoProxyURL(resultURL, task.TaskID) {
		return false, nil
	}
	parsed, err := url.Parse(resultURL)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false, nil
	}
	if taskVideoURLContainsProviderCredential(parsed) {
		return false, nil
	}
	if isCloudflareR2URL(parsed) {
		return true, nil
	}
	if !isBrowserRoutableVideoHost(parsed.Hostname()) {
		return false, nil
	}
	if !taskVideoDirectHostAllowed(parsed.Hostname()) {
		return false, nil
	}
	return taskVideoDirectProbe(ctx, resultURL)
}

// taskVideoDirectHostAllowed checks the operator-maintained list of official
// media hosts. Exact names and leading-wildcard subdomains are supported;
// Cloudflare R2 is handled separately and never needs to be listed.
func taskVideoDirectHostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		return false
	}
	raw := common.GetEnvOrDefaultString("TASK_VIDEO_DIRECT_HOSTS", "")
	patterns := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(pattern), "."))
		if pattern == "" || pattern == "*" {
			continue
		}
		if strings.HasPrefix(pattern, "*.") {
			suffix := strings.TrimPrefix(pattern, "*.")
			if suffix != "" && host != suffix && strings.HasSuffix(host, "."+suffix) {
				return true
			}
			continue
		}
		if host == pattern {
			return true
		}
	}
	return false
}

func taskVideoURLContainsProviderCredential(parsed *url.URL) bool {
	for key := range parsed.Query() {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "key", "api_key", "apikey", "access_token", "token", "authorization":
			return true
		}
	}
	return false
}

func isCloudflareR2URL(parsed *url.URL) bool {
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	return strings.HasSuffix(host, ".r2.cloudflarestorage.com")
}

func isBrowserRoutableVideoHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified()
	}
	return strings.Contains(host, ".")
}

func probeTaskVideoDirectAccess(ctx context.Context, resultURL string) (bool, error) {
	if err := ValidateSSRFProtectedFetchURL(resultURL); err != nil {
		return false, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resultURL, nil)
	if err != nil {
		return false, fmt.Errorf("create direct video probe: %w", err)
	}
	req.Header.Set("Accept", "video/*, application/octet-stream;q=0.9")
	req.Header.Set("Range", "bytes=0-0")
	resp, err := GetSSRFProtectedHTTPClient().Do(req)
	if err != nil {
		return false, fmt.Errorf("probe direct video result from %s failed", req.URL.Hostname())
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusPartialContent:
		contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if strings.HasPrefix(contentType, "video/") || contentType == "application/octet-stream" || taskVideoPathHasVideoExtension(resultURL) {
			return true, nil
		}
		return false, fmt.Errorf("direct task result is not video content: %s", contentType)
	case http.StatusUnauthorized, http.StatusForbidden:
		return false, nil
	default:
		return false, fmt.Errorf("direct task video returned status %d", resp.StatusCode)
	}
}

func taskVideoPathHasVideoExtension(resultURL string) bool {
	parsed, err := url.Parse(resultURL)
	if err != nil {
		return false
	}
	switch strings.ToLower(path.Ext(parsed.Path)) {
	case ".mp4", ".m4v", ".mov", ".webm":
		return true
	default:
		return false
	}
}

func isTaskVideoProxyURL(resultURL string, taskID string) bool {
	if strings.TrimSpace(resultURL) == "" || strings.TrimSpace(taskID) == "" {
		return false
	}
	parsed, err := url.Parse(resultURL)
	if err != nil {
		return false
	}
	return strings.Contains(parsed.Path, "/v1/videos/"+taskID+"/content")
}

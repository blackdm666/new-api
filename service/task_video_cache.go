package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/QuantumNous/new-api/service/invoicefile"
)

const taskVideoCacheKind = "s3"

const (
	defaultTaskVideoPreviewURLTTL = 7 * 24 * time.Hour
	minTaskVideoPreviewURLTTL     = time.Minute
	maxTaskVideoPreviewURLTTL     = 7 * 24 * time.Hour
	defaultTaskVideoCacheMaxBytes = int64(2 * 1024 * 1024 * 1024)
	minTaskVideoCacheMaxBytes     = int64(1024 * 1024)
	maxTaskVideoCacheMaxBytes     = int64(10 * 1024 * 1024 * 1024)
)

// TaskVideoCacheEnabled reports whether protected, untrusted or data-URL video
// results must be copied to the configured private S3-compatible bucket.
// Cloudflare R2 and explicitly trusted official hosts may stay direct.
func TaskVideoCacheEnabled() bool {
	return common.GetEnvOrDefaultBool("TASK_VIDEO_CACHE_ENABLED", false)
}

func newTaskVideoCacheStorage() (invoicefile.Storage, error) {
	kind := strings.ToLower(strings.TrimSpace(common.GetEnvOrDefaultString("TASK_VIDEO_CACHE_STORAGE", taskVideoCacheKind)))
	if kind != taskVideoCacheKind {
		return nil, fmt.Errorf("unsupported task video cache storage %q: only s3-compatible storage is allowed", kind)
	}
	return invoicefile.NewStorage(invoicefile.Config{
		StorageType:  kind,
		Endpoint:     common.GetEnvOrDefaultString("TASK_VIDEO_CACHE_ENDPOINT", ""),
		Bucket:       common.GetEnvOrDefaultString("TASK_VIDEO_CACHE_BUCKET", ""),
		Region:       common.GetEnvOrDefaultString("TASK_VIDEO_CACHE_REGION", "auto"),
		AccessKeyId:  common.GetEnvOrDefaultString("TASK_VIDEO_CACHE_ACCESS_KEY_ID", ""),
		AccessSecret: common.GetEnvOrDefaultString("TASK_VIDEO_CACHE_ACCESS_KEY_SECRET", ""),
	})
}

var taskVideoCacheStorageFactory = newTaskVideoCacheStorage

// TaskVideoPreviewURLTTL returns the configured lifetime of an R2/S3 preview
// link. Cloudflare R2 limits presigned URLs to at most seven days.
func TaskVideoPreviewURLTTL() time.Duration {
	raw := strings.TrimSpace(common.GetEnvOrDefaultString("TASK_VIDEO_CACHE_SIGNED_URL_TTL_SECONDS", ""))
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return defaultTaskVideoPreviewURLTTL
	}
	ttl := time.Duration(seconds) * time.Second
	if ttl < minTaskVideoPreviewURLTTL || ttl > maxTaskVideoPreviewURLTTL {
		return defaultTaskVideoPreviewURLTTL
	}
	return ttl
}

// TaskVideoCacheMaxBytes bounds the one-time transfer from a protected
// provider result into private object storage.
func TaskVideoCacheMaxBytes() int64 {
	raw := strings.TrimSpace(common.GetEnvOrDefaultString("TASK_VIDEO_CACHE_MAX_BYTES", ""))
	maxBytes, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || maxBytes < minTaskVideoCacheMaxBytes || maxBytes > maxTaskVideoCacheMaxBytes {
		return defaultTaskVideoCacheMaxBytes
	}
	return maxBytes
}

func cacheTaskVideoDataURL(ctx context.Context, task *model.Task, resultURL string) (bool, error) {
	if task == nil || strings.TrimSpace(task.TaskID) == "" {
		return false, errors.New("task id is required")
	}
	mimeType, payload, encoding, err := parseVideoDataURL(resultURL)
	if err != nil {
		return false, err
	}
	size := decodedBase64Size(payload)
	if size <= 0 {
		return false, errors.New("empty decoded video payload")
	}
	if size > TaskVideoCacheMaxBytes() {
		return false, fmt.Errorf("video result exceeds cache limit of %d bytes", TaskVideoCacheMaxBytes())
	}
	decoder := base64.NewDecoder(encoding, strings.NewReader(payload))
	return storeTaskVideo(ctx, task, decoder, size, mimeType)
}

func cacheTaskVideoRemoteSource(ctx context.Context, task *model.Task, resultURL string) (bool, error) {
	if taskVideoWorkerEligible(task, resultURL) {
		cached, err := cacheTaskVideoWithWorker(ctx, task, resultURL)
		if err == nil && cached {
			return true, nil
		}
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf(
				"Cloudflare video transfer failed for task %s (source host %s), falling back to VPS: %v",
				task.TaskID,
				taskVideoResultHost(resultURL),
				err,
			))
		}
	}
	if OpenTaskVideoSourceFunc == nil {
		return false, errors.New("task video source opener is not configured")
	}
	source, err := OpenTaskVideoSourceFunc(ctx, task, resultURL)
	if err != nil {
		return false, err
	}
	if source == nil || source.Body == nil {
		return false, errors.New("task video source is empty")
	}
	defer source.Body.Close()

	maxBytes := TaskVideoCacheMaxBytes()
	if source.ContentLength > maxBytes {
		return false, fmt.Errorf("video result exceeds cache limit of %d bytes", maxBytes)
	}
	temp, err := os.CreateTemp("", "new-api-task-video-*")
	if err != nil {
		return false, fmt.Errorf("create video cache temp file: %w", err)
	}
	defer func() {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
	}()

	size, err := io.Copy(temp, io.LimitReader(source.Body, maxBytes+1))
	if err != nil {
		return false, fmt.Errorf("read task video source: %w", err)
	}
	if size <= 0 {
		return false, errors.New("task video source is empty")
	}
	if size > maxBytes {
		return false, fmt.Errorf("video result exceeds cache limit of %d bytes", maxBytes)
	}
	mimeType, err := taskVideoSourceMIMEType(source.ContentType, resultURL, temp)
	if err != nil {
		return false, err
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		return false, fmt.Errorf("rewind video cache temp file: %w", err)
	}
	return storeTaskVideo(ctx, task, temp, size, mimeType)
}

func storeTaskVideo(ctx context.Context, task *model.Task, reader io.Reader, size int64, mimeType string) (bool, error) {
	storage, err := taskVideoCacheStorageFactory()
	if err != nil {
		return false, err
	}
	key := taskVideoCacheKeyPrefix(task) + videoExtension(mimeType)
	exists, err := storage.Exists(ctx, key)
	if err != nil {
		return false, fmt.Errorf("check cached object: %w", err)
	}
	if !exists {
		if err := storage.Put(ctx, key, reader, size, mimeType); err != nil {
			return false, fmt.Errorf("store cached object: %w", err)
		}
	}
	exists, err = storage.Exists(ctx, key)
	if err != nil {
		return false, fmt.Errorf("verify cached object: %w", err)
	}
	if !exists {
		return false, errors.New("cached object is not readable after upload")
	}
	installTaskVideoCacheResult(task, storage.Kind(), key, mimeType)
	return true, nil
}

func taskVideoCacheKeyPrefix(task *model.Task) string {
	digest := sha256.Sum256([]byte(task.TaskID))
	createdAt := task.CreatedAt
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}
	createdMonth := time.Unix(createdAt, 0).UTC()
	return fmt.Sprintf("task-videos/%s/%s/%s", createdMonth.Format("2006"), createdMonth.Format("01"), hex.EncodeToString(digest[:]))
}

func installTaskVideoCacheResult(task *model.Task, storageKind string, key string, mimeType string) {
	task.PrivateData.ResultStorageKind = storageKind
	task.PrivateData.ResultStorageKey = key
	task.PrivateData.ResultMimeType = mimeType
	task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
}

func taskVideoSourceMIMEType(contentType string, resultURL string, file *os.File) (string, error) {
	if parsed, _, err := mime.ParseMediaType(contentType); err == nil && strings.HasPrefix(parsed, "video/") {
		return parsed, nil
	}
	if parsedURL, err := url.Parse(resultURL); err == nil {
		switch strings.ToLower(path.Ext(parsedURL.Path)) {
		case ".webm":
			return "video/webm", nil
		case ".mov":
			return "video/quicktime", nil
		case ".mp4", ".m4v":
			return "video/mp4", nil
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("inspect cached video: %w", err)
	}
	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("inspect cached video: %w", err)
	}
	detected := http.DetectContentType(header[:n])
	if strings.HasPrefix(detected, "video/") {
		return detected, nil
	}
	if parsed, _, err := mime.ParseMediaType(contentType); err == nil && parsed == "application/octet-stream" {
		return "video/mp4", nil
	}
	return "", fmt.Errorf("task result is not video content: %s", detected)
}

// OpenTaskVideoCache opens an already persisted object. The enabled flag is
// intentionally ignored here: disabling future writes must not strand tasks
// that were completed while caching was enabled.
func OpenTaskVideoCache(ctx context.Context, task *model.Task) (reader io.ReadCloser, mimeType string, cached bool, err error) {
	if task == nil || task.PrivateData.ResultStorageKey == "" {
		return nil, "", false, nil
	}
	storage, err := taskVideoCacheStorageFactory()
	if err != nil {
		return nil, "", true, err
	}
	if task.PrivateData.ResultStorageKind != "" && task.PrivateData.ResultStorageKind != storage.Kind() {
		return nil, "", true, fmt.Errorf("cached task video storage changed from %q to %q", task.PrivateData.ResultStorageKind, storage.Kind())
	}
	reader, err = storage.Get(ctx, task.PrivateData.ResultStorageKey)
	if err != nil {
		return nil, "", true, err
	}
	mimeType = task.PrivateData.ResultMimeType
	if mimeType == "" {
		mimeType = "video/mp4"
	}
	return reader, mimeType, true, nil
}

// GetTaskVideoPreviewURL creates an on-demand presigned URL for a cached task
// video. The browser downloads directly from R2/S3, so video bytes do not pass
// through the NewAPI server.
func GetTaskVideoPreviewURL(ctx context.Context, task *model.Task) (previewURL string, cached bool, err error) {
	if task == nil || task.PrivateData.ResultStorageKey == "" {
		return "", false, nil
	}
	storage, err := taskVideoCacheStorageFactory()
	if err != nil {
		return "", true, err
	}
	if task.PrivateData.ResultStorageKind != "" && task.PrivateData.ResultStorageKind != storage.Kind() {
		return "", true, fmt.Errorf("cached task video storage changed from %q to %q", task.PrivateData.ResultStorageKind, storage.Kind())
	}
	previewURL, err = storage.SignedURL(ctx, task.PrivateData.ResultStorageKey, TaskVideoPreviewURLTTL(), "", true)
	if err != nil {
		return "", true, err
	}
	return previewURL, true, nil
}

func parseVideoDataURL(value string) (mimeType string, payload string, encoding *base64.Encoding, err error) {
	header, payload, ok := strings.Cut(value, ",")
	if !ok || !strings.HasPrefix(header, "data:") || !strings.HasSuffix(header, ";base64") {
		return "", "", nil, errors.New("unsupported video data URL")
	}
	mimeType = strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
	if !strings.HasPrefix(mimeType, "video/") {
		return "", "", nil, fmt.Errorf("unexpected cached video MIME type %q", mimeType)
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", "", nil, errors.New("empty cached video payload")
	}
	encoding = base64.StdEncoding
	if len(payload)%4 != 0 {
		encoding = base64.RawStdEncoding
	}
	return mimeType, payload, encoding, nil
}

func decodedBase64Size(payload string) int64 {
	size := int64(len(payload) * 3 / 4)
	if strings.HasSuffix(payload, "==") {
		return size - 2
	}
	if strings.HasSuffix(payload, "=") {
		return size - 1
	}
	return size
}

func videoExtension(mimeType string) string {
	switch mimeType {
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	default:
		return ".mp4"
	}
}

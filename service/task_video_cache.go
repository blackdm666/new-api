package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/QuantumNous/new-api/service/invoicefile"
)

const taskVideoCacheKind = "s3"

const (
	defaultTaskVideoPreviewURLTTL = 7 * 24 * time.Hour
	minTaskVideoPreviewURLTTL     = time.Minute
	maxTaskVideoPreviewURLTTL     = 7 * 24 * time.Hour
)

// TaskVideoCacheEnabled reports whether newly completed data-URL videos must be
// copied to the configured private S3-compatible bucket before SUCCESS is
// persisted. Cloudflare R2 uses the same S3 protocol and configuration.
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

// CacheTaskVideoResult persists a base64 data URL to private object storage.
// Returning cached=false is expected when caching is disabled or the result is
// already a direct URL. A cache error prevents the caller from marking the task
// successful, so users never receive SUCCESS before the media is readable.
func CacheTaskVideoResult(ctx context.Context, task *model.Task, resultURL string) (cached bool, err error) {
	if !TaskVideoCacheEnabled() || !strings.HasPrefix(resultURL, "data:") {
		return false, nil
	}
	if task == nil || strings.TrimSpace(task.TaskID) == "" {
		return false, errors.New("task id is required")
	}
	mimeType, payload, encoding, err := parseVideoDataURL(resultURL)
	if err != nil {
		return false, err
	}
	storage, err := taskVideoCacheStorageFactory()
	if err != nil {
		return false, err
	}

	digest := sha256.Sum256([]byte(task.TaskID))
	createdAt := task.CreatedAt
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}
	createdMonth := time.Unix(createdAt, 0).UTC()
	key := fmt.Sprintf("task-videos/%s/%s/%s%s", createdMonth.Format("2006"), createdMonth.Format("01"), hex.EncodeToString(digest[:]), videoExtension(mimeType))
	exists, err := storage.Exists(ctx, key)
	if err != nil {
		return false, fmt.Errorf("check cached object: %w", err)
	}
	if !exists {
		size := decodedBase64Size(payload)
		if size <= 0 {
			return false, errors.New("empty decoded video payload")
		}
		decoder := base64.NewDecoder(encoding, strings.NewReader(payload))
		if err := storage.Put(ctx, key, decoder, size, mimeType); err != nil {
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

	task.PrivateData.ResultStorageKind = storage.Kind()
	task.PrivateData.ResultStorageKey = key
	task.PrivateData.ResultMimeType = mimeType
	task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
	return true, nil
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

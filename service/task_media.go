package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
)

const TaskMediaMaxUploadBytes int64 = 100_000_000

var taskMediaKeyPattern = regexp.MustCompile(`^(task-videos/[0-9]{4}/[0-9]{2}/[a-f0-9]{64}\.(mp4|webm|mov)|reference-media/[a-f0-9-]{36}\.(mp4|webm|mov|mp3|wav|ogg|m4a|flac|png|jpg|webp|gif))$`)

// Public delivery is an explicit operator choice, not a relaxation of task,
// token or artifact authorization. Only stored media bytes become public.
func TaskMediaPublicEnabled() bool {
	return common.GetEnvOrDefaultBool("TASK_MEDIA_PUBLIC_ENABLED", false)
}

func TaskMediaPublicURL(key string) (string, error) {
	if !TaskMediaPublicEnabled() || !taskMediaKeyPattern.MatchString(key) {
		return "", errors.New("public media is unavailable")
	}
	base := strings.TrimRight(common.GetEnvOrDefaultString("TASK_MEDIA_PUBLIC_BASE_URL", ""), "/")
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsAny(base, "\r\n\\") {
		return "", errors.New("public media HTTPS base URL is not configured")
	}
	return base + "/" + key, nil
}

func PublicTaskVideoURL(task *model.Task) (string, error) {
	if task == nil || task.Status != model.TaskStatusSuccess || task.PrivateData.ResultStorageKind != "s3" || !strings.HasPrefix(task.PrivateData.ResultStorageKey, "task-videos/") || !strings.HasPrefix(task.PrivateData.ResultMimeType, "video/") {
		return "", errors.New("public video is unavailable")
	}
	return TaskMediaPublicURL(task.PrivateData.ResultStorageKey)
}

// PresentPublicTaskVideo applies one delivery policy after any model renderer.
// Provider metadata is retained; known result URL slots point to our one copy.
func PresentPublicTaskVideo(payload []byte, task *model.Task) ([]byte, error) {
	if !TaskMediaPublicEnabled() || task == nil || task.Status != model.TaskStatusSuccess {
		return payload, nil
	}
	publicURL := TaskVideoDeliveryURL(context.Background(), task)
	var value map[string]any
	if err := common.Unmarshal(payload, &value); err != nil || value == nil {
		return nil, errors.New("invalid video result response")
	}
	for _, key := range []string{"url", "video_url", "result_url"} {
		value[key] = publicURL
	}
	for _, key := range []string{"content", "metadata"} {
		if nested, ok := value[key].(map[string]any); ok {
			for _, field := range []string{"url", "video_url", "result_url"} {
				if _, exists := nested[field]; exists {
					nested[field] = publicURL
				}
			}
		}
	}
	return common.Marshal(value)
}

type MediaUploadRequest struct {
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
}

type MediaUploadReceipt struct {
	URL       string            `json:"url"`
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt int64             `json:"expires_at"`
}

// IssueMediaUpload grants one write-once object, never bucket credentials or
// arbitrary key access. The Worker verifies the exact length, digest and MIME.
func IssueMediaUpload(request MediaUploadRequest) (*MediaUploadReceipt, error) {
	extensions := map[string]string{
		"video/mp4": "mp4", "video/webm": "webm", "video/quicktime": "mov",
		"audio/mpeg": "mp3", "audio/wav": "wav", "audio/ogg": "ogg", "audio/mp4": "m4a", "audio/flac": "flac",
		"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp", "image/gif": "gif",
	}
	extension, ok := extensions[request.MimeType]
	digest, err := base64.RawURLEncoding.DecodeString(request.SHA256)
	if !ok || request.Size <= 0 || request.Size > TaskMediaMaxUploadBytes || err != nil || len(digest) != sha256.Size {
		return nil, errors.New("supported media MIME, SHA-256 and a size of 1–100000000 bytes are required")
	}
	secret := strings.TrimSpace(common.GetEnvOrDefaultString("TASK_VIDEO_WORKER_SECRET", ""))
	if len(secret) < 32 {
		return nil, errors.New("media upload signing is not configured")
	}
	key := "reference-media/" + uuid.NewString() + "." + extension
	publicURL, err := TaskMediaPublicURL(key)
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(10 * time.Minute).Unix()
	payload, err := common.Marshal(map[string]any{"key": key, "size": request.Size, "mime_type": request.MimeType, "sha256": request.SHA256, "expires": expires})
	if err != nil {
		return nil, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("media-upload.v1." + encoded))
	ticket := encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return &MediaUploadReceipt{
		URL: publicURL, UploadURL: publicURL, Method: "PUT", ExpiresAt: expires,
		Headers: map[string]string{"Content-Type": request.MimeType, "X-Media-Upload-Token": ticket},
	}, nil
}

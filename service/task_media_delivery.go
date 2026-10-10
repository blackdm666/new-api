package service

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
)

// TaskVideoDeliveryURL is presentation-only: it never archives, renews or
// changes the provider snapshot. Internal download code still uses GetResultURL.
func TaskVideoDeliveryURL(ctx context.Context, task *model.Task) string {
	if task == nil {
		return ""
	}
	if !TaskMediaPublicEnabled() || task.Status != model.TaskStatusSuccess {
		return task.GetResultURL()
	}
	if publicURL, err := PublicTaskVideoURL(task); err == nil {
		return publicURL
	}
	if source, ok := TaskVideoDirectContent(task); ok && !source.Expired(time.Now()) {
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if direct, err := taskVideoDirectProbe(probeCtx, source.URL); err == nil && direct {
			return source.URL
		}
	}
	fallback, err := BuildTaskArtifactContentURL(task.TaskID, "video")
	if err != nil {
		return "" // Fail closed; never expose a private provider URL on configuration failure.
	}
	return fallback
}

// TaskVideoDirectSource is an allow-listed upstream result. ExpiresAt is the
// Unix time the URL's own signature states, or 0 when it states none.
type TaskVideoDirectSource struct {
	URL       string
	ExpiresAt int64
}

// Expired reports whether a signed link can no longer be opened in time.
// Links without a stated expiry are never treated as expired.
func (s TaskVideoDirectSource) Expired(now time.Time) bool {
	return s.ExpiresAt > 0 && !now.Add(taskVideoDirectExpirySkew).Before(time.Unix(s.ExpiresAt, 0))
}

// TaskVideoDirectContent returns the upstream result of a successful,
// unarchived task when it passes the shared direct-delivery policy. It is the
// same source TaskVideoDeliveryURL presents, without the liveness probe, so
// anonymous content requests never trigger outbound traffic. Callers decide
// what an expired source means.
func TaskVideoDirectContent(task *model.Task) (TaskVideoDirectSource, bool) {
	if task == nil || task.Status != model.TaskStatusSuccess || task.PrivateData.ResultStorageKey != "" {
		return TaskVideoDirectSource{}, false
	}
	source := strings.TrimSpace(ResolveTaskVideoResultURL(task, task.GetResultURL()))
	if !taskVideoURLDirectEligible(task, source) {
		return TaskVideoDirectSource{}, false
	}
	direct := TaskVideoDirectSource{URL: source}
	if parsed, err := url.Parse(source); err == nil {
		if expiresAt, ok := taskVideoURLExpiry(parsed); ok {
			direct.ExpiresAt = expiresAt.Unix()
		}
	}
	return direct, true
}

// TaskArtifactDeliverySource is an already validated plugin content descriptor.
// Credentials and request bodies never enter public response construction.
type TaskArtifactDeliverySource struct {
	URL       string
	Method    string
	Anonymous bool
}

// TaskArtifactDeliveryURL resolves one explicit artifact. A legacy task-wide
// video key is usable only when the sole video's source matches the task result.
// Ambiguous/multiple outputs retain their individual capability links.
func TaskArtifactDeliveryURL(ctx context.Context, task *model.Task, artifact types.TaskArtifact, singleVideo bool, source *TaskArtifactDeliverySource, fallback string) string {
	if !TaskMediaPublicEnabled() || task == nil || task.Status != model.TaskStatusSuccess {
		return fallback
	}
	if ref, err := GetTaskArtifactStore().Resolve(task, artifact.Key); err == nil && ref != nil {
		bucket := common.GetEnvOrDefaultString("TASK_VIDEO_CACHE_BUCKET", "")
		if ref.Backend == "s3" && bucket != "" && ref.Bucket == bucket && strings.HasPrefix(ref.MimeType, artifact.Type+"/") && artifact.Type != "file" {
			if publicURL, err := TaskMediaPublicURL(ref.ObjectKey); err == nil {
				return publicURL
			}
		}
		return fallback
	}
	if source == nil {
		return fallback
	}
	if artifact.Type == "video" && singleVideo && source.URL != "" && source.URL == ResolveTaskVideoResultURL(task, "") {
		if publicURL, err := PublicTaskVideoURL(task); err == nil {
			return publicURL
		}
	}
	// Generic image/audio storage is not implemented. Never infer their mapping
	// from a video result, or bypass their original access boundary.
	if artifact.Type != "video" || !singleVideo || !source.Anonymous || (source.Method != http.MethodGet && source.Method != http.MethodHead) {
		return fallback
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if direct, err := taskVideoURLCanOpenDirectly(probeCtx, task, source.URL); err == nil && direct {
		return source.URL
	}
	return fallback
}

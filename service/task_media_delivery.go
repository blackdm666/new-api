package service

import (
	"context"
	"net/http"
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
	if task.PrivateData.ResultStorageKey == "" {
		source := ResolveTaskVideoResultURL(task, task.GetResultURL())
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if direct, err := taskVideoURLCanOpenDirectly(probeCtx, task, source); err == nil && direct {
			return source
		}
	}
	fallback, err := BuildTaskArtifactContentURL(task.TaskID, "video")
	if err != nil {
		return "" // Fail closed; never expose a private provider URL on configuration failure.
	}
	return fallback
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

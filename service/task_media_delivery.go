package service

import (
	"context"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
)

// TaskVideoDeliveryURL is presentation-only: it never archives, renews or
// changes the provider snapshot. SDGO returns Volcano-hosted result URLs as
// its public delivery URL; other providers keep the NewAPI capability URL.
func TaskVideoDeliveryURL(_ context.Context, task *model.Task) string {
	if task == nil {
		return ""
	}
	if task.Status != model.TaskStatusSuccess {
		return ""
	}
	if isSDGOVideoTask(task) {
		if resultURL := strings.TrimSpace(task.GetResultURL()); resultURL != "" {
			return resultURL
		}
	}
	if publicURL, err := PublicTaskVideoURL(task); err == nil {
		return publicURL
	}
	fallback, err := BuildTaskArtifactContentURL(task.TaskID, "video")
	if err != nil {
		return "" // Fail closed; never expose a private provider URL on configuration failure.
	}
	return fallback
}

// TaskArtifactDeliverySource is an already validated plugin content descriptor.
// Credentials and request bodies never enter public response construction.
// The provider URL is used only for exact-source matching to a stored object;
// it is never returned directly to a client.
type TaskArtifactDeliverySource struct {
	URL       string
	Method    string
	Anonymous bool
}

// TaskArtifactDeliveryURL resolves one explicit artifact. SDGO's sole video
// artifact is returned as the provider result URL. Other providers retain
// their individual capability links and never expose provider URLs.
func TaskArtifactDeliveryURL(ctx context.Context, task *model.Task, artifact types.TaskArtifact, singleVideo bool, source *TaskArtifactDeliverySource, fallback string) string {
	if !TaskMediaPublicEnabled() || task == nil || task.Status != model.TaskStatusSuccess {
		return fallback
	}
	if isSDGOVideoTask(task) && artifact.Type == "video" && singleVideo && source != nil {
		if resultURL := strings.TrimSpace(source.URL); resultURL != "" {
			return resultURL
		}
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
	// Do not redirect to or expose a provider URL. The artifact capability URL
	// keeps the browser on the NewAPI origin and the content endpoint proxies
	// the provider response server-side.
	return fallback
}

func isSDGOVideoTask(task *model.Task) bool {
	if task == nil {
		return false
	}
	if task.Platform == constant.TaskPlatformSDGOVideo {
		return true
	}
	return task.PrivateData.Execution != nil &&
		task.PrivateData.Execution.TaskPlugin != nil &&
		task.PrivateData.Execution.TaskPlugin.Key == string(constant.TaskPlatformSDGOVideo)
}

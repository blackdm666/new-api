package relay

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func init() {
	model.TaskSearchTermResolver = TaskSearchTerms
}

// RootOfficialTaskID is the best-effort official task ID shown to root when
// the task plugin publishes none: an ID in the upstream snapshot, otherwise
// the upstream task ID.
func RootOfficialTaskID(task *model.Task) string {
	if task == nil {
		return ""
	}

	var snapshot struct {
		ID     string `json:"id"`
		TaskID string `json:"task_id"`
		Data   struct {
			ID     string `json:"id"`
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if len(task.Data) > 0 && common.Unmarshal(task.Data, &snapshot) == nil {
		for _, candidate := range []string{
			snapshot.ID,
			snapshot.TaskID,
			snapshot.Data.ID,
			snapshot.Data.TaskID,
		} {
			if candidate = strings.TrimSpace(candidate); candidate != "" {
				return candidate
			}
		}
	}
	return strings.TrimSpace(task.PrivateData.UpstreamTaskID)
}

// TaskSearchTerms lists the IDs a task log can be searched by, each tagged
// with who may see it: the published official ID for everyone, the root
// fallback ID and the upstream task ID for root only.
func TaskSearchTerms(task *model.Task) []model.TaskSearchTerm {
	terms := []model.TaskSearchTerm{}
	if id := PublicOfficialTaskID(task); id != "" {
		terms = append(terms, model.TaskSearchTerm{Kind: model.TaskSearchTermKindOfficial, Value: id})
	}
	if id := RootOfficialTaskID(task); id != "" {
		terms = append(terms, model.TaskSearchTerm{Kind: model.TaskSearchTermKindRootOfficial, Value: id})
	}
	if id := strings.TrimSpace(task.PrivateData.UpstreamTaskID); id != "" {
		terms = append(terms, model.TaskSearchTerm{Kind: model.TaskSearchTermKindUpstream, Value: id})
	}
	return terms
}

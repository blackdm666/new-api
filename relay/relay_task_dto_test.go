package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestTaskModel2DtoExposesTaskLogDisplayFields(t *testing.T) {
	task := &model.Task{
		ChannelId:   71,
		ChannelName: "Vertex video",
		Quota:       500_000,
		Properties: model.Properties{
			OriginModelName:   "gemini-omni-flash",
			UpstreamModelName: "gemini-omni-flash-preview",
		},
		PrivateData: model.TaskPrivateData{
			BillingSource: "subscription",
			ResultURL:     "https://api.example.com/v1/videos/task_123/content",
		},
	}

	result := TaskModel2Dto(task)

	assert.Equal(t, 71, result.ChannelId)
	assert.Equal(t, "Vertex video", result.ChannelName)
	assert.Equal(t, 500_000, result.Quota)
	assert.Equal(t, "subscription", result.BillingSource)
	assert.Equal(t, task.Properties, result.Properties)
	assert.Equal(t, task.GetResultURL(), result.ResultURL)
}

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelSupportsCanonicalPlaygroundPathForModel(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeAdvancedCustom}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{
			Routes: []dto.AdvancedCustomRoute{
				{
					IncomingPath: "/v1/chat/completions",
					UpstreamPath: "/v1/responses",
					Models:       []string{"deepseek-v4-pro"},
				},
			},
		},
	})

	if !channelSupportsRequestPath(channel, "/pg/chat/completions", "deepseek-v4-pro") {
		t.Fatal("Playground path should match the channel's canonical /v1 route")
	}
	if channelSupportsRequestPath(channel, "/pg/chat/completions", "other-model") {
		t.Fatal("canonical path matching must still enforce route model restrictions")
	}
}

func TestGetModelRequestSupportsPlaygroundMediaRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name      string
		path      string
		wantModel string
		wantMode  int
	}{
		{name: "image", path: "/pg/images/generations", wantModel: "gpt-image-1", wantMode: relayconstant.RelayModeUnknown},
		{name: "video", path: "/pg/videos", wantModel: "seedance-2.5", wantMode: relayconstant.RelayModeVideoSubmit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(
				http.MethodPost,
				tt.path,
				strings.NewReader(`{"model":"`+tt.wantModel+`","group":"vip","prompt":"test"}`),
			)
			context.Request.Header.Set("Content-Type", "application/json")

			request, shouldSelect, err := getModelRequest(context)
			require.NoError(t, err)
			require.True(t, shouldSelect, "Playground media submission should select a channel")
			assert.Equal(t, tt.wantModel, request.Model)
			assert.Equal(t, "vip", request.Group)
			assert.Equal(t, tt.wantMode, context.GetInt("relay_mode"))
		})
	}
}

func TestGetModelRequestSupportsPlaygroundVideoFetch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/pg/videos/task_123", nil)

	_, shouldSelect, err := getModelRequest(context)
	require.NoError(t, err)
	assert.False(t, shouldSelect, "Playground video fetch must use the task's stored channel")
	assert.Equal(t, relayconstant.RelayModeVideoFetchByID, context.GetInt("relay_mode"))
}

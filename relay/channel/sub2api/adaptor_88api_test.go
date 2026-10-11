package sub2api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func doSub2APIRequest(t *testing.T, header http.Header, preset string) *gin.Context {
	t.Helper()
	service.InitHttpClient()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key, values := range header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	apiType, ok := common.ChannelType2APIType(constant.ChannelTypeSub2API)
	require.True(t, ok)
	adaptor := relay.GetAdaptor(apiType)
	require.NotNil(t, adaptor)

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeSub2API,
			ChannelBaseUrl: upstream.URL,
			ApiKey:         "test-key",
		},
		RequestURLPath: "/v1/responses",
		RelayMode:      relayconstant.RelayModeResponses,
		RelayFormat:    types.RelayFormatOpenAIResponses,
	}
	adaptor.Init(info)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(context.Background())
	if preset != "" {
		c.Set(common.UpstreamRequestIdKey, preset)
	}

	resp, err := adaptor.DoRequest(c, info, bytes.NewReader([]byte(`{"model":"gpt-test"}`)))
	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.(*http.Response).Body.Close()
	return c
}

func TestSub2APIDoRequestRecordsRequestID(t *testing.T) {
	c := doSub2APIRequest(t, http.Header{"X-Request-ID": {"7a0e6f0c-1b7e-4a39-9d4c-2f7c1f2b9e11"}}, "")
	assert.Equal(t, "7a0e6f0c-1b7e-4a39-9d4c-2f7c1f2b9e11", c.GetString(common.UpstreamRequestIdKey))
}

func TestSub2APIDoRequestFallsBackToOneAPIRequestID(t *testing.T) {
	c := doSub2APIRequest(t, http.Header{common.RequestIdKey: {"oneapi-upstream"}}, "")
	assert.Equal(t, "oneapi-upstream", c.GetString(common.UpstreamRequestIdKey))
}

func TestSub2APIDoRequestClearsEarlierAttemptRequestID(t *testing.T) {
	c := doSub2APIRequest(t, http.Header{}, "id-from-earlier-attempt")
	value, exists := c.Get(common.UpstreamRequestIdKey)
	assert.True(t, exists)
	assert.Equal(t, "", value)
}

func TestSub2APIDoRequestDropsOversizedRequestID(t *testing.T) {
	c := doSub2APIRequest(t, http.Header{"X-Request-Id": {strings.Repeat("a", 129)}}, "")
	assert.Equal(t, "", c.GetString(common.UpstreamRequestIdKey))
}

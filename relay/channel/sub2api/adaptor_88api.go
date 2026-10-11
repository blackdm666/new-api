package sub2api

import (
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

// sub2api echoes its own request id on every response and stores the same id
// as ops_error_logs.request_id, so recording it as the upstream request id lets
// a new-api log row be joined exactly to sub2api's error and policy records.
const sub2apiRequestIDHeader = "X-Request-Id"

// maxUpstreamRequestIDLen matches the logs.upstream_request_id column size.
const maxUpstreamRequestIDLen = 128

// DoRequest shadows the embedded newapi.Adaptor.DoRequest to capture sub2api's
// request id for this attempt.
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	resp, err := a.Adaptor.DoRequest(c, info, requestBody)
	if httpResp, ok := resp.(*http.Response); ok && httpResp != nil {
		captureUpstreamRequestID(c, httpResp)
	}
	return resp, err
}

// captureUpstreamRequestID always overwrites the context value so a retry never
// logs the id of an earlier attempt.
func captureUpstreamRequestID(c *gin.Context, resp *http.Response) {
	id := resp.Header.Get(sub2apiRequestIDHeader)
	if id == "" {
		id = resp.Header.Get(common.RequestIdKey)
	}
	if len(id) > maxUpstreamRequestIDLen {
		id = ""
	}
	c.Set(common.UpstreamRequestIdKey, id)
}

package sub2api

import (
	"io"
	"net/http"
	"regexp"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

// sub2api echoes its own request id on every response and stores the same id
// as ops_error_logs.request_id, so recording it as the upstream request id lets
// a new-api log row be joined exactly to sub2api's error and policy records.
const sub2apiRequestIDHeader = "X-Request-Id"

// upstreamRequestIDPattern keeps ids within the logs.upstream_request_id column
// (varchar(128)) and plain ASCII, so a malformed upstream header can never make
// the log insert fail.
var upstreamRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// DoRequest shadows the embedded newapi.Adaptor.DoRequest to capture sub2api's
// request id for this attempt. The id is cleared first so an attempt that gets
// no response never logs the id of an earlier attempt.
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	c.Set(common.UpstreamRequestIdKey, "")
	resp, err := a.Adaptor.DoRequest(c, info, requestBody)
	if httpResp, ok := resp.(*http.Response); ok && httpResp != nil {
		captureUpstreamRequestID(c, httpResp)
	}
	return resp, err
}

func captureUpstreamRequestID(c *gin.Context, resp *http.Response) {
	id := resp.Header.Get(sub2apiRequestIDHeader)
	if id == "" {
		id = resp.Header.Get(common.RequestIdKey)
	}
	if !upstreamRequestIDPattern.MatchString(id) {
		id = ""
	}
	c.Set(common.UpstreamRequestIdKey, id)
}

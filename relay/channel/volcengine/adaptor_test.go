package volcengine

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ark bills the web search plugin per keyword and reports the count in
// usage.tool_usage.web_search; one web_search_call item can carry several
// keywords joined by ";". The vendor count replaces the per-item count, an
// absent count keeps it, and Chat Completions never bills a search.
func TestVolcResponsesWebSearchBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})

	searchCall := func(query string) string {
		return `{"type":"web_search_call","status":"completed","action":{"type":"search","query":"` + query + `"}}`
	}
	const message = `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}`
	const tokens = `"input_tokens":10,"output_tokens":3,"total_tokens":13`
	twoKeywords := searchCall("北京天气;上海天气")

	tests := []struct {
		name   string
		stream bool
		output []string
		usage  string
		want   map[string]int
	}{
		{
			name:   "json keywords replace call items",
			output: []string{twoKeywords, twoKeywords},
			usage:  `{` + tokens + `,"tool_usage":{"web_search":4},"tool_usage_details":{"web_search":{"search_engine":4}}}`,
			want:   map[string]int{"web_search": 4},
		},
		{
			name:   "stream keywords replace call items",
			stream: true,
			output: []string{twoKeywords, twoKeywords},
			usage:  `{` + tokens + `,"tool_usage":{"web_search":4}}`,
			want:   map[string]int{"web_search": 4},
		},
		{
			name:   "json without tool_usage keeps call items",
			output: []string{searchCall("北京天气"), searchCall("上海天气")},
			usage:  `{` + tokens + `}`,
			want:   map[string]int{"web_search": 2},
		},
		{
			name:   "stream reported zero bills nothing",
			stream: true,
			output: []string{searchCall("北京天气")},
			usage:  `{` + tokens + `,"tool_usage":{"web_search":0}}`,
			want:   map[string]int{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := strings.Join(append(tt.output, message), ",")
			response := `{"id":"resp_1","object":"response","model":"deepseek-v4-1-flash-261017","status":"completed","output":[` + output + `],"usage":` + tt.usage + `}`
			body, contentType := response, "application/json"
			if tt.stream {
				var frames strings.Builder
				for _, item := range tt.output {
					frames.WriteString(`data: {"type":"response.output_item.done","output_index":0,"item":` + item + "}\n\n")
				}
				frames.WriteString(`data: {"type":"response.completed","response":` + response + "}\n\ndata: [DONE]\n\n")
				body, contentType = frames.String(), "text/event-stream"
			}

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{
				RelayMode:       relayconstant.RelayModeResponses,
				RelayFormat:     types.RelayFormatOpenAIResponses,
				OriginModelName: "deepseek-v4.1-flash",
				IsStream:        tt.stream,
				DisablePing:     true,
				ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeVolcEngine, ChannelBaseUrl: "https://ark.invalid", UpstreamModelName: "deepseek-v4-1-flash-261017"},
				// The client declared tools:[{"type":"web_search"}].
				ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{
					BuiltInTools: map[string]*relaycommon.BuildInToolInfo{
						dto.BuildInToolWebSearch: {ToolName: dto.BuildInToolWebSearch},
					},
				},
			}
			adaptor := &Adaptor{}
			adaptor.Init(info)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{contentType}},
			}
			_, apiErr := adaptor.DoResponse(c, resp, info)
			require.Nil(t, apiErr)

			billed := map[string]int{}
			for name, tool := range info.ResponsesUsageInfo.BuiltInTools {
				if tool.CallCount > 0 {
					billed[name] = tool.CallCount
				}
			}
			assert.Equal(t, tt.want, billed)
		})
	}

	chat := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions}
	(&Adaptor{}).Init(chat)
	assert.Nil(t, chat.VendorToolUsage)
}

package volcengine

import (
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/tidwall/gjson"
)

// volcResponsesToolUsage bills Ark Responses web search by the count the
// service reports in usage.tool_usage.web_search. Ark bills the web search
// plugin per search keyword: one web_search_call item can carry several
// keywords (the action query joins them with ";"), so counting the items
// undercounts what Ark charges. The vendor count replaces the
// web_search_call items counted under the protocol keys; the terminal stream
// frame carries the same usage under response. gjson truncates fractional
// values and reads non-numbers as zero; SetBillableToolCount ignores
// negatives and settlement applies MaxBillableToolCallCount.
func volcResponsesToolUsage(raw []byte) map[string]int {
	count := gjson.GetBytes(raw, "usage.tool_usage.web_search")
	if !count.Exists() {
		count = gjson.GetBytes(raw, "response.usage.tool_usage.web_search")
	}
	if !count.Exists() {
		return nil
	}
	return map[string]int{
		dto.BuildInToolWebSearch:        int(count.Int()),
		dto.BuildInToolWebSearchPreview: 0,
	}
}

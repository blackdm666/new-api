package taskcommon

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

func TestModelConfigCandidatesPreserveSalesProfileThenUseUpstreamFallback(t *testing.T) {
	info := &relaycommon.RelayInfo{
		OriginModelName: "sales-1080p",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "upstream-model",
			IsModelMapped:     true,
		},
	}

	assert.Equal(t, []string{"sales-1080p", "upstream-model"}, ModelConfigCandidates(info, "sales-1080p"))
	assert.Equal(t, "upstream-model", ResolvedModelName(info, "sales-1080p"))
}

package helper

import (
	basecommon "github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

func ModelMappedHelper(c *gin.Context, info *relaycommon.RelayInfo, request dto.Request) error {
	if info.ChannelMeta == nil {
		info.ChannelMeta = &relaycommon.ChannelMeta{}
	}

	mappedModel, isMapped, err := basecommon.ResolveMappedModelName(info.OriginModelName, c.GetString("model_mapping"))
	if err != nil {
		return err
	}
	info.IsModelMapped = isMapped
	if isMapped {
		info.UpstreamModelName = mappedModel
	}

	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	return nil
}

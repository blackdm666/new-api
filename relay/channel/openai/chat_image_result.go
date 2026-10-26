package openai

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// chatImageLinkPattern matches the image links image models put in a chat
// reply: Markdown images ("![image](https://host/a.png)") and bare links that
// make up the whole reply.
var chatImageLinkPattern = regexp.MustCompile(`!\[[^\]]*\]\((https?://[^\s)]+)\)|^\s*(https?://\S+)\s*$`)

// isImageModelChatReply reports whether a non-stream chat reply comes from an
// image model, whose message content carries the generated images as links.
func isImageModelChatReply(info *relaycommon.RelayInfo) bool {
	return info != nil && info.ChannelMeta != nil &&
		info.RelayMode == relayconstant.RelayModeChatCompletions &&
		common.IsImageGenerationModel(info.UpstreamModelName)
}

// deliverChatImageLinks gives chat replies from image models the treatment
// deliverImageResults gives the images endpoints: upstream result links name
// the provider and expire, so each one is stored as our own reference media
// and the reply points there instead.
func deliverChatImageLinks(c *gin.Context, responseBody []byte) ([]byte, *types.NewAPIError) {
	choices := gjson.GetBytes(responseBody, "choices")
	if !choices.IsArray() {
		return responseBody, nil
	}
	ctx := c.Request.Context()
	body := responseBody
	for i, choice := range choices.Array() {
		content := choice.Get("message.content")
		if content.Type != gjson.String {
			continue
		}
		text := content.String()
		var deliverErr error
		replaced := chatImageLinkPattern.ReplaceAllStringFunc(text, func(match string) string {
			if deliverErr != nil {
				return match
			}
			groups := chatImageLinkPattern.FindStringSubmatch(match)
			source := groups[1]
			if source == "" {
				source = groups[2]
			}
			image, err := downloadImageResult(ctx, source)
			if err != nil {
				deliverErr = err
				return match
			}
			link, err := uploadImageResult(ctx, bytes.NewReader(image))
			if err != nil {
				deliverErr = err
				return match
			}
			return strings.Replace(match, source, link, 1)
		})
		if deliverErr == nil && replaced != text {
			body, deliverErr = sjson.SetBytes(body, fmt.Sprintf("choices.%d.message.content", i), replaced)
		}
		if deliverErr != nil {
			// Same policy as deliverImageResults: keep the upstream host out of
			// the error and do not retry a generation the upstream has charged.
			logger.LogError(c, common.LogText("chat image result delivery failed: %s", deliverErr.Error()))
			return nil, types.NewOpenAIError(errors.New("failed to deliver generated images"), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
		}
	}
	return body, nil
}

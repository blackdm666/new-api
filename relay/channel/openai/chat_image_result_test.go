package openai

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Image models reached through /v1/chat/completions answer with the image as a
// link in the message content (Markdown or a bare URL). Those links name the
// upstream and expire, so they are stored as our media like image endpoint
// results.
func TestChatImageLinksAreDelivered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	results := map[string][]byte{
		"https://images.silkroadai.io/gen/a.png":                       []byte("first"),
		"https://pre-signed-firefly.s3.amazonaws.com/images/b?x=1&y=2": []byte("second"),
	}
	reply := func(content string) string {
		return fmt.Sprintf(`{"id":"chatcmpl-1","object":"chat.completion","model":"gpt-image-2.5-sunburst","choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":196,"total_tokens":216}}`, content)
	}

	for _, tc := range []struct {
		name, model, content, want string
	}{
		{"markdown image", "gpt-image-2.5-sunburst", "![image](https://images.silkroadai.io/gen/a.png)",
			"![image](https://assets.example/media/reference-media/1.png)"},
		{"bare link reply", "gpt-image-2", "https://pre-signed-firefly.s3.amazonaws.com/images/b?x=1&y=2",
			"https://assets.example/media/reference-media/1.png"},
		{"text models are untouched", "gpt-5.5", "see https://images.silkroadai.io/gen/a.png",
			"see https://images.silkroadai.io/gen/a.png"},
		{"text around an image link is kept", "gpt-image-2", "Here you go:\n![cat](https://images.silkroadai.io/gen/a.png)\nEnjoy",
			"Here you go:\n![cat](https://assets.example/media/reference-media/1.png)\nEnjoy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubXinMengMedia(t, results)
			c, recorder, resp, info := newImageTestContext(t, reply(tc.content), "application/json", false)
			info.RelayMode = relayconstant.RelayModeChatCompletions
			info.RelayFormat = types.RelayFormatOpenAI
			info.UpstreamModelName = tc.model

			_, apiErr := OpenaiHandler(c, info, resp)
			require.Nil(t, apiErr)
			assert.Equal(t, tc.want, gjson.Get(recorder.Body.String(), "choices.0.message.content").String())
			assert.Equal(t, int64(196), gjson.Get(recorder.Body.String(), "usage.completion_tokens").Int())
		})
	}

	t.Run("delivery failure hides the upstream host and is not retried", func(t *testing.T) {
		stubXinMengMedia(t, results)
		downloadImageResult = func(_ context.Context, sourceURL string) ([]byte, error) {
			return nil, fmt.Errorf("Get %q: timeout", sourceURL)
		}
		c, recorder, resp, info := newImageTestContext(t, reply("![image](https://images.silkroadai.io/gen/a.png)"), "application/json", false)
		info.RelayMode = relayconstant.RelayModeChatCompletions
		info.RelayFormat = types.RelayFormatOpenAI
		info.UpstreamModelName = "gpt-image-2.5-sunburst"

		_, apiErr := OpenaiHandler(c, info, resp)
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
		assert.NotContains(t, apiErr.Error(), "silkroadai")
		assert.Empty(t, recorder.Body.String())
	})
}

package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynamicProtocolUsesChannelModelsAndPriority(t *testing.T) {
	for _, memory := range []bool{true} {
		t.Run(fmt.Sprintf("memory=%t", memory), func(t *testing.T) {
			setupTaskPluginRouteDB(t)
			require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Ability{}))
			oldMemory := common.MemoryCacheEnabled
			common.MemoryCacheEnabled = memory
			t.Cleanup(func() { common.MemoryCacheEnabled = oldMemory })
			for i, key := range []string{"dynamic-route-alpha", "dynamic-route-beta", "dynamic-route-unbound"} {
				source := taskProtocolPluginSource(key, "1.0.0", "[]", "/v1/videos", `return {model:ctx.model,requestBody:ctx.requestBody,action:"text_to_video"};`)
				source = strings.Replace(source, "models: [],", "models: [], dynamicModels: true,", 1)
				_, err := jsplugin.DefaultRegistry.Register(source, jsplugin.Options{})
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, jsplugin.DefaultRegistry.Unregister(key)) })
				if i == 2 {
					continue
				}
				setting := fmt.Sprintf(`{"task_plugin_key":%q}`, key)
				priority := int64(20 - i*10)
				ch := model.Channel{Id: 930001 + i, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Name: key, Key: "fixture", Models: "dynamic-new-video", Group: "default", Setting: &setting, Priority: &priority}
				require.NoError(t, ch.Insert())
			}
			model.InitChannelCache()
			var selected []int
			var reached bool
			router := gin.New()
			router.POST("/v1/videos", PinTaskPluginEndpoint(), PrepareTaskPluginEndpoint(), func(c *gin.Context) {
				reached = true
				value, pinned := c.Get(jsplugin.ContextKeyPinnedEndpoint)
				if !pinned {
					c.Status(http.StatusNoContent)
					return
				}
				endpoint := value.(jsplugin.PinnedEndpoint)
				require.Len(t, endpoint.Candidates, 2, "unbound dynamic plugin must not enter the request")
				for retry := 0; retry < 2; retry++ {
					channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{Ctx: c, TokenGroup: "default", ModelName: "dynamic-new-video", Retry: &retry})
					require.NoError(t, err)
					require.NotNil(t, channel)
					require.Nil(t, SetupContextForSelectedChannel(c, channel, "dynamic-new-video"))
					selected = append(selected, channel.Id)
					pin := c.MustGet(jsplugin.ContextKeyPinnedPlugin).(jsplugin.PinnedPlugin)
					assert.Equal(t, channel.GetSetting().TaskPluginKey, pin.Plugin.Meta.Key)
				}
				c.Status(http.StatusNoContent)
			})
			request := func(name string) {
				reached = false
				r := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(fmt.Sprintf(`{"model":%q,"prompt":"test"}`, name)))
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
				assert.True(t, reached)
			}
			request("dynamic-new-video")
			assert.Equal(t, []int{930001, 930002}, selected)
			selected = nil
			request("unrelated-video")
			assert.Empty(t, selected, "dynamic plugins must leave unrelated models untouched")
		})
	}
}

package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynamicTaskCandidatesFollowEnabledChannelBindings(t *testing.T) {
	truncateTables(t)
	registry := jsplugin.NewRegistry()
	for _, key := range []string{"dynamic-alpha", "dynamic-beta", "dynamic-unused"} {
		_, err := registry.Register(fmt.Sprintf(`
export const meta = {apiVersion: 1, key: %q, name: "Dynamic", version: "1.0.0", author: {name: "Test"}, models: [], dynamicModels: true, fetchMode: "per_task", protocols: ["openai_video"]};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {}; }
export function listArtifacts() { return []; }
export function buildContentRequest() { return {}; }
export const protocols = {openai_video: {decodeRequest: function(ctx) {return ctx;}, render: function(ctx, task) {return task;}}};
`, key), jsplugin.Options{})
		require.NoError(t, err)
	}
	for i, key := range []string{"dynamic-alpha", "dynamic-beta", "dynamic-unused"} {
		setting := fmt.Sprintf(`{"task_plugin_key":%q}`, key)
		priority := int64(20 - i*10)
		channel := Channel{Id: 920001 + i, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Name: key, Key: "fixture", Models: "shared,new-model", Group: "default", Setting: &setting, Priority: &priority}
		if i == 2 {
			channel.Status = common.ChannelStatusManuallyDisabled
		}
		require.NoError(t, channel.Insert())
	}
	g := registry.Generation()
	previous := taskAliasViewPtr.Load()
	t.Cleanup(func() { taskAliasViewPtr.Store(previous) })
	refresh := func() { taskAliasViewPtr.Store(buildTaskAliasView(g)) }
	refresh()
	keys := func(name string) []string {
		var result []string
		for _, candidate := range DynamicTaskEndpointCandidates(g, "POST", "/v1/videos", name) {
			assert.Equal(t, name, candidate.Model)
			result = append(result, candidate.Plugin.Meta.Key)
		}
		return result
	}
	assert.Equal(t, []string{"dynamic-alpha", "dynamic-beta"}, keys("shared"))
	assert.Equal(t, []string{"dynamic-alpha", "dynamic-beta"}, keys("new-model"))
	assert.Empty(t, keys("not-offered"), "a protocol declaration must not globally claim models")
	assert.Empty(t, DynamicTaskEndpointCandidates(g, "POST", "/v1/chat/completions", "shared"))
	oldMemory := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemory })
	for _, memory := range []bool{false, true} {
		common.MemoryCacheEnabled = memory
		InitChannelCache()
		filters := []dto.ChannelFilter{{Kind: dto.FilterTaskPluginIdentity, TaskPluginKey: "dynamic-alpha", TaskPluginKeys: []string{"dynamic-alpha", "dynamic-beta"}}}
		for retry, id := range []int{920001, 920002} {
			channel, err := GetRandomSatisfiedChannel("default", "shared", retry, filters)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, id, channel.Id, "memory=%v retry=%d", memory, retry)
		}
	}
	refresh()
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 920001).Update("models", "new-model").Error)
	refresh()
	assert.Equal(t, []string{"dynamic-beta"}, keys("shared"), "removing a channel model releases that binding")
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 920002).Update("status", common.ChannelStatusManuallyDisabled).Error)
	refresh()
	assert.Empty(t, keys("shared"))
	assert.Equal(t, []string{"dynamic-alpha"}, keys("new-model"))
}

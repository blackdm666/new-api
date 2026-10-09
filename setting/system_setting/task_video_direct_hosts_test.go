package system_setting

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useTaskVideoDirectHostsOption(t *testing.T, value *string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = map[string]string{}
	if value != nil {
		common.OptionMap[TaskVideoDirectHostsOptionKey] = *value
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})
}

func TestTaskVideoDirectHostWildcardSemantics(t *testing.T) {
	list := "*.volces.com, store.vod-qcloud.com;*.Klingai.COM.\n dashscope-a717.oss-accelerate.aliyuncs.com *"
	useTaskVideoDirectHostsOption(t, &list)

	for _, host := range []string{
		"ark-acg-cn-beijing.tos-cn-beijing.volces.com",
		"a.b.c.volces.com",
		"ARK.VOLCES.COM",
		"cdn.volces.com.",
		"store.vod-qcloud.com",
		"Store.Vod-Qcloud.Com",
		"v1.klingai.com",
		"dashscope-a717.oss-accelerate.aliyuncs.com",
	} {
		assert.True(t, TaskVideoDirectHostAllowed(host), host)
	}
	for _, host := range []string{
		"volces.com",                   // wildcard never matches the bare domain
		"evil-volces.com",              // suffix must start at a label boundary
		"volces.com.evil.com",          // listed name as a prefix of another domain
		"cdn.volces.com.evil.com",      // listed subdomain as a prefix of another domain
		"evilvolces.com",               // no dot boundary
		"cdn.store.vod-qcloud.com",     // exact entries do not cover subdomains
		"store.vod-qcloud.com.evil.cn", // exact entries are not prefixes
		"klingai.com",
		"other.example",
		"",
		".",
	} {
		assert.False(t, TaskVideoDirectHostAllowed(host), host)
	}
}

func TestTaskVideoDirectHostsOptionOverridesEnvironment(t *testing.T) {
	t.Setenv(TaskVideoDirectHostsEnv, "*.env.example")

	useTaskVideoDirectHostsOption(t, nil)
	assert.Equal(t, "*.env.example", TaskVideoDirectHosts())
	assert.True(t, TaskVideoDirectHostAllowed("cdn.env.example"))

	saved := "*.saved.example"
	useTaskVideoDirectHostsOption(t, &saved)
	assert.Equal(t, saved, TaskVideoDirectHosts())
	assert.True(t, TaskVideoDirectHostAllowed("cdn.saved.example"))
	assert.False(t, TaskVideoDirectHostAllowed("cdn.env.example"))

	empty := ""
	useTaskVideoDirectHostsOption(t, &empty)
	assert.Empty(t, TaskVideoDirectHosts())
	assert.False(t, TaskVideoDirectHostAllowed("cdn.env.example"), "a saved empty list archives everything")
}

func TestValidateTaskVideoDirectHosts(t *testing.T) {
	production := "*.volces.com,*.volcvideo.com,dashscope-result-bj.oss-cn-beijing.aliyuncs.com,*.klingai.com,dashscope-a717.oss-accelerate.aliyuncs.com,*.minimax.io,*.minimax.cn,*.minimaxi.com,cdn.hailuoai.video,filecdn.minimax.chat,store.vod-qcloud.com"
	for _, value := range []string{"", production, "Cdn.Example.COM.\n*.media.example"} {
		require.NoError(t, ValidateTaskVideoDirectHosts(value), value)
	}
	for _, value := range []string{
		"*",
		"*.com",
		"localhost",
		"https://cdn.example.com",
		"cdn.example.com/path",
		"cdn.example.com:443",
		"1.2.3.4",
		"*.*.example.com",
		"cdn*.example.com",
		"-cdn.example.com",
		"cdn..example.com",
		"cdn.example.123",
		"user@cdn.example.com",
		strings.Repeat("a", 64) + ".example.com",
	} {
		assert.Error(t, ValidateTaskVideoDirectHosts(value), value)
	}
	assert.Error(t, ValidateTaskVideoDirectHosts(strings.Repeat("a.example.com,", 300)))
}

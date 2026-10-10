package service

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useTaskVideoDirectHostsOption(t *testing.T, value *string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = map[string]string{}
	if value != nil {
		common.OptionMap[system_setting.TaskVideoDirectHostsOptionKey] = *value
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})
}

func setTaskVideoDirectHostsOption(value string) {
	common.OptionMapRWMutex.Lock()
	common.OptionMap[system_setting.TaskVideoDirectHostsOptionKey] = value
	common.OptionMapRWMutex.Unlock()
}

func TestTaskVideoDirectPolicyIsSharedByArchiveQueryAndContent(t *testing.T) {
	t.Setenv("TASK_MEDIA_PUBLIC_ENABLED", "true")
	t.Setenv("TASK_MEDIA_PUBLIC_BASE_URL", "https://assets.88api.ai/media")
	t.Setenv("TASK_VIDEO_CACHE_ENABLED", "true")
	t.Setenv("TASK_VIDEO_DIRECT_HOSTS", "*.env.example")
	useLocalTaskVideoCache(t)
	useStaticTaskVideoSource(t, "archived-video")
	previousProbe := taskVideoDirectProbe
	taskVideoDirectProbe = func(context.Context, string) (bool, error) { return true, nil }
	t.Cleanup(func() { taskVideoDirectProbe = previousProbe })
	useTaskVideoDirectHostsOption(t, nil)

	const resultURL = "https://store.vod.example/result.mp4?sign=abc"
	unarchived := func() *model.Task {
		return &model.Task{TaskID: "task_shared_policy", Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{ResultURL: resultURL}}
	}
	decide := func() (archived bool, query string, content string, contentOK bool) {
		prepared, err := PrepareTaskVideoResult(context.Background(), &model.Task{TaskID: "task_shared_policy"}, resultURL)
		require.NoError(t, err)
		task := unarchived()
		source, contentOK := TaskVideoDirectContent(task)
		return prepared.Cached, TaskVideoDeliveryURL(context.Background(), task), source.URL, contentOK
	}

	archived, query, _, contentOK := decide()
	assert.True(t, archived, "unlisted host is archived")
	assert.NotEqual(t, resultURL, query)
	assert.False(t, contentOK)

	// A saved option replaces the environment list immediately; no restart.
	setTaskVideoDirectHostsOption("store.vod.example")
	archived, query, content, contentOK := decide()
	assert.False(t, archived, "listed host stays direct")
	assert.Equal(t, resultURL, query)
	require.True(t, contentOK)
	assert.Equal(t, query, content, "content redirects to the URL the query presents")

	setTaskVideoDirectHostsOption("")
	archived, query, _, contentOK = decide()
	assert.True(t, archived)
	assert.NotEqual(t, resultURL, query)
	assert.False(t, contentOK)
}

func TestTaskVideoDirectContentURLRejectsUnsafeSources(t *testing.T) {
	t.Setenv("TASK_VIDEO_DIRECT_HOSTS", "*.official.example,official.example")
	useTaskVideoDirectHostsOption(t, nil)

	for _, tc := range []struct {
		name, url string
		direct    bool
	}{
		{"exact", "https://official.example/v.mp4", true},
		{"wildcard", "https://cdn.official.example/v.mp4", true},
		{"default-port", "https://cdn.official.example:443/v.mp4", true},
		{"uppercase", "https://CDN.Official.Example/v.mp4", true},
		{"other-port", "https://cdn.official.example:8443/v.mp4", false},
		{"plain-http", "http://cdn.official.example/v.mp4", false},
		{"hyphen-bypass", "https://evil-official.example/v.mp4", false},
		{"suffix-bypass", "https://official.example.evil.test/v.mp4", false},
		{"userinfo", "https://official.example@evil.test/v.mp4", false},
		{"credential", "https://cdn.official.example/v.mp4?api_key=secret", false},
		{"fragment", "https://cdn.official.example/v.mp4#x", false},
		{"proxy", "https://gateway.example/v1/videos/task_direct_content/content", false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := &model.Task{TaskID: "task_direct_content", Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{ResultURL: tc.url}}
			got, ok := TaskVideoDirectContent(task)
			assert.Equal(t, tc.direct, ok)
			if tc.direct {
				assert.Equal(t, tc.url, got.URL)
			}
		})
	}

	listed := "https://cdn.official.example/v.mp4"
	for name, task := range map[string]*model.Task{
		"archived":    {TaskID: "task_direct_content", Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{ResultURL: listed, ResultStorageKey: "task-videos/x.mp4"}},
		"in-progress": {TaskID: "task_direct_content", Status: model.TaskStatusInProgress, PrivateData: model.TaskPrivateData{ResultURL: listed}},
		"failed":      {TaskID: "task_direct_content", Status: model.TaskStatusFailure, FailReason: listed},
	} {
		_, ok := TaskVideoDirectContent(task)
		assert.False(t, ok, name)
	}
	_, ok := TaskVideoDirectContent(nil)
	assert.False(t, ok)
}

func TestTaskVideoURLExpiryReadsSignedLinks(t *testing.T) {
	signed := time.Date(2026, 10, 6, 1, 54, 19, 0, time.UTC)
	for _, tc := range []struct {
		name, rawURL string
		want         time.Time
		ok           bool
	}{
		{"volcengine-tos", "https://ark.tos-cn-beijing.volces.com/v.mp4?X-Tos-Algorithm=TOS4-HMAC-SHA256&X-Tos-Date=20261006T015419Z&X-Tos-Expires=86400&X-Tos-Signature=s", signed.Add(24 * time.Hour), true},
		{"s3-sigv4", "https://bucket.s3.example/v.mp4?X-Amz-Date=20261006T015419Z&X-Amz-Expires=3600&X-Amz-Signature=s", signed.Add(time.Hour), true},
		{"key-case", "https://bucket.example/v.mp4?x-tos-date=20261006T015419Z&x-tos-expires=60", signed.Add(time.Minute), true},
		{"aliyun-oss-v1", "https://dashscope.oss.example/v.mp4?Expires=1791251659&OSSAccessKeyId=id&Signature=s", time.Unix(1791251659, 0), true},
		{"tencent-cos", "https://b.cos.ap-guangzhou.myqcloud.com/v.mp4?q-sign-algorithm=sha1&q-sign-time=1791165259;1791251659&q-signature=s", time.Unix(1791251659, 0), true},
		{"unsigned", "https://store.vod-qcloud.com/v.mp4", time.Time{}, false},
		{"bare-expires", "https://cdn.example/v.mp4?Expires=1791251659", time.Time{}, false},
		{"bad-date", "https://cdn.example/v.mp4?X-Tos-Date=yesterday&X-Tos-Expires=60", time.Time{}, false},
		{"bad-seconds", "https://cdn.example/v.mp4?X-Amz-Date=20261006T015419Z&X-Amz-Expires=-5", time.Time{}, false},
		{"bad-cos-window", "https://cdn.example/v.mp4?q-sign-time=1791251659", time.Time{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := url.Parse(tc.rawURL)
			require.NoError(t, err)
			got, ok := taskVideoURLExpiry(parsed)
			assert.Equal(t, tc.ok, ok)
			if tc.ok {
				assert.True(t, tc.want.Equal(got), "got %s", got)
			}
		})
	}
}

func TestTaskVideoDirectSourceExpiry(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	assert.False(t, TaskVideoDirectSource{URL: "https://cdn.example/v.mp4"}.Expired(now), "unknown expiry is never expired")
	assert.False(t, TaskVideoDirectSource{ExpiresAt: now.Add(2 * time.Minute).Unix()}.Expired(now))
	assert.True(t, TaskVideoDirectSource{ExpiresAt: now.Add(30 * time.Second).Unix()}.Expired(now), "too close to open in time")
	assert.True(t, TaskVideoDirectSource{ExpiresAt: now.Add(-time.Hour).Unix()}.Expired(now))
}

func TestTaskVideoDeliverySkipsProbeForExpiredDirectLink(t *testing.T) {
	t.Setenv("TASK_MEDIA_PUBLIC_ENABLED", "true")
	t.Setenv("TASK_MEDIA_PUBLIC_BASE_URL", "https://assets.88api.ai/media")
	t.Setenv("TASK_VIDEO_DIRECT_HOSTS", "*.volces.example")
	useTaskVideoDirectHostsOption(t, nil)
	probes := 0
	previousProbe := taskVideoDirectProbe
	taskVideoDirectProbe = func(context.Context, string) (bool, error) { probes++; return true, nil }
	t.Cleanup(func() { taskVideoDirectProbe = previousProbe })

	signedAt := func(at time.Time) string {
		return "https://ark.volces.example/v.mp4?X-Tos-Date=" + at.UTC().Format("20060102T150405Z") + "&X-Tos-Expires=86400&X-Tos-Signature=s"
	}
	expired := &model.Task{TaskID: "task_expired_direct", Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{ResultURL: signedAt(time.Now().Add(-25 * time.Hour))}}
	delivered := TaskVideoDeliveryURL(context.Background(), expired)
	assert.Zero(t, probes, "an expired signature is decided locally")
	assert.NotEqual(t, expired.PrivateData.ResultURL, delivered, "an expired link is never presented")

	valid := &model.Task{TaskID: "task_valid_direct", Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{ResultURL: signedAt(time.Now().Add(-time.Hour))}}
	assert.Equal(t, valid.PrivateData.ResultURL, TaskVideoDeliveryURL(context.Background(), valid))
	assert.Equal(t, 1, probes)

	payload, err := PresentPublicTaskVideo([]byte(`{"id":"task_valid_direct","url":"old"}`), valid)
	require.NoError(t, err)
	var presented map[string]any
	require.NoError(t, common.Unmarshal(payload, &presented))
	source, ok := TaskVideoDirectContent(valid)
	require.True(t, ok)
	assert.EqualValues(t, source.ExpiresAt, presented["expires_at"])
	assert.Greater(t, source.ExpiresAt, time.Now().Unix())

	unsigned := &model.Task{TaskID: "task_unsigned_direct", Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{ResultURL: "https://cdn.volces.example/v.mp4"}}
	payload, err = PresentPublicTaskVideo([]byte(`{"id":"task_unsigned_direct","url":"old","expires_at":123}`), unsigned)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(payload, &presented))
	assert.EqualValues(t, 123, presented["expires_at"], "renderer value kept when the link states no expiry")
}

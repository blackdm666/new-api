package jsplugin

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const queryStreamPlugin = `
export const meta = {apiVersion:1,key:"query-stream",name:"Query stream",version:"1.0.0",
 author:{name:"Test"},models:["query-model"],fetchMode:"per_task",auth:"none",
 requiredCapabilities:["query-sse-delta@1"]};
export function buildSubmitRequest(){return {url:"https://example.invalid"};}
export function parseSubmitResponse(){return {taskId:"fixture"};}
export function buildQueryRequest(ctx){return {url:ctx.baseUrl+"/query",method:"GET",responseType:"sse"};}
export function parseQueryEventDelta(ctx,event,state){
 const body=event.body;
 if(body.kind==="part")return {changes:[{op:"set",path:[],value:body.payload}],state:{seen:true},done:false};
 if(body.kind==="end")return {changes:[],state:{seen:true},done:true};
 throw new Error("invalid fixture event");
}
export function parseTaskResult(ctx,body){return {status:body.status==="completed"?"SUCCESS":"UNKNOWN",url:"data:video/mp4;base64,"+body.outputs[0].data};}
`

func queryTestAdaptor(t *testing.T, source string) *TaskAdaptor {
	t.Helper()
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	return New(plugin)
}

func TestQuerySSECanonicalJSONAndLargeMedia(t *testing.T) {
	// This crosses the old 1MiB submit ceiling without changing that contract.
	video := bytes.Repeat([]byte("media"), (1<<20)/5+1)
	encoded := base64.StdEncoding.EncodeToString(video)
	event, err := common.Marshal(map[string]any{"kind": "part", "payload": map[string]any{
		"status": "completed", "outputs": []any{map[string]any{"type": "video", "mime_type": "video/mp4", "data": encoded}},
	}})
	require.NoError(t, err)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		assert.Equal(t, http.MethodGet, r.Method, "query replay must not create a job")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(append(append([]byte("data: "), event...), []byte("\n\ndata: {\"kind\":\"end\"}\n\n")...))
	}))
	defer server.Close()
	adaptor := queryTestAdaptor(t, queryStreamPlugin)
	task := &model.Task{TaskID: "task_fixture", Properties: model.Properties{OriginModelName: "query-model"}}
	resp, err := adaptor.FetchTask(server.URL, "", task, "")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	payload, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.EqualValues(t, len(payload), resp.ContentLength)
	assert.False(t, bytes.Contains(payload, []byte("data: ")), "raw SSE must never reach Task.Data persistence")
	var normalized map[string]any
	require.NoError(t, common.Unmarshal(payload, &normalized))
	assert.Equal(t, "completed", normalized["status"])
	parsed, err := adaptor.ParseTaskResult(task, resp, payload)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", parsed.Status)
	actual, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(parsed.Url, "data:video/mp4;base64,"))
	require.NoError(t, err)
	assert.Equal(t, video, actual)
	assert.Equal(t, 1, requests)
}

func TestQuerySSECapabilityAndHTTPClassification(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		status       int
		wantErr      bool
	}{
		{"undeclared capability", strings.Replace(queryStreamPlugin, `requiredCapabilities:["query-sse-delta@1"]`, `requiredCapabilities:[]`, 1), 200, true},
		{"auth error stays readable JSON", queryStreamPlugin, 403, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"error":{"message":"auth rejected"}}`)
			}))
			defer server.Close()
			adaptor := queryTestAdaptor(t, tc.source)
			resp, err := adaptor.FetchTask(server.URL, "", &model.Task{}, "")
			if tc.wantErr {
				require.ErrorContains(t, err, "require")
				assert.Zero(t, requests, "reject unsupported descriptors before network I/O")
				return
			}
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, tc.status, resp.StatusCode)
			assert.Contains(t, string(body), "auth rejected")
		})
	}
}

func TestQuerySSERejectsPartialMalformedOrOversizedFrames(t *testing.T) {
	adaptor := queryTestAdaptor(t, queryStreamPlugin)
	for _, tc := range []struct {
		name, stream, want string
		eventLimit         int
		wireLimit          int64
	}{
		{"premature EOF", "data: {\"kind\":\"part\",\"payload\":{\"status\":\"completed\"}}\n\n", "before", 256, 1024},
		{"unframed last event", "data: {\"kind\":\"end\"}\n", "before", 256, 1024},
		{"malformed JSON", "data: {bad}\n\n", "invalid query SSE JSON", 256, 1024},
		{"event too large", "data: {\"kind\":\"part\",\"payload\":\"" + strings.Repeat("x", 100) + "\"}\n\n", "token too long", 64, 1024},
		{"total wire budget", "data: {\"kind\":\"part\",\"payload\":\"" + strings.Repeat("x", 50) + "\"}\n\n" +
			"data: {\"kind\":\"end\"}\n\n", "size limit", 256, 90},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{"Content-Type": {"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(tc.stream))}
			defer resp.Body.Close()
			value, err := adaptor.readTaskEvents(context.Background(), resp, map[string]any{}, taskStreamOptions{
				hook: "parseQueryEventDelta", label: "query", eventLimit: tc.eventLimit, wireLimit: tc.wireLimit,
				accumulated: pluginruntime.NewQueryJSONState(), encodedResult: true, decodedEvent: true,
			})
			require.ErrorContains(t, err, tc.want)
			assert.Nil(t, value, "partial media cannot be published")
		})
	}
	legacy := pluginruntime.NewJSONState(pluginruntime.MaxQueryJSONBytes)
	err := legacy.Apply(context.Background(), []any{map[string]any{"op": "set", "path": []any{},
		"value": strings.Repeat("x", (1<<20)+1)}})
	require.Error(t, err, "query capability must not relax legacy JSON tools/submission limits")
}

func TestQuerySSEControlBudgetAndCancellation(t *testing.T) {
	source := strings.Replace(queryStreamPlugin, "const body=event.body;",
		`const body=event.body; return {changes:[],state:{oversized:"x".repeat(65537)},done:true};`, 1)
	adaptor := queryTestAdaptor(t, source)
	resp := &http.Response{Header: http.Header{"Content-Type": {"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader("data: {\"kind\":\"end\"}\n\n"))}
	defer resp.Body.Close()
	value, err := adaptor.readQueryEvents(resp, map[string]any{})
	require.ErrorContains(t, err, "control state")
	assert.Nil(t, value)

	adaptor = queryTestAdaptor(t, queryStreamPlugin)
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp = &http.Response{Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader}
	valueAny, err := adaptor.readTaskEvents(ctx, resp, map[string]any{}, taskStreamOptions{
		hook: "parseQueryEventDelta", label: "query", eventLimit: 1024,
		accumulated: pluginruntime.NewQueryJSONState(), encodedResult: true, decodedEvent: true,
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, valueAny, "cancellation must not return partial media")
}

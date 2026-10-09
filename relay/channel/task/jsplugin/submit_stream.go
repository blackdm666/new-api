package jsplugin

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay/helper"
)

// readSubmitEvents owns only SSE framing and bounded JSON state. The plugin
// interprets data, accumulates provider output and decides when it is complete.
// No response bytes are written to the client at this boundary.
func (a *TaskAdaptor) readSubmitEvents(parent context.Context, resp *http.Response, driverContext map[string]any) (any, error) {
	options := taskStreamOptions{
		hook: "parseSubmitEvent", eventLimit: maxTaskPluginPersistedJSONBytes, label: "submit",
	}
	if slices.Contains(a.plugin.Meta.RequiredCapabilities, pluginruntime.CapabilitySubmitSSEDelta) {
		options.hook = "parseSubmitEventDelta"
		options.accumulated = pluginruntime.NewJSONState(maxTaskPluginPersistedJSONBytes)
	}
	return a.readTaskEvents(parent, resp, driverContext, options)
}

type taskStreamOptions struct {
	hook, label   string
	eventLimit    int
	wireLimit     int64
	accumulated   *pluginruntime.JSONState
	encodedResult bool
	decodedEvent  bool
}

func (a *TaskAdaptor) readQueryEvents(resp *http.Response, taskContext map[string]any) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	value, err := a.readTaskEvents(ctx, resp, taskContext, taskStreamOptions{
		hook: "parseQueryEventDelta", label: "query", eventLimit: pluginruntime.MaxQueryJSONBytes,
		wireLimit: int64(pluginruntime.MaxQueryJSONBytes), accumulated: pluginruntime.NewQueryJSONState(),
		encodedResult: true,
		decodedEvent:  true,
	})
	if err != nil {
		return nil, err
	}
	encoded, ok := value.([]byte)
	if !ok {
		return nil, fmt.Errorf("invalid normalized query response")
	}
	return encoded, nil
}

// readTaskEvents frames SSE without publishing partial results. Only small
// control state goes back into JS; large accumulated media stays host-owned.
func (a *TaskAdaptor) readTaskEvents(parent context.Context, resp *http.Response, hookContext map[string]any, options taskStreamOptions) (any, error) {
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "text/event-stream" {
		return nil, fmt.Errorf("expected a text/event-stream %s response", options.label)
	}
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	stopClose := context.AfterFunc(ctx, func() { _ = resp.Body.Close() })
	defer stopClose()
	idleTimeout := time.Duration(constant.StreamingTimeout) * time.Second
	if idleTimeout <= 0 {
		idleTimeout = 30 * time.Second
	}
	timer := time.AfterFunc(idleTimeout, func() { cancel(fmt.Errorf("task %s stream idle timeout", options.label)) })
	defer timer.Stop()

	var reader io.Reader = resp.Body
	if options.wireLimit > 0 {
		reader = io.LimitReader(reader, options.wireLimit+1)
	}
	scanner := helper.NewStreamScanner(reader, options.eventLimit+1)
	hook, accumulated := options.hook, options.accumulated
	var state any
	var data []string
	var eventName, lastID string
	frameBytes := 0
	wireBytes := int64(0)
	firstLine := true
	for scanner.Scan() {
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		timer.Reset(idleTimeout)
		line := scanner.Text()
		if firstLine {
			line = strings.TrimPrefix(line, "\ufeff")
			firstLine = false
		}
		frameBytes += len(line) + 1
		wireBytes += int64(len(line) + 1)
		if options.wireLimit > 0 && wireBytes > options.wireLimit {
			return nil, fmt.Errorf("task %s SSE response exceeds size limit", options.label)
		}
		if frameBytes > options.eventLimit {
			return nil, fmt.Errorf("task %s SSE event exceeds size limit", options.label)
		}
		if line != "" {
			field, value, _ := strings.Cut(line, ":")
			value, _ = strings.CutPrefix(value, " ")
			switch field {
			case "data":
				data = append(data, value)
			case "event":
				eventName = value
			case "id":
				if !strings.ContainsRune(value, '\x00') {
					lastID = value
				}
			}
			continue
		}
		frameBytes = 0
		if len(data) == 0 {
			eventName = ""
			continue
		}
		if eventName == "" {
			eventName = "message"
		}
		event := map[string]any{"event": eventName, "id": lastID}
		dataText := strings.Join(data, "\n")
		if options.decodedEvent {
			// Decode huge media events once in the native codec. Passing raw
			// JSON into JS would require parsing the same 100MB string again.
			var body any
			if dataText == "[DONE]" {
				body = dataText
			} else if err := common.Unmarshal([]byte(dataText), &body); err != nil {
				return nil, fmt.Errorf("invalid query SSE JSON event")
			}
			event["body"] = body
		} else {
			event["data"] = dataText
		}
		value, err := a.plugin.Engine.Call(ctx, hook, hookContext, event, state)
		if err != nil {
			return nil, err
		}
		result, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must return an object", hook)
		}
		nextState, hasState := result["state"]
		done, hasDone := result["done"].(bool)
		if !hasState || !hasDone {
			return nil, fmt.Errorf("%s must return state and a boolean done", hook)
		}
		encoded, err := common.Marshal(nextState)
		if err != nil {
			return nil, fmt.Errorf("invalid %s stream state: %w", options.label, err)
		}
		if len(encoded) > maxTaskPluginPersistedJSONBytes {
			return nil, fmt.Errorf("task %s stream state exceeds size limit", options.label)
		}
		if accumulated != nil {
			if len(result) != 3 {
				return nil, fmt.Errorf("%s must return only changes, state and done", hook)
			}
			// Control state is separate from the full result and is the only
			// snapshot passed back to JS on the next event.
			if len(encoded) > 64<<10 {
				return nil, fmt.Errorf("task %s stream control state exceeds size limit", options.label)
			}
			if err = accumulated.Apply(ctx, result["changes"]); err != nil {
				return nil, fmt.Errorf("invalid %s stream changes: %w", options.label, err)
			}
		}
		// Keep the exact encoded-byte limit above. Plain JSON state can be
		// isolated without parsing large strings again; codec-specific values
		// (for example exported typed arrays) retain the old normalization.
		var plainJSON bool
		state, plainJSON = cloneJSONValue(nextState, 0)
		if !plainJSON {
			if err = common.Unmarshal(encoded, &state); err != nil {
				return nil, err
			}
		}
		if done {
			if accumulated != nil {
				if options.encodedResult {
					return accumulated.EncodedValue()
				}
				return accumulated.Value()
			}
			return state, nil
		}
		data = nil
		eventName = ""
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read task %s stream: %w", options.label, err)
	}
	return nil, fmt.Errorf("task %s stream ended before the plugin reported completion", options.label)
}

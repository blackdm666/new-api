package types

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToClaudeErrorType(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  OpenAIError
		want string
	}{
		{
			name: "Anthropic upstream error without a code keeps its type",
			err:  OpenAIError{Message: "tool_choice: type \"tool\" and \"any\" are not supported for this model.", Type: "invalid_request_error"},
			want: "invalid_request_error",
		},
		{
			name: "an OpenAI code still wins over the type",
			err:  OpenAIError{Message: "model not found", Type: "invalid_request_error", Code: "model_not_found"},
			want: "model_not_found",
		},
		{
			name: "an empty code falls back to the type",
			err:  OpenAIError{Message: "bad request", Type: "invalid_request_error", Code: ""},
			want: "invalid_request_error",
		},
		{
			name: "no code and no type uses the upstream_error default",
			err:  OpenAIError{Message: "bad request"},
			want: "upstream_error",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := WithOpenAIError(tc.err, http.StatusBadRequest).ToClaudeError()
			assert.Equal(t, tc.want, got.Type)
			assert.Equal(t, tc.err.Message, got.Message)
		})
	}
}

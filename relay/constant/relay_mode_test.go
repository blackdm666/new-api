package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPath2RelayMode(t *testing.T) {
	tests := []struct {
		path string
		want int
	}{
		{path: "/v1/alpha/search", want: RelayModeAlphaSearch},
		{path: "/v1/alpha/search?foo=1", want: RelayModeAlphaSearch},
		{path: "/pg/chat/completions", want: RelayModeChatCompletions},
		{path: "/pg/chat/completions?foo=1", want: RelayModeChatCompletions},
		{path: "/pg/images/generations", want: RelayModeImagesGenerations},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, Path2RelayMode(tt.path))
		})
	}
}

func TestCanonicalRelayRequestPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "/pg", want: "/v1"},
		{path: "/pg/", want: "/v1/"},
		{path: "/pg/chat/completions", want: "/v1/chat/completions"},
		{path: "/pg/chat/completions?foo=1", want: "/v1/chat/completions?foo=1"},
		{path: "/pg/images/generations", want: "/v1/images/generations"},
		{path: "/pg/videos/task_123", want: "/v1/videos/task_123"},
		{path: "/pgx/chat/completions", want: "/pgx/chat/completions"},
		{path: "/v1/chat/completions", want: "/v1/chat/completions"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, CanonicalRelayRequestPath(tt.path))
		})
	}
}

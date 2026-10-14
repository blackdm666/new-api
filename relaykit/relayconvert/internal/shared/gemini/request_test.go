package gemini

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Chat, Responses and Claude conversions reject inline data whose MIME type
// fails this check with a local 500, so GIF and other Blob.mimeType entries
// from the Gemini API reference must pass.
func TestIsSupportedMimeTypeFollowsGeminiBlobList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mimeType string
		want     bool
	}{
		{mimeType: "image/gif", want: true},
		{mimeType: "IMAGE/GIF", want: true},
		{mimeType: "image/avif", want: true},
		{mimeType: "image/png", want: true},
		{mimeType: "application/pdf", want: true},
		{mimeType: "text/markdown", want: true},
		{mimeType: "audio/ogg", want: true},
		{mimeType: "video/webm", want: true},
		{mimeType: "application/zip", want: false},
		{mimeType: "image/svg+xml", want: false},
		{mimeType: "", want: false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, IsSupportedMimeType(tt.mimeType), tt.mimeType)
	}
}

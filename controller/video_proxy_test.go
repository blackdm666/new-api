package controller

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCopyVideoResponseHeadersPreservesMediaHeadersWithoutOverwritingCORS(t *testing.T) {
	dst := http.Header{}
	dst.Set("Access-Control-Allow-Origin", "*")
	src := http.Header{}
	src.Set("Access-Control-Allow-Origin", "*")
	src.Set("Content-Type", "video/mp4")
	src.Set("Content-Length", "8707469")
	src.Set("ETag", `"video-etag"`)
	src.Set("Set-Cookie", "upstream=secret")

	copyTaskMediaResponseHeaders(dst, src)

	assert.Equal(t, []string{"*"}, dst.Values("Access-Control-Allow-Origin"))
	assert.Equal(t, "video/mp4", dst.Get("Content-Type"))
	assert.Equal(t, "8707469", dst.Get("Content-Length"))
	assert.Equal(t, `"video-etag"`, dst.Get("ETag"))
	assert.Empty(t, dst.Values("Set-Cookie"))
}

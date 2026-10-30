package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// relayconvert wraps the gin context in its diagnostics collector before the
// media resolver runs; disk cached media must still be removed when the
// request ends.
func TestConvertRequestDiskCachedMediaIsCleanedUp(t *testing.T) {
	previous := common.GetDiskCacheConfig()
	t.Cleanup(func() { common.SetDiskCacheConfig(previous) })
	common.SetDiskCacheConfig(common.DiskCacheConfig{
		Enabled:     true,
		ThresholdMB: 1,
		MaxSizeMB:   64,
		Path:        t.TempDir(),
	})

	image := make([]byte, 1<<20)
	_, err := rand.Read(image)
	require.NoError(t, err)
	body, err := json.Marshal(map[string]any{
		"model":      "gemini-3-flash",
		"max_tokens": 16,
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "image", "source": map[string]any{
					"type":       "base64",
					"media_type": "image/png",
					"data":       base64.StdEncoding.EncodeToString(image),
				}},
				map[string]any{"type": "text", "text": "describe"},
			},
		}},
	})
	require.NoError(t, err)
	var request dto.ClaudeRequest
	require.NoError(t, json.Unmarshal(body, &request))

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &convmeta.Values{
		ConversionChain:     []types.RelayFormat{types.RelayFormatClaude},
		UpstreamModelName:   "gemini-3-flash",
		ChannelMetaAttached: true,
	}
	_, err = relayconvert.ConvertRequest(c, info, types.RelayFormatGemini, &request)
	require.NoError(t, err)

	cached, err := filepath.Glob(filepath.Join(common.GetDiskCacheDir(), "file-*.tmp"))
	require.NoError(t, err)
	require.Len(t, cached, 1, "the image should be cached on disk")

	CleanupFileSources(c)

	_, err = os.Stat(cached[0])
	require.True(t, os.IsNotExist(err), "the disk cached image must be removed with the request")
}

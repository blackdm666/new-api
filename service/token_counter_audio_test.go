package service

import (
	"bytes"
	"encoding/binary"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pcmWAV(seconds int) []byte {
	const sampleRate = 16000
	data := make([]byte, sampleRate*2*seconds)
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+len(data)))
	buf.WriteString("WAVEfmt ")
	for _, field := range []any{uint32(16), uint16(1), uint16(1), uint32(sampleRate), uint32(sampleRate * 2), uint16(2), uint16(16)} {
		_ = binary.Write(&buf, binary.LittleEndian, field)
	}
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(data)))
	buf.Write(data)
	return buf.Bytes()
}

func TestCountRequestTokenTranscriptionDuration(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  []byte
		want     int
	}{
		// 可解析的文件按真实时长：3 秒 → 3/60*1000
		{name: "wav", filename: "a.wav", content: pcmWAV(3), want: 50},
		// webm 本地解析不了：120000 字节 / 2000 B/s = 60 秒 → 1000
		{name: "webm", filename: "a.webm", content: bytes.Repeat([]byte{0x1a}, 120000), want: 1000},
		{name: "no_extension", filename: "recording", content: bytes.Repeat([]byte{1}, 30000), want: 250},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			require.NoError(t, writer.WriteField("model", "grok-stt"))
			part, err := writer.CreateFormFile("file", tt.filename)
			require.NoError(t, err)
			_, err = part.Write(tt.content)
			require.NoError(t, err)
			require.NoError(t, writer.Close())

			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", &body)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			t.Cleanup(func() { common.CleanupBodyStorage(c) })

			info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioTranscription, RelayFormat: types.RelayFormatOpenAIAudio}
			tokens, err := CountRequestToken(c, &types.TokenCountMeta{TokenType: types.TokenTypeTextNumber}, info)
			require.NoError(t, err)
			assert.Equal(t, tt.want, tokens)
		})
	}
}

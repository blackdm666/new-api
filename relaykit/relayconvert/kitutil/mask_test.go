package kitutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 88API masks every address in relayed errors, not only the channel's
// upstream: supplier errors name their other domains (top-up pages, support
// mail), which would reveal the upstream. The upstream URLs callers pass must
// never narrow the masking.
func TestMaskSensitiveInfoMasksEveryAddress(t *testing.T) {
	tests := []struct {
		name         string
		upstreamURLs []string
		input        string
		want         string
	}{
		{
			name:         "upstream URL and its IP without an upstream address",
			upstreamURLs: nil,
			input:        `Post "https://api.upstream.example.com/v1/chat/completions": dial tcp 10.0.0.1:443: connect: connection refused`,
			want:         `Post "https://***.com/***/***/***": dial tcp ***.***.***.***:443: connect: connection refused`,
		},
		{
			name:         "supplier domains other than the channel base URL",
			upstreamURLs: []string{"https://api.upstream.example.com"},
			input:        `余额不足，请前往 https://shop.supplier-relay.com/topup?uid=42 充值，或联系 support@supplier-relay.com`,
			want:         `余额不足，请前往 https://***.com/***?uid=*** 充值，或联系 support@***.com`,
		},
		{
			name:         "bare host in a DNS error",
			upstreamURLs: nil,
			input:        `dial tcp: lookup tenant.api.example.com: no such host`,
			want:         `dial tcp: lookup ***.***.***.com: no such host`,
		},
		{
			name:         "bracketed IPv6 addresses",
			upstreamURLs: nil,
			input:        `dial tcp [2a01:4f8::1]:443: i/o timeout; lookup on [::1]:53; dial tcp [::ffff:10.0.0.1]:443; items[0] and [1:2] stay`,
			want:         `dial tcp [***]:443: i/o timeout; lookup on [***]:53; dial tcp [***]:443; items[0] and [1:2] stay`,
		},
		{
			name:         "API key values",
			upstreamURLs: nil,
			input:        `invalid api_key:AIzaSyAAAaUooTUni8AdaOkSRMda30n_Q4vrV70`,
			want:         `invalid api_key:***`,
		},
		{
			name:         "userinfo and query values",
			upstreamURLs: nil,
			input:        `downloading https://alice:secret@cdn.example/file.png and https://bob@cdn.example/a?sig=1`,
			want:         `downloading https://***.example/*** and https://***.example/***?sig=***`,
		},
		{
			name:         "parentheses inside a URL and the trailing parenthesis outside it",
			upstreamURLs: nil,
			input:        `failed to download https://cdn.example/image(1).png?token=secret (status 403)`,
			want:         `failed to download https://***.example/***?token=*** (status 403)`,
		},
		{
			name:         "quoted URL keeps its delimiters and trailing punctuation",
			upstreamURLs: []string{"https://api.upstream.example.com"},
			input:        `Post "https://api.upstream.example.com/v1/chat/completions": EOF; retry https://api.upstream.example.com/v1/models.`,
			want:         `Post "https://***.com/***/***/***": EOF; retry https://***.com/***/***.`,
		},
		{
			name:         "IP URL host and IPv6 WebSocket URL keep their ports",
			upstreamURLs: nil,
			input:        `request to http://10.0.0.1:8080/v1/chat failed; dial failed to ws://[2001:db8::1]:8080/v1/realtime`,
			want:         `request to http://***.***.***.***:8080/***/*** failed; dial failed to ws://***:8080/***/***`,
		},
		{
			name:         "dotted identifiers that are not domains stay readable",
			upstreamURLs: nil,
			input:        `Unknown event response.output_text.delta from model gpt-4.1`,
			want:         `Unknown event response.output_text.delta from model gpt-4.1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MaskSensitiveInfo(tt.input, tt.upstreamURLs...))
		})
	}
}

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTurnstileConfigByProvider(t *testing.T) {
	tests := []struct {
		name      string
		config    TurnstileConfig
		wantError string
	}{
		{
			name: "Cloudflare complete",
			config: TurnstileConfig{
				Enabled: true, Provider: TurnstileProviderCloudflare,
				SiteKey: "site", SecretKey: "secret",
			},
		},
		{
			name: "Cloudflare requires site key",
			config: TurnstileConfig{
				Enabled: true, Provider: TurnstileProviderCloudflare, SecretKey: "secret",
			},
			wantError: "site key",
		},
		{
			name: "Cloudflare requires secret key",
			config: TurnstileConfig{
				Enabled: true, Provider: TurnstileProviderCloudflare, SiteKey: "site",
			},
			wantError: "secret key",
		},
		{
			name: "custom complete",
			config: TurnstileConfig{
				Enabled: true, Provider: TurnstileProviderCustom, SecretKey: "secret",
				WidgetScriptURL: "https://captcha.example/widget.js",
				WidgetEndpoint:  "https://captcha.example",
				VerifyURL:       "https://captcha.example/siteverify",
			},
		},
		{
			name: "custom allows an empty secret",
			config: TurnstileConfig{
				Enabled: true, Provider: TurnstileProviderCustom,
				WidgetScriptURL: "https://captcha.example/widget.js",
				WidgetEndpoint:  "https://captcha.example",
				VerifyURL:       "https://captcha.example/siteverify",
			},
		},
		{
			name: "custom rejects script credentials",
			config: TurnstileConfig{
				Provider:        TurnstileProviderCustom,
				WidgetScriptURL: "https://user:pass@captcha.example/widget.js",
			},
			wantError: "without credentials",
		},
		{
			name: "disabled allows incomplete custom setup",
			config: TurnstileConfig{
				Enabled: false, Provider: TurnstileProviderCustom,
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateTurnstileConfig(testCase.config)
			if testCase.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.wantError)
		})
	}
}

func TestTurnstileConfigSnapshotIsAtomic(t *testing.T) {
	previous := CurrentTurnstileConfig()
	t.Cleanup(func() { ApplyTurnstileConfig(previous) })
	first := TurnstileConfig{
		Enabled: true, Provider: TurnstileProviderCloudflare,
		SiteKey: "cloudflare-site", SecretKey: "cloudflare-secret", Action: "first",
	}
	second := TurnstileConfig{
		Enabled: true, Provider: TurnstileProviderCustom,
		SecretKey: "custom-secret", WidgetScriptURL: "https://captcha.example/widget.js",
		WidgetEndpoint: "https://captcha.example", VerifyURL: "https://captcha.example/siteverify",
		Action: "second",
	}
	ApplyTurnstileConfig(first)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for index := 0; index < 10_000; index++ {
			if index%2 == 0 {
				ApplyTurnstileConfig(second)
			} else {
				ApplyTurnstileConfig(first)
			}
		}
	}()

	for {
		config := CurrentTurnstileConfig()
		if config != first && config != second {
			t.Fatalf("observed partial Turnstile configuration: %+v", config)
		}
		select {
		case <-done:
			return
		default:
		}
	}
}

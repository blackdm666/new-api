package common

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
)

const (
	TurnstileProviderCloudflare = "cloudflare"
	TurnstileProviderCustom     = "custom"
)

var TurnstileProvider = TurnstileProviderCloudflare
var TurnstileWidgetScriptURL = ""
var TurnstileWidgetEndpoint = ""
var TurnstileVerifyURL = ""
var TurnstileAction = "register"
var turnstileConfigMutex sync.RWMutex

type TurnstileConfig struct {
	Enabled         bool
	Provider        string
	SiteKey         string
	SecretKey       string
	WidgetScriptURL string
	WidgetEndpoint  string
	VerifyURL       string
	Action          string
}

func NormalizeTurnstileProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == TurnstileProviderCustom {
		return TurnstileProviderCustom
	}
	return TurnstileProviderCloudflare
}

func ValidateTurnstileHTTPURL(name, value string, required bool) error {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return fmt.Errorf("%s is required", name)
		}
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute HTTP(S) URL without credentials", name)
	}
	return nil
}

func ValidateTurnstileConfig(config TurnstileConfig) error {
	provider := strings.ToLower(strings.TrimSpace(config.Provider))
	if provider != TurnstileProviderCloudflare && provider != TurnstileProviderCustom {
		return fmt.Errorf("unsupported human verification provider %q", config.Provider)
	}

	if provider == TurnstileProviderCustom {
		if err := ValidateTurnstileHTTPURL("widget script URL", config.WidgetScriptURL, config.Enabled); err != nil {
			return err
		}
		if err := ValidateTurnstileHTTPURL("widget endpoint", config.WidgetEndpoint, config.Enabled); err != nil {
			return err
		}
		if err := ValidateTurnstileHTTPURL("verification URL", config.VerifyURL, config.Enabled); err != nil {
			return err
		}
	}

	if !config.Enabled {
		return nil
	}
	if provider == TurnstileProviderCloudflare {
		if strings.TrimSpace(config.SiteKey) == "" {
			return fmt.Errorf("Cloudflare site key is required")
		}
		if strings.TrimSpace(config.SecretKey) == "" {
			return fmt.Errorf("Cloudflare secret key is required")
		}
	}
	return nil
}

func CurrentTurnstileConfig() TurnstileConfig {
	turnstileConfigMutex.RLock()
	defer turnstileConfigMutex.RUnlock()
	return TurnstileConfig{
		Enabled:         TurnstileCheckEnabled,
		Provider:        NormalizeTurnstileProvider(TurnstileProvider),
		SiteKey:         strings.TrimSpace(TurnstileSiteKey),
		SecretKey:       strings.TrimSpace(TurnstileSecretKey),
		WidgetScriptURL: strings.TrimSpace(TurnstileWidgetScriptURL),
		WidgetEndpoint:  strings.TrimSpace(TurnstileWidgetEndpoint),
		VerifyURL:       strings.TrimSpace(TurnstileVerifyURL),
		Action:          strings.TrimSpace(TurnstileAction),
	}
}

func ApplyTurnstileConfig(config TurnstileConfig) {
	turnstileConfigMutex.Lock()
	defer turnstileConfigMutex.Unlock()
	TurnstileCheckEnabled = config.Enabled
	TurnstileProvider = NormalizeTurnstileProvider(config.Provider)
	TurnstileSiteKey = strings.TrimSpace(config.SiteKey)
	TurnstileSecretKey = strings.TrimSpace(config.SecretKey)
	TurnstileWidgetScriptURL = strings.TrimSpace(config.WidgetScriptURL)
	TurnstileWidgetEndpoint = strings.TrimRight(strings.TrimSpace(config.WidgetEndpoint), "/")
	TurnstileVerifyURL = strings.TrimSpace(config.VerifyURL)
	TurnstileAction = strings.TrimSpace(config.Action)
}

func UpdateTurnstileConfigValue(key, value string) bool {
	turnstileConfigMutex.Lock()
	defer turnstileConfigMutex.Unlock()
	switch key {
	case "TurnstileCheckEnabled":
		TurnstileCheckEnabled = value == "true"
	case "TurnstileProvider":
		TurnstileProvider = NormalizeTurnstileProvider(value)
	case "TurnstileSiteKey":
		TurnstileSiteKey = strings.TrimSpace(value)
	case "TurnstileSecretKey":
		TurnstileSecretKey = strings.TrimSpace(value)
	case "TurnstileWidgetScriptURL":
		TurnstileWidgetScriptURL = strings.TrimSpace(value)
	case "TurnstileWidgetEndpoint":
		TurnstileWidgetEndpoint = strings.TrimRight(strings.TrimSpace(value), "/")
	case "TurnstileVerifyURL":
		TurnstileVerifyURL = strings.TrimSpace(value)
	case "TurnstileAction":
		TurnstileAction = strings.TrimSpace(value)
	default:
		return false
	}
	return true
}

package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type botProtectionUpdateRequest struct {
	Enabled         bool   `json:"enabled"`
	Provider        string `json:"provider"`
	SiteKey         string `json:"site_key"`
	SecretKey       string `json:"secret_key"`
	WidgetScriptURL string `json:"widget_script_url"`
	WidgetEndpoint  string `json:"widget_endpoint"`
	VerifyURL       string `json:"verify_url"`
	Action          string `json:"action"`
	ClearSecret     bool   `json:"clear_secret"`
}

func resolveBotProtectionUpdate(request botProtectionUpdateRequest) (common.TurnstileConfig, map[string]string, error) {
	provider := strings.ToLower(strings.TrimSpace(request.Provider))
	secretKey := strings.TrimSpace(request.SecretKey)
	if request.ClearSecret && secretKey != "" {
		return common.TurnstileConfig{}, nil, fmt.Errorf("verification secret cannot be entered and cleared at the same time")
	}
	if secretKey == "" && !request.ClearSecret {
		secretKey = common.CurrentTurnstileConfig().SecretKey
	}
	action := strings.TrimSpace(request.Action)
	if action == "" {
		action = "register"
	}
	config := common.TurnstileConfig{
		Enabled:         request.Enabled,
		Provider:        provider,
		SiteKey:         strings.TrimSpace(request.SiteKey),
		SecretKey:       secretKey,
		WidgetScriptURL: strings.TrimSpace(request.WidgetScriptURL),
		WidgetEndpoint:  strings.TrimRight(strings.TrimSpace(request.WidgetEndpoint), "/"),
		VerifyURL:       strings.TrimSpace(request.VerifyURL),
		Action:          action,
	}
	if err := common.ValidateTurnstileConfig(config); err != nil {
		return common.TurnstileConfig{}, nil, err
	}

	values := map[string]string{
		"TurnstileCheckEnabled":    strconv.FormatBool(config.Enabled),
		"TurnstileProvider":        config.Provider,
		"TurnstileSiteKey":         config.SiteKey,
		"TurnstileWidgetScriptURL": config.WidgetScriptURL,
		"TurnstileWidgetEndpoint":  config.WidgetEndpoint,
		"TurnstileVerifyURL":       config.VerifyURL,
		"TurnstileAction":          config.Action,
	}
	if request.ClearSecret || strings.TrimSpace(request.SecretKey) != "" {
		values["TurnstileSecretKey"] = secretKey
	}
	return config, values, nil
}

func UpdateBotProtectionSettings(c *gin.Context) {
	var request botProtectionUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	config, values, err := resolveBotProtectionUpdate(request)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.UpdateOptionsBulk(values); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "bot_protection.settings.update", map[string]interface{}{
		"enabled":  config.Enabled,
		"provider": config.Provider,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

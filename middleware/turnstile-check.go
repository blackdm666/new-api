package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

type turnstileCheckResponse struct {
	Success bool `json:"success"`
}

type turnstileRequest struct {
	Turnstile string `json:"turnstile"`
}

const defaultTurnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

func turnstileVerifyURL(config common.TurnstileConfig) (string, error) {
	if err := common.ValidateTurnstileConfig(config); err != nil {
		return "", err
	}
	if config.Provider == common.TurnstileProviderCustom {
		return config.VerifyURL, nil
	}
	return defaultTurnstileVerifyURL, nil
}

// OAuthStateTurnstileCheck requires Turnstile for anonymous login flows while
// leaving authenticated account-binding flows to their existing session check.
func OAuthStateTurnstileCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err == nil {
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			var request struct {
				Intent string `json:"intent"`
			}
			if common.Unmarshal(body, &request) == nil && strings.TrimSpace(request.Intent) == "bind" {
				c.Next()
				return
			}
		}
		TurnstileCheck()(c)
	}
}

func TurnstileCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !verifyTurnstile(c, c.Query("turnstile")) {
			return
		}
		c.Next()
	}
}

// TurnstileCheckFromBody keeps one-use verification tokens out of request
// URLs. It restores the JSON body so the controller can decode it normally.
func TurnstileCheckFromBody() gin.HandlerFunc {
	return func(c *gin.Context) {
		var request turnstileRequest
		if common.CurrentTurnstileConfig().Enabled {
			body, err := io.ReadAll(c.Request.Body)
			if err != nil {
				common.ApiError(c, err)
				c.Abort()
				return
			}
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			if common.Unmarshal(body, &request) != nil {
				rejectEmptyTurnstile(c)
				return
			}
		}
		if !verifyTurnstile(c, request.Turnstile) {
			return
		}
		c.Next()
	}
}

func rejectEmptyTurnstile(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "Turnstile token 为空",
	})
	c.Abort()
}

func verifyTurnstile(c *gin.Context, response string) bool {
	config := common.CurrentTurnstileConfig()
	if !config.Enabled {
		return true
	}
	if strings.TrimSpace(response) == "" {
		rejectEmptyTurnstile(c)
		return false
	}
	verifyURL, err := turnstileVerifyURL(config)
	if err != nil {
		common.SysLog("human verification configuration error: " + err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "人机验证配置无效，请联系管理员！",
		})
		c.Abort()
		return false
	}
	client := &http.Client{Timeout: 15 * time.Second}
	rawRes, err := client.PostForm(verifyURL, url.Values{
		"secret":   {config.SecretKey},
		"response": {response},
		"remoteip": {c.ClientIP()},
	})
	if err != nil {
		common.SysLog(err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		c.Abort()
		return false
	}
	defer rawRes.Body.Close()
	var res turnstileCheckResponse
	if err = common.DecodeJson(rawRes.Body, &res); err != nil {
		common.SysLog(err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		c.Abort()
		return false
	}
	if !res.Success {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Turnstile 校验失败，请刷新重试！",
		})
		c.Abort()
		return false
	}
	return true
}

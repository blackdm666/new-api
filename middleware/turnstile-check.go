package middleware

import (
	"net/http"
	"net/url"
	"os"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

type turnstileCheckResponse struct {
	Success bool `json:"success"`
}

// turnstileVerifyURL 返回 siteverify 校验地址。默认使用 Cloudflare 官方端点，
// 若设置环境变量 TURNSTILE_VERIFY_URL 则改用自定义端点(例如自建的
// https://verify.88api.ai/turnstile/v0/siteverify，接口保持 Cloudflare 兼容)。
// 不设置该变量时行为与上游完全一致，可随时回退。
func turnstileVerifyURL() string {
	if u := os.Getenv("TURNSTILE_VERIFY_URL"); u != "" {
		return u
	}
	return "https://challenges.cloudflare.com/turnstile/v0/siteverify"
}

func TurnstileCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		if common.TurnstileCheckEnabled {
			response := c.Query("turnstile")
			if response == "" {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile token 为空",
				})
				c.Abort()
				return
			}
			rawRes, err := http.PostForm(turnstileVerifyURL(), url.Values{
				"secret":   {common.TurnstileSecretKey},
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
				return
			}
			defer rawRes.Body.Close()
			var res turnstileCheckResponse
			err = common.DecodeJson(rawRes.Body, &res)
			if err != nil {
				common.SysLog(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				c.Abort()
				return
			}
			if !res.Success {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile 校验失败，请刷新重试！",
				})
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

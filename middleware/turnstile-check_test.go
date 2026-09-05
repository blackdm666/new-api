/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withTurnstileConfig(t *testing.T, config common.TurnstileConfig) {
	t.Helper()
	previous := common.CurrentTurnstileConfig()
	common.TurnstileCheckEnabled = config.Enabled
	common.TurnstileProvider = config.Provider
	common.TurnstileSiteKey = config.SiteKey
	common.TurnstileSecretKey = config.SecretKey
	common.TurnstileWidgetScriptURL = config.WidgetScriptURL
	common.TurnstileWidgetEndpoint = config.WidgetEndpoint
	common.TurnstileVerifyURL = config.VerifyURL
	common.TurnstileAction = config.Action
	t.Cleanup(func() {
		common.TurnstileCheckEnabled = previous.Enabled
		common.TurnstileProvider = previous.Provider
		common.TurnstileSiteKey = previous.SiteKey
		common.TurnstileSecretKey = previous.SecretKey
		common.TurnstileWidgetScriptURL = previous.WidgetScriptURL
		common.TurnstileWidgetEndpoint = previous.WidgetEndpoint
		common.TurnstileVerifyURL = previous.VerifyURL
		common.TurnstileAction = previous.Action
	})
}

func TestOAuthStateTurnstileCheckAllowsBindAndRestoresBody(t *testing.T) {
	oldEnabled := common.TurnstileCheckEnabled
	common.TurnstileCheckEnabled = true
	t.Cleanup(func() { common.TurnstileCheckEnabled = oldEnabled })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/oauth/state", OAuthStateTurnstileCheck(), func(c *gin.Context) {
		var request struct {
			Intent string `json:"intent"`
		}
		require.NoError(t, common.DecodeJson(c.Request.Body, &request))
		assert.Equal(t, "bind", request.Intent)
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/oauth/state", bytes.NewBufferString(`{"intent":"bind"}`))
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestOAuthStateTurnstileCheckRejectsLoginWithoutToken(t *testing.T) {
	oldEnabled := common.TurnstileCheckEnabled
	common.TurnstileCheckEnabled = true
	t.Cleanup(func() { common.TurnstileCheckEnabled = oldEnabled })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	nextCalled := false
	router.POST("/oauth/state", OAuthStateTurnstileCheck(), func(c *gin.Context) {
		nextCalled = true
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/oauth/state", bytes.NewBufferString(`{"intent":"login"}`))
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.False(t, nextCalled)
	assert.Contains(t, recorder.Body.String(), "Turnstile token")
}

func TestTurnstileVerifyURL(t *testing.T) {
	t.Run("uses the self-hosted compatible endpoint", func(t *testing.T) {
		withTurnstileConfig(t, common.TurnstileConfig{
			Enabled: true, Provider: common.TurnstileProviderCustom,
			SecretKey: "secret", WidgetScriptURL: "https://captcha.example/widget.js",
			WidgetEndpoint: "https://captcha.example", VerifyURL: "https://captcha.example/siteverify",
		})
		verifyURL, err := turnstileVerifyURL(common.CurrentTurnstileConfig())
		require.NoError(t, err)
		assert.Equal(t, "https://captcha.example/siteverify", verifyURL)
	})

	t.Run("uses Cloudflare for the Cloudflare provider", func(t *testing.T) {
		withTurnstileConfig(t, common.TurnstileConfig{
			Enabled: true, Provider: common.TurnstileProviderCloudflare,
			SiteKey: "site", SecretKey: "secret",
		})
		verifyURL, err := turnstileVerifyURL(common.CurrentTurnstileConfig())
		require.NoError(t, err)
		assert.Equal(t, defaultTurnstileVerifyURL, verifyURL)
	})
}

func TestTurnstileCheckFromBodyVerifiesTokenAndRestoresBody(t *testing.T) {
	var verifiedToken string
	verifyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		verifiedToken = r.Form.Get("response")
		_, _ = io.WriteString(w, `{"success":true}`)
	}))
	t.Cleanup(verifyServer.Close)
	withTurnstileConfig(t, common.TurnstileConfig{
		Enabled: true, Provider: common.TurnstileProviderCustom,
		SecretKey: "test-secret", WidgetScriptURL: "https://captcha.example/widget.js",
		WidgetEndpoint: "https://captcha.example", VerifyURL: verifyServer.URL,
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	nextCalled := false
	router.POST("/verification", TurnstileCheckFromBody(), func(c *gin.Context) {
		nextCalled = true
		var request struct {
			Email     string `json:"email"`
			Turnstile string `json:"turnstile"`
		}
		require.NoError(t, common.DecodeJson(c.Request.Body, &request))
		assert.Equal(t, "bind@example.com", request.Email)
		assert.Equal(t, "one-use-token", request.Turnstile)
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/verification", bytes.NewBufferString(
		`{"email":"bind@example.com","turnstile":"one-use-token"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.True(t, nextCalled)
	assert.Equal(t, "one-use-token", verifiedToken)
	assert.Empty(t, request.URL.Query().Get("turnstile"))
}

func TestTurnstileCheckFromBodyRejectsMissingToken(t *testing.T) {
	oldEnabled := common.TurnstileCheckEnabled
	common.TurnstileCheckEnabled = true
	t.Cleanup(func() { common.TurnstileCheckEnabled = oldEnabled })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	nextCalled := false
	router.POST("/verification", TurnstileCheckFromBody(), func(c *gin.Context) {
		nextCalled = true
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/verification", bytes.NewBufferString(`{"email":"bind@example.com"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.False(t, nextCalled)
	assert.Contains(t, recorder.Body.String(), "Turnstile token")
}

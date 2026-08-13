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
		t.Setenv("TURNSTILE_VERIFY_URL", " https://verify.88api.ai/turnstile/v0/siteverify ")
		assert.Equal(t, "https://verify.88api.ai/turnstile/v0/siteverify", turnstileVerifyURL())
	})

	t.Run("falls back to Cloudflare when unset", func(t *testing.T) {
		t.Setenv("TURNSTILE_VERIFY_URL", "")
		assert.Equal(t, defaultTurnstileVerifyURL, turnstileVerifyURL())
	})
}

func TestTurnstileCheckFromBodyVerifiesTokenAndRestoresBody(t *testing.T) {
	oldEnabled := common.TurnstileCheckEnabled
	oldSecret := common.TurnstileSecretKey
	common.TurnstileCheckEnabled = true
	common.TurnstileSecretKey = "test-secret"
	t.Cleanup(func() {
		common.TurnstileCheckEnabled = oldEnabled
		common.TurnstileSecretKey = oldSecret
	})

	var verifiedToken string
	verifyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		verifiedToken = r.Form.Get("response")
		_, _ = io.WriteString(w, `{"success":true}`)
	}))
	t.Cleanup(verifyServer.Close)
	t.Setenv("TURNSTILE_VERIFY_URL", verifyServer.URL)

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

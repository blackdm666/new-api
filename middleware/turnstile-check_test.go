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

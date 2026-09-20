package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionRejectsInvalidBillingMapsAtHTTPBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{
			name:        "null model price",
			body:        `{"key":"ModelPrice","value":"{\"video-model\":null}"}`,
			wantMessage: "must be a number",
		},
		{
			name:        "unknown billing mode",
			body:        `{"key":"billing_setting.billing_mode","value":"{\"video-model\":\"per_minute\"}"}`,
			wantMessage: "unsupported billing mode",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequest(
				http.MethodPut,
				"/api/option/",
				strings.NewReader(test.body),
			)

			UpdateOption(context)

			assert.Equal(t, http.StatusOK, response.Code)
			var payload struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
			assert.False(t, payload.Success)
			assert.Contains(t, payload.Message, test.wantMessage)
		})
	}
}

package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPasswordLoginDistinguishesDisabledUserAfterPasswordVerification(t *testing.T) {
	previousDB := model.DB
	previousPasswordLoginEnabled := common.PasswordLoginEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	require.NoError(t, i18n.Init())
	model.DB = db
	common.PasswordLoginEnabled = true
	t.Cleanup(func() {
		model.DB = previousDB
		common.PasswordLoginEnabled = previousPasswordLoginEnabled
	})

	passwordHash, err := common.Password2Hash("CorrectPassword123")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Username: "disabled-login-user",
		Password: passwordHash,
		Status:   common.UserStatusDisabled,
	}).Error)

	tests := []struct {
		name            string
		username        string
		password        string
		expectedCode    string
		expectedMessage string
	}{
		{
			name:            "correct password for disabled user",
			username:        "disabled-login-user",
			password:        "CorrectPassword123",
			expectedCode:    "AUTH_USER_DISABLED",
			expectedMessage: "用户已被封禁",
		},
		{
			name:            "wrong password for disabled user",
			username:        "disabled-login-user",
			password:        "WrongPassword123",
			expectedCode:    "AUTH_INVALID_CREDENTIALS",
			expectedMessage: "用户名或密码错误",
		},
		{
			name:            "unknown user",
			username:        "unknown-login-user",
			password:        "CorrectPassword123",
			expectedCode:    "AUTH_INVALID_CREDENTIALS",
			expectedMessage: "用户名或密码错误",
		},
	}

	gin.SetMode(gin.TestMode)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := common.Marshal(LoginRequest{
				Username: test.username,
				Password: test.password,
			})
			require.NoError(t, err)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("Accept-Language", "zh-CN")

			Login(c)

			assert.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, test.expectedCode, response.Code)
			assert.Equal(t, test.expectedMessage, response.Message)
		})
	}
}

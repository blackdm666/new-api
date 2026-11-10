package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
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
			response := postPasswordLogin(t, test.username, test.password)
			assert.False(t, response.Success)
			assert.Equal(t, test.expectedCode, response.Code)
			assert.Equal(t, test.expectedMessage, response.Message)
		})
	}
}

type passwordLoginResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func postPasswordLogin(t *testing.T, username string, password string) passwordLoginResponse {
	t.Helper()
	body, err := common.Marshal(LoginRequest{Username: username, Password: password})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Accept-Language", "zh-CN")

	Login(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response passwordLoginResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

// Account lockout per OWASP Authentication Cheat Sheet / ASVS 5.0 V6.3.1.
func TestPasswordLoginLocksAccountAfterRepeatedFailures(t *testing.T) {
	for backend, firstUserID := range map[string]int{"memory": 1000, "redis": 2000} {
		t.Run(backend, func(t *testing.T) {
			previousDB := model.DB
			previousPasswordLoginEnabled := common.PasswordLoginEnabled
			previousEncryption := common.PasswordLoginEncryptionEnabled
			previousRedis, previousClient := common.RedisEnabled, common.RDB
			previousLimit, previousWindow, previousLockout := service.LoginFailureLimit, service.LoginFailureWindow, service.LoginLockoutDuration
			t.Cleanup(func() {
				model.DB = previousDB
				common.PasswordLoginEnabled = previousPasswordLoginEnabled
				common.PasswordLoginEncryptionEnabled = previousEncryption
				common.RedisEnabled, common.RDB = previousRedis, previousClient
				service.LoginFailureLimit, service.LoginFailureWindow, service.LoginLockoutDuration = previousLimit, previousWindow, previousLockout
			})

			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.TwoFA{}, &model.PasskeyCredential{}, &model.AuthFlow{}, &model.AuditLog{}))
			require.NoError(t, i18n.Init())
			model.DB = db
			common.PasswordLoginEnabled = true
			common.PasswordLoginEncryptionEnabled = false
			common.RedisEnabled, common.RDB = false, nil
			if backend == "redis" {
				server := miniredis.RunT(t)
				common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
				common.RedisEnabled = true
			}
			service.LoginFailureLimit = 3
			service.LoginFailureWindow = 15 * time.Minute
			service.LoginLockoutDuration = 15 * time.Minute

			// Usernames and user IDs are unique per backend so the shared
			// in-memory store cannot carry counters between tests.
			prefix := "lockout-" + backend + "-"
			passwordHash, err := common.Password2Hash("CorrectPassword123")
			require.NoError(t, err)
			for i, name := range []string{"victim", "reset"} {
				require.NoError(t, db.Create(&model.User{
					Id:       firstUserID + i,
					Username: prefix + name,
					Email:    prefix + name + "@example.com",
					Password: passwordHash,
					Status:   common.UserStatusEnabled,
					AffCode:  prefix + name,
				}).Error)
			}

			const lockedCode, lockedMessage = "AUTH_TOO_MANY_ATTEMPTS", "登录失败次数过多，请稍后再试，或改用其他方式登录"

			// The third wrong password still reads as a wrong password; the
			// lock applies from the next attempt, even with the right password
			// and even when the client switches to the account's email.
			for range 3 {
				response := postPasswordLogin(t, prefix+"victim", "WrongPassword123")
				assert.Equal(t, "AUTH_INVALID_CREDENTIALS", response.Code)
			}
			for _, identifier := range []string{prefix + "victim", prefix + "victim@example.com", "  " + strings.ToUpper(prefix+"victim") + " "} {
				response := postPasswordLogin(t, identifier, "CorrectPassword123")
				assert.False(t, response.Success)
				assert.Equal(t, lockedCode, response.Code, identifier)
				assert.Equal(t, lockedMessage, response.Message)
			}

			// Username and email share one budget: failures split across both
			// still lock the account.
			for _, identifier := range []string{prefix + "reset", prefix + "reset@example.com"} {
				assert.Equal(t, "AUTH_INVALID_CREDENTIALS", postPasswordLogin(t, identifier, "WrongPassword123").Code)
			}
			// A correct password before the limit clears the counters, so the
			// earlier failures no longer count towards a lock.
			assert.True(t, postPasswordLogin(t, prefix+"reset", "CorrectPassword123").Success)
			for range 2 {
				assert.Equal(t, "AUTH_INVALID_CREDENTIALS", postPasswordLogin(t, prefix+"reset@example.com", "WrongPassword123").Code)
			}
			assert.True(t, postPasswordLogin(t, prefix+"reset", "CorrectPassword123").Success)

			// An identifier with no account locks the same way, so the lock
			// reveals nothing about which accounts exist.
			for range 3 {
				assert.Equal(t, "AUTH_INVALID_CREDENTIALS", postPasswordLogin(t, prefix+"nobody", "WrongPassword123").Code)
			}
			assert.Equal(t, lockedCode, postPasswordLogin(t, prefix+"nobody", "WrongPassword123").Code)
		})
	}
}

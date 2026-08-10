package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureGeeTestTest(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	oldURL := geeTestValidateURL
	oldClient := geeTestHTTPClient
	oldEnabled := common.GeeTestRegisterCheckEnabled
	oldID := common.GeeTestRegisterCaptchaID
	oldKey := common.GeeTestRegisterCaptchaKey
	geeTestValidateURL = server.URL
	geeTestHTTPClient = server.Client()
	common.GeeTestRegisterCheckEnabled = true
	common.GeeTestRegisterCaptchaID = "register-id"
	common.GeeTestRegisterCaptchaKey = "register-key"
	t.Cleanup(func() {
		server.Close()
		geeTestValidateURL = oldURL
		geeTestHTTPClient = oldClient
		common.GeeTestRegisterCheckEnabled = oldEnabled
		common.GeeTestRegisterCaptchaID = oldID
		common.GeeTestRegisterCaptchaKey = oldKey
	})
	return server
}

func successfulGeeTestHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "register-id", r.URL.Query().Get("captcha_id"))
		require.NoError(t, r.ParseForm())
		require.Equal(t, "lot-1", r.Form.Get("lot_number"))
		mac := hmac.New(sha256.New, []byte("register-key"))
		_, err := mac.Write([]byte("lot-1"))
		require.NoError(t, err)
		require.Equal(t, hex.EncodeToString(mac.Sum(nil)), r.Form.Get("sign_token"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"success","reason":""}`))
	}
}

func TestHumanVerificationValidatesJSONAndRestoresBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureGeeTestTest(t, successfulGeeTestHandler(t))
	router := gin.New()
	router.POST("/register", HumanVerification(GeeTestSceneRegister, GeeTestProofFromJSONBody), func(c *gin.Context) {
		var request struct {
			Username string `json:"username"`
		}
		require.NoError(t, common.DecodeJson(c.Request.Body, &request))
		c.JSON(http.StatusOK, gin.H{"success": true, "username": request.Username})
	})

	body := `{"username":"alice","geetest":{"lot_number":"lot-1","captcha_output":"output","pass_token":"pass","gen_time":"123"}}`
	request := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"success":true`)
	assert.Contains(t, response.Body.String(), `"username":"alice"`)
}

func TestHumanVerificationRejectsMissingProofWithoutCallingBusinessHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamCalled := false
	configureGeeTestTest(t, func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
	})
	businessCalled := false
	router := gin.New()
	router.POST("/register", HumanVerification(GeeTestSceneRegister, GeeTestProofFromJSONBody), func(c *gin.Context) {
		businessCalled = true
	})

	request := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"username":"alice"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.False(t, upstreamCalled)
	assert.False(t, businessCalled)
	assert.Contains(t, response.Body.String(), `"success":false`)
}

func TestHumanVerificationFailsClosedWhenGeeTestIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureGeeTestTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	businessCalled := false
	router := gin.New()
	router.POST("/register", HumanVerification(GeeTestSceneRegister, GeeTestProofFromJSONBody), func(c *gin.Context) {
		businessCalled = true
	})

	body := `{"geetest":{"lot_number":"lot-1","captcha_output":"output","pass_token":"pass","gen_time":"123"}}`
	request := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.False(t, businessCalled)
	assert.Contains(t, response.Body.String(), `"success":false`)
}

func TestHumanVerificationAcceptsRegistrationProofFromQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureGeeTestTest(t, successfulGeeTestHandler(t))
	businessCalled := false
	router := gin.New()
	router.GET("/verification", HumanVerification(GeeTestSceneRegister, GeeTestProofFromQuery), func(c *gin.Context) {
		businessCalled = true
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	query := url.Values{
		"geetest_lot_number":     {"lot-1"},
		"geetest_captcha_output": {"output"},
		"geetest_pass_token":     {"pass"},
		"geetest_gen_time":       {"123"},
	}
	request := httptest.NewRequest(http.MethodGet, "/verification?"+query.Encode(), nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.True(t, businessCalled)
}

func TestOAuthStateHumanVerificationRequiresRequestedSceneAndRestoresBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureGeeTestTest(t, successfulGeeTestHandler(t))
	router := gin.New()
	router.POST("/oauth/state", OAuthStateHumanVerification(), func(c *gin.Context) {
		var request struct {
			Provider string `json:"provider"`
		}
		require.NoError(t, common.DecodeJson(c.Request.Body, &request))
		c.JSON(http.StatusOK, gin.H{"success": true, "provider": request.Provider})
	})

	body := `{"provider":"google","intent":"login","geetest_scene":"register","geetest":{"lot_number":"lot-1","captcha_output":"output","pass_token":"pass","gen_time":"123"}}`
	request := httptest.NewRequest(http.MethodPost, "/oauth/state", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"success":true`)
	assert.Contains(t, response.Body.String(), `"provider":"google"`)
}

func TestOAuthStateHumanVerificationSkipsSessionBoundBindFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamCalled := false
	configureGeeTestTest(t, func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
	})
	businessCalled := false
	router := gin.New()
	router.POST("/oauth/state", OAuthStateHumanVerification(), func(c *gin.Context) {
		businessCalled = true
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	request := httptest.NewRequest(http.MethodPost, "/oauth/state", strings.NewReader(`{"provider":"google","intent":"bind"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.False(t, upstreamCalled)
	assert.True(t, businessCalled)
}

func TestOAuthLoginHumanVerificationAcceptsRequestedSceneFromQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureGeeTestTest(t, successfulGeeTestHandler(t))
	businessCalled := false
	router := gin.New()
	router.GET("/oauth/wechat", OAuthLoginHumanVerification(GeeTestProofFromQuery), func(c *gin.Context) {
		businessCalled = true
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	query := url.Values{
		"geetest_scene":          {"register"},
		"geetest_lot_number":     {"lot-1"},
		"geetest_captcha_output": {"output"},
		"geetest_pass_token":     {"pass"},
		"geetest_gen_time":       {"123"},
	}
	request := httptest.NewRequest(http.MethodGet, "/oauth/wechat?"+query.Encode(), nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.True(t, businessCalled)
}

func TestGeeTestConfigKeepsRegisterAndLoginCredentialsSeparate(t *testing.T) {
	oldRegisterEnabled := common.GeeTestRegisterCheckEnabled
	oldRegisterID := common.GeeTestRegisterCaptchaID
	oldRegisterKey := common.GeeTestRegisterCaptchaKey
	oldLoginEnabled := common.GeeTestLoginCheckEnabled
	oldLoginID := common.GeeTestLoginCaptchaID
	oldLoginKey := common.GeeTestLoginCaptchaKey
	t.Cleanup(func() {
		common.GeeTestRegisterCheckEnabled = oldRegisterEnabled
		common.GeeTestRegisterCaptchaID = oldRegisterID
		common.GeeTestRegisterCaptchaKey = oldRegisterKey
		common.GeeTestLoginCheckEnabled = oldLoginEnabled
		common.GeeTestLoginCaptchaID = oldLoginID
		common.GeeTestLoginCaptchaKey = oldLoginKey
	})

	common.GeeTestRegisterCheckEnabled = true
	common.GeeTestRegisterCaptchaID = "register-id"
	common.GeeTestRegisterCaptchaKey = "register-key"
	common.GeeTestLoginCheckEnabled = true
	common.GeeTestLoginCaptchaID = "login-id"
	common.GeeTestLoginCaptchaKey = "login-key"

	registerID, registerKey, registerEnabled := geeTestConfig(GeeTestSceneRegister)
	loginID, loginKey, loginEnabled := geeTestConfig(GeeTestSceneLogin)

	assert.True(t, registerEnabled)
	assert.Equal(t, "register-id", registerID)
	assert.Equal(t, "register-key", registerKey)
	assert.True(t, loginEnabled)
	assert.Equal(t, "login-id", loginID)
	assert.Equal(t, "login-key", loginKey)
}

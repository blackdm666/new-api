package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
)

type GeeTestScene string

const (
	GeeTestSceneRegister GeeTestScene = "register"
	GeeTestSceneLogin    GeeTestScene = "login"
)

type GeeTestProofSource int

const (
	GeeTestProofFromJSONBody GeeTestProofSource = iota
	GeeTestProofFromQuery
)

const maxGeeTestProofFieldBytes = 16 * 1024

var (
	geeTestValidateURL = "https://gcaptcha4.geetest.com/validate"
	geeTestHTTPClient  = &http.Client{Timeout: 8 * time.Second}
)

type geeTestProof struct {
	LotNumber     string `json:"lot_number"`
	CaptchaOutput string `json:"captcha_output"`
	PassToken     string `json:"pass_token"`
	GenTime       string `json:"gen_time"`
}

type geeTestRequestBody struct {
	GeeTest      geeTestProof `json:"geetest"`
	GeeTestScene GeeTestScene `json:"geetest_scene"`
	Intent       string       `json:"intent"`
}

type geeTestResponse struct {
	Result string `json:"result"`
	Reason string `json:"reason"`
}

func HumanVerification(scene GeeTestScene, source GeeTestProofSource) gin.HandlerFunc {
	return func(c *gin.Context) {
		proof, err := readGeeTestProof(c, source)
		runHumanVerification(c, scene, proof, err)
	}
}

// OAuthStateHumanVerification protects anonymous OAuth login flows before a
// state token is issued. Account binding is already session-bound and cannot
// create a user, so it intentionally skips the challenge.
func OAuthStateHumanVerification() gin.HandlerFunc {
	return func(c *gin.Context) {
		request, err := readGeeTestRequestBody(c)
		if err == nil && strings.TrimSpace(request.Intent) == "bind" {
			c.Next()
			return
		}
		runHumanVerification(c, request.GeeTestScene, request.GeeTest, err)
	}
}

// OAuthLoginHumanVerification protects non-standard third-party login routes
// that can be reached without first obtaining an OAuth state token.
func OAuthLoginHumanVerification(source GeeTestProofSource) gin.HandlerFunc {
	return func(c *gin.Context) {
		scene, proof, err := readRequestedGeeTestProof(c, source)
		runHumanVerification(c, scene, proof, err)
	}
}

func runHumanVerification(c *gin.Context, scene GeeTestScene, proof geeTestProof, readErr error) {
	if scene != GeeTestSceneRegister && scene != GeeTestSceneLogin {
		common.ApiErrorI18n(c, i18n.MsgHumanVerificationRequired)
		c.Abort()
		return
	}

	captchaID, captchaKey, enabled := geeTestConfig(scene)
	if !enabled {
		TurnstileCheck()(c)
		return
	}

	if readErr != nil || !proof.valid() {
		common.ApiErrorI18n(c, i18n.MsgHumanVerificationRequired)
		c.Abort()
		return
	}

	result, err := verifyGeeTest(c, captchaID, captchaKey, proof)
	if err != nil {
		common.SysLog(fmt.Sprintf("GeeTest %s verification unavailable: %v", scene, err))
		common.ApiErrorI18n(c, i18n.MsgHumanVerificationUnavailable)
		c.Abort()
		return
	}
	if result.Result != "success" {
		common.SysLog(fmt.Sprintf("GeeTest %s verification failed: %s", scene, result.Reason))
		common.ApiErrorI18n(c, i18n.MsgHumanVerificationFailed)
		c.Abort()
		return
	}

	c.Next()
}

func geeTestConfig(scene GeeTestScene) (captchaID string, captchaKey string, enabled bool) {
	switch scene {
	case GeeTestSceneRegister:
		return common.GeeTestRegisterCaptchaID, common.GeeTestRegisterCaptchaKey, common.GeeTestRegisterCheckEnabled
	case GeeTestSceneLogin:
		return common.GeeTestLoginCaptchaID, common.GeeTestLoginCaptchaKey, common.GeeTestLoginCheckEnabled
	default:
		return "", "", false
	}
}

func readGeeTestProof(c *gin.Context, source GeeTestProofSource) (geeTestProof, error) {
	if source == GeeTestProofFromQuery {
		return geeTestProof{
			LotNumber:     c.Query("geetest_lot_number"),
			CaptchaOutput: c.Query("geetest_captcha_output"),
			PassToken:     c.Query("geetest_pass_token"),
			GenTime:       c.Query("geetest_gen_time"),
		}, nil
	}

	request, err := readGeeTestRequestBody(c)
	return request.GeeTest, err
}

func readRequestedGeeTestProof(c *gin.Context, source GeeTestProofSource) (GeeTestScene, geeTestProof, error) {
	if source == GeeTestProofFromQuery {
		proof, err := readGeeTestProof(c, source)
		return GeeTestScene(c.Query("geetest_scene")), proof, err
	}
	request, err := readGeeTestRequestBody(c)
	return request.GeeTestScene, request.GeeTest, err
}

func readGeeTestRequestBody(c *gin.Context) (geeTestRequestBody, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return geeTestRequestBody{}, err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	var request geeTestRequestBody
	if err := common.Unmarshal(body, &request); err != nil {
		return geeTestRequestBody{}, err
	}
	return request, nil
}

func (proof geeTestProof) valid() bool {
	fields := []string{proof.LotNumber, proof.CaptchaOutput, proof.PassToken, proof.GenTime}
	for _, field := range fields {
		if strings.TrimSpace(field) == "" || len(field) > maxGeeTestProofFieldBytes {
			return false
		}
	}
	return true
}

func verifyGeeTest(c *gin.Context, captchaID string, captchaKey string, proof geeTestProof) (geeTestResponse, error) {
	mac := hmac.New(sha256.New, []byte(captchaKey))
	_, _ = mac.Write([]byte(proof.LotNumber))
	signToken := hex.EncodeToString(mac.Sum(nil))

	endpoint, err := url.Parse(geeTestValidateURL)
	if err != nil {
		return geeTestResponse{}, err
	}
	query := endpoint.Query()
	query.Set("captcha_id", captchaID)
	endpoint.RawQuery = query.Encode()

	form := url.Values{
		"lot_number":     {proof.LotNumber},
		"captcha_output": {proof.CaptchaOutput},
		"pass_token":     {proof.PassToken},
		"gen_time":       {proof.GenTime},
		"sign_token":     {signToken},
	}
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return geeTestResponse{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := geeTestHTTPClient.Do(request)
	if err != nil {
		return geeTestResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return geeTestResponse{}, fmt.Errorf("unexpected GeeTest status: %d", response.StatusCode)
	}

	var result geeTestResponse
	if err := common.DecodeJson(io.LimitReader(response.Body, 64*1024), &result); err != nil {
		return geeTestResponse{}, err
	}
	return result, nil
}

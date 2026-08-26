package controller

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

func Playground(c *gin.Context) {
	if newAPIError := setupPlaygroundContext(c); newAPIError != nil {
		respondPlaygroundError(c, newAPIError)
		return
	}
	Relay(c, types.RelayFormatOpenAI)
}

func PlaygroundImage(c *gin.Context) {
	if newAPIError := setupPlaygroundContext(c); newAPIError != nil {
		respondPlaygroundError(c, newAPIError)
		return
	}
	Relay(c, types.RelayFormatOpenAIImage)
}

func PlaygroundVideo(c *gin.Context) {
	if newAPIError := setupPlaygroundContext(c); newAPIError != nil {
		respondPlaygroundError(c, newAPIError)
		return
	}
	RelayTask(c)
}

func PlaygroundVideoFetch(c *gin.Context) {
	if newAPIError := setupPlaygroundContext(c); newAPIError != nil {
		respondPlaygroundError(c, newAPIError)
		return
	}
	RelayTaskFetch(c)
}

func setupPlaygroundContext(c *gin.Context) *types.NewAPIError {
	useAccessToken := c.GetBool("use_access_token")
	if useAccessToken {
		return types.NewError(errors.New("暂不支持使用 access token"), types.ErrorCodeAccessDenied, types.ErrOptionWithSkipRetry())
	}

	userId := c.GetInt("id")

	// Write user context to ensure acceptUnsetRatio is available
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		return types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}
	userCache.WriteContext(c)

	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if usingGroup == "" {
		usingGroup = userCache.Group
		common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
	}
	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("playground-%s", usingGroup),
		Group:  usingGroup,
	}
	if err := middleware.SetupContextForToken(c, tempToken); err != nil {
		return types.NewError(err, types.ErrorCodeAccessDenied, types.ErrOptionWithSkipRetry())
	}
	return nil
}

func respondPlaygroundError(c *gin.Context, newAPIError *types.NewAPIError) {
	c.JSON(newAPIError.StatusCode, gin.H{
		"error": newAPIError.ToOpenAIError(),
	})
}

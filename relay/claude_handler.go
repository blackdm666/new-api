package relay

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
)

func ClaudeHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {

	info.InitChannelMeta(c)

	claudeReq, ok := info.Request.(*dto.ClaudeRequest)

	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected *dto.ClaudeRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(claudeReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ClaudeRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ApplyReasoningModelSuffix(c, info, request); err != nil {
		return newConvertRequestFailedError(c, info, err)
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	if info.ChannelSetting.SystemPrompt != "" {
		if request.System == nil {
			request.SetStringSystem(info.ChannelSetting.SystemPrompt)
		} else if info.ChannelSetting.SystemPromptOverride {
			common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
			if request.IsStringSystem() {
				existing := strings.TrimSpace(request.GetStringSystem())
				if existing == "" {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt)
				} else {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt + "\n" + existing)
				}
			} else {
				systemContents := request.ParseSystem()
				newSystem := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
				newSystem.SetText(info.ChannelSetting.SystemPrompt)
				if len(systemContents) == 0 {
					request.System = []dto.ClaudeMediaMessage{newSystem}
				} else {
					request.System = append([]dto.ClaudeMediaMessage{newSystem}, systemContents...)
				}
			}
		}
	}

	if !model_setting.GetGlobalSettings().PassThroughRequestEnabled &&
		!info.ChannelSetting.PassThroughBodyEnabled &&
		service.ShouldChatCompletionsUseResponsesGlobal(info.ChannelId, info.ChannelType, info.OriginModelName) {
		usage, newApiErr := textRequestViaResponses(c, info, adaptor, request)
		if newApiErr != nil {
			return newApiErr
		}

		service.PostTextConsumeQuota(c, info, usage, nil)
		return nil
	}

	httpResp, closeRequestBodies, newAPIError := sendClaudeRequest(c, info, adaptor, request)
	defer closeRequestBodies()
	if newAPIError != nil {
		return newAPIError
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, c.GetString("status_code_mapping"))
		return newAPIError
	}

	service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
	return nil
}

// sendClaudeRequest sends a Claude Messages request upstream and returns the
// successful upstream response (nil for adaptors that do not use HTTP
// responses). Native Claude requests follow the channel's Claude thinking
// compatibility mode, which may retry once on the same channel and key after
// an upstream 400 (see claudeThinkingCompat). The returned function releases
// the outgoing bodies and must run only after the response has been handled.
func sendClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, request *dto.ClaudeRequest) (*http.Response, func(), *types.NewAPIError) {
	var bodies []io.Closer
	closeBodies := func() {
		for _, body := range bodies {
			_ = body.Close()
		}
	}
	statusCodeMappingStr := c.GetString("status_code_mapping")
	passThrough := model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled
	var thinkingCompat *claudeThinkingCompat
	if !passThrough {
		thinkingCompat = newClaudeThinkingCompat(info, request)
	}

	// retriedErr is the failed attempt that the thinking compatibility retry
	// replaces; it is returned when the retry cannot change the request.
	var retriedErr *types.NewAPIError
	for {
		var requestBody io.Reader
		if passThrough {
			storage, err := common.GetBodyStorage(c)
			if err != nil {
				return nil, closeBodies, types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			requestBody = common.NewReplayableBodyReader(storage)
		} else {
			convertedRequest, err := adaptor.ConvertClaudeRequest(c, info, request)
			if err != nil {
				return nil, closeBodies, newConvertRequestFailedError(c, info, err)
			}
			thinkingCompat.observe(convertedRequest)
			if thinkingCompat.retryUnchanged() {
				service.ResetStatusCode(retriedErr, statusCodeMappingStr)
				return nil, closeBodies, retriedErr
			}
			relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
			jsonData, err := common.Marshal(convertedRequest)
			if err != nil {
				return nil, closeBodies, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}

			// remove disabled fields for Claude API
			jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
			if err != nil {
				return nil, closeBodies, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}

			// apply param override
			if len(info.ParamOverride) > 0 {
				jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
				if err != nil {
					return nil, closeBodies, newAPIErrorFromParamOverride(err)
				}
			}

			logger.LogDebug(c, "requestBody: %s", jsonData)
			body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
			if err != nil {
				return nil, closeBodies, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			bodies = append(bodies, closer)
			requestBody = body
		}
		if retriedErr != nil {
			thinkingCompat.recordRetry(c, info)
		}

		resp, err := adaptor.DoRequest(c, info, requestBody)
		if err != nil {
			return nil, closeBodies, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
		}
		if resp == nil {
			return nil, closeBodies, nil
		}
		httpResp := resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode == http.StatusOK {
			return httpResp, closeBodies, nil
		}
		newAPIError := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
		if !thinkingCompat.prepareRetry(c, info, request, httpResp.StatusCode, newAPIError) {
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return nil, closeBodies, newAPIError
		}
		retriedErr = newAPIError
	}
}

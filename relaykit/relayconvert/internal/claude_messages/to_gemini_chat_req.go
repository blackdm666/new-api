package claudemessages

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	relaymedia "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/media"
	sharedgemini "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/gemini"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

// ClaudeMessagesRequestToGeminiGenerateContent converts an Anthropic Messages
// request directly to Gemini generateContent without using an OpenAI DTO as an
// intermediate representation.
func ClaudeMessagesRequestToGeminiGenerateContent(c context.Context, request dto.ClaudeRequest, info convmeta.Meta) (*dto.GeminiChatRequest, error) {
	opts := convmeta.OptionsOf(info)
	geminiRequest := &dto.GeminiChatRequest{
		Contents: make([]dto.GeminiChatContent, 0, len(request.Messages)),
		GenerationConfig: dto.GeminiChatGenerationConfig{
			Temperature: request.Temperature,
		},
	}

	if request.TopP != nil && *request.TopP > 0 {
		geminiRequest.GenerationConfig.TopP = kitutil.GetPointer(*request.TopP)
	}
	if request.TopK != nil && *request.TopK > 0 {
		topK := float64(*request.TopK)
		geminiRequest.GenerationConfig.TopK = &topK
	}
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		geminiRequest.GenerationConfig.MaxOutputTokens = kitutil.GetPointer(*request.MaxTokens)
	} else if request.MaxTokensToSample != nil && *request.MaxTokensToSample > 0 {
		geminiRequest.GenerationConfig.MaxOutputTokens = kitutil.GetPointer(*request.MaxTokensToSample)
	}
	if len(request.StopSequences) > 0 {
		geminiRequest.GenerationConfig.StopSequences = append([]string(nil), request.StopSequences...)
		if len(geminiRequest.GenerationConfig.StopSequences) > 5 {
			geminiRequest.GenerationConfig.StopSequences = geminiRequest.GenerationConfig.StopSequences[:5]
		}
	}

	upstreamModelName := request.Model
	if modelName := convmeta.UpstreamModelName(info); modelName != "" {
		upstreamModelName = modelName
	}
	if opts.Gemini.SupportsImagineModel(upstreamModelName) {
		geminiRequest.GenerationConfig.ResponseModalities = []string{"TEXT", "IMAGE"}
	}

	if err := applyClaudeGeminiExtraBody(geminiRequest, request.ExtraBody); err != nil {
		return nil, err
	}
	if geminiRequest.GenerationConfig.ThinkingConfig == nil {
		applyClaudeThinkingConfig(geminiRequest, request.Thinking)
	}
	if geminiRequest.GenerationConfig.ThinkingConfig == nil {
		sharedgemini.ApplyThinkingConfig(geminiRequest, info)
	}

	applyClaudeGeminiSafetySettings(geminiRequest, opts)
	if err := applyClaudeGeminiTools(geminiRequest, request.Tools, request.ToolChoice); err != nil {
		return nil, err
	}
	applyClaudeGeminiSystemInstruction(geminiRequest, request)

	for _, message := range request.Messages {
		content, err := claudeMessageToGeminiContent(c, request, message, opts)
		if err != nil {
			return nil, err
		}
		if len(content.Parts) > 0 {
			geminiRequest.Contents = append(geminiRequest.Contents, content)
		}
	}

	return geminiRequest, nil
}

func applyClaudeGeminiExtraBody(request *dto.GeminiChatRequest, raw []byte) error {
	if request == nil || len(raw) == 0 {
		return nil
	}

	var extraBody map[string]interface{}
	if err := kitutil.Unmarshal(raw, &extraBody); err != nil {
		return fmt.Errorf("invalid extra body: %w", err)
	}
	googleBody, ok := extraBody["google"].(map[string]interface{})
	if !ok {
		return nil
	}

	if rawThinkingConfig, ok := firstMapValue(googleBody, "thinking_config", "thinkingConfig"); ok {
		data, err := kitutil.Marshal(rawThinkingConfig)
		if err != nil {
			return fmt.Errorf("failed to marshal thinking config: %w", err)
		}
		var thinkingConfig dto.GeminiThinkingConfig
		if err := kitutil.Unmarshal(data, &thinkingConfig); err != nil {
			return fmt.Errorf("invalid thinking config: %w", err)
		}
		request.GenerationConfig.ThinkingConfig = &thinkingConfig
	}

	imageConfig, _ := firstMapValue(googleBody, "image_config", "imageConfig")
	if imageConfig == nil {
		imageConfig = make(map[string]interface{})
	}
	if aspectRatio, ok := firstValue(imageConfig, "aspect_ratio", "aspectRatio"); ok {
		imageConfig["aspectRatio"] = aspectRatio
	}
	delete(imageConfig, "aspect_ratio")
	if imageSize, ok := firstValue(imageConfig, "image_size", "imageSize"); ok {
		imageConfig["imageSize"] = imageSize
	}
	delete(imageConfig, "image_size")

	// Accept the flattened form as a compatibility convenience while keeping
	// extra_body.google.image_config as the documented shape.
	if aspectRatio, ok := firstValue(googleBody, "aspect_ratio", "aspectRatio"); ok {
		imageConfig["aspectRatio"] = aspectRatio
	}
	if imageSize, ok := firstValue(googleBody, "image_size", "imageSize"); ok {
		imageConfig["imageSize"] = imageSize
	}

	if len(imageConfig) > 0 {
		data, err := kitutil.Marshal(imageConfig)
		if err != nil {
			return fmt.Errorf("failed to marshal image config: %w", err)
		}
		request.GenerationConfig.ImageConfig = data
	}
	return nil
}

func firstMapValue(values map[string]interface{}, keys ...string) (map[string]interface{}, bool) {
	for _, key := range keys {
		if value, ok := values[key].(map[string]interface{}); ok {
			return value, true
		}
	}
	return nil, false
}

func firstValue(values map[string]interface{}, keys ...string) (interface{}, bool) {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func applyClaudeThinkingConfig(request *dto.GeminiChatRequest, thinking *dto.Thinking) {
	if request == nil || thinking == nil {
		return
	}
	switch thinking.Type {
	case "enabled":
		request.GenerationConfig.ThinkingConfig = &dto.GeminiThinkingConfig{
			IncludeThoughts: true,
			ThinkingBudget:  thinking.BudgetTokens,
		}
	case "adaptive":
		request.GenerationConfig.ThinkingConfig = &dto.GeminiThinkingConfig{IncludeThoughts: true}
	case "disabled":
		request.GenerationConfig.ThinkingConfig = &dto.GeminiThinkingConfig{ThinkingBudget: kitutil.GetPointer(0)}
	}
}

func applyClaudeGeminiSafetySettings(request *dto.GeminiChatRequest, opts *convmeta.Options) {
	if request == nil || opts == nil {
		return
	}
	for _, category := range sharedgemini.SafetySettingCategories {
		threshold := opts.Gemini.SafetySettingFor(category)
		if threshold == "" {
			continue
		}
		request.SafetySettings = append(request.SafetySettings, dto.GeminiChatSafetySettings{
			Category:  category,
			Threshold: threshold,
		})
	}
}

func applyClaudeGeminiTools(request *dto.GeminiChatRequest, rawTools any, rawToolChoice any) error {
	if request == nil || rawTools == nil {
		return nil
	}

	tools, err := kitutil.Any2Type[[]dto.Tool](rawTools)
	if err != nil {
		return fmt.Errorf("invalid Anthropic tools: %w", err)
	}
	functions := make([]dto.FunctionRequest, 0, len(tools))
	for _, tool := range tools {
		parameters := sharedgemini.CleanFunctionParameters(tool.InputSchema)
		if schema, ok := parameters.(map[string]interface{}); ok {
			if properties, exists := schema["properties"].(map[string]interface{}); exists && len(properties) == 0 {
				parameters = nil
			}
		}
		functions = append(functions, dto.FunctionRequest{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  parameters,
		})
	}
	if len(functions) > 0 {
		request.SetTools([]dto.GeminiChatTool{{FunctionDeclarations: functions}})
	}
	request.ToolConfig = claudeToolChoiceToGeminiConfig(rawToolChoice)
	return nil
}

func claudeToolChoiceToGeminiConfig(raw any) *dto.ToolConfig {
	if raw == nil {
		return nil
	}
	choice, err := kitutil.Any2Type[dto.ClaudeToolChoice](raw)
	if err != nil {
		return nil
	}
	config := &dto.ToolConfig{FunctionCallingConfig: &dto.FunctionCallingConfig{}}
	switch choice.Type {
	case "none":
		config.FunctionCallingConfig.Mode = "NONE"
	case "any":
		config.FunctionCallingConfig.Mode = "ANY"
	case "tool":
		config.FunctionCallingConfig.Mode = "ANY"
		if choice.Name != "" {
			config.FunctionCallingConfig.AllowedFunctionNames = []string{choice.Name}
		}
	default:
		config.FunctionCallingConfig.Mode = "AUTO"
	}
	return config
}

func applyClaudeGeminiSystemInstruction(request *dto.GeminiChatRequest, claudeRequest dto.ClaudeRequest) {
	var systemParts []string
	if claudeRequest.IsStringSystem() {
		if text := claudeRequest.GetStringSystem(); text != "" {
			systemParts = append(systemParts, text)
		}
	} else {
		for _, part := range claudeRequest.ParseSystem() {
			if text := part.GetText(); text != "" {
				systemParts = append(systemParts, text)
			}
		}
	}
	if len(systemParts) > 0 {
		request.SystemInstructions = &dto.GeminiChatContent{
			Parts: []dto.GeminiPart{{Text: strings.Join(systemParts, "\n")}},
		}
	}
}

func claudeMessageToGeminiContent(c context.Context, request dto.ClaudeRequest, message dto.ClaudeMessage, opts *convmeta.Options) (dto.GeminiChatContent, error) {
	role := message.Role
	if role == "assistant" {
		role = "model"
	}
	content := dto.GeminiChatContent{Role: role}
	if message.IsStringContent() {
		if text := message.GetStringContent(); text != "" {
			content.Parts = append(content.Parts, dto.GeminiPart{Text: text})
		}
		if role == "model" {
			sharedgemini.AttachFirstTextThoughtSignature(opts, content.Parts)
		}
		return content, nil
	}

	parts, err := message.ParseContent()
	if err != nil {
		return content, err
	}
	signatureAttached := false
	for _, part := range parts {
		switch part.Type {
		case "text", "input_text":
			if text := part.GetText(); text != "" {
				content.Parts = append(content.Parts, dto.GeminiPart{Text: text})
			}
		case "image", "document":
			source := part.ToFileSource()
			if source == nil {
				continue
			}
			base64Data, mimeType, err := relaymedia.ResolveBase64Data(c, source, "formatting Anthropic media for Gemini")
			if err != nil {
				return content, fmt.Errorf("get file data from '%s' failed: %w", source.GetIdentifier(), err)
			}
			if _, ok := sharedgemini.SupportedMimeTypes[strings.ToLower(mimeType)]; !ok {
				return content, fmt.Errorf("mime type is not supported by Gemini: '%s'", mimeType)
			}
			content.Parts = append(content.Parts, dto.GeminiPart{
				InlineData: &dto.GeminiInlineData{MimeType: mimeType, Data: base64Data},
			})
		case "tool_use":
			functionPart := dto.GeminiPart{
				FunctionCall: &dto.FunctionCall{
					FunctionName: part.Name,
					Arguments:    part.Input,
				},
			}
			if role == "model" && !signatureAttached && sharedgemini.AttachFunctionCallThoughtSignature(opts, &functionPart) {
				signatureAttached = true
			}
			content.Parts = append(content.Parts, functionPart)
		case "tool_result":
			name := part.Name
			if name == "" {
				name = request.SearchToolNameByToolCallId(part.ToolUseId)
			}
			content.Parts = append(content.Parts, dto.GeminiPart{
				FunctionResponse: &dto.GeminiFunctionResponse{
					Name:     name,
					Response: claudeToolResultToGeminiResponse(part.Content),
				},
			})
		}
	}
	if role == "model" && !signatureAttached {
		sharedgemini.AttachFirstTextThoughtSignature(opts, content.Parts)
	}
	return content, nil
}

func claudeToolResultToGeminiResponse(content any) map[string]interface{} {
	if text, ok := content.(string); ok {
		var object map[string]interface{}
		if err := kitutil.Unmarshal([]byte(text), &object); err == nil {
			return object
		}
		var values []interface{}
		if err := kitutil.Unmarshal([]byte(text), &values); err == nil {
			return map[string]interface{}{"result": values}
		}
		return map[string]interface{}{"content": text}
	}
	if object, ok := content.(map[string]interface{}); ok {
		return object
	}
	return map[string]interface{}{"result": content}
}

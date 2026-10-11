package gemini

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"mime/multipart"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	// Gemini image models return one image per generateContent call and do not
	// accept candidateCount > 1, so n is served by parallel calls. The cap is an
	// upstream fan-out bound (OpenAI's own Images API limit), not a billing
	// bound; dto.MaxImageN still validates n first.
	maxGeminiImageCalls        = 10
	geminiImageCallConcurrency = 4
	// Gemini 3 image models accept at most 14 reference images per request.
	maxGeminiImageInputs = 14
)

// geminiImageAspectRatios lists the imageConfig.aspectRatio values accepted by
// Gemini image models; the extreme ratios are Gemini 3.1 Flash Image additions.
var geminiImageAspectRatios = []string{
	"1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9",
	"1:4", "4:1", "1:8", "8:1",
}

// openAIImageSizeAspectRatios maps the fixed OpenAI Images sizes. They select an
// aspect ratio only; they never raise imageSize, which would multiply cost.
var openAIImageSizeAspectRatios = map[string]string{
	"256x256":   "1:1",
	"512x512":   "1:1",
	"1024x1024": "1:1",
	"1536x1024": "3:2",
	"1024x1536": "2:3",
	"1792x1024": "16:9",
	"1024x1792": "9:16",
}

var geminiImageInputMimeTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/jpg":  true,
	"image/webp": true,
	"image/heic": true,
	"image/heif": true,
}

var geminiImageSafetyFinishReasons = map[string]bool{
	"SAFETY":                   true,
	"RECITATION":               true,
	"BLOCKLIST":                true,
	"PROHIBITED_CONTENT":       true,
	"SPII":                     true,
	"IMAGE_SAFETY":             true,
	"IMAGE_PROHIBITED_CONTENT": true,
	"IMAGE_RECITATION":         true,
}

type geminiImageConfig struct {
	AspectRatio string `json:"aspectRatio,omitempty"`
	ImageSize   string `json:"imageSize,omitempty"`
}

// IsGeminiImageModel reports whether a model is a Gemini native image model
// served by generateContent with an IMAGE response modality. Imagen models use
// :predict and are excluded. The configured imagine list (also used by the
// chat conversion) wins; name patterns cover aliases such as nano-banana.
func IsGeminiImageModel(modelName string) bool {
	if model_setting.IsGeminiModelSupportImagine(modelName) {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(modelName))
	if strings.HasPrefix(name, "imagen") {
		return false
	}
	return strings.Contains(name, "nano-banana") ||
		strings.HasPrefix(name, "gemini-") && (strings.Contains(name, "-image") || strings.Contains(name, "image-generation"))
}

// IsGeminiImageRelay reports whether an OpenAI Images request must be converted
// to Gemini generateContent. The upstream name decides the endpoint, so an
// imagen upstream keeps :predict; the origin name also counts so that a
// mapping to a proxy-specific upstream alias keeps working.
func IsGeminiImageRelay(info *relaycommon.RelayInfo) bool {
	if info == nil || info.ChannelMeta == nil {
		return false
	}
	if info.RelayMode != relayconstant.RelayModeImagesGenerations && info.RelayMode != relayconstant.RelayModeImagesEdits {
		return false
	}
	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return false
	}
	return IsGeminiImageModel(info.UpstreamModelName) || IsGeminiImageModel(info.OriginModelName)
}

// geminiImageModelSupportsImageSize excludes the Gemini 2.x image models
// (nano-banana), which only render ~1K and fail when imageSize is sent.
func geminiImageModelSupportsImageSize(modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	return !strings.HasPrefix(name, "gemini-2.") && name != "nano-banana" && !strings.HasPrefix(name, "nano-banana-preview")
}

func newGeminiImageRequestError(format string, args ...any) *types.NewAPIError {
	return types.NewErrorWithStatusCode(fmt.Errorf(format, args...), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

// resolveGeminiImageConfig maps OpenAI size/quality onto Gemini imageConfig.
// size accepts the OpenAI fixed sizes, "auto", "W:H" ratios and arbitrary
// "WxH" pixel sizes. imageSize precedence: an explicit 1K/2K/4K quality, then
// a size whose long edge implies 2K/4K, then hd/high as 2K.
func resolveGeminiImageConfig(size, quality string, supportsImageSize bool) (geminiImageConfig, *types.NewAPIError) {
	config := geminiImageConfig{}
	sizeImageSize := ""
	size = strings.ToLower(strings.TrimSpace(size))
	switch {
	case size == "" || size == "auto":
	case openAIImageSizeAspectRatios[size] != "":
		config.AspectRatio = openAIImageSizeAspectRatios[size]
	case strings.Contains(size, ":"):
		width, height, ok := parseImageDimensions(size, ":")
		if !ok {
			return config, newGeminiImageRequestError("invalid size %q, expected WxH or an aspect ratio such as 16:9", size)
		}
		// Compare values, not strings: 32:18 is 16:9 and 7:3 is Gemini's 21:9.
		index := slices.IndexFunc(geminiImageAspectRatios, func(ratio string) bool {
			ratioWidth, ratioHeight, _ := parseImageDimensions(ratio, ":")
			return width*ratioHeight == height*ratioWidth
		})
		if index < 0 {
			return config, newGeminiImageRequestError("unsupported aspect ratio %q, supported: %s", size, strings.Join(geminiImageAspectRatios, ", "))
		}
		config.AspectRatio = geminiImageAspectRatios[index]
	default:
		width, height, ok := parseImageDimensions(size, "x")
		if !ok {
			return config, newGeminiImageRequestError("invalid size %q, expected WxH or an aspect ratio such as 16:9", size)
		}
		ratio, ok := nearestGeminiImageAspectRatio(width, height)
		if !ok {
			return config, newGeminiImageRequestError("size %q has no matching Gemini aspect ratio, supported: %s", size, strings.Join(geminiImageAspectRatios, ", "))
		}
		config.AspectRatio = ratio
		switch longEdge := max(width, height); {
		case longEdge >= 3000:
			sizeImageSize = "4K"
		case longEdge >= 1800:
			sizeImageSize = "2K"
		}
	}

	qualityImageSize := ""
	explicitQuality := false
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "", "auto", "standard", "low", "medium":
	case "hd", "high":
		qualityImageSize = "2K"
	case "1k", "2k", "4k":
		qualityImageSize = strings.ToUpper(strings.TrimSpace(quality))
		explicitQuality = true
	default:
		return config, newGeminiImageRequestError("unsupported quality %q, supported: auto, standard, low, medium, high, hd, 1K, 2K, 4K", quality)
	}

	if !supportsImageSize {
		return config, nil
	}
	switch {
	case explicitQuality:
		config.ImageSize = qualityImageSize
	case sizeImageSize != "":
		config.ImageSize = sizeImageSize
	default:
		config.ImageSize = qualityImageSize
	}
	return config, nil
}

func parseImageDimensions(value, separator string) (int, int, bool) {
	rawWidth, rawHeight, ok := strings.Cut(value, separator)
	if !ok {
		return 0, 0, false
	}
	width, err := strconv.Atoi(strings.TrimSpace(rawWidth))
	if err != nil || width <= 0 || width > 100000 {
		return 0, 0, false
	}
	height, err := strconv.Atoi(strings.TrimSpace(rawHeight))
	if err != nil || height <= 0 || height > 100000 {
		return 0, 0, false
	}
	return width, height, true
}

// nearestGeminiImageAspectRatio accepts pixel sizes within 2% of a supported
// ratio, e.g. 1920x1080 -> 16:9, 2560x1080 -> 21:9 and 2048x2048 -> 1:1.
func nearestGeminiImageAspectRatio(width, height int) (string, bool) {
	target := math.Log(float64(width) / float64(height))
	best := ""
	bestDistance := math.Inf(1)
	for _, ratio := range geminiImageAspectRatios {
		ratioWidth, ratioHeight, _ := parseImageDimensions(ratio, ":")
		distance := math.Abs(target - math.Log(float64(ratioWidth)/float64(ratioHeight)))
		if distance < bestDistance {
			best, bestDistance = ratio, distance
		}
	}
	return best, bestDistance <= math.Log(1.02)
}

// ConvertOpenAIImageRequestToGemini converts an OpenAI Images generation or
// edit request into one Gemini generateContent request. n is not encoded in
// the body; DoGeminiImageRequest sends one call per requested image.
//
// Deliberately ignored OpenAI fields: user (Gemini has no end-user field and
// it must not leak upstream), style (no Gemini equivalent), background and
// output_format/output_compression (Gemini picks the encoding; the response
// reports the real MIME type), moderation (safety comes from the channel's
// Gemini safety settings), partial_images and watermark.
func ConvertOpenAIImageRequestToGemini(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (*dto.GeminiChatRequest, error) {
	// The Images endpoints are non-streaming for Gemini; streamGenerateContent
	// frames would need a separate image_generation SSE conversion.
	if request.Stream != nil && *request.Stream {
		return nil, newGeminiImageRequestError("stream is not supported for Gemini image models on the Images API")
	}
	if request.N != nil && *request.N > maxGeminiImageCalls {
		return nil, newGeminiImageRequestError("n must be an integer between 1 and %d for Gemini image models", maxGeminiImageCalls)
	}

	if strings.TrimSpace(request.Prompt) == "" {
		return nil, newGeminiImageRequestError("prompt is required")
	}
	imageConfig, apiErr := resolveGeminiImageConfig(request.Size, request.Quality, geminiImageModelSupportsImageSize(info.UpstreamModelName))
	if apiErr != nil {
		return nil, apiErr
	}
	imageParts, err := collectGeminiImageInputs(c, info, request)
	if err != nil {
		return nil, err
	}

	parts := make([]dto.GeminiPart, 0, len(imageParts)+1)
	parts = append(parts, dto.GeminiPart{Text: request.Prompt})
	parts = append(parts, imageParts...)
	geminiRequest := &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{{Role: "user", Parts: parts}},
		GenerationConfig: dto.GeminiChatGenerationConfig{
			// Same modalities as the chat conversion that already serves these
			// models; Gemini 3 image models expect TEXT alongside IMAGE.
			ResponseModalities: []string{"TEXT", "IMAGE"},
		},
	}
	if imageConfig != (geminiImageConfig{}) {
		rawConfig, err := common.Marshal(imageConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal imageConfig: %w", err)
		}
		geminiRequest.GenerationConfig.ImageConfig = rawConfig
	}
	for _, category := range SafetySettingList {
		geminiRequest.SafetySettings = append(geminiRequest.SafetySettings, dto.GeminiChatSafetySettings{
			Category:  category,
			Threshold: model_setting.GetGeminiSafetySetting(category),
		})
	}
	return geminiRequest, nil
}

// collectGeminiImageInputs turns edit inputs into inlineData parts. Multipart
// edits read image / image[] / image[N] files and URL or data-URI values; JSON
// bodies read image and images. URLs go through service.GetBase64Data, which
// applies the download size limit and SSRF protection. Gemini has no mask
// input, so a mask is rejected rather than silently ignored.
func collectGeminiImageInputs(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) ([]dto.GeminiPart, error) {
	if len(request.Mask) > 0 && string(request.Mask) != "null" {
		return nil, newGeminiImageRequestError("mask is not supported by Gemini image models")
	}

	var references []string
	var parts []dto.GeminiPart
	isMultipart := c != nil && c.Request != nil && strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data")
	if isMultipart {
		form := c.Request.MultipartForm
		if form == nil {
			parsed, err := common.ParseMultipartFormReusable(c)
			if err != nil {
				return nil, newGeminiImageRequestError("failed to parse image edit form: %s", err.Error())
			}
			form = parsed
		}
		if len(form.File["mask"]) > 0 || strings.TrimSpace(strings.Join(form.Value["mask"], "")) != "" {
			return nil, newGeminiImageRequestError("mask is not supported by Gemini image models")
		}
		// Field names are sorted so image[0], image[1] keep the client's order;
		// repeated image / image[] parts keep their order within the field.
		var files []*multipart.FileHeader
		for _, field := range slices.Sorted(maps.Keys(form.File)) {
			if field == "image" || strings.HasPrefix(field, "image[") {
				files = append(files, form.File[field]...)
			}
		}
		for _, field := range slices.Sorted(maps.Keys(form.Value)) {
			if field != "image" && !strings.HasPrefix(field, "image[") {
				continue
			}
			for _, value := range form.Value[field] {
				if value = strings.TrimSpace(value); value != "" {
					references = append(references, value)
				}
			}
		}
		if len(files)+len(references) > maxGeminiImageInputs {
			return nil, newGeminiImageRequestError("at most %d input images are supported by Gemini image models", maxGeminiImageInputs)
		}
		// Uploaded files are already bounded by the request body size limit.
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open image %q: %w", fileHeader.Filename, err)
			}
			data, err := io.ReadAll(file)
			_ = file.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read image %q: %w", fileHeader.Filename, err)
			}
			mimeType := strings.ToLower(strings.TrimSpace(fileHeader.Header.Get("Content-Type")))
			if mimeType == "" || mimeType == "application/octet-stream" {
				mimeType = http.DetectContentType(data)
			}
			if !geminiImageInputMimeTypes[mimeType] {
				return nil, newGeminiImageRequestError("image %q has unsupported type %q", fileHeader.Filename, mimeType)
			}
			parts = append(parts, dto.GeminiPart{InlineData: &dto.GeminiInlineData{
				MimeType: mimeType,
				Data:     base64.StdEncoding.EncodeToString(data),
			}})
		}
	} else {
		for _, raw := range []json.RawMessage{request.Image, request.Images} {
			found, err := openAIImageReferences(raw)
			if err != nil {
				return nil, err
			}
			references = append(references, found...)
		}
		if len(references) > maxGeminiImageInputs {
			return nil, newGeminiImageRequestError("at most %d input images are supported by Gemini image models", maxGeminiImageInputs)
		}
	}

	for _, reference := range references {
		base64Data, mimeType, err := service.GetBase64Data(c, types.NewFileSourceFromData(reference, ""), "formatting image for Gemini")
		if err != nil {
			return nil, newGeminiImageRequestError("failed to load input image: %s", err.Error())
		}
		mimeType = strings.ToLower(mimeType)
		if !geminiImageInputMimeTypes[mimeType] {
			return nil, newGeminiImageRequestError("input image has unsupported type %q", mimeType)
		}
		parts = append(parts, dto.GeminiPart{InlineData: &dto.GeminiInlineData{MimeType: mimeType, Data: base64Data}})
	}

	if info.RelayMode == relayconstant.RelayModeImagesEdits && len(parts) == 0 {
		return nil, newGeminiImageRequestError("image is required")
	}
	return parts, nil
}

// openAIImageReferences flattens JSON image inputs: a URL or data URI string,
// an {"image_url": "..."} or {"image_url": {"url": "..."}} object, or an array
// of either. OpenAI file IDs cannot be resolved for Gemini.
func openAIImageReferences(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil, newGeminiImageRequestError("invalid image input: %s", err.Error())
	}
	items, isArray := value.([]any)
	if !isArray {
		items = []any{value}
	}
	references := make([]string, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case string:
			if typed = strings.TrimSpace(typed); typed != "" {
				references = append(references, typed)
			}
		case map[string]any:
			if _, hasFileID := typed["file_id"]; hasFileID {
				return nil, newGeminiImageRequestError("file_id image inputs are not supported by Gemini image models, use an image URL or base64 data")
			}
			imageURL := typed["image_url"]
			if nested, ok := imageURL.(map[string]any); ok {
				imageURL = nested["url"]
			}
			reference, ok := imageURL.(string)
			if !ok || strings.TrimSpace(reference) == "" {
				return nil, newGeminiImageRequestError("image input object must contain image_url")
			}
			references = append(references, strings.TrimSpace(reference))
		default:
			return nil, newGeminiImageRequestError("unsupported image input type %T", item)
		}
	}
	return references, nil
}

// geminiImageCallResult keeps the parsed response of a call that delivered an
// image, or the raw upstream reply of a failed call for error propagation.
type geminiImageCallResult struct {
	response     *dto.GeminiChatResponse
	modelVersion string
	status       int
	header       http.Header
	body         []byte
	err          error
}

// DoGeminiImageRequest sends the converted generateContent request once per
// effective requested image (info.ImageRequestCount, refreshed per attempt by
// image billing), at most geminiImageCallConcurrency at a time. Calls that
// return images are merged into one Gemini response whose usageMetadata is the
// sum of those calls only, so a partial failure bills just the images that
// were delivered. When no call yields an image, the first failed upstream
// response is returned unchanged so the normal error handling applies.
//
// Every call goes through the unchanged channel.DoApiRequest with its own
// adaptor from newAdaptor: adaptors may cache state while building the URL
// and headers (Vertex stores credentials), so concurrent calls must not share
// one instance.
func DoGeminiImageRequest(newAdaptor func() channel.Adaptor, c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	calls := min(max(info.ImageRequestCount, 1), maxGeminiImageCalls)
	if calls == 1 {
		return channel.DoApiRequest(newAdaptor(), c, info, requestBody)
	}
	body, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, fmt.Errorf("read Gemini image request body failed: %w", err)
	}

	results := make([]geminiImageCallResult, calls)
	slots := make(chan struct{}, geminiImageCallConcurrency)
	var wg sync.WaitGroup
	for i := range calls {
		adaptor := newAdaptor()
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			results[i] = doGeminiImageCall(adaptor, c, info, body)
		})
	}
	wg.Wait()

	merged := struct {
		dto.GeminiChatResponse
		ModelVersion string `json:"modelVersion,omitempty"`
	}{}
	var usage *dto.GeminiUsageMetadata
	succeeded := 0
	for _, result := range results {
		if result.response == nil {
			continue
		}
		succeeded++
		merged.Candidates = append(merged.Candidates, result.response.Candidates...)
		if merged.ModelVersion == "" {
			merged.ModelVersion = result.modelVersion
		}
		if metadata := result.response.GetUsageMetadata(); dto.HasGeminiUsageMetadataTokens(metadata) {
			usage = addGeminiUsageMetadata(usage, metadata)
		}
	}
	if succeeded == 0 {
		for _, result := range results {
			if result.status != 0 {
				return &http.Response{StatusCode: result.status, Header: result.header, Body: io.NopCloser(bytes.NewReader(result.body))}, nil
			}
		}
		return nil, results[0].err
	}
	if succeeded < calls {
		logger.LogWarn(c, fmt.Sprintf("Gemini image fan-out: %d of %d upstream calls returned images", succeeded, calls))
	}
	if usage != nil {
		merged.UsageMetadata = *usage
	}
	mergedBody, err := common.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("merge Gemini image responses failed: %w", err)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(mergedBody)),
	}, nil
}

func doGeminiImageCall(adaptor channel.Adaptor, c *gin.Context, info *relaycommon.RelayInfo, requestBody []byte) geminiImageCallResult {
	resp, err := channel.DoApiRequest(adaptor, c, info, bytes.NewReader(requestBody))
	if err != nil {
		return geminiImageCallResult{err: err}
	}
	body, err := io.ReadAll(resp.Body)
	service.CloseResponseBodyGracefully(resp)
	if err != nil {
		return geminiImageCallResult{err: fmt.Errorf("read Gemini image response failed: %w", err)}
	}
	result := geminiImageCallResult{status: resp.StatusCode, header: resp.Header, body: body}
	if resp.StatusCode != http.StatusOK {
		return result
	}
	var response dto.GeminiChatResponse
	if err := common.Unmarshal(body, &response); err != nil || len(geminiResponseImages(&response)) == 0 {
		return result
	}
	return geminiImageCallResult{response: &response, modelVersion: gjson.GetBytes(body, "modelVersion").Str, status: resp.StatusCode}
}

// addGeminiUsageMetadata sums independent generateContent calls. This differs
// from dto.MergeGeminiUsageMetadataNonZero, which overlays cumulative stream
// snapshots of a single call.
func addGeminiUsageMetadata(total *dto.GeminiUsageMetadata, add *dto.GeminiUsageMetadata) *dto.GeminiUsageMetadata {
	if total == nil {
		total = &dto.GeminiUsageMetadata{}
	}
	total.PromptTokenCount += add.PromptTokenCount
	total.ToolUsePromptTokenCount += add.ToolUsePromptTokenCount
	total.CandidatesTokenCount += add.CandidatesTokenCount
	total.TotalTokenCount += add.TotalTokenCount
	total.ThoughtsTokenCount += add.ThoughtsTokenCount
	total.CachedContentTokenCount += add.CachedContentTokenCount
	total.PromptTokensDetails = addGeminiTokenDetails(total.PromptTokensDetails, add.PromptTokensDetails)
	total.ToolUsePromptTokensDetails = addGeminiTokenDetails(total.ToolUsePromptTokensDetails, add.ToolUsePromptTokensDetails)
	total.CandidatesTokensDetails = addGeminiTokenDetails(total.CandidatesTokensDetails, add.CandidatesTokensDetails)
	return total
}

func addGeminiTokenDetails(total []dto.GeminiPromptTokensDetails, add []dto.GeminiPromptTokensDetails) []dto.GeminiPromptTokensDetails {
	for _, detail := range add {
		found := false
		for i := range total {
			if strings.EqualFold(total[i].Modality, detail.Modality) {
				total[i].TokenCount += detail.TokenCount
				found = true
				break
			}
		}
		if !found {
			total = append(total, detail)
		}
	}
	return total
}

// geminiResponseImages returns the final image parts. Thought parts are interim
// drafts of thinking image models and are not delivered or counted.
func geminiResponseImages(response *dto.GeminiChatResponse) []*dto.GeminiInlineData {
	var images []*dto.GeminiInlineData
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Thought || part.InlineData == nil || part.InlineData.Data == "" {
				continue
			}
			if strings.HasPrefix(strings.ToLower(part.InlineData.MimeType), "image/") {
				images = append(images, part.InlineData)
			}
		}
	}
	return images
}

// GeminiImageGenerateContentHandler converts a Gemini generateContent response
// into an OpenAI Images response. Usage is built exactly like the chat path
// (usageMetadata, including image output tokens), and the billable image count
// is the number of delivered image payloads. A response without images is an
// error and is not settled as a success.
func GeminiImageGenerateContentHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	service.CloseResponseBodyGracefully(resp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	var geminiResponse dto.GeminiChatResponse
	if err := common.Unmarshal(responseBody, &geminiResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	info.ObserveResponseModel(gjson.GetBytes(responseBody, "modelVersion").Str)

	images := geminiResponseImages(&geminiResponse)
	if len(images) == 0 {
		return nil, geminiImageMissingError(c, info, &geminiResponse)
	}

	// b64_json is the default, matching the imagen path and gpt-image models.
	// An explicit url request gets a data URL: this gateway has no image
	// storage to host generated files.
	responseFormat := ""
	if imageRequest, ok := info.Request.(*dto.ImageRequest); ok {
		responseFormat = imageRequest.ResponseFormat
	}
	if responseFormat == "" && c.Request != nil {
		// Multipart edit validation does not copy response_format into the DTO.
		responseFormat = c.Request.PostForm.Get("response_format")
	}
	wantsURL := strings.EqualFold(strings.TrimSpace(responseFormat), "url")
	openAIResponse := dto.ImageResponse{
		Created: common.GetTimestamp(),
		Data:    make([]dto.ImageData, 0, len(images)),
	}
	for _, image := range images {
		if wantsURL {
			openAIResponse.Data = append(openAIResponse.Data, dto.ImageData{Url: "data:" + image.MimeType + ";base64," + image.Data})
			continue
		}
		openAIResponse.Data = append(openAIResponse.Data, dto.ImageData{B64Json: image.Data})
	}
	jsonResponse, err := common.Marshal(openAIResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	usage := buildUsageFromGeminiResponse(c, info, &geminiResponse)
	// Without usageMetadata the chat estimate only counts text; bill image
	// output with the same per-image estimate the stream path uses.
	patchGeminiZeroCompletionUsage(c, info, &usage, "", len(images))
	info.UpdateImageCount(int64(len(images)))

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(jsonResponse)
	return &usage, nil
}

func geminiImageMissingError(c *gin.Context, info *relaycommon.RelayInfo, response *dto.GeminiChatResponse) *types.NewAPIError {
	if response.PromptFeedback != nil && response.PromptFeedback.BlockReason != nil {
		reason := *response.PromptFeedback.BlockReason
		info.PerformanceBusinessRejection = true
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "gemini_block_reason="+reason)
		return types.NewErrorWithStatusCode(fmt.Errorf("image request blocked by Gemini: %s", reason), types.ErrorCodePromptBlocked, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	finishReason := ""
	for _, candidate := range response.Candidates {
		if candidate.FinishReason == nil {
			continue
		}
		finishReason = *candidate.FinishReason
		if geminiImageSafetyFinishReasons[finishReason] {
			info.PerformanceBusinessRejection = true
			common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "gemini_finish_reason="+finishReason)
			return types.NewErrorWithStatusCode(fmt.Errorf("image generation blocked by Gemini: finish_reason=%s", finishReason), types.ErrorCodePromptBlocked, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
	}

	text := []rune(strings.TrimSpace(geminiResponseUsageText(response)))
	if len(text) == 0 {
		// Nothing came back at all: treat as an upstream fault so another
		// channel may be tried.
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "gemini_image_empty_response")
		return types.NewOpenAIError(fmt.Errorf("Gemini returned no image (finish_reason=%s)", finishReason), types.ErrorCodeEmptyResponse, http.StatusInternalServerError)
	}
	// The model answered with text only, usually a refusal or a request for
	// clarification; retrying on another channel would not change that.
	if len(text) > 300 {
		text = append(text[:300], []rune("...")...)
	}
	common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "gemini_image_text_only")
	return types.NewErrorWithStatusCode(errors.New("Gemini returned text instead of an image: "+string(text)), types.ErrorCodeBadResponse, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

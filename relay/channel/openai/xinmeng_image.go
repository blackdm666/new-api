package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	maxXinMengResultImageBytes = 32 << 20
	xinMengResultImageTimeout  = 60 * time.Second
)

// Replaced in tests; production stores inputs and results as public
// reference media and fetches result links through the SSRF-protected client.
var (
	uploadXinMengReferenceImage = service.UploadReferenceImage
	downloadXinMengResultImage  = fetchXinMengResultImage
)

func isXinMengImageRequest(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	modelName := info.GetUpstreamModelName()
	return common.IsImageGenerationModel(modelName) && common.IsXinMengImageModel(modelName)
}

// convertXinMengImageRequest builds the JSON body XinMeng accepts. It reads
// reference images only by URL (its image field is capped at 2048 characters)
// and never parses multipart, so uploaded files and inline base64 images are
// stored as public reference media first. GPT Image fields are dropped because
// XinMeng rejects them with "property ... should not exist".
func convertXinMengImageRequest(c *gin.Context, request dto.ImageRequest) (dto.ImageRequest, error) {
	request.Style = nil
	request.User = nil
	request.ExtraFields = nil
	request.Background = nil
	request.Moderation = nil
	request.OutputFormat = nil
	request.OutputCompression = nil
	request.PartialImages = nil
	request.Stream = nil
	request.Mask = nil
	request.InputFidelity = nil
	request.Watermark = nil
	request.WatermarkEnabled = nil
	request.UserId = nil

	ctx := c.Request.Context()
	if strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		form := c.Request.MultipartForm
		if form == nil {
			parsed, err := common.ParseMultipartFormReusable(c)
			if err != nil {
				return request, fmt.Errorf("failed to parse multipart form: %w", err)
			}
			form = parsed
		}
		if values := form.Value["response_format"]; request.ResponseFormat == "" && len(values) > 0 {
			request.ResponseFormat = strings.TrimSpace(values[0])
		}
		var images []string
		for _, field := range []string{"image", "image[]"} {
			for _, value := range form.Value[field] {
				if value = strings.TrimSpace(value); value != "" {
					images = append(images, value)
				}
			}
		}
		fields := make([]string, 0, len(form.File))
		for field := range form.File {
			if field == "image" || strings.HasPrefix(field, "image[") {
				fields = append(fields, field)
			}
		}
		slices.Sort(fields)
		for _, field := range fields {
			for _, header := range form.File[field] {
				file, err := header.Open()
				if err != nil {
					return request, fmt.Errorf("failed to open image file: %w", err)
				}
				imageURL, err := uploadXinMengReferenceImage(ctx, file)
				_ = file.Close()
				if err != nil {
					return request, xinMengReferenceImageError(err)
				}
				images = append(images, imageURL)
			}
		}
		if len(images) == 0 {
			return request, types.NewErrorWithStatusCode(errors.New("image is required"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		var encoded []byte
		var err error
		if len(images) == 1 {
			encoded, err = common.Marshal(images[0])
		} else {
			encoded, err = common.Marshal(images)
		}
		if err != nil {
			return request, err
		}
		request.Image = encoded
	}

	var err error
	if request.Image, err = xinMengReferenceImageURLs(ctx, request.Image); err != nil {
		return request, err
	}
	if request.Images, err = xinMengReferenceImageURLs(ctx, request.Images); err != nil {
		return request, err
	}
	return request, nil
}

// xinMengReferenceImageURLs keeps the client's string or string-array shape
// and replaces each data URL or bare base64 image with a public URL.
func xinMengReferenceImageURLs(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	switch common.GetJsonType(raw) {
	case "string":
		var value string
		if err := common.Unmarshal(raw, &value); err != nil {
			return raw, err
		}
		imageURL, err := xinMengReferenceImageURL(ctx, value)
		if err != nil {
			return raw, err
		}
		return common.Marshal(imageURL)
	case "array":
		var values []string
		if err := common.Unmarshal(raw, &values); err != nil {
			// Other array shapes are provider-specific; leave them to XinMeng.
			return raw, nil
		}
		for i := range values {
			imageURL, err := xinMengReferenceImageURL(ctx, values[i])
			if err != nil {
				return raw, err
			}
			values[i] = imageURL
		}
		return common.Marshal(values)
	default:
		return raw, nil
	}
}

func xinMengReferenceImageURL(ctx context.Context, value string) (string, error) {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		return value, nil
	}
	payload := value
	if header, data, ok := strings.Cut(value, ","); ok && strings.HasPrefix(strings.ToLower(header), "data:") {
		payload = data
	}
	payload = strings.Join(strings.Fields(payload), "")
	content, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		content, err = base64.RawStdEncoding.DecodeString(payload)
	}
	if err != nil || len(content) == 0 {
		return "", types.NewErrorWithStatusCode(errors.New("image must be an HTTP(S) URL, a data URL or base64 image data"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	imageURL, err := uploadXinMengReferenceImage(ctx, bytes.NewReader(content))
	if err != nil {
		return "", xinMengReferenceImageError(err)
	}
	return imageURL, nil
}

func xinMengReferenceImageError(err error) error {
	if errors.Is(err, service.ErrUnsupportedReferenceImage) {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	return fmt.Errorf("failed to store reference image: %w", err)
}

type xinMengImageResponse struct {
	Created int64           `json:"created"`
	Data    []dto.ImageData `json:"data"`
	Usage   json.RawMessage `json:"usage,omitempty"`
}

// presentXinMengImageResponse rebuilds a XinMeng result as an OpenAI image
// response that names only our own media. Result links point at XinMeng's
// image host, so each linked image is stored as public reference media, or
// inlined when the client asked for b64_json. Entries keep their order and
// payload kind, so the billed image count is unchanged.
func presentXinMengImageResponse(ctx context.Context, info *relaycommon.RelayInfo, responseBody []byte) ([]byte, error) {
	wantBase64 := false
	if request, ok := info.Request.(*dto.ImageRequest); ok {
		wantBase64 = request.ResponseFormat == "b64_json"
	}
	items := gjson.GetBytes(responseBody, "data").Array()
	response := xinMengImageResponse{
		Created: gjson.GetBytes(responseBody, "created").Int(),
		Data:    make([]dto.ImageData, 0, len(items)),
	}
	if usage := gjson.GetBytes(responseBody, "usage"); usage.IsObject() {
		response.Usage = json.RawMessage(usage.Raw)
	}
	if response.Created == 0 {
		response.Created = time.Now().Unix()
	}
	for _, item := range items {
		image := dto.ImageData{
			B64Json:       item.Get("b64_json").String(),
			RevisedPrompt: item.Get("revised_prompt").String(),
		}
		sourceURL := strings.TrimSpace(item.Get("url").String())
		if image.B64Json == "" && sourceURL != "" {
			content, err := downloadXinMengResultImage(ctx, sourceURL)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch generated image: %w", err)
			}
			if wantBase64 {
				image.B64Json = base64.StdEncoding.EncodeToString(content)
			} else if image.Url, err = uploadXinMengReferenceImage(ctx, bytes.NewReader(content)); err != nil {
				return nil, fmt.Errorf("failed to store generated image: %w", err)
			}
		}
		response.Data = append(response.Data, image)
	}
	return common.Marshal(response)
}

func fetchXinMengResultImage(ctx context.Context, sourceURL string) ([]byte, error) {
	if err := service.ValidateSSRFProtectedFetchURL(sourceURL); err != nil {
		return nil, err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, xinMengResultImageTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := service.GetSSRFProtectedHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxXinMengResultImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxXinMengResultImageBytes {
		return nil, errors.New("generated image exceeds size limit")
	}
	return content, nil
}

package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	maxImageResultBytes = 32 << 20
	imageResultTimeout  = 60 * time.Second
)

// Replaced in tests; production stores results as public reference media and
// fetches result links through the SSRF-protected client.
var (
	uploadImageResult   = service.UploadReferenceImage
	downloadImageResult = fetchImageResult
)

// deliverImageResults keeps clients off upstream image hosts. Those hosts are
// often slow or unreachable from mainland China, expire within hours and name
// the provider, while the upstream has already generated and charged for the
// images. Every result link is therefore fetched here: it becomes inline
// b64_json, or our own reference media when the client asked for
// response_format=url. Inline entries stay as they are. A link entry that only
// repeats an inline image (the split shape openaiImageResponseCount bills
// once) is dropped instead of being inlined twice. The billed count is taken
// from the upstream body before this runs, so it does not change here.
func deliverImageResults(c *gin.Context, info *relaycommon.RelayInfo, responseBody []byte) ([]byte, *types.NewAPIError) {
	data := gjson.GetBytes(responseBody, "data")
	if !data.IsArray() && !data.IsObject() {
		return responseBody, nil
	}
	items := data.Array()
	var inlined, linked, linkOnly int
	for _, item := range items {
		hasInline, hasLink := openaiImageDataHasField(item, "b64_json"), openaiImageDataHasField(item, "url")
		if hasInline {
			inlined++
		}
		if hasLink {
			linked++
			if !hasInline {
				linkOnly++
			}
		}
	}
	if linked == 0 {
		return responseBody, nil
	}

	// Multipart edit forms are parsed field by field and do not fill
	// ResponseFormat, so read the form value as well.
	format := ""
	if request, ok := info.Request.(*dto.ImageRequest); ok {
		format = request.ResponseFormat
	}
	if format == "" && c.Request.MultipartForm != nil {
		if values := c.Request.MultipartForm.Value["response_format"]; len(values) > 0 {
			format = values[0]
		}
	}
	wantURL := strings.EqualFold(strings.TrimSpace(format), "url")
	dropRepeats := !wantURL && data.IsArray() && inlined >= linkOnly

	ctx := c.Request.Context()
	body := responseBody
	removed := 0
	for i, item := range items {
		if !openaiImageDataHasField(item, "url") {
			continue
		}
		path := "data"
		if data.IsArray() {
			// Entries dropped earlier shift the later ones down.
			path = fmt.Sprintf("data.%d", i-removed)
		}
		hasInline := openaiImageDataHasField(item, "b64_json")
		var err error
		switch {
		case !wantURL && hasInline:
			body, err = sjson.DeleteBytes(body, path+".url")
		case dropRepeats:
			body, err = sjson.DeleteBytes(body, path)
			removed++
		default:
			var content []byte
			content, err = downloadImageResult(ctx, item.Get("url").String())
			if err != nil {
				break
			}
			if wantURL {
				var link string
				if link, err = uploadImageResult(ctx, bytes.NewReader(content)); err == nil {
					body, err = sjson.SetBytes(body, path+".url", link)
				}
				break
			}
			if body, err = sjson.SetBytes(body, path+".b64_json", base64.StdEncoding.EncodeToString(content)); err == nil {
				body, err = sjson.DeleteBytes(body, path+".url")
			}
		}
		if err != nil {
			// The detail can name the upstream image host; keep it server-side.
			// The upstream already generated and charged for these images, so a
			// retry would only pay for another generation.
			logger.LogError(c, common.LogText("image result delivery failed: %s", err.Error()))
			return nil, types.NewOpenAIError(errors.New("failed to deliver generated images"), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
		}
	}
	return body, nil
}

// fetchImageResult reads one generated image from an upstream result link.
// Some providers put the image itself in the link as a base64 data URL.
func fetchImageResult(ctx context.Context, sourceURL string) ([]byte, error) {
	sourceURL = strings.TrimSpace(sourceURL)
	if header, payload, ok := strings.Cut(sourceURL, ","); ok && strings.HasPrefix(strings.ToLower(header), "data:") {
		if !strings.HasSuffix(strings.ToLower(header), ";base64") {
			return nil, errors.New("image data URL is not base64")
		}
		if base64.StdEncoding.DecodedLen(len(payload)) > maxImageResultBytes {
			return nil, errors.New("generated image exceeds size limit")
		}
		return base64.StdEncoding.DecodeString(strings.Join(strings.Fields(payload), ""))
	}
	if err := service.ValidateSSRFProtectedFetchURL(sourceURL); err != nil {
		return nil, err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, imageResultTimeout)
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
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxImageResultBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxImageResultBytes {
		return nil, errors.New("generated image exceeds size limit")
	}
	return content, nil
}

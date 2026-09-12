package gemini

import (
	"encoding/base64"
	"fmt"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

const maxVeoImageSize = 20 * 1024 * 1024 // 20 MB

// ExtractMultipartImage reads the first `input_reference` file from a multipart
// form upload and returns a VeoImageInput. Returns nil if no file is present.
func ExtractMultipartImage(c *gin.Context, info *relaycommon.RelayInfo) *VeoImageInput {
	mf, err := c.MultipartForm()
	if err != nil {
		return nil
	}
	files, exists := mf.File["input_reference"]
	if !exists || len(files) == 0 {
		return nil
	}
	fh := files[0]
	if fh.Size > maxVeoImageSize {
		return nil
	}
	file, err := fh.Open()
	if err != nil {
		return nil
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return nil
	}

	mimeType := fh.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(fileBytes)
	}

	info.Action = constant.TaskActionImageToVideo
	return &VeoImageInput{
		BytesBase64Encoded: base64.StdEncoding.EncodeToString(fileBytes),
		MimeType:           mimeType,
	}
}

// ParseImageInput parses an image string (data URI or raw base64) into a
// VeoImageInput. Returns nil if the input is empty or invalid.
// TODO: support downloading HTTP URL images and converting to base64
func ParseImageInput(imageStr string) *VeoImageInput {
	imageStr = strings.TrimSpace(imageStr)
	if imageStr == "" {
		return nil
	}

	if strings.HasPrefix(imageStr, "data:") {
		return parseDataURI(imageStr)
	}

	raw, err := base64.StdEncoding.DecodeString(imageStr)
	if err != nil {
		return nil
	}
	return &VeoImageInput{
		BytesBase64Encoded: imageStr,
		MimeType:           http.DetectContentType(raw),
	}
}

func parseDataURI(uri string) *VeoImageInput {
	// data:image/png;base64,iVBOR...
	rest := uri[len("data:"):]
	idx := strings.Index(rest, ",")
	if idx < 0 {
		return nil
	}
	meta := rest[:idx]
	b64 := rest[idx+1:]
	if b64 == "" {
		return nil
	}

	mimeType := "application/octet-stream"
	parts := strings.SplitN(meta, ";", 2)
	if len(parts) >= 1 && parts[0] != "" {
		mimeType = parts[0]
	}

	return &VeoImageInput{
		BytesBase64Encoded: b64,
		MimeType:           mimeType,
	}
}

// BuildVeoInstance preserves legacy first-image requests and explicitly maps new
// frame/reference requests for both Gemini and Vertex. Called before billing too.
func BuildVeoInstance(c *gin.Context, info *relaycommon.RelayInfo) (VeoInstance, error) {
	instance := VeoInstance{}
	value, _ := c.Get("task_request")
	req, ok := value.(relaycommon.TaskSubmitReq)
	if !ok {
		return instance, fmt.Errorf("request not found in context")
	}
	instance.Prompt = req.Prompt
	modeValue, explicit := req.Metadata["video_mode"]
	mode, valid := modeValue.(string)
	if explicit && (!valid || (mode != "frames" && mode != "reference")) {
		return instance, fmt.Errorf("video_mode must be frames or reference")
	}
	if !explicit {
		if img := ExtractMultipartImage(c, info); img != nil {
			instance.Image = img
		} else if len(req.Images) > 0 {
			instance.Image = ParseImageInput(req.Images[0])
		}
		if instance.Image != nil {
			info.Action = constant.TaskActionImageToVideo
		}
		return instance, nil
	}
	maxImages := 2
	if mode == "reference" {
		maxImages = 3
	}
	if len(req.Images) > maxImages {
		return instance, fmt.Errorf("Veo %s mode accepts at most %d images", mode, maxImages)
	}
	if mode == "reference" || len(req.Images) == 2 {
		model := strings.NewReplacer(".", "-", "_", "-").Replace(strings.ToLower(info.UpstreamModelName))
		if !strings.HasPrefix(model, "veo-3-1") || strings.Contains(model, "lite") {
			return instance, fmt.Errorf("reference images and last frame require Veo 3.1")
		}
	}
	if mode == "reference" && len(req.Images) > 0 {
		params := &VeoParameters{}
		if err := taskcommon.UnmarshalMetadata(req.Metadata, params); err != nil {
			return instance, err
		}
		duration := params.DurationSeconds
		if duration == 0 {
			duration = req.Duration
		}
		if duration != 8 {
			return instance, fmt.Errorf("Veo reference images require duration 8")
		}
	}
	for index, source := range req.Images {
		// Bound encoded input before decoding it, then validate the decoded size.
		if len(source) > base64.StdEncoding.EncodedLen(maxVeoImageSize)+128 {
			return instance, fmt.Errorf("Veo image exceeds 20 MB")
		}
		img := ParseImageInput(source)
		if img == nil {
			return instance, fmt.Errorf("Veo images must be base64 PNG or JPEG")
		}
		raw, err := base64.StdEncoding.DecodeString(img.BytesBase64Encoded)
		if err != nil || len(raw) == 0 {
			return instance, fmt.Errorf("invalid Veo image base64")
		}
		if len(raw) > maxVeoImageSize {
			return instance, fmt.Errorf("Veo image exceeds 20 MB")
		}
		detected := http.DetectContentType(raw)
		if detected != "image/png" && detected != "image/jpeg" {
			return instance, fmt.Errorf("Veo images must be PNG or JPEG")
		}
		img.MimeType = detected
		if mode == "reference" {
			instance.ReferenceImages = append(instance.ReferenceImages, VeoReferenceImage{Image: img, ReferenceType: "asset"})
		} else if index == 0 {
			instance.Image = img
		} else {
			instance.LastFrame = img
		}
	}
	// Multipart legacy callers can still explicitly choose first-frame mode.
	if len(req.Images) == 0 {
		if img := ExtractMultipartImage(c, info); img != nil {
			if mode == "reference" {
				return instance, fmt.Errorf("reference mode requires JSON images")
			}
			instance.Image = img
		}
	}
	if instance.Image != nil || len(instance.ReferenceImages) > 0 {
		info.Action = constant.TaskActionImageToVideo
	}
	return instance, nil
}

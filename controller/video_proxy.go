package controller

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	vertexcore "github.com/QuantumNous/new-api/relay/channel/vertex"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

// videoProxyError returns a standardized OpenAI-style error response.
func videoProxyError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errType,
		},
	})
}

// OpenTaskVideoSource resolves a provider result and applies any channel
// credentials needed to read it. The service layer injects this function for
// one-time R2 caching, while VideoProxy uses the same path for compatibility.
func OpenTaskVideoSource(ctx context.Context, task *model.Task, resultURL string) (*service.TaskVideoSource, error) {
	if task == nil {
		return nil, errors.New("task is required")
	}
	channel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}
	baseURL := channel.GetBaseURL()
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[channel.Type]
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	videoURL := strings.TrimSpace(resultURL)
	if isTaskProxyContentURL(videoURL, task.TaskID) {
		videoURL = ""
	}
	proxy := channel.GetSetting().Proxy
	client := service.GetSSRFProtectedHTTPClient()
	if proxy != "" {
		client, err = service.GetHttpClientWithProxy(proxy)
		if err != nil {
			return nil, fmt.Errorf("create channel proxy client: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "", nil)
	if err != nil {
		return nil, fmt.Errorf("create video source request: %w", err)
	}
	req.Header.Set("Accept", "video/*, application/octet-stream;q=0.9")

	switch channel.Type {
	case constant.ChannelTypeGemini:
		apiKey := strings.TrimSpace(task.PrivateData.Key)
		if apiKey == "" {
			return nil, errors.New("Gemini API key is not available for task")
		}
		if videoURL == "" {
			videoURL, err = getGeminiVideoURL(channel, task, apiKey)
			if err != nil {
				return nil, fmt.Errorf("resolve Gemini video URL: %w", err)
			}
		} else {
			videoURL = ensureGeminiVideoAccess(videoURL, apiKey)
		}
		req.Header.Set("x-goog-api-key", apiKey)
	case constant.ChannelTypeVertexAi:
		if videoURL == "" || (!strings.HasPrefix(videoURL, "http://") && !strings.HasPrefix(videoURL, "https://")) {
			videoURL, err = getVertexVideoURL(channel, task)
			if err != nil {
				return nil, fmt.Errorf("resolve Vertex video URL: %w", err)
			}
		}
		if strings.HasPrefix(videoURL, "http://") || strings.HasPrefix(videoURL, "https://") {
			credentials := &vertexcore.Credentials{}
			if err := common.Unmarshal([]byte(getVertexTaskKey(channel, task)), credentials); err != nil {
				return nil, fmt.Errorf("decode Vertex credentials: %w", err)
			}
			accessToken, err := vertexcore.AcquireAccessToken(*credentials, proxy)
			if err != nil {
				return nil, fmt.Errorf("authenticate Vertex video download: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+accessToken)
			req.Header.Set("x-goog-user-project", credentials.ProjectID)
		}
	case constant.ChannelTypeOpenAI, constant.ChannelTypeSora:
		if videoURL == "" {
			videoURL = fmt.Sprintf("%s/v1/videos/%s/content", strings.TrimRight(baseURL, "/"), task.GetUpstreamTaskID())
		}
		req.Header.Set("Authorization", "Bearer "+channel.Key)
	case constant.ChannelTypeSub2API:
		if videoURL == "" {
			videoURL = fmt.Sprintf("%s/v1/videos/%s/content", strings.TrimRight(baseURL, "/"), task.GetUpstreamTaskID())
		}
		key := strings.TrimSpace(task.PrivateData.Key)
		if key == "" {
			key = channel.Key
		}
		req.Header.Set("Authorization", "Bearer "+key)
	default:
		if videoURL == "" {
			videoURL = strings.TrimSpace(task.GetResultURL())
		}
		resultTarget, resultErr := url.Parse(videoURL)
		channelTarget, channelErr := url.Parse(baseURL)
		if strings.HasPrefix(videoURL, "/") || (resultErr == nil && channelErr == nil && strings.EqualFold(resultTarget.Host, channelTarget.Host)) {
			key := strings.TrimSpace(task.PrivateData.Key)
			if key == "" {
				key = strings.TrimSpace(channel.Key)
			}
			if key != "" {
				req.Header.Set("Authorization", "Bearer "+key)
			}
		}
	}

	videoURL = strings.TrimSpace(videoURL)
	if strings.HasPrefix(videoURL, "/") {
		videoURL = strings.TrimRight(baseURL, "/") + videoURL
	}
	if videoURL == "" {
		return nil, errors.New("video result URL is empty")
	}
	if strings.HasPrefix(videoURL, "data:") {
		return nil, errors.New("data video results must be decoded before opening a provider source")
	}

	if proxy == "" {
		err = service.ValidateSSRFProtectedFetchURL(videoURL)
	} else {
		fetchSetting := system_setting.GetFetchSetting()
		err = common.ValidateURLWithFetchSetting(videoURL, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain)
	}
	if err != nil {
		return nil, fmt.Errorf("video source blocked: %w", err)
	}
	req.URL, err = url.Parse(videoURL)
	if err != nil {
		return nil, fmt.Errorf("parse video source URL: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch video source from %s failed", req.URL.Hostname())
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("upstream video source returned status %d", resp.StatusCode)
	}
	return &service.TaskVideoSource{
		Body:          resp.Body,
		ContentLength: resp.ContentLength,
		ContentType:   resp.Header.Get("Content-Type"),
		Header:        resp.Header.Clone(),
		StatusCode:    resp.StatusCode,
	}, nil
}

func VideoProxy(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "task_id is required")
		return
	}

	userID := c.GetInt("id")
	task, exists, err := model.GetByTaskId(userID, taskID)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to query task %s: %s", taskID, err.Error()))
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to query task")
		return
	}
	if !exists || task == nil {
		videoProxyError(c, http.StatusNotFound, "invalid_request_error", "Task not found")
		return
	}

	if task.Status != model.TaskStatusSuccess {
		videoProxyError(c, http.StatusBadRequest, "invalid_request_error",
			fmt.Sprintf("Task is not completed yet, current status: %s", task.Status))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	if reader, mimeType, cached, cacheErr := service.OpenTaskVideoCache(ctx, task); cached && cacheErr == nil {
		defer reader.Close()
		c.Writer.Header().Set("Content-Type", mimeType)
		c.Writer.Header().Set("Content-Disposition", "inline")
		c.Writer.Header().Set("Cache-Control", "private, max-age=86400")
		c.Writer.Header().Set("X-Video-Cache", "HIT")
		c.Writer.WriteHeader(http.StatusOK)
		if _, copyErr := io.Copy(c.Writer, reader); copyErr != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to stream cached video for task %s: %s", taskID, copyErr.Error()))
		}
		return
	} else if cached && cacheErr != nil {
		// A transient object-storage problem should not make a completed task
		// unreadable. Fall back to the existing provider proxy path.
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to read cached video for task %s, falling back upstream: %s", taskID, cacheErr.Error()))
	}

	resultURL := service.ResolveTaskVideoResultURL(task, task.GetResultURL())
	if strings.HasPrefix(resultURL, "data:") {
		if err := writeVideoDataURL(c, resultURL); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to decode video data URL for task %s: %s", taskID, err.Error()))
			videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch video content")
		}
		return
	}
	source, err := OpenTaskVideoSource(ctx, task, resultURL)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to open video source for task %s: %s", taskID, err.Error()))
		videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch video content")
		return
	}
	defer source.Body.Close()
	copyVideoResponseHeaders(c.Writer.Header(), source.Header)

	c.Writer.Header().Set("Cache-Control", "public, max-age=86400")
	c.Writer.WriteHeader(source.StatusCode)
	if _, err = io.Copy(c.Writer, source.Body); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to stream video content: %s", err.Error()))
	}
}

func copyVideoResponseHeaders(dst, src http.Header) {
	for _, key := range []string{"Content-Type", "Content-Length", "Content-Disposition", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		for _, value := range src.Values(key) {
			dst.Add(key, value)
		}
	}
}

func writeVideoDataURL(c *gin.Context, dataURL string) error {
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid data url")
	}

	header := parts[0]
	payload := parts[1]
	if !strings.HasPrefix(header, "data:") || !strings.Contains(header, ";base64") {
		return fmt.Errorf("unsupported data url")
	}

	mimeType := strings.TrimPrefix(header, "data:")
	mimeType = strings.TrimSuffix(mimeType, ";base64")
	if mimeType == "" {
		mimeType = "video/mp4"
	}

	videoBytes, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		videoBytes, err = base64.RawStdEncoding.DecodeString(payload)
		if err != nil {
			return err
		}
	}

	c.Writer.Header().Set("Content-Type", mimeType)
	c.Writer.Header().Set("Cache-Control", "public, max-age=86400")
	c.Writer.WriteHeader(http.StatusOK)
	_, err = c.Writer.Write(videoBytes)
	return err
}

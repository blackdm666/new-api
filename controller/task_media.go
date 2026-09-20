package controller

import (
	"net/http"
	"regexp"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

var publicMediaTaskIDPattern = regexp.MustCompile(`^task_[A-Za-z0-9_-]{8,186}$`)

// PublicVideoContent only redirects persisted video objects. It never invokes
// providers, renders task data, backfills storage, or revives an expired object.
func PublicVideoContent(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	taskID := c.Param("task_id")
	if !service.TaskMediaPublicEnabled() || !publicMediaTaskIDPattern.MatchString(taskID) {
		videoProxyError(c, http.StatusNotFound, "media_not_found", "Media not found")
		return
	}
	task, exists, err := model.GetUniqueByOnlyTaskId(taskID)
	if err != nil {
		videoProxyError(c, http.StatusServiceUnavailable, "media_unavailable", "Media temporarily unavailable")
		return
	}
	if !exists || task == nil {
		videoProxyError(c, http.StatusNotFound, "media_not_found", "Media not found")
		return
	}
	publicURL, err := service.PublicTaskVideoURL(task)
	if err != nil {
		videoProxyError(c, http.StatusNotFound, "media_not_found", "Media not found")
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, publicURL)
}

func CreateMediaUpload(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !service.TaskMediaPublicEnabled() {
		videoProxyError(c, http.StatusServiceUnavailable, "media_unavailable", "Media upload is not enabled")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var request service.MediaUploadRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		videoProxyError(c, http.StatusBadRequest, "invalid_media", "Invalid media upload request")
		return
	}
	receipt, err := service.IssueMediaUpload(request)
	if err != nil {
		videoProxyError(c, http.StatusBadRequest, "invalid_media", err.Error())
		return
	}
	c.JSON(http.StatusOK, receipt)
}

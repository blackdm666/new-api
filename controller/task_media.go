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

// PublicVideoContent redirects a persisted video object, or for unarchived
// results the allow-listed official upstream URL the task query also presents.
// It never invokes providers, proxies bytes, backfills storage, or revives an
// expired object; other unarchived results stay hidden.
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
		directURL, direct := "", false
		if task.ResultRetrievable() {
			directURL, direct = service.TaskVideoDirectContentURL(task)
		}
		if !direct {
			videoProxyError(c, http.StatusNotFound, "media_not_found", "Media not found")
			return
		}
		publicURL = directURL
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

package controller

import (
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// SubmitPartnerApplication accepts the official site's partner application
// form and emails it to the referral application recipients. Like the
// referral application nothing is stored apart from the email outbox row,
// whose subject and body are cleared after delivery; unlike it, the applicant
// gets no confirmation email.
func SubmitPartnerApplication(c *gin.Context) {
	payload := map[string]any{}
	if err := common.DecodeJson(c.Request.Body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid parameters"})
		return
	}
	// A filled honeypot is a bot. Answer like a success so it learns nothing.
	if service.IsReferralApplicationSpam(payload) {
		common.ApiSuccess(c, gin.H{"reference": ""})
		return
	}
	sourcePage, _ := payload["source_page"].(string)
	locale, _ := payload["locale"].(string)
	application, err := service.ValidatePartnerApplication(payload, service.ReferralApplicationContext{
		SourcePage: sourcePage,
		Referer:    c.Request.Referer(),
		Locale:     locale,
		ClientIP:   c.ClientIP(),
		Now:        time.Now(),
	})
	var fieldErr *service.ReferralApplicationFieldError
	if errors.As(err, &fieldErr) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Please check the application form",
			"data":    gin.H{"field": fieldErr.Field, "code": fieldErr.Code},
		})
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := service.QueuePartnerApplicationEmail(application); err != nil {
		common.SysError("failed to queue partner application " + application.Reference + ": " + err.Error())
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "The application could not be sent, please contact online support"})
		return
	}
	common.ApiSuccess(c, gin.H{"reference": application.Reference})
}

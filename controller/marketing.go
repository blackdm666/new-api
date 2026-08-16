package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type marketingCampaignPayload struct {
	Name             string                                     `json:"name"`
	AudienceRule     model.MarketingAudienceRule                `json:"audience_rule"`
	LocalizedContent map[string]model.MarketingLocalizedContent `json:"localized_content"`
	ScheduledTime    int64                                      `json:"scheduled_time"`
}

type marketingSchedulePayload struct {
	ScheduledTime int64 `json:"scheduled_time"`
}

type marketingTestPayload struct {
	LocalizedContent map[string]model.MarketingLocalizedContent `json:"localized_content"`
	Language         string                                     `json:"language"`
}

type marketingAutomationPayload struct {
	Enabled          bool                                       `json:"enabled"`
	ApplyExisting    bool                                       `json:"apply_existing"`
	LocalizedContent map[string]model.MarketingLocalizedContent `json:"localized_content"`
}

type marketingSuppressionPayload struct {
	UserId int    `json:"user_id"`
	Email  string `json:"email"`
	Reason string `json:"reason"`
}

func MarketingOverview(c *gin.Context) {
	overview, err := model.GetMarketingOverview()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, overview)
}

func ListMarketingCampaigns(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	rows, total, err := model.ListMarketingCampaigns(pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(rows)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func CreateMarketingCampaign(c *gin.Context) {
	payload := marketingCampaignPayload{}
	if err := c.ShouldBindJSON(&payload); err != nil || !validMarketingCampaignPayload(payload) {
		common.ApiError(c, model.ErrMarketingInvalid)
		return
	}
	rule, err := common.Marshal(payload.AudienceRule)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	content, err := common.Marshal(payload.LocalizedContent)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	campaign := &model.MarketingCampaign{Name: strings.TrimSpace(payload.Name), Scene: model.MarketingSceneCustom, Status: model.MarketingCampaignStatusDraft, AudienceRule: string(rule), LocalizedContent: string(content), ActionPath: "/wallet", CreatedBy: c.GetInt("id"), ScheduledTime: payload.ScheduledTime}
	if err := model.CreateMarketingCampaign(campaign); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, campaign)
}

func UpdateMarketingCampaign(c *gin.Context) {
	id, ok := marketingID(c)
	if !ok {
		return
	}
	payload := marketingCampaignPayload{}
	if err := c.ShouldBindJSON(&payload); err != nil || !validMarketingCampaignPayload(payload) {
		common.ApiError(c, model.ErrMarketingInvalid)
		return
	}
	rule, _ := common.Marshal(payload.AudienceRule)
	content, _ := common.Marshal(payload.LocalizedContent)
	campaign := &model.MarketingCampaign{Id: id, Name: strings.TrimSpace(payload.Name), AudienceRule: string(rule), LocalizedContent: string(content), ActionPath: "/wallet", ScheduledTime: payload.ScheduledTime}
	if err := model.UpdateMarketingCampaign(campaign); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"updated": true})
}

func PreviewMarketingCampaignAudience(c *gin.Context) {
	payload := marketingCampaignPayload{}
	if err := c.ShouldBindJSON(&payload); err != nil || !validMarketingAudienceRule(payload.AudienceRule) {
		common.ApiError(c, model.ErrMarketingInvalid)
		return
	}
	total, err := service.PreviewMarketingAudience(payload.AudienceRule)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"total": total})
}

func ScheduleMarketingCampaign(c *gin.Context) {
	id, ok := marketingID(c)
	if !ok {
		return
	}
	payload := marketingSchedulePayload{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiError(c, model.ErrMarketingInvalid)
		return
	}
	if err := model.ScheduleMarketingCampaign(id, payload.ScheduledTime); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"scheduled": true})
}

func PauseMarketingCampaign(c *gin.Context) {
	marketingCampaignTransition(c, []string{model.MarketingCampaignStatusRunning, model.MarketingCampaignStatusScheduled}, model.MarketingCampaignStatusPaused)
}

func ResumeMarketingCampaign(c *gin.Context) {
	id, ok := marketingID(c)
	if !ok {
		return
	}
	campaign, err := model.GetMarketingCampaign(id)
	if err != nil || campaign.Status != model.MarketingCampaignStatusPaused {
		common.ApiError(c, model.ErrMarketingInvalid)
		return
	}
	status := model.MarketingCampaignStatusRunning
	if !campaign.Automatic && campaign.StartedTime == 0 && campaign.ScheduledTime > common.GetTimestamp() {
		status = model.MarketingCampaignStatusScheduled
	}
	if err := model.SetMarketingCampaignStatus(id, []string{model.MarketingCampaignStatusPaused}, status, ""); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"status": status})
}

func CancelMarketingCampaign(c *gin.Context) {
	marketingCampaignTransition(c, []string{model.MarketingCampaignStatusDraft, model.MarketingCampaignStatusScheduled, model.MarketingCampaignStatusRunning, model.MarketingCampaignStatusPaused}, model.MarketingCampaignStatusCancelled)
}

func CloneMarketingCampaign(c *gin.Context) {
	id, ok := marketingID(c)
	if !ok {
		return
	}
	clone, err := model.CloneMarketingCampaign(id, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, clone)
}

func TestMarketingEmail(c *gin.Context) {
	payload := marketingTestPayload{}
	if err := c.ShouldBindJSON(&payload); err != nil || !validMarketingContent(payload.LocalizedContent) {
		common.ApiError(c, model.ErrMarketingInvalid)
		return
	}
	content, _ := common.Marshal(payload.LocalizedContent)
	root, err := model.GetUserById(c.GetInt("id"), true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := service.SendMarketingTestEmail(root, string(content), payload.Language); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"queued": true})
}

func ListMarketingAutomations(c *gin.Context) {
	if err := model.EnsureMarketingAutomations(); err != nil {
		common.ApiError(c, err)
		return
	}
	rows, err := model.ListMarketingAutomations()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func UpdateMarketingAutomation(c *gin.Context) {
	scene := strings.TrimSpace(c.Param("scene"))
	payload := marketingAutomationPayload{}
	if err := c.ShouldBindJSON(&payload); err != nil || !validMarketingContent(payload.LocalizedContent) {
		common.ApiError(c, model.ErrMarketingInvalid)
		return
	}
	content, _ := common.Marshal(payload.LocalizedContent)
	if err := model.UpdateMarketingAutomation(scene, payload.Enabled, payload.ApplyExisting, string(content)); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"updated": true})
}

func PreviewMarketingAutomation(c *gin.Context) {
	total, err := service.PreviewMarketingAutomation(strings.TrimSpace(c.Param("scene")))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"total": total})
}

func ListMarketingRecipients(c *gin.Context) {
	id, ok := marketingID(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	rows, total, err := model.ListMarketingRecipients(id, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(rows)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func ListMarketingSuppressions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	rows, total, err := model.ListMarketingSuppressions(pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(rows)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func CreateMarketingSuppression(c *gin.Context) {
	payload := marketingSuppressionPayload{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiError(c, model.ErrMarketingInvalid)
		return
	}
	if err := model.CreateMarketingSuppression(payload.UserId, payload.Email, payload.Reason, c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"created": true})
}

func DeleteMarketingSuppression(c *gin.Context) {
	id, ok := marketingID(c)
	if !ok {
		return
	}
	if err := model.DeleteMarketingSuppression(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"deleted": true})
}

func MarketingClick(c *gin.Context) {
	target, err := service.MarketingClickTarget(strings.TrimSpace(c.Param("token")))
	if err != nil {
		c.Redirect(http.StatusFound, "/dashboard/overview")
		return
	}
	c.Redirect(http.StatusFound, target)
}

func marketingCampaignTransition(c *gin.Context, from []string, status string) {
	id, ok := marketingID(c)
	if !ok {
		return
	}
	if err := model.SetMarketingCampaignStatus(id, from, status, ""); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"status": status})
}

func marketingID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrMarketingInvalid)
		return 0, false
	}
	return id, true
}

func validMarketingCampaignPayload(payload marketingCampaignPayload) bool {
	name := strings.TrimSpace(payload.Name)
	return name != "" && len([]rune(name)) <= 160 && validMarketingAudienceRule(payload.AudienceRule) && validMarketingContent(payload.LocalizedContent)
}

func validMarketingAudienceRule(rule model.MarketingAudienceRule) bool {
	if rule.InactiveDays < 0 || rule.InactiveDays > 36500 || rule.LastTopUpBefore < 0 {
		return false
	}
	for _, value := range []*int{rule.TopUpCountMin, rule.TopUpCountMax, rule.QuotaMin, rule.QuotaMax} {
		if value != nil && *value < 0 {
			return false
		}
	}
	if rule.TopUpCountMin != nil && rule.TopUpCountMax != nil && *rule.TopUpCountMin > *rule.TopUpCountMax {
		return false
	}
	if rule.QuotaMin != nil && rule.QuotaMax != nil && *rule.QuotaMin > *rule.QuotaMax {
		return false
	}
	if len(rule.Groups) > 50 {
		return false
	}
	for _, group := range rule.Groups {
		if strings.TrimSpace(group) == "" || len([]rune(group)) > 64 {
			return false
		}
	}
	return true
}

func validMarketingContent(contents map[string]model.MarketingLocalizedContent) bool {
	if len(contents) == 0 {
		return false
	}
	for language, content := range contents {
		if strings.TrimSpace(language) == "" || strings.TrimSpace(content.Subject) == "" || strings.TrimSpace(content.Body) == "" || len([]rune(content.Subject)) > 120 || len([]rune(content.Body)) > 5000 {
			return false
		}
	}
	_, ok := contents["zh-CN"]
	return ok
}

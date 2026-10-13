package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// 88api cyberguard: root-only admin API behind /api/cyber-guard.

const cyberGuardPollNowTimeout = 2 * time.Minute

type cyberGuardReasonRequest struct {
	Reason string `json:"reason"`
}

type cyberGuardBanRequest struct {
	UserId int    `json:"user_id"`
	Reason string `json:"reason"`
	Hours  int    `json:"hours"`
}

type cyberGuardAdoptRequest struct {
	UserId        int    `json:"user_id"`
	OriginalGroup string `json:"original_group"`
	Reason        string `json:"reason"`
	Hours         int    `json:"hours"`
}

func cyberGuardPathId(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, service.ErrCyberGuardInvalidInput)
		return 0, false
	}
	return id, true
}

// decodeCyberGuardRequest decodes an optional JSON body; an empty body keeps
// the zero value.
func decodeCyberGuardRequest(c *gin.Context, target any) bool {
	if c.Request.ContentLength == 0 {
		return true
	}
	if err := common.DecodeJson(c.Request.Body, target); err != nil {
		common.ApiError(c, service.ErrCyberGuardInvalidInput)
		return false
	}
	return true
}

func cyberGuardReason(c *gin.Context, required bool) (string, bool) {
	var req cyberGuardReasonRequest
	if !decodeCyberGuardRequest(c, &req) {
		return "", false
	}
	reason := strings.TrimSpace(req.Reason)
	if (required && reason == "") || len([]rune(reason)) > 255 {
		common.ApiErrorMsg(c, "reason is required (at most 255 characters)")
		return "", false
	}
	return reason, true
}

func recordCyberGuardAudit(c *gin.Context, targetUserId int, action string, params map[string]any, content string) {
	if params == nil {
		params = map[string]any{}
	}
	if targetUserId > 0 {
		params["target_user_id"] = targetUserId
	}
	model.RecordOperationAuditLog(c.GetInt("id"), c.GetInt("role"), content, c.ClientIP(), action, params, auditOperatorInfo(c), nil, c)
	markAuditLogged(c)
}

// cyberGuardPageQuery reads p/page_size and clamps them to page >= 1 and
// 1 <= page_size <= 100 (GetPageQuery keeps negative values).
func cyberGuardPageQuery(c *gin.Context) *common.PageInfo {
	pageInfo := common.GetPageQuery(c)
	pageInfo.Page = max(pageInfo.Page, 1)
	pageInfo.PageSize = min(max(pageInfo.PageSize, 1), 100)
	return pageInfo
}

func GetCyberGuardOverview(c *gin.Context) {
	overview, err := service.GetCyberGuardOverview()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, overview)
}

func ListCyberGuardUsers(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	switch status {
	case "", "all", "active", "observed", "clean":
	default:
		common.ApiError(c, service.ErrCyberGuardInvalidInput)
		return
	}
	pageInfo := cyberGuardPageQuery(c)
	rows, total, err := service.ListCyberGuardUsers(c.Query("keyword"), status, pageInfo.GetPage(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(rows)
	pageInfo.SetTotal(total)
	common.ApiSuccess(c, pageInfo)
}

func ListCyberGuardEvents(c *gin.Context) {
	pageInfo := cyberGuardPageQuery(c)
	filter := model.CyberGuardEventFilter{
		Category: strings.TrimSpace(c.Query("category")),
		State:    strings.TrimSpace(c.Query("state")),
		Keyword:  strings.TrimSpace(c.Query("keyword")),
	}
	filter.UserId, _ = strconv.Atoi(c.Query("user_id"))
	filter.Start, _ = strconv.ParseInt(c.Query("start"), 10, 64)
	filter.End, _ = strconv.ParseInt(c.Query("end"), 10, 64)
	events, total, err := model.ListCyberGuardEvents(filter, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(events)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func VoidCyberGuardEvent(c *gin.Context) {
	id, ok := cyberGuardPathId(c)
	if !ok {
		return
	}
	reason, ok := cyberGuardReason(c, true)
	if !ok {
		return
	}
	event, err := service.CyberGuardVoidEvent(c.GetInt("id"), id, reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordCyberGuardAudit(c, event.UserId, "cyber_guard.void_event",
		map[string]any{"event_id": event.Id, "ops_id": event.OpsId, "category": event.Category, "reason": reason},
		fmt.Sprintf("CyberGuard voided event %d", event.Id))
	common.ApiSuccess(c, event)
}

func ListCyberGuardBans(c *gin.Context) {
	pageInfo := cyberGuardPageQuery(c)
	userId, _ := strconv.Atoi(c.Query("user_id"))
	rows, total, err := service.ListCyberGuardBanRows(strings.TrimSpace(c.Query("status")), userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(rows)
	pageInfo.SetTotal(int(total))
	common.ApiSuccess(c, pageInfo)
}

func CreateCyberGuardBan(c *gin.Context) {
	var req cyberGuardBanRequest
	if !decodeCyberGuardRequest(c, &req) {
		return
	}
	if req.UserId <= 0 || len([]rune(strings.TrimSpace(req.Reason))) > 500 {
		common.ApiError(c, service.ErrCyberGuardInvalidInput)
		return
	}
	ban, err := service.CyberGuardManualBan(c.GetInt("id"), req.UserId, req.Reason, req.Hours)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordCyberGuardAudit(c, ban.UserId, "cyber_guard.ban", service.CyberGuardBanAuditParams(ban),
		fmt.Sprintf("CyberGuard banned user %d (%s)", ban.UserId, ban.CaseId))
	common.ApiSuccess(c, ban)
}

func UnbanCyberGuard(c *gin.Context) {
	id, ok := cyberGuardPathId(c)
	if !ok {
		return
	}
	reason, ok := cyberGuardReason(c, false)
	if !ok {
		return
	}
	outcome, err := service.CyberGuardUnban(c.GetInt("id"), id, reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordCyberGuardAudit(c, outcome.Ban.UserId, "cyber_guard.unban", map[string]any{
		"case_id": outcome.Ban.CaseId, "end_reason": outcome.Ban.EndReason, "from_group": outcome.FromGroup,
		"to_group": outcome.ToGroup, "group_changed": outcome.GroupChanged, "reason": reason,
	}, fmt.Sprintf("CyberGuard ended %s (%s)", outcome.Ban.CaseId, outcome.Ban.EndReason))
	common.ApiSuccess(c, gin.H{"ban": outcome.Ban, "from_group": outcome.FromGroup, "to_group": outcome.ToGroup, "group_changed": outcome.GroupChanged})
}

func PromoteCyberGuardBan(c *gin.Context) {
	id, ok := cyberGuardPathId(c)
	if !ok {
		return
	}
	ban, err := service.CyberGuardPromote(c.GetInt("id"), id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	params := service.CyberGuardBanAuditParams(ban)
	params["proposal_case_id"] = model.CyberGuardCaseId(id)
	recordCyberGuardAudit(c, ban.UserId, "cyber_guard.promote", params,
		fmt.Sprintf("CyberGuard promoted %s to ban %s", model.CyberGuardCaseId(id), ban.CaseId))
	common.ApiSuccess(c, ban)
}

func DismissCyberGuardBan(c *gin.Context) {
	id, ok := cyberGuardPathId(c)
	if !ok {
		return
	}
	reason, ok := cyberGuardReason(c, false)
	if !ok {
		return
	}
	ban, err := service.CyberGuardDismiss(c.GetInt("id"), id, reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordCyberGuardAudit(c, ban.UserId, "cyber_guard.dismiss", map[string]any{"case_id": ban.CaseId, "reason": reason},
		fmt.Sprintf("CyberGuard dismissed proposal %s", ban.CaseId))
	common.ApiSuccess(c, ban)
}

func ResendCyberGuardBanEmail(c *gin.Context) {
	id, ok := cyberGuardPathId(c)
	if !ok {
		return
	}
	ban, err := service.CyberGuardResendEmail(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordCyberGuardAudit(c, ban.UserId, "cyber_guard.resend_email",
		map[string]any{"case_id": ban.CaseId, "email_state": ban.EmailState, "delivery_key": ban.EmailDeliveryKey},
		fmt.Sprintf("CyberGuard resent the ban email of %s", ban.CaseId))
	common.ApiSuccess(c, ban)
}

func ListCyberGuardAdoptable(c *gin.Context) {
	rows, err := service.ListCyberGuardAdoptable()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func AdoptCyberGuardBan(c *gin.Context) {
	var req cyberGuardAdoptRequest
	if !decodeCyberGuardRequest(c, &req) {
		return
	}
	if req.UserId <= 0 || len([]rune(strings.TrimSpace(req.Reason))) > 500 {
		common.ApiError(c, service.ErrCyberGuardInvalidInput)
		return
	}
	ban, err := service.CyberGuardAdopt(c.GetInt("id"), req.UserId, req.OriginalGroup, req.Reason, req.Hours)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordCyberGuardAudit(c, ban.UserId, "cyber_guard.adopt", service.CyberGuardBanAuditParams(ban),
		fmt.Sprintf("CyberGuard adopted the existing ban of user %d (%s)", ban.UserId, ban.CaseId))
	common.ApiSuccess(c, ban)
}

func ListCyberGuardAccounts(c *gin.Context) {
	days := 7
	if c.Query("days") == "30" {
		days = 30
	}
	rows, err := service.ListCyberGuardAccounts(c.Request.Context(), days)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func GetCyberGuardSettings(c *gin.Context) {
	common.ApiSuccess(c, service.GetCyberGuardSettingView())
}

func UpdateCyberGuardSettings(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		common.ApiError(c, service.ErrCyberGuardInvalidInput)
		return
	}
	input, err := service.MergeCyberGuardSettingUpdate(body)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	changed, err := service.UpdateCyberGuardSettings(input)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	view := service.GetCyberGuardSettingView()
	recordCyberGuardAudit(c, 0, "cyber_guard.settings_update", map[string]any{"changed": changed, "mode": view.Mode},
		fmt.Sprintf("CyberGuard settings updated (%s)", strings.Join(changed, ", ")))
	common.ApiSuccess(c, view)
}

func PollCyberGuardNow(c *gin.Context) {
	if !common.IsMasterNode {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "CyberGuard polls run on the master node only"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cyberGuardPollNowTimeout)
	defer cancel()
	result, err := service.RunCyberGuardPoll(ctx)
	if errors.Is(err, service.ErrCyberGuardPollBusy) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordCyberGuardAudit(c, 0, "cyber_guard.poll_now", map[string]any{"status": result.Status, "new_events": result.NewEvents},
		"CyberGuard poll triggered manually")
	common.ApiSuccess(c, result)
}

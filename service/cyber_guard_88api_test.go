package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	cyberGuardTestBanGroup = "cg-banned"
	cyberGuardTestAdminKey = "test-admin-key"
)

func setupCyberGuardTest(t *testing.T, mutate func(*operation_setting.CyberGuardSetting)) operation_setting.CyberGuardSetting {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.CyberGuardEvent{}, &model.CyberGuardBan{}, &model.CyberGuardPollState{}, &model.AuditLog{}))
	s := operation_setting.DefaultCyberGuardSetting()
	s.Mode = operation_setting.CyberGuardModeEnforce
	s.BanGroup = cyberGuardTestBanGroup
	s.Sub2apiAdminApiKey = cyberGuardTestAdminKey
	s.Sub2apiApiKeyIds = []int64{1}
	s.NotifyRoot = false
	if mutate != nil {
		mutate(&s)
	}
	operation_setting.SetCyberGuardSettingForTest(s)
	clean := func() {
		for _, table := range []string{"cyber_guard_events", "cyber_guard_bans", "cyber_guard_poll_states", "audit_logs", "users", "logs", "email_deliveries"} {
			model.DB.Exec("DELETE FROM " + table)
		}
	}
	clean()
	t.Cleanup(func() {
		clean()
		operation_setting.SetCyberGuardSettingForTest(operation_setting.DefaultCyberGuardSetting())
		cyberGuardDetailSeen = map[int64]int64{}
		cyberGuardAccountCacheMu.Lock()
		cyberGuardAccountCache = map[int64]cyberGuardAccountInfo{}
		cyberGuardAccountCacheMu.Unlock()
	})
	seedCyberGuardUser(t, 1, "default", common.RoleRootUser, "")
	return operation_setting.GetCyberGuardSetting()
}

func seedCyberGuardUser(t *testing.T, id int, group string, role int, remark string) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{Id: id, Username: fmt.Sprintf("cg_user_%d", id), Password: "unused-password",
		Role: role, Status: common.UserStatusEnabled, Group: group, Email: fmt.Sprintf("u%d@example.com", id),
		AffCode: fmt.Sprintf("cg%d", id), Remark: remark}).Error)
}

func seedCyberGuardLog(t *testing.T, userId int, channelId int, tokenId int, requestId string, createdAt int64, keyFp string) {
	t.Helper()
	other := ""
	if keyFp != "" {
		other = `{"admin_info":{"channel_affinity":{"key_fp":"` + keyFp + `"}}}`
	}
	require.NoError(t, model.LOG_DB.Create(&model.Log{UserId: userId, Username: fmt.Sprintf("cg_user_%d", userId), CreatedAt: createdAt,
		Type: model.LogTypeConsume, TokenId: tokenId, TokenName: "tk", ModelName: "gpt-5", ChannelId: channelId, Group: "default",
		UpstreamRequestId: requestId, Other: other}).Error)
}

func seedCyberGuardEvent(t *testing.T, opsId int64, occurredAt int64, requestId string, state string, userId int, unitKey string) {
	t.Helper()
	created, err := model.InsertCyberGuardEvent(&model.CyberGuardEvent{OpsId: opsId, OccurredAt: occurredAt, Sub2apiRequestId: requestId,
		Category: operation_setting.CyberGuardCategoryCyber, Model: "gpt-5", State: state, UserId: userId, UnitKey: unitKey})
	require.NoError(t, err)
	require.True(t, created)
}

func cyberGuardEventByOps(t *testing.T, opsId int64) model.CyberGuardEvent {
	t.Helper()
	var event model.CyberGuardEvent
	require.NoError(t, model.DB.Where("ops_id = ?", opsId).First(&event).Error)
	return event
}

func cyberGuardUserGroup(t *testing.T, userId int) string {
	t.Helper()
	user, err := model.GetCyberGuardUser(userId)
	require.NoError(t, err)
	return user.Group
}

func opsRow(id int64, createdAt int64, errorType string, status int, requestId string, message string, accountId int64) map[string]any {
	return map[string]any{
		"id": id, "created_at": time.Unix(createdAt, 0).UTC().Format(time.RFC3339Nano), "type": errorType, "status_code": status,
		"model": "gpt-5", "request_id": requestId, "message": message, "api_key_id": 1, "account_id": accountId,
		"account_name": fmt.Sprintf("acc-%d", accountId), "inbound_endpoint": "/v1/responses",
	}
}

// newFakeSub2api imitates sub2api v2.10.5: x-api-key admin auth, business-
// limited cyber rows only visible with view=all, page_size capped at 2.
func newFakeSub2api(t *testing.T, rows []map[string]any, details map[int64]map[string]any) (*httptest.Server, func() []url.Values) {
	t.Helper()
	var mu sync.Mutex
	var queries []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		write := func(status int, payload any) {
			body, err := common.Marshal(payload)
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write(body)
		}
		if r.Header.Get("x-api-key") != cyberGuardTestAdminKey {
			write(http.StatusUnauthorized, map[string]any{"code": 401, "message": "INVALID_ADMIN_KEY"})
			return
		}
		if rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/admin/accounts/"); ok {
			id, _ := strconv.ParseInt(rest, 10, 64)
			write(http.StatusOK, map[string]any{"code": 0, "message": "success", "data": map[string]any{
				"id": id, "name": fmt.Sprintf("acc-%d", id), "status": "active", "schedulable": id != 443, "error_message": "",
			}})
			return
		}
		if rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/admin/ops/errors/"); ok {
			id, _ := strconv.ParseInt(rest, 10, 64)
			write(http.StatusOK, map[string]any{"code": 0, "message": "success", "data": details[id]})
			return
		}
		query := r.URL.Query()
		mu.Lock()
		queries = append(queries, query)
		mu.Unlock()
		items := []map[string]any{}
		if query.Get("view") == "all" {
			for _, row := range rows {
				if fmt.Sprint(row["api_key_id"]) == query.Get("api_key_id") {
					items = append(items, row)
				}
			}
		}
		const pageSize = 2
		page, _ := strconv.Atoi(query.Get("page"))
		pages := max(1, (len(items)+pageSize-1)/pageSize)
		start := min(max(page-1, 0)*pageSize, len(items))
		write(http.StatusOK, map[string]any{"code": 0, "message": "success", "data": map[string]any{
			"items": items[start:min(start+pageSize, len(items))], "total": len(items), "page": page, "page_size": pageSize, "pages": pages,
		}})
	}))
	t.Cleanup(server.Close)
	return server, func() []url.Values {
		mu.Lock()
		defer mu.Unlock()
		return append([]url.Values(nil), queries...)
	}
}

func TestCyberGuardClassifyOpsRow(t *testing.T) {
	cases := []struct {
		errorType string
		text      string
		expected  string
	}{
		{"cyber_policy_session_blocked", "This session is blocked by cyber-security policy", "session_blocked"},
		{"cyber_policy", "cyber_policy: flagged", "cyber"},
		{"upstream_error", "[cyber] request rejected", "cyber"},
		{"upstream_error", "This request was flagged for possible Cybersecurity Risk", "cyber"},
		{"upstream_error", "[bio] request rejected", "bio"},
		{"invalid_request_error", "flagged for possible biological risk", "bio"},
		{"upstream_error", "[usage] request rejected", "usage"},
		{"upstream_error", "The prompt violates our Usage Policy", "usage"},
		{"cyber_policy", "Excel BPS cybersecurity risk", ""},
		{"upstream_error", "rate limit exceeded", ""},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.expected, ClassifyCyberGuardOpsRow(tc.errorType, tc.text), tc.text)
	}
}

func TestCyberGuardPollAttributesCountsAndBans(t *testing.T) {
	setupCyberGuardTest(t, nil)
	now := common.GetTimestamp()
	seedCyberGuardUser(t, 9001, "default", common.RoleCommonUser, "")
	seedCyberGuardUser(t, 9002, "default", common.RoleAdminUser, "")
	seedCyberGuardUser(t, 9004, "default", common.RoleCommonUser, "")
	seedCyberGuardUser(t, 9005, "default", common.RoleCommonUser, "")
	seedCyberGuardUser(t, 9006, "default", common.RoleCommonUser, "")
	seedCyberGuardLog(t, 9001, 1, 70, "rid-a1", now-300, "aaaaaaaa")
	seedCyberGuardLog(t, 9001, 9, 70, "rid-a2", now-200, "aaaaaaaa")
	seedCyberGuardLog(t, 9001, 37, 70, "rid-a3", now-100, "")
	seedCyberGuardLog(t, 9002, 1, 80, "rid-admin", now-100, "")
	seedCyberGuardLog(t, 9004, 1, 81, "rid-amb", now-100, "")
	seedCyberGuardLog(t, 9005, 9, 82, "rid-amb", now-95, "")
	seedCyberGuardLog(t, 9001, 1, 70, "rid-usage", now-50, "")
	seedCyberGuardLog(t, 9001, 1, 70, "rid-sb", now-40, "")
	seedCyberGuardLog(t, 9006, 1, 83, "rid-bio", now-20, "")
	seedCyberGuardLog(t, 9004, 5, 81, "rid-other-channel", now-100, "")

	rows := []map[string]any{
		opsRow(1, now-300, "cyber_policy", 200, "rid-a1", "cyber_policy: flagged", 332),
		opsRow(2, now-200, "upstream_error", 400, "rid-a2", "[cyber] flagged for possible cybersecurity risk", 332),
		opsRow(3, now-100, "cyber_policy", 400, "rid-a3", "cyber_policy: flagged", 443),
		opsRow(4, now-100, "cyber_policy", 200, "rid-admin", "cyber_policy: flagged", 443),
		opsRow(5, now-100, "cyber_policy", 200, "rid-amb", "cyber_policy: flagged", 443),
		opsRow(6, now-50, "upstream_error", 400, "rid-usage", "[usage] violates usage policy", 443),
		opsRow(7, now-40, "cyber_policy_session_blocked", 403, "rid-sb", "session blocked", 0),
		opsRow(8, now-30, "cyber_policy", 200, "rid-none", "cyber_policy: flagged", 443),
		opsRow(9, now-700, "cyber_policy", 200, "rid-late", "cyber_policy: flagged", 443),
		opsRow(10, now-60, "upstream_error", 400, "rid-excel", "Excel BPS [cyber] quota", 443),
		opsRow(11, now-20, "upstream_error", 502, "rid-bio", "Upstream request failed", 443),
		opsRow(12, now-100, "cyber_policy", 200, "rid-other-channel", "cyber_policy: flagged", 443),
	}
	details := map[int64]map[string]any{11: {"id": 11, "upstream_error_message": "This request was flagged for possible biological risk"}}
	server, queries := newFakeSub2api(t, rows, details)
	s := operation_setting.GetCyberGuardSetting()
	s.Sub2apiBaseUrl = server.URL
	operation_setting.SetCyberGuardSettingForTest(s)

	result, err := RunCyberGuardPoll(context.Background())
	require.NoError(t, err)
	require.Equal(t, CyberGuardPollStatusOK, result.Status, result.Error)
	assert.Equal(t, 12, result.Fetched, "every page must be fetched")
	assert.Equal(t, 11, result.NewEvents, "Excel BPS rows are ignored")
	assert.Equal(t, 1, result.BansApplied)

	for _, query := range queries() {
		assert.Equal(t, "all", query.Get("view"))
		assert.Equal(t, "1", query.Get("api_key_id"))
		assert.Equal(t, "created_at", query.Get("sort_by"))
		assert.Equal(t, "asc", query.Get("sort_order"))
		start, err := time.Parse(time.RFC3339, query.Get("start_time"))
		require.NoError(t, err)
		assert.InDelta(t, now-15*60, start.Unix(), 5, "first poll only looks back lookback_minutes")
	}
	assert.Len(t, queries(), 6, "12 rows at page_size 2")

	first, second, third := cyberGuardEventByOps(t, 1), cyberGuardEventByOps(t, 2), cyberGuardEventByOps(t, 3)
	assert.Equal(t, model.CyberGuardEventStateCounted, first.State)
	assert.Equal(t, 9001, first.UserId)
	assert.Equal(t, 1, first.ChannelId)
	assert.Equal(t, int64(332), first.AccountId)
	assert.Equal(t, "fp:aaaaaaaa", first.UnitKey)
	assert.True(t, first.IsNewUnit)
	assert.Equal(t, "fp:aaaaaaaa", second.UnitKey, "same session fingerprint is the same unit")
	assert.False(t, second.IsNewUnit)
	assert.Equal(t, fmt.Sprintf("ns:70:gpt-5:%d", now-100), third.UnitKey)
	assert.True(t, third.IsNewUnit)

	expectedStates := map[int64]string{
		4:  model.CyberGuardEventStateExempt,
		5:  model.CyberGuardEventStateUnattributed,
		6:  model.CyberGuardEventStateRecordOnly,
		7:  model.CyberGuardEventStateRecordOnly,
		8:  model.CyberGuardEventStatePending,
		9:  model.CyberGuardEventStateUnattributed,
		11: model.CyberGuardEventStateCounted,
		12: model.CyberGuardEventStatePending,
	}
	for opsId, state := range expectedStates {
		assert.Equal(t, state, cyberGuardEventByOps(t, opsId).State, "ops %d", opsId)
	}
	assert.Equal(t, operation_setting.CyberGuardCategoryBio, cyberGuardEventByOps(t, 11).Category, "detail text classifies uncoded 502 rows")

	ban, err := model.GetActiveCyberGuardBanByUser(9001)
	require.NoError(t, err)
	require.NotNil(t, ban)
	assert.Equal(t, model.CyberGuardBanSourceAuto, ban.Source)
	assert.Equal(t, 0, ban.TierIndex)
	assert.Equal(t, "default", ban.OriginalGroup)
	assert.InDelta(t, now+72*3600, ban.ExpiresAt, 5)
	assert.Equal(t, "7天2个/30天2个", ban.UnitsSummary)
	assert.Equal(t, cyberGuardTestBanGroup, cyberGuardUserGroup(t, 9001))
	assert.Equal(t, model.CyberGuardEmailStateQueued, ban.EmailState)
	var delivery model.EmailDelivery
	require.NoError(t, model.DB.Where("delivery_key = ?", fmt.Sprintf("cyberguard-ban:%d", ban.Id)).First(&delivery).Error)
	assert.Equal(t, CyberGuardBanEmailCategory, delivery.Category)
	assert.Equal(t, common.SMTPProfileSecurity, delivery.SMTPProfile)
	assert.Equal(t, "u9001@example.com", delivery.Recipient)

	state, err := model.GetCyberGuardPollState()
	require.NoError(t, err)
	assert.Equal(t, CyberGuardPollStatusOK, state.LastStatus)
	assert.GreaterOrEqual(t, state.CursorEnd, now)

	overview, err := GetCyberGuardOverview()
	require.NoError(t, err)
	assert.Equal(t, model.CyberGuardOverviewCounts{HitsToday: overview.Counts.HitsToday, Hits7d: 10, Units7d: 3, UsersFlagged7d: 2,
		ActiveBans: 1, Unattributed7d: 2, Pending: 2}, overview.Counts)
	assert.Equal(t, model.CyberGuardReconciliation{Sub2apiRows: 11, Attributed: 7, Unattributed: 2, Pending: 2}, overview.Reconciliation24h)
	assert.True(t, overview.LastPoll.Ok)

	users, total, err := ListCyberGuardUsers("", "active", 1, 20)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, 9001, users[0].UserId)
	assert.Equal(t, 2, users[0].Units7d)
	assert.Equal(t, int64(4), users[0].Hits30d)
	assert.Equal(t, ban.Id, users[0].Ban.Id)
	assert.NotContains(t, users[0].Email, "u9001", "emails are masked")
	_, total, err = ListCyberGuardUsers("", "all", 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 3, total)

	events, total64, err := model.ListCyberGuardEvents(model.CyberGuardEventFilter{Keyword: ban.CaseId}, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total64, "a case id finds the banned user's events")
	assert.Len(t, events, 5)
	banRows, _, err := ListCyberGuardBanRows(model.CyberGuardBanStatusActive, 0, 0, 20)
	require.NoError(t, err)
	require.Len(t, banRows, 1)
	assert.Equal(t, cyberGuardTestBanGroup, banRows[0].CurrentGroup)
	assert.NotEmpty(t, banRows[0].EmailDeliveryState)

	accounts, err := ListCyberGuardAccounts(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, accounts, 2)
	assert.Equal(t, int64(443), accounts[0].AccountId)
	assert.Equal(t, [4]int64{6, 1, 1, 3}, [4]int64{accounts[0].HitsCyber, accounts[0].HitsBio, accounts[0].HitsUsage, accounts[0].Users})
	assert.Equal(t, "active", accounts[0].Status)
	require.NotNil(t, accounts[0].Schedulable)
	assert.False(t, *accounts[0].Schedulable)
	assert.Equal(t, int64(332), accounts[1].AccountId)
	assert.Equal(t, int64(2), accounts[1].HitsCyber)

	again, err := RunCyberGuardPoll(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, again.NewEvents, "ops rows are stored once")
	assert.Equal(t, 0, again.BansApplied)
	state, err = model.GetCyberGuardPollState()
	require.NoError(t, err)

	s.Sub2apiAdminApiKey = "wrong-key"
	operation_setting.SetCyberGuardSettingForTest(s)
	failed, err := RunCyberGuardPoll(context.Background())
	require.NoError(t, err)
	assert.Equal(t, CyberGuardPollStatusError, failed.Status)
	assert.Contains(t, failed.Error, "HTTP 401")
	assert.NotContains(t, failed.Error, "wrong-key")
	failedState, err := model.GetCyberGuardPollState()
	require.NoError(t, err)
	assert.Equal(t, state.CursorEnd, failedState.CursorEnd, "a failed fetch keeps the cursor")
	assert.Equal(t, CyberGuardPollStatusError, failedState.LastStatus)
}

func TestCyberGuardNoSessionUnitsMergeRetries(t *testing.T) {
	cases := []struct {
		mergeSeconds int
		units        int64
	}{
		{mergeSeconds: 60, units: 3},
		{mergeSeconds: 0, units: 5},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.mergeSeconds), func(t *testing.T) {
			s := setupCyberGuardTest(t, func(s *operation_setting.CyberGuardSetting) { s.NosessionMergeSeconds = tc.mergeSeconds })
			now := common.GetTimestamp()
			base := now - 1000
			seedCyberGuardUser(t, 9001, "default", common.RoleCommonUser, "")
			hits := []struct {
				offset  int64
				tokenId int
			}{{0, 70}, {30, 70}, {80, 70}, {200, 70}, {210, 71}}
			for i, hit := range hits {
				rid := fmt.Sprintf("rid-%d", i)
				seedCyberGuardLog(t, 9001, 1, hit.tokenId, rid, base+hit.offset, "")
				seedCyberGuardEvent(t, int64(i+1), base+hit.offset, rid, model.CyberGuardEventStatePending, 0, "")
			}
			result := &CyberGuardPollResult{}
			flagged, err := resolveCyberGuardPendingEvents(s, now, result)
			require.NoError(t, err)
			assert.Equal(t, []int{9001}, flagged)
			assert.Equal(t, 5, result.Counted)
			units, err := model.CountCyberGuardUnits(9001, 0)
			require.NoError(t, err)
			assert.Equal(t, tc.units, units)
			if tc.mergeSeconds > 0 {
				run := fmt.Sprintf("ns:70:gpt-5:%d", base)
				assert.Equal(t, run, cyberGuardEventByOps(t, 2).UnitKey, "retry 30s later joins the run")
				assert.Equal(t, run, cyberGuardEventByOps(t, 3).UnitKey, "each hit extends the run")
				assert.False(t, cyberGuardEventByOps(t, 3).IsNewUnit)
				assert.Equal(t, fmt.Sprintf("ns:70:gpt-5:%d", base+200), cyberGuardEventByOps(t, 4).UnitKey)
				assert.Equal(t, fmt.Sprintf("ns:71:gpt-5:%d", base+210), cyberGuardEventByOps(t, 5).UnitKey, "other token is another unit")
			}
		})
	}
}

func TestCyberGuardTierStrictestWinsAndFloorAfterUnban(t *testing.T) {
	s := setupCyberGuardTest(t, func(s *operation_setting.CyberGuardSetting) {
		s.Tiers = []operation_setting.CyberGuardTier{{WindowDays: 7, MinUnits: 2, BanHours: 24}, {WindowDays: 7, MinUnits: 2, BanHours: 72}, {WindowDays: 30, MinUnits: 3, BanHours: 0}}
	})
	now := common.GetTimestamp()
	seedCyberGuardUser(t, 9001, "default", common.RoleCommonUser, "")
	seedCyberGuardEvent(t, 1, now-100, "r1", model.CyberGuardEventStateCounted, 9001, "fp:11111111")
	seedCyberGuardEvent(t, 2, now-200, "r2", model.CyberGuardEventStateCounted, 9001, "fp:22222222")

	match, _, err := matchCyberGuardTier(s, 9001, now)
	require.NoError(t, err)
	require.NotNil(t, match)
	assert.Equal(t, 1, match.Index, "72h beats 24h")

	seedCyberGuardEvent(t, 3, now-20*86400, "r3", model.CyberGuardEventStateCounted, 9001, "fp:33333333")
	match, _, err = matchCyberGuardTier(s, 9001, now)
	require.NoError(t, err)
	require.NotNil(t, match)
	assert.Equal(t, 2, match.Index, "indefinite beats any duration")

	ban, err := CyberGuardManualBan(1, 9001, "test", 0)
	require.NoError(t, err)
	_, err = CyberGuardUnban(1, ban.Id, "reviewed")
	require.NoError(t, err)
	match, _, err = matchCyberGuardTier(s, 9001, now)
	require.NoError(t, err)
	assert.Nil(t, match, "units before the unban never count again")

	seedCyberGuardEvent(t, 4, now+10, "r4", model.CyberGuardEventStateCounted, 9001, "fp:44444444")
	result := &CyberGuardPollResult{}
	require.NoError(t, evaluateCyberGuardUser(s, 9001, now+20, result))
	assert.Equal(t, 0, result.BansApplied, "a single hit after unban does not re-ban")
	seedCyberGuardEvent(t, 5, now+15, "r5", model.CyberGuardEventStateCounted, 9001, "fp:55555555")
	require.NoError(t, evaluateCyberGuardUser(s, 9001, now+20, result))
	assert.Equal(t, 1, result.BansApplied)
	assert.Equal(t, cyberGuardTestBanGroup, cyberGuardUserGroup(t, 9001))
}

func TestCyberGuardBanLifecycleRestoresGroups(t *testing.T) {
	setupCyberGuardTest(t, nil)
	seedCyberGuardUser(t, 9001, "代理商", common.RoleCommonUser, "")
	setGroup := func(group string) {
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 9001).Update("group", group).Error)
	}

	ban, err := CyberGuardManualBan(1, 9001, "manual", 1)
	require.NoError(t, err)
	assert.Equal(t, "代理商", ban.OriginalGroup)
	assert.Equal(t, cyberGuardTestBanGroup, cyberGuardUserGroup(t, 9001))
	outcome, err := CyberGuardUnban(1, ban.Id, "ok")
	require.NoError(t, err)
	assert.Equal(t, model.CyberGuardEndReasonManualUnban, outcome.Ban.EndReason)
	assert.Equal(t, "代理商", cyberGuardUserGroup(t, 9001), "unban restores the original group")
	_, err = CyberGuardUnban(1, ban.Id, "again")
	assert.ErrorIs(t, err, model.ErrCyberGuardBanStateChanged)

	ban, err = CyberGuardManualBan(1, 9001, "manual", 0)
	require.NoError(t, err)
	setGroup("vip")
	outcome, err = CyberGuardUnban(1, ban.Id, "")
	require.NoError(t, err)
	assert.Equal(t, model.CyberGuardEndReasonAdminOverride, outcome.Ban.EndReason)
	assert.False(t, outcome.GroupChanged)
	assert.Equal(t, "vip", cyberGuardUserGroup(t, 9001), "a changed group is left alone")

	ban, err = CyberGuardManualBan(1, 9001, "manual", 0)
	require.NoError(t, err)
	CyberGuardOnAdminGroupChange(9001, 1, common.RoleRootUser)
	still, err := model.GetCyberGuardBan(ban.Id)
	require.NoError(t, err)
	assert.Equal(t, model.CyberGuardBanStatusActive, still.Status, "hook ignores users still in the ban group")
	setGroup("svip")
	CyberGuardOnAdminGroupChange(9001, 1, common.RoleRootUser)
	ended, err := model.GetCyberGuardBan(ban.Id)
	require.NoError(t, err)
	assert.Equal(t, model.CyberGuardEndReasonAdminOverride, ended.EndReason)
	assert.Equal(t, "svip", cyberGuardUserGroup(t, 9001))

	ban, err = CyberGuardManualBan(1, 9001, "manual", 1)
	require.NoError(t, err)
	setGroup("default") // e.g. a subscription expiry downgrade
	result := &CyberGuardPollResult{}
	require.NoError(t, reapplyCyberGuardDrift(common.GetTimestamp(), result))
	assert.Equal(t, 1, result.DriftReapplied)
	assert.Equal(t, cyberGuardTestBanGroup, cyberGuardUserGroup(t, 9001))
	require.NoError(t, model.DB.Model(&model.CyberGuardBan{}).Where("id = ?", ban.Id).Update("expires_at", common.GetTimestamp()-1).Error)
	require.NoError(t, expireCyberGuardBans(common.GetTimestamp(), result))
	assert.Equal(t, 1, result.Expired)
	expired, err := model.GetCyberGuardBan(ban.Id)
	require.NoError(t, err)
	assert.Equal(t, model.CyberGuardEndReasonExpired, expired.EndReason)
	assert.Equal(t, "default", expired.RestoreGroup)
	assert.Equal(t, "default", cyberGuardUserGroup(t, 9001), "expiry restores the group seen at drift time")
}

func TestCyberGuardAdoptProtectedAndObserveFlow(t *testing.T) {
	s := setupCyberGuardTest(t, func(s *operation_setting.CyberGuardSetting) {
		s.Mode = operation_setting.CyberGuardModeObserve
		s.ExemptUserIds = []int{9003}
	})
	now := common.GetTimestamp()
	seedCyberGuardUser(t, 9001, "default", common.RoleCommonUser, "")
	seedCyberGuardUser(t, 9002, "default", common.RoleAdminUser, "")
	seedCyberGuardUser(t, 9003, "default", common.RoleCommonUser, "")
	seedCyberGuardUser(t, 9010, cyberGuardTestBanGroup, common.RoleCommonUser, "[风控封号 2026-10-12 cyber 人工 原分组:default]")
	seedCyberGuardUser(t, 9011, cyberGuardTestBanGroup, common.RoleCommonUser, "原分组：代理商")

	_, err := CyberGuardManualBan(1, 9002, "", 0)
	assert.ErrorIs(t, err, model.ErrCyberGuardProtectedUser, "admins are never banned")
	_, err = CyberGuardManualBan(1, 9003, "", 0)
	assert.ErrorIs(t, err, model.ErrCyberGuardProtectedUser, "exempt users are never banned")
	_, err = CyberGuardManualBan(1, 9010, "", 0)
	assert.ErrorIs(t, err, model.ErrCyberGuardInBanGroup)

	adoptable, err := ListCyberGuardAdoptable()
	require.NoError(t, err)
	suggested := map[int]string{}
	for _, row := range adoptable {
		suggested[row.UserId] = row.SuggestedOriginalGroup
	}
	assert.Equal(t, map[int]string{9010: "default", 9011: "代理商"}, suggested)
	_, err = CyberGuardAdopt(1, 9001, "default", "", 0)
	assert.ErrorIs(t, err, model.ErrCyberGuardNotInBanGroup)
	adopted, err := CyberGuardAdopt(1, 9011, "代理商", "import", 0)
	require.NoError(t, err)
	assert.Equal(t, model.CyberGuardBanSourceAdopted, adopted.Source)
	assert.Equal(t, model.CyberGuardEmailStateNone, adopted.EmailState)
	assert.Equal(t, cyberGuardTestBanGroup, cyberGuardUserGroup(t, 9011), "adopt never changes the group")
	_, err = CyberGuardUnban(1, adopted.Id, "")
	require.NoError(t, err)
	assert.Equal(t, "代理商", cyberGuardUserGroup(t, 9011))

	for i, userId := range []int{9001, 9003} {
		seedCyberGuardEvent(t, int64(10*i+1), now-100, "", model.CyberGuardEventStateCounted, userId, "fp:aaaaaaa1")
		seedCyberGuardEvent(t, int64(10*i+2), now-50, "", model.CyberGuardEventStateCounted, userId, "fp:aaaaaaa2")
	}
	result := &CyberGuardPollResult{}
	require.NoError(t, evaluateCyberGuardUser(s, 9001, now, result))
	require.NoError(t, evaluateCyberGuardUser(s, 9003, now, result))
	assert.Equal(t, 1, result.Proposals, "observe mode proposes; exempt users get nothing")
	assert.Equal(t, 0, result.BansApplied)
	assert.Equal(t, "default", cyberGuardUserGroup(t, 9001), "observe mode never changes groups")
	open, err := model.CyberGuardOpenBansByUser([]int{9001, 9003})
	require.NoError(t, err)
	require.Contains(t, open, 9001)
	assert.NotContains(t, open, 9003)
	proposal := open[9001]
	assert.Equal(t, model.CyberGuardBanStatusObserved, proposal.Status)

	ban, err := CyberGuardPromote(1, proposal.Id)
	require.NoError(t, err)
	assert.Equal(t, model.CyberGuardBanStatusActive, ban.Status)
	assert.Equal(t, cyberGuardTestBanGroup, cyberGuardUserGroup(t, 9001))
	promoted, err := model.GetCyberGuardBan(proposal.Id)
	require.NoError(t, err)
	assert.Equal(t, model.CyberGuardEndReasonPromoted, promoted.EndReason)
	floor, err := model.CyberGuardCountingFloor(9001)
	require.NoError(t, err)
	assert.Zero(t, floor, "a promoted proposal is not a counting floor")

	seedCyberGuardUser(t, 9004, "default", common.RoleCommonUser, "")
	seedCyberGuardEvent(t, 31, now-100, "", model.CyberGuardEventStateCounted, 9004, "fp:bbbbbbb1")
	seedCyberGuardEvent(t, 32, now-50, "", model.CyberGuardEventStateCounted, 9004, "fp:bbbbbbb2")
	require.NoError(t, evaluateCyberGuardUser(s, 9004, now, result))
	open, err = model.CyberGuardOpenBansByUser([]int{9004})
	require.NoError(t, err)
	dismissed, err := CyberGuardDismiss(1, open[9004].Id, "false positive")
	require.NoError(t, err)
	assert.Equal(t, model.CyberGuardBanStatusDismissed, dismissed.Status)
	floor, err = model.CyberGuardCountingFloor(9004)
	require.NoError(t, err)
	assert.Equal(t, dismissed.EndedAt, floor, "dismissing restarts the count")
}

func TestCyberGuardSettingsKeepSecretAndValidate(t *testing.T) {
	setupCyberGuardTest(t, nil)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM options")
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})

	input, err := MergeCyberGuardSettingUpdate([]byte(`{"mode":"observe","ban_group":"vip","tiers":[{"window_days":7,"min_units":2,"ban_hours":24}]}`))
	require.NoError(t, err)
	assert.Empty(t, input.Sub2apiAdminApiKey, "the stored secret is never prefilled")
	assert.Equal(t, []int{1, 9, 37}, input.ProtectedChannelIds, "omitted fields keep their values")
	changed, err := UpdateCyberGuardSettings(input)
	require.NoError(t, err)
	assert.Contains(t, changed, "tiers")
	assert.NotContains(t, changed, "sub2api_admin_api_key", "empty secret keeps the stored key")

	saved := operation_setting.GetCyberGuardSetting()
	assert.Equal(t, operation_setting.CyberGuardModeObserve, saved.Mode)
	assert.Equal(t, cyberGuardTestAdminKey, saved.Sub2apiAdminApiKey)
	assert.Equal(t, []operation_setting.CyberGuardTier{{WindowDays: 7, MinUnits: 2, BanHours: 24}}, saved.Tiers)
	view, err := common.Marshal(GetCyberGuardSettingView())
	require.NoError(t, err)
	assert.NotContains(t, string(view), cyberGuardTestAdminKey)
	assert.NotContains(t, string(view), `"sub2api_admin_api_key"`)
	assert.Contains(t, string(view), `"has_sub2api_admin_api_key":true`)

	for _, body := range []string{
		`{"tiers":[{"window_days":0,"min_units":2,"ban_hours":24}]}`,
		`{"mode":"panic"}`,
		`{"ban_group":"no-such-group"}`,
		`{"poll_interval_seconds":5}`,
	} {
		bad, err := MergeCyberGuardSettingUpdate([]byte(body))
		require.NoError(t, err)
		_, err = UpdateCyberGuardSettings(bad)
		assert.ErrorIs(t, err, ErrCyberGuardInvalidInput, body)
	}
}

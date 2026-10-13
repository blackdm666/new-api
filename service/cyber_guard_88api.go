package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// 88api cyberguard: master-only poller that pulls sub2api ops_error_logs,
// attributes each upstream content-policy refusal to the exact new-api user
// through logs.upstream_request_id, counts distinct flagged sessions and bans
// users that reach a configured tier.

const (
	cyberGuardHTTPTimeout          = 10 * time.Second
	cyberGuardPageSize             = 500
	cyberGuardMaxPagesPerKey       = 40
	cyberGuardMaxCatchUpSeconds    = int64(operation_setting.CyberGuardMaxLookbackMinutes) * 60
	cyberGuardAttributionWindow    = int64(600)
	cyberGuardMaxDetailFetches     = 10
	cyberGuardPendingBatch         = 500
	cyberGuardMaxResponseBodyBytes = 16 << 20
	// A poll reads at most lookback + this many seconds past its start, so a
	// backlog drains in bounded slices instead of one unbounded window.
	cyberGuardCatchUpSliceSeconds = int64(1800)
	// Users with counted events this recent are re-evaluated every poll.
	cyberGuardRecheckSeconds        = int64(1800)
	cyberGuardHistoryCleanupSeconds = int64(3600)

	CyberGuardPollStatusOK      = "ok"
	CyberGuardPollStatusPartial = "partial" // catch-up in progress; the next poll continues
	CyberGuardPollStatusError   = "error"
	CyberGuardPollStatusSkipped = "skipped"
)

var (
	cyberGuardCyberPattern = regexp.MustCompile(`(?i)\[cyber\]|cybersecurity risk`)
	cyberGuardBioPattern   = regexp.MustCompile(`(?i)\[bio\]|biological risk`)
	cyberGuardUsagePattern = regexp.MustCompile(`(?i)\[usage\]|usage policy`)
	cyberGuardFpPattern    = regexp.MustCompile(`^[0-9a-f]{8}$`)

	ErrCyberGuardPollBusy = errors.New("a CyberGuard poll is already running")

	cyberGuardPollMu sync.Mutex
	// Redirects are never followed, so the admin API key is only ever sent to
	// the configured sub2api host.
	cyberGuardHTTPClient = &http.Client{
		Timeout: cyberGuardHTTPTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	// cyberGuardDetailSeen remembers ops rows whose detail was already fetched
	// (guarded by cyberGuardPollMu) so overlapping windows never refetch them.
	cyberGuardDetailSeen = map[int64]int64{}
	// cyberGuardHistoryCleanedAt is when the poll's succeeded system task
	// history was last pruned (unix seconds).
	cyberGuardHistoryCleanedAt atomic.Int64
)

// ClassifyCyberGuardOpsRow mirrors the operator report (cg_report.py): rows
// mentioning Excel BPS are ignored, sub2api's cyber types win, then the
// upstream text decides between cyber, bio and usage. "" means not stored.
func ClassifyCyberGuardOpsRow(errorType string, text string) string {
	if strings.Contains(strings.ToLower(text), "excel bps") {
		return ""
	}
	switch strings.TrimSpace(errorType) {
	case "cyber_policy_session_blocked":
		return operation_setting.CyberGuardCategorySessionBlocked
	case "cyber_policy":
		return operation_setting.CyberGuardCategoryCyber
	}
	switch {
	case cyberGuardCyberPattern.MatchString(text):
		return operation_setting.CyberGuardCategoryCyber
	case cyberGuardBioPattern.MatchString(text):
		return operation_setting.CyberGuardCategoryBio
	case cyberGuardUsagePattern.MatchString(text):
		return operation_setting.CyberGuardCategoryUsage
	}
	return ""
}

// cyberGuardOpsRow is the subset of sub2api v2.10.5 service.OpsErrorLog (list)
// and OpsErrorLogDetail (detail) that CyberGuard stores.
type cyberGuardOpsRow struct {
	Id                   int64     `json:"id"`
	CreatedAt            time.Time `json:"created_at"`
	Type                 string    `json:"type"`
	StatusCode           int       `json:"status_code"`
	Model                string    `json:"model"`
	RequestedModel       string    `json:"requested_model"`
	RequestId            string    `json:"request_id"`
	Message              string    `json:"message"`
	ApiKeyId             *int64    `json:"api_key_id"`
	AccountId            *int64    `json:"account_id"`
	AccountName          string    `json:"account_name"`
	RequestPath          string    `json:"request_path"`
	InboundEndpoint      string    `json:"inbound_endpoint"`
	UpstreamErrorMessage string    `json:"upstream_error_message"`
	UpstreamErrorDetail  string    `json:"upstream_error_detail"`
}

type cyberGuardEnvelope struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Data    common.RawMessage `json:"data"`
}

type cyberGuardOpsPage struct {
	Items []cyberGuardOpsRow `json:"items"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Pages int                `json:"pages"`
}

// cyberGuardSub2apiGet calls a sub2api admin endpoint with the admin API key.
// Errors never contain the key.
func cyberGuardSub2apiGet(ctx context.Context, s operation_setting.CyberGuardSetting, path string, query url.Values, out any) error {
	target := s.Sub2apiBaseUrl + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("invalid sub2api request: %w", err)
	}
	req.Header.Set("x-api-key", s.Sub2apiAdminApiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := cyberGuardHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("sub2api request failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, cyberGuardMaxResponseBodyBytes))
	if err != nil {
		return fmt.Errorf("sub2api response read failed: %v", err)
	}
	var envelope cyberGuardEnvelope
	decodeErr := common.Unmarshal(body, &envelope)
	if resp.StatusCode != http.StatusOK {
		message := ""
		if decodeErr == nil {
			message = model.TruncateCyberGuardText(envelope.Message, 200)
		}
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("sub2api rejected the admin API key (HTTP 401) %s", message)
		case http.StatusLocked:
			return fmt.Errorf("sub2api admin must acknowledge the compliance terms first (HTTP 423) %s", message)
		default:
			return fmt.Errorf("sub2api HTTP %d %s", resp.StatusCode, message)
		}
	}
	if decodeErr != nil {
		return fmt.Errorf("sub2api returned invalid JSON: %v", decodeErr)
	}
	if envelope.Code != 0 {
		return fmt.Errorf("sub2api error %d: %s", envelope.Code, model.TruncateCyberGuardText(envelope.Message, 200))
	}
	if out == nil {
		return nil
	}
	if err := common.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("sub2api returned unexpected data: %v", err)
	}
	return nil
}

// fetchCyberGuardOpsPage lists one page of the ops error rows of a sub2api API
// key in [start, end), oldest first. view=all is required because sub2api
// stores cyber rows as business-limited and hides them from the default view.
func fetchCyberGuardOpsPage(ctx context.Context, s operation_setting.CyberGuardSetting, apiKeyId int64, start int64, end int64, page int) (*cyberGuardOpsPage, error) {
	query := url.Values{}
	query.Set("view", "all")
	query.Set("api_key_id", strconv.FormatInt(apiKeyId, 10))
	query.Set("start_time", time.Unix(start, 0).UTC().Format(time.RFC3339))
	query.Set("end_time", time.Unix(end, 0).UTC().Format(time.RFC3339))
	query.Set("sort_by", "created_at")
	query.Set("sort_order", "asc")
	query.Set("page", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(cyberGuardPageSize))
	var result cyberGuardOpsPage
	if err := cyberGuardSub2apiGet(ctx, s, "/api/v1/admin/ops/errors", query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func fetchCyberGuardOpsDetail(ctx context.Context, s operation_setting.CyberGuardSetting, opsId int64) (*cyberGuardOpsRow, error) {
	var detail cyberGuardOpsRow
	if err := cyberGuardSub2apiGet(ctx, s, "/api/v1/admin/ops/errors/"+strconv.FormatInt(opsId, 10), nil, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

type CyberGuardPollResult struct {
	Status         string `json:"status"`
	Error          string `json:"error,omitempty"`
	WindowStart    int64  `json:"window_start"`
	WindowEnd      int64  `json:"window_end"`
	Fetched        int    `json:"fetched"`
	NewEvents      int    `json:"new_events"`
	Attributed     int    `json:"attributed"`
	Unattributed   int    `json:"unattributed"`
	Counted        int    `json:"counted"`
	BansApplied    int    `json:"bans_applied"`
	Proposals      int    `json:"proposals"`
	Escalated      int    `json:"escalated"`
	Expired        int    `json:"expired"`
	DriftReapplied int    `json:"drift_reapplied"`
}

// RunCyberGuardPoll runs one poll. It is serialized in-process; the system
// task lease keeps it to one runner across nodes.
func RunCyberGuardPoll(ctx context.Context) (*CyberGuardPollResult, error) {
	if !cyberGuardPollMu.TryLock() {
		return nil, ErrCyberGuardPollBusy
	}
	defer cyberGuardPollMu.Unlock()

	s := operation_setting.GetCyberGuardSetting()
	now := common.GetTimestamp()
	state, err := model.GetCyberGuardPollState()
	if err != nil {
		return nil, err
	}
	result := &CyberGuardPollResult{Status: CyberGuardPollStatusOK}
	var problems []string
	switch {
	case s.Mode == operation_setting.CyberGuardModeOff:
		result.Status, result.Error = CyberGuardPollStatusSkipped, "CyberGuard mode is off"
		// Re-enabling starts fresh instead of backfilling the off period.
		state.CursorEnd, state.CatchUp = 0, false
	case s.Sub2apiAdminApiKey == "":
		result.Status, result.Error = CyberGuardPollStatusSkipped, "sub2api admin API key is not configured"
	default:
		lookback := int64(s.LookbackMinutes) * 60
		start := state.CursorEnd - lookback
		if state.CatchUp {
			start = state.CursorEnd // resume exactly where the page budget ran out
		}
		if state.CursorEnd <= 0 || state.CursorEnd < now-cyberGuardMaxCatchUpSeconds {
			start = now - lookback // first run or long gap: never backfill beyond the lookback
		}
		end := min(now, start+lookback+cyberGuardCatchUpSliceSeconds)
		result.WindowStart, result.WindowEnd = start, end
		ingestedTo, err := ingestCyberGuardWindow(ctx, s, start, end, result)
		switch {
		case err != nil:
			// Rows stored before the failure stay; the cursor does not move.
			result.Status = CyberGuardPollStatusError
			problems = append(problems, err.Error())
		case ingestedTo < end:
			state.CursorEnd, state.CatchUp = ingestedTo, true
			result.Status = CyberGuardPollStatusPartial
		default:
			state.CursorEnd, state.CatchUp = end, false
			if end < now {
				result.Status = CyberGuardPollStatusPartial
			}
		}
	}

	if s.Mode != operation_setting.CyberGuardModeOff {
		flagged, err := resolveCyberGuardPendingEvents(s, now, result)
		if err != nil {
			problems = append(problems, "attribution: "+err.Error())
		}
		// Also re-check recently flagged users, so that a failed or interrupted
		// evaluation (or escalation) is retried instead of waiting for a new hit.
		recheck, err := model.ListCyberGuardRecheckUsers(now - cyberGuardRecheckSeconds)
		if err != nil {
			problems = append(problems, "recheck: "+err.Error())
		}
		slices.Sort(recheck)
		for _, userId := range recheck {
			if !slices.Contains(flagged, userId) {
				flagged = append(flagged, userId)
			}
		}
		for _, userId := range flagged {
			if err := evaluateCyberGuardUser(s, userId, now, result); err != nil {
				problems = append(problems, fmt.Sprintf("tier evaluation for user %d: %v", userId, err))
			}
		}
		if err := reapplyCyberGuardDrift(now, result); err != nil {
			problems = append(problems, "drift check: "+err.Error())
		}
	}
	// Expiry runs in every mode so that timed bans always end on time.
	if err := expireCyberGuardBans(now, result); err != nil {
		problems = append(problems, "expiry: "+err.Error())
	}

	if len(problems) > 0 {
		result.Status = CyberGuardPollStatusError
		result.Error = strings.Join(problems, "; ")
	}
	state.LastRunAt = now
	state.LastStatus = result.Status
	state.LastError = result.Error
	state.LastFetched = result.Fetched
	state.LastNewEvents = result.NewEvents
	if err := model.SaveCyberGuardPollState(state); err != nil {
		return result, err
	}
	return result, nil
}

// ingestCyberGuardWindow stores the classified ops rows of [start, end) page
// by page, so rows already fetched are kept when a later page fails or the
// page budget runs out. It returns the time up to which every API key was
// fully ingested: end, or, for a key whose page budget ran out, the created_at
// second of the last row it stored. Rows come oldest first, so resuming there
// re-reads at most that second; the unique ops_id makes the overlap harmless.
func ingestCyberGuardWindow(ctx context.Context, s operation_setting.CyberGuardSetting, start int64, end int64, result *CyberGuardPollResult) (int64, error) {
	for seenId, seenAt := range cyberGuardDetailSeen {
		if seenAt < start-cyberGuardMaxCatchUpSeconds {
			delete(cyberGuardDetailSeen, seenId)
		}
	}
	ingestedTo := end
	detailBudget := cyberGuardMaxDetailFetches
	for _, apiKeyId := range s.Sub2apiApiKeyIds {
		lastAt := start
		for page := 1; ; page++ {
			if page > cyberGuardMaxPagesPerKey {
				// More rows than one poll may read: continue from the last
				// stored row next time. A full budget inside one second skips
				// the rest of that second rather than stalling forever.
				ingestedTo = min(ingestedTo, max(lastAt, start+1))
				break
			}
			opsPage, err := fetchCyberGuardOpsPage(ctx, s, apiKeyId, start, end, page)
			if err != nil {
				return ingestedTo, err
			}
			result.Fetched += len(opsPage.Items)
			for _, row := range opsPage.Items {
				if err := storeCyberGuardOpsRow(ctx, s, row, &detailBudget, result); err != nil {
					return ingestedTo, err
				}
				lastAt = max(lastAt, row.CreatedAt.Unix())
			}
			if len(opsPage.Items) == 0 || page >= opsPage.Pages {
				break
			}
		}
	}
	return ingestedTo, nil
}

// storeCyberGuardOpsRow classifies one ops row and stores it once when it is
// a content-policy refusal. The list omits the upstream message/detail; only
// uncoded 400/502 rows can hide a policy refusal there, so a bounded number of
// those details is fetched once per row.
func storeCyberGuardOpsRow(ctx context.Context, s operation_setting.CyberGuardSetting, row cyberGuardOpsRow, detailBudget *int, result *CyberGuardPollResult) error {
	if row.ApiKeyId == nil || !slices.Contains(s.Sub2apiApiKeyIds, *row.ApiKeyId) {
		return nil
	}
	text := row.Message
	category := ClassifyCyberGuardOpsRow(row.Type, text)
	if category == "" && *detailBudget > 0 && (row.StatusCode == http.StatusBadRequest || row.StatusCode == http.StatusBadGateway) &&
		!strings.Contains(strings.ToLower(text), "excel bps") {
		if _, seen := cyberGuardDetailSeen[row.Id]; !seen {
			cyberGuardDetailSeen[row.Id] = row.CreatedAt.Unix()
			*detailBudget--
			if detail, err := fetchCyberGuardOpsDetail(ctx, s, row.Id); err == nil {
				text = strings.TrimSpace(text + " " + detail.UpstreamErrorMessage + " " + detail.UpstreamErrorDetail)
				category = ClassifyCyberGuardOpsRow(row.Type, text)
			}
		}
	}
	if category == "" {
		return nil
	}
	event := &model.CyberGuardEvent{
		OpsId:            row.Id,
		OccurredAt:       row.CreatedAt.Unix(),
		Sub2apiRequestId: model.TruncateCyberGuardText(row.RequestId, 128),
		ApiKeyId:         *row.ApiKeyId,
		AccountName:      model.TruncateCyberGuardText(row.AccountName, 128),
		Category:         category,
		ErrorType:        model.TruncateCyberGuardText(row.Type, 64),
		StatusCode:       row.StatusCode,
		Endpoint:         model.TruncateCyberGuardText(firstNonEmpty(row.InboundEndpoint, row.RequestPath), 128),
		Model:            model.TruncateCyberGuardText(firstNonEmpty(row.RequestedModel, row.Model), 128),
		Message:          model.TruncateCyberGuardText(text, 300),
		State:            model.CyberGuardEventStatePending,
	}
	if row.AccountId != nil {
		event.AccountId = *row.AccountId
	}
	created, err := model.InsertCyberGuardEvent(event)
	if err != nil {
		return fmt.Errorf("store ops row %d: %w", row.Id, err)
	}
	if created {
		result.NewEvents++
	}
	return nil
}

type cyberGuardLogOther struct {
	AdminInfo struct {
		ChannelAffinity struct {
			KeyFp string `json:"key_fp"`
		} `json:"channel_affinity"`
	} `json:"admin_info"`
}

func cyberGuardSessionFingerprint(other string) string {
	if other == "" {
		return ""
	}
	var parsed cyberGuardLogOther
	if err := common.UnmarshalJsonStr(other, &parsed); err != nil {
		return ""
	}
	fp := strings.TrimSpace(parsed.AdminInfo.ChannelAffinity.KeyFp)
	if !cyberGuardFpPattern.MatchString(fp) {
		return ""
	}
	return fp
}

// resolveCyberGuardPendingEvents attributes pending events through the exact
// upstream request id join and assigns their counting unit. It returns the
// users that received a new counted event, in first-seen order.
func resolveCyberGuardPendingEvents(s operation_setting.CyberGuardSetting, now int64, result *CyberGuardPollResult) ([]int, error) {
	pending, err := model.ListPendingCyberGuardEvents(cyberGuardPendingBatch)
	if err != nil || len(pending) == 0 {
		return nil, err
	}
	requestIds := make([]string, 0, len(pending))
	minAt, maxAt := int64(math.MaxInt64), int64(0)
	for _, event := range pending {
		if event.Sub2apiRequestId != "" && !slices.Contains(requestIds, event.Sub2apiRequestId) {
			requestIds = append(requestIds, event.Sub2apiRequestId)
		}
		minAt, maxAt = min(minAt, event.OccurredAt), max(maxAt, event.OccurredAt)
	}
	logs, err := model.FindCyberGuardAttributionLogs(requestIds, s.ProtectedChannelIds,
		minAt-cyberGuardAttributionWindow, maxAt+cyberGuardAttributionWindow)
	if err != nil {
		return nil, err
	}
	byRequest := make(map[string][]*model.Log, len(requestIds))
	for _, log := range logs {
		byRequest[log.UpstreamRequestId] = append(byRequest[log.UpstreamRequestId], log)
	}
	users := map[int]*model.CyberGuardUser{}
	var flagged []int
	for _, event := range pending {
		var match *model.Log
		matchDistance := int64(math.MaxInt64)
		distinct := map[int]bool{}
		for _, log := range byRequest[event.Sub2apiRequestId] {
			distance := log.CreatedAt - event.OccurredAt
			if distance < 0 {
				distance = -distance
			}
			if distance > cyberGuardAttributionWindow {
				continue
			}
			distinct[log.UserId] = true
			if distance < matchDistance {
				match, matchDistance = log, distance
			}
		}
		if len(distinct) != 1 {
			if len(distinct) == 0 && now <= event.OccurredAt+cyberGuardAttributionWindow {
				continue // the log row may still be written; retry next poll
			}
			event.State = model.CyberGuardEventStateUnattributed
			if _, err := model.SaveCyberGuardEventResolution(event); err != nil {
				return flagged, err
			}
			result.Unattributed++
			continue
		}
		event.UserId, event.Username = match.UserId, match.Username
		event.TokenId, event.TokenName, event.TokenGroup = match.TokenId, match.TokenName, match.Group
		event.ChannelId, event.LogId = match.ChannelId, match.Id
		event.SessionFp = cyberGuardSessionFingerprint(match.Other)
		user, ok := users[event.UserId]
		if !ok {
			user, err = model.GetCyberGuardUser(event.UserId)
			if err != nil && !errors.Is(err, model.ErrCyberGuardUserNotFound) {
				return flagged, err
			}
			users[event.UserId] = user
		}
		if err := assignCyberGuardUnit(s, event, user); err != nil {
			return flagged, err
		}
		saved, err := model.SaveCyberGuardEventResolution(event)
		if err != nil {
			return flagged, err
		}
		if !saved {
			continue
		}
		result.Attributed++
		if event.State == model.CyberGuardEventStateCounted {
			result.Counted++
			if !slices.Contains(flagged, event.UserId) {
				flagged = append(flagged, event.UserId)
			}
		}
	}
	return flagged, nil
}

// assignCyberGuardUnit sets state, unit_key and is_new_unit. A unit is one
// flagged session (fp:<key_fp>); without a fingerprint, hits of the same token
// and model chain into one unit while each follows the previous one within
// nosession_merge_seconds (collapsing client auto-retries).
func assignCyberGuardUnit(s operation_setting.CyberGuardSetting, event *model.CyberGuardEvent, user *model.CyberGuardUser) error {
	if user == nil || !s.CountsCategory(event.Category) {
		event.State = model.CyberGuardEventStateRecordOnly
		return nil
	}
	if event.SessionFp != "" {
		event.UnitKey = "fp:" + event.SessionFp
	} else {
		modelPart := event.Model
		if len(modelPart) > 48 {
			modelPart = common.Sha1([]byte(modelPart))[:12]
		}
		event.UnitKey = fmt.Sprintf("ns:%d:%s:%d", event.TokenId, modelPart, event.OccurredAt)
		if s.NosessionMergeSeconds > 0 {
			previous, err := model.FindPreviousCyberGuardNoFpEvent(event.UserId, event.TokenId, event.Model, event.OccurredAt, event.Id)
			if err != nil {
				return err
			}
			if previous != nil && event.OccurredAt-previous.OccurredAt <= int64(s.NosessionMergeSeconds) {
				event.UnitKey = previous.UnitKey
			}
		}
	}
	if user.Role >= common.RoleAdminUser || s.IsExempt(user.Id) {
		event.State = model.CyberGuardEventStateExempt
		event.IsNewUnit = false
		return nil
	}
	seen, err := model.CyberGuardUnitAlreadyCounted(event.UserId, event.UnitKey, event.Id)
	if err != nil {
		return err
	}
	event.State = model.CyberGuardEventStateCounted
	event.IsNewUnit = !seen
	return nil
}

func cyberGuardTierStricter(candidate operation_setting.CyberGuardTier, current operation_setting.CyberGuardTier) bool {
	if candidate.BanHours == 0 {
		return current.BanHours != 0
	}
	if current.BanHours == 0 {
		return false
	}
	return candidate.BanHours > current.BanHours
}

type cyberGuardTierMatch struct {
	Index   int
	Tier    operation_setting.CyberGuardTier
	Summary string
}

// matchCyberGuardTier counts the user's distinct units per tier window (never
// before the counting floor) and returns the strictest reached tier, or nil.
func matchCyberGuardTier(s operation_setting.CyberGuardSetting, userId int, now int64) (*cyberGuardTierMatch, string, error) {
	floor, err := model.CyberGuardCountingFloor(userId)
	if err != nil {
		return nil, "", err
	}
	spans, err := model.ListCyberGuardUnitSpans([]int{userId})
	if err != nil {
		return nil, "", err
	}
	parts := make([]string, 0, len(s.Tiers))
	var best *cyberGuardTierMatch
	for i, tier := range s.Tiers {
		since := now - int64(tier.WindowDays)*86400
		units := 0
		for _, span := range spans {
			if span.Counts(since, floor) {
				units++
			}
		}
		parts = append(parts, fmt.Sprintf("%d天%d个", tier.WindowDays, units))
		if units >= tier.MinUnits && (best == nil || cyberGuardTierStricter(tier, best.Tier)) {
			best = &cyberGuardTierMatch{Index: i, Tier: tier}
		}
	}
	summary := strings.Join(parts, "/")
	if best != nil {
		best.Summary = summary
	}
	return best, summary, nil
}

func evaluateCyberGuardUser(s operation_setting.CyberGuardSetting, userId int, now int64, result *CyberGuardPollResult) error {
	match, _, err := matchCyberGuardTier(s, userId, now)
	if err != nil || match == nil {
		return err
	}
	active, err := model.GetActiveCyberGuardBanByUser(userId)
	if err != nil {
		return err
	}
	if active != nil {
		previousExpiry := active.ExpiresAt
		if previousExpiry == 0 || (match.Tier.BanHours > 0 && active.StartedAt+int64(match.Tier.BanHours)*3600 <= previousExpiry) {
			return nil // nothing stricter to apply; skip the locking transaction
		}
		ban, changed, err := model.EscalateCyberGuardBan(active.Id, match.Index, match.Tier.BanHours, match.Summary, now)
		if err != nil || !changed {
			return err
		}
		result.Escalated++
		recordCyberGuardSystemAudit("cyber_guard.escalate", userId, map[string]any{
			"case_id": ban.CaseId, "previous_expires_at": previousExpiry, "expires_at": ban.ExpiresAt, "units_summary": match.Summary,
		}, fmt.Sprintf("CyberGuard escalated %s for user %d", ban.CaseId, userId))
		return nil
	}
	user, err := model.GetCyberGuardUser(userId)
	if err != nil {
		if errors.Is(err, model.ErrCyberGuardUserNotFound) {
			return nil
		}
		return err
	}
	if user.Group == s.BanGroup {
		return nil // manual ban without a record: listed as adoptable, never touched automatically
	}
	input := model.CyberGuardBanInput{
		UserId:        userId,
		BanGroup:      s.BanGroup,
		Source:        model.CyberGuardBanSourceAuto,
		TierIndex:     match.Index,
		BanHours:      match.Tier.BanHours,
		UnitsSummary:  match.Summary,
		Reason:        fmt.Sprintf("自动：第 %d 档（%d 天内 ≥%d 个被标记会话）", match.Index+1, match.Tier.WindowDays, match.Tier.MinUnits),
		ExemptUserIds: s.ExemptUserIds,
		Now:           now,
	}
	if s.Mode == operation_setting.CyberGuardModeEnforce {
		ban, _, err := applyCyberGuardBan(s, input, true)
		if err != nil {
			if isCyberGuardSkippableBanError(err) {
				return nil
			}
			return err
		}
		result.BansApplied++
		recordCyberGuardSystemAudit("cyber_guard.ban", userId, CyberGuardBanAuditParams(ban),
			fmt.Sprintf("CyberGuard banned user %d (%s)", userId, ban.CaseId))
		return nil
	}
	proposal, created, err := model.UpsertCyberGuardProposal(input)
	if err != nil {
		if isCyberGuardSkippableBanError(err) {
			return nil
		}
		return err
	}
	if created {
		result.Proposals++
		if s.NotifyRoot {
			notifyCyberGuardRoot(proposal.CaseId, "proposal", fmt.Sprintf("风控中心封禁建议 %s", proposal.CaseId),
				fmt.Sprintf("观察模式：用户 %s（ID %d，分组 %s）达到封禁线（%s，%s）。请在「风控中心」确认封禁或忽略。",
					proposal.Username, userId, proposal.OriginalGroup, match.Summary, proposal.Reason))
		}
	}
	return nil
}

func isCyberGuardSkippableBanError(err error) bool {
	return errors.Is(err, model.ErrCyberGuardProtectedUser) || errors.Is(err, model.ErrCyberGuardInBanGroup) ||
		errors.Is(err, model.ErrCyberGuardAlreadyBanned) || errors.Is(err, model.ErrCyberGuardUserNotFound)
}

func expireCyberGuardBans(now int64, result *CyberGuardPollResult) error {
	due, err := model.ListDueCyberGuardBans(now)
	if err != nil {
		return err
	}
	for _, ban := range due {
		outcome, err := endCyberGuardBan(ban.Id, 0, model.CyberGuardEndReasonExpired, "", now)
		if err != nil {
			if errors.Is(err, model.ErrCyberGuardBanStateChanged) {
				continue
			}
			return err
		}
		result.Expired++
		recordCyberGuardSystemAudit("cyber_guard.unban", outcome.Ban.UserId, cyberGuardEndAuditParams(outcome),
			fmt.Sprintf("CyberGuard ended %s for user %d (%s)", outcome.Ban.CaseId, outcome.Ban.UserId, outcome.Ban.EndReason))
	}
	return nil
}

// reapplyCyberGuardDrift handles active bans whose user left the ban group
// without an admin edit (admin edits end the ban through the UpdateUser hook):
// subscription purchase or expiry, affiliate upgrade, unknown writers.
func reapplyCyberGuardDrift(now int64, result *CyberGuardPollResult) error {
	active, err := model.ListActiveCyberGuardBans()
	if err != nil || len(active) == 0 {
		return err
	}
	userIds := make([]int, 0, len(active))
	for _, ban := range active {
		userIds = append(userIds, ban.UserId)
	}
	groups, err := model.GetCyberGuardUserGroups(userIds)
	if err != nil {
		return err
	}
	for _, ban := range active {
		group, exists := groups[ban.UserId]
		if exists && group == ban.BanGroup {
			continue
		}
		if !exists {
			outcome, err := endCyberGuardBan(ban.Id, 0, model.CyberGuardEndReasonUserMissing, "", now)
			if err == nil {
				recordCyberGuardSystemAudit("cyber_guard.unban", ban.UserId, cyberGuardEndAuditParams(outcome),
					fmt.Sprintf("CyberGuard ended %s: user %d no longer exists", ban.CaseId, ban.UserId))
			} else if !errors.Is(err, model.ErrCyberGuardBanStateChanged) {
				return err
			}
			continue
		}
		// Stamp the real time: an admin edit made while this poll was running
		// recognises a re-apply after it by updated_at (EndCyberGuardBanForAdminEdit).
		outcome, err := model.ReapplyCyberGuardBanDrift(ban.Id, common.GetTimestamp())
		if err != nil {
			return err
		}
		if outcome == nil {
			continue
		}
		refreshCyberGuardGroupCache(ban.UserId)
		result.DriftReapplied++
		recordCyberGuardSystemAudit("cyber_guard.drift_reapplied", ban.UserId, map[string]any{
			"case_id": ban.CaseId, "from_group": outcome.FromGroup, "to_group": outcome.ToGroup, "restore_group": outcome.FromGroup,
		}, fmt.Sprintf("CyberGuard moved user %d back to the ban group (%s)", ban.UserId, ban.CaseId))
	}
	return nil
}

// cyberGuardPollHandler runs the poller as a master-only scheduled system
// task (DB lease, run history on System Info). The scheduler wakes every 15s,
// so the effective interval is poll_interval_seconds rounded up to that pass.
type cyberGuardPollHandler struct{}

func (cyberGuardPollHandler) Type() string { return model.SystemTaskTypeCyberGuardPoll }

// Enabled is always true outside off mode. While off, a run is needed only to
// end a timed ban that is due, or once to clear the cursor so that turning
// the mode back on (through any settings path) never backfills the off period.
func (cyberGuardPollHandler) Enabled() bool {
	if operation_setting.GetCyberGuardSetting().Mode != operation_setting.CyberGuardModeOff {
		return true
	}
	return model.HasDueCyberGuardBans(common.GetTimestamp()) || model.CyberGuardPollCursorSet()
}

func (cyberGuardPollHandler) Interval() time.Duration {
	return time.Duration(operation_setting.GetCyberGuardSetting().PollIntervalSeconds) * time.Second
}

func (cyberGuardPollHandler) NewPayload() any { return nil }

func (cyberGuardPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	result, err := RunCyberGuardPoll(ctx)
	status := model.SystemTaskStatusSucceeded
	errorMessage := ""
	switch {
	case errors.Is(err, ErrCyberGuardPollBusy):
		result = &CyberGuardPollResult{Status: CyberGuardPollStatusSkipped, Error: err.Error()}
	case err != nil:
		status, errorMessage = model.SystemTaskStatusFailed, err.Error()
	case result.Status == CyberGuardPollStatusError:
		status, errorMessage = model.SystemTaskStatusFailed, result.Error
	}
	if finishErr := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); finishErr != nil {
		logSystemTaskLockError(ctx, task, finishErr)
	}
	// One row per poll would grow system_tasks by thousands of rows a day.
	// Hourly, drop this type's succeeded history; the newest row (which the
	// scheduler needs) and failed runs are kept.
	if now := common.GetTimestamp(); now-cyberGuardHistoryCleanedAt.Load() >= cyberGuardHistoryCleanupSeconds {
		cyberGuardHistoryCleanedAt.Store(now)
		if _, err := model.DeleteSystemTaskHistory(model.SystemTaskFilter{
			Type: model.SystemTaskTypeCyberGuardPoll, Status: model.SystemTaskStatusSucceeded,
		}); err != nil {
			common.SysError("cyberguard: failed to prune poll task history: " + err.Error())
		}
	}
}

func init() {
	RegisterSystemTaskHandler(cyberGuardPollHandler{})
}

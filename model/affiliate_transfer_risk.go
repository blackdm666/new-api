package model

import (
	"net"
	"regexp"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// Self-referral risk scoring for commission-to-balance review. The rules were
// agreed with operations on 2026-10-26 from a manual audit of past referrals:
//
//	inviter and invitee share a cold IP            +3
//	invitee registered within 24h of the inviter   +2
//	inviter has no usage while the invitee uses    +2
//	similar username / email                       +2
//	same browser UA on a shared cold IP            +1
//	inviter has exactly one paying invitee         +1
//	invitees of the inviter share cold IPs         +2
//
// A cold IP is one seen on at most affiliateRiskColdIPMaxUsers accounts, which
// keeps proxies, company exits and our own servers from matching everyone.
// The score is the highest invitee-pair score plus the inviter-level bonuses.
const (
	AffiliateRiskHighScore   = 5
	AffiliateRiskMediumScore = 3

	affiliateRiskColdIPMaxUsers      = 4
	affiliateRiskMaxInvitees         = 2000
	affiliateRiskLogSampleUsers      = 50
	affiliateRiskLogSampleRows       = 300
	affiliateRiskPopularitySample    = 5000
	affiliateRiskMaxPopularityChecks = 300
	affiliateRiskRegistrationWindow  = int64(24 * 60 * 60)
	affiliateRiskMaxReportedPairs    = 5
	affiliateRiskIdQueryChunk        = 500

	AffiliateRiskSignalSharedIP        = "shared_ip"
	AffiliateRiskSignalSameUserAgent   = "same_user_agent"
	AffiliateRiskSignalRegisteredClose = "registered_within_day"
	AffiliateRiskSignalInviterIdle     = "inviter_idle"
	AffiliateRiskSignalSimilarName     = "similar_name"
)

var affiliateRiskGeneratedUsername = regexp.MustCompile(`^[a-z]+_\d+$`)

type AffiliateRiskPair struct {
	InviteeId              int      `json:"invitee_id"`
	InviteeUsername        string   `json:"invitee_username"`
	Score                  int      `json:"score"`
	Signals                []string `json:"signals"`
	SharedIp               string   `json:"shared_ip,omitempty"`
	RegistrationGapSeconds int64    `json:"registration_gap_seconds"`
	CommissionCents        int64    `json:"commission_cents"`
}

type AffiliateRiskDetail struct {
	InviteeCount              int                 `json:"invitee_count"`
	SingleInvitee             bool                `json:"single_invitee"`
	InviteeClusterCount       int                 `json:"invitee_cluster_count"`
	SuspiciousCommissionCents int64               `json:"suspicious_commission_cents"`
	Pairs                     []AffiliateRiskPair `json:"pairs"`
}

type affiliateRiskUser struct {
	Id        int
	Username  string
	Email     string
	CreatedAt int64
	UsedQuota int
}

type affiliateRiskFootprint struct {
	ips        map[string]bool
	userAgents map[string]bool
}

// EvaluateAffiliateInviterRisk scores how likely the inviter's paying invitees
// are the inviter's own accounts.
func EvaluateAffiliateInviterRisk(inviterId int) (int, *AffiliateRiskDetail, error) {
	detail := &AffiliateRiskDetail{Pairs: []AffiliateRiskPair{}}
	type inviteeCommission struct {
		InviteeId int
		Cents     int64
	}
	commissions := []inviteeCommission{}
	if err := DB.Model(&AffiliateCommission{}).
		Select("invitee_id, COALESCE(SUM(commission_cents), 0) AS cents").
		Where("inviter_id = ? AND status = ?", inviterId, AffiliateCommissionStatusApproved).
		Group("invitee_id").
		Order("cents DESC").
		Limit(affiliateRiskMaxInvitees).
		Scan(&commissions).Error; err != nil {
		return 0, nil, err
	}
	detail.InviteeCount = len(commissions)
	if len(commissions) == 0 {
		return 0, detail, nil
	}

	ids := []int{inviterId}
	for _, row := range commissions {
		ids = append(ids, row.InviteeId)
	}
	users := map[int]affiliateRiskUser{}
	for chunk := range slices.Chunk(ids, affiliateRiskIdQueryChunk) {
		rows := []affiliateRiskUser{}
		if err := DB.Unscoped().Model(&User{}).Select("id", "username", "email", "created_at", "used_quota").Where("id IN ?", chunk).Scan(&rows).Error; err != nil {
			return 0, nil, err
		}
		for _, row := range rows {
			users[row.Id] = row
		}
	}
	inviter, ok := users[inviterId]
	if !ok {
		return 0, detail, nil
	}

	footprints, err := collectAffiliateRiskFootprints(ids)
	if err != nil {
		return 0, nil, err
	}
	coldIPs, err := affiliateRiskColdIPs(inviterId, ids, footprints)
	if err != nil {
		return 0, nil, err
	}

	// Union invitees that share a cold IP with each other (inviter excluded).
	parent := map[int]int{}
	var find func(int) int
	find = func(id int) int {
		for parent[id] != id {
			parent[id] = parent[parent[id]]
			id = parent[id]
		}
		return id
	}
	ipOwner := map[string]int{}
	for _, row := range commissions {
		parent[row.InviteeId] = row.InviteeId
	}
	for _, row := range commissions {
		for ip := range footprints[row.InviteeId].ips {
			if !coldIPs[ip] {
				continue
			}
			if owner, seen := ipOwner[ip]; seen {
				parent[find(row.InviteeId)] = find(owner)
				continue
			}
			ipOwner[ip] = row.InviteeId
		}
	}
	clusterSizes := map[int]int{}
	for _, row := range commissions {
		clusterSizes[find(row.InviteeId)]++
	}
	for _, size := range clusterSizes {
		if size >= 2 {
			detail.InviteeClusterCount++
		}
	}

	inviterNames := affiliateRiskNameTokens(inviter)
	quotaPerUnit := int(common.QuotaPerUnit)
	maxPairScore := 0
	for _, row := range commissions {
		invitee, ok := users[row.InviteeId]
		if !ok {
			continue
		}
		pair := AffiliateRiskPair{
			InviteeId:       invitee.Id,
			InviteeUsername: invitee.Username,
			Signals:         []string{},
			CommissionCents: row.Cents,
		}
		if inviter.CreatedAt > 0 && invitee.CreatedAt > 0 {
			pair.RegistrationGapSeconds = invitee.CreatedAt - inviter.CreatedAt
		}
		for ip := range footprints[invitee.Id].ips {
			if coldIPs[ip] && footprints[inviterId].ips[ip] {
				pair.SharedIp = ip
				break
			}
		}
		if pair.SharedIp != "" {
			pair.Score += 3
			pair.Signals = append(pair.Signals, AffiliateRiskSignalSharedIP)
			for userAgent := range footprints[invitee.Id].userAgents {
				if footprints[inviterId].userAgents[userAgent] {
					pair.Score++
					pair.Signals = append(pair.Signals, AffiliateRiskSignalSameUserAgent)
					break
				}
			}
		}
		if inviter.CreatedAt > 0 && invitee.CreatedAt > 0 && max(pair.RegistrationGapSeconds, -pair.RegistrationGapSeconds) < affiliateRiskRegistrationWindow {
			pair.Score += 2
			pair.Signals = append(pair.Signals, AffiliateRiskSignalRegisteredClose)
		}
		if inviter.UsedQuota < quotaPerUnit && invitee.UsedQuota >= quotaPerUnit {
			pair.Score += 2
			pair.Signals = append(pair.Signals, AffiliateRiskSignalInviterIdle)
		}
		if affiliateRiskNamesSimilar(inviterNames, affiliateRiskNameTokens(invitee)) {
			pair.Score += 2
			pair.Signals = append(pair.Signals, AffiliateRiskSignalSimilarName)
		}
		if pair.Score >= AffiliateRiskMediumScore {
			detail.SuspiciousCommissionCents += pair.CommissionCents
		}
		maxPairScore = max(maxPairScore, pair.Score)
		if pair.Score > 0 {
			detail.Pairs = append(detail.Pairs, pair)
		}
	}
	slices.SortStableFunc(detail.Pairs, func(a, b AffiliateRiskPair) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		return int(b.CommissionCents - a.CommissionCents)
	})
	if len(detail.Pairs) > affiliateRiskMaxReportedPairs {
		detail.Pairs = detail.Pairs[:affiliateRiskMaxReportedPairs]
	}

	score := maxPairScore
	if len(commissions) == 1 {
		detail.SingleInvitee = true
		score++
	}
	if detail.InviteeClusterCount > 0 {
		score += 2
	}
	return score, detail, nil
}

// collectAffiliateRiskFootprints gathers login IPs and browsers for every
// account plus a bounded sample of recent API-call IPs for the inviter and
// the invitees that produced the most commission.
func collectAffiliateRiskFootprints(ids []int) (map[int]*affiliateRiskFootprint, error) {
	footprints := map[int]*affiliateRiskFootprint{}
	for _, id := range ids {
		footprints[id] = &affiliateRiskFootprint{ips: map[string]bool{}, userAgents: map[string]bool{}}
	}
	add := func(userId int, ip string, userAgent string) {
		footprint := footprints[userId]
		if footprint == nil {
			return
		}
		ip = normalizeAffiliateRiskIP(ip)
		if ip == "" {
			return
		}
		footprint.ips[ip] = true
		if userAgent = strings.TrimSpace(userAgent); userAgent != "" {
			footprint.userAgents[userAgent] = true
		}
	}
	type loginRow struct {
		UserId    int
		Ip        string
		UserAgent string
	}
	for chunk := range slices.Chunk(ids, affiliateRiskIdQueryChunk) {
		sessions := []loginRow{}
		if err := DB.Model(&UserSession{}).Select("user_id, ip, user_agent").Where("user_id IN ?", chunk).Scan(&sessions).Error; err != nil {
			return nil, err
		}
		for _, row := range sessions {
			add(row.UserId, row.Ip, row.UserAgent)
		}
		if LOG_DB == nil {
			continue
		}
		logins := []loginRow{}
		if err := LOG_DB.Table("audit_logs").Select("user_id, ip, user_agent").Where("user_id IN ? AND category = ?", chunk, AuditCategoryLogin).Scan(&logins).Error; err != nil {
			return nil, err
		}
		for _, row := range logins {
			add(row.UserId, row.Ip, row.UserAgent)
		}
	}
	if LOG_DB == nil {
		return footprints, nil
	}
	for _, id := range ids[:min(len(ids), affiliateRiskLogSampleUsers+1)] {
		ips := []string{}
		if err := LOG_DB.Model(&Log{}).Where("user_id = ? AND ip <> ?", id, "").Order("id DESC").Limit(affiliateRiskLogSampleRows).Pluck("ip", &ips).Error; err != nil {
			return nil, err
		}
		for _, ip := range ips {
			add(id, ip, "")
		}
	}
	return footprints, nil
}

// affiliateRiskColdIPs returns the IPs that appear on two or more evaluated
// accounts and on no more than affiliateRiskColdIPMaxUsers accounts overall.
// IPs shared with the inviter are checked first so the pair signal survives
// the popularity-check cap on very large promoters.
func affiliateRiskColdIPs(inviterId int, ids []int, footprints map[int]*affiliateRiskFootprint) (map[string]bool, error) {
	owners := map[string]map[int]bool{}
	for _, id := range ids {
		for ip := range footprints[id].ips {
			if owners[ip] == nil {
				owners[ip] = map[int]bool{}
			}
			owners[ip][id] = true
		}
	}
	candidates := []string{}
	for ip, users := range owners {
		if len(users) >= 2 && len(users) <= affiliateRiskColdIPMaxUsers {
			candidates = append(candidates, ip)
		}
	}
	slices.SortFunc(candidates, func(a, b string) int {
		aInviter, bInviter := owners[a][inviterId], owners[b][inviterId]
		if aInviter != bInviter {
			if aInviter {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	cold := map[string]bool{}
	for _, ip := range candidates[:min(len(candidates), affiliateRiskMaxPopularityChecks)] {
		seen := map[int]bool{}
		for id := range owners[ip] {
			seen[id] = true
		}
		sessionUsers := []int{}
		if err := DB.Model(&UserSession{}).Where("ip = ?", ip).Limit(affiliateRiskPopularitySample).Pluck("user_id", &sessionUsers).Error; err != nil {
			return nil, err
		}
		for _, id := range sessionUsers {
			seen[id] = true
		}
		if LOG_DB != nil && len(seen) <= affiliateRiskColdIPMaxUsers {
			logUsers := []int{}
			if err := LOG_DB.Model(&Log{}).Where("ip = ?", ip).Limit(affiliateRiskPopularitySample).Pluck("user_id", &logUsers).Error; err != nil {
				return nil, err
			}
			for _, id := range logUsers {
				seen[id] = true
			}
		}
		if len(seen) <= affiliateRiskColdIPMaxUsers {
			cold[ip] = true
		}
	}
	return cold, nil
}

func normalizeAffiliateRiskIP(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed := net.ParseIP(raw)
	if parsed == nil || parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsUnspecified() || parsed.IsLinkLocalUnicast() {
		return ""
	}
	return parsed.String()
}

func affiliateRiskNameTokens(user affiliateRiskUser) []string {
	tokens := []string{}
	username := strings.ToLower(strings.TrimSpace(user.Username))
	if !affiliateRiskGeneratedUsername.MatchString(username) {
		tokens = append(tokens, username)
	}
	if local, _, found := strings.Cut(strings.ToLower(strings.TrimSpace(user.Email)), "@"); found {
		tokens = append(tokens, local)
	}
	return slices.DeleteFunc(tokens, func(token string) bool { return len(token) < 4 })
}

// affiliateRiskNamesSimilar matches names such as lee-x / lee-s,
// rainc / rainc0627 and 1332537578 / us1332537578.
func affiliateRiskNamesSimilar(left []string, right []string) bool {
	for _, a := range left {
		for _, b := range right {
			if a == b {
				return true
			}
			shorter, longer := a, b
			if len(shorter) > len(longer) {
				shorter, longer = longer, shorter
			}
			if len(shorter) >= 4 && strings.Contains(longer, shorter) {
				return true
			}
			prefix := 0
			for prefix < len(shorter) && a[prefix] == b[prefix] {
				prefix++
			}
			if prefix >= 4 && len(a)-prefix <= 3 && len(b)-prefix <= 3 {
				return true
			}
			if affiliateRiskCommonDigitRun(a, b) >= 7 {
				return true
			}
		}
	}
	return false
}

func affiliateRiskCommonDigitRun(a string, b string) int {
	longest := 0
	previous := make([]int, len(b)+1)
	for i := range len(a) {
		current := make([]int, len(b)+1)
		for j := range len(b) {
			if a[i] == b[j] && a[i] >= '0' && a[i] <= '9' {
				current[j+1] = previous[j] + 1
				longest = max(longest, current[j+1])
			}
		}
		previous = current
	}
	return longest
}

// EvaluateAffiliateTransferRisk stores the requester's self-referral score on
// a transfer so reviewers see it in the pending list.
func EvaluateAffiliateTransferRisk(transferId int) error {
	transfer := &AffiliateTransfer{}
	if err := DB.Select("id", "user_id").First(transfer, transferId).Error; err != nil {
		return err
	}
	score, detail, err := EvaluateAffiliateInviterRisk(transfer.UserId)
	if err != nil {
		return err
	}
	encoded, err := common.Marshal(detail)
	if err != nil {
		return err
	}
	return DB.Model(&AffiliateTransfer{}).Where("id = ?", transferId).Updates(map[string]any{
		"risk_score":          score,
		"risk_detail":         string(encoded),
		"risk_evaluated_time": common.GetTimestamp(),
	}).Error
}

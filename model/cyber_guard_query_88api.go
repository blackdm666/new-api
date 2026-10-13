package model

import (
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// 88api cyberguard: read-side queries for the CyberGuard admin page.

var cyberGuardCaseIdPattern = regexp.MustCompile(`(?i)^CG-(\d+)$`)

type CyberGuardEventFilter struct {
	UserId   int
	Category string
	State    string
	Keyword  string
	Start    int64
	End      int64
}

func ListCyberGuardEvents(filter CyberGuardEventFilter, offset int, limit int) ([]*CyberGuardEvent, int64, error) {
	query := DB.Model(&CyberGuardEvent{})
	if filter.UserId > 0 {
		query = query.Where("user_id = ?", filter.UserId)
	}
	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	if filter.State != "" {
		query = query.Where("state = ?", filter.State)
	}
	if filter.Start > 0 {
		query = query.Where("occurred_at >= ?", filter.Start)
	}
	if filter.End > 0 {
		query = query.Where("occurred_at < ?", filter.End)
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		if match := cyberGuardCaseIdPattern.FindStringSubmatch(keyword); match != nil {
			banId, _ := strconv.Atoi(match[1])
			userId := -1
			if ban, err := GetCyberGuardBan(banId); err == nil {
				userId = ban.UserId
			}
			query = query.Where("user_id = ?", userId)
		} else {
			like := "%" + keyword + "%"
			query = query.Where("(username LIKE ? OR token_name LIKE ? OR sub2api_request_id = ?)", like, like, keyword)
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var events []*CyberGuardEvent
	err := query.Order("occurred_at desc, id desc").Offset(offset).Limit(limit).Find(&events).Error
	return events, total, err
}

func ListCyberGuardBans(status string, userId int, offset int, limit int) ([]*CyberGuardBan, int64, error) {
	query := DB.Model(&CyberGuardBan{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var bans []*CyberGuardBan
	err := query.Order("id desc").Offset(offset).Limit(limit).Find(&bans).Error
	return bans, total, err
}

// CyberGuardEmailDeliveryStates maps outbox delivery keys to their live state.
func CyberGuardEmailDeliveryStates(keys []string) (map[string]string, error) {
	states := make(map[string]string, len(keys))
	if len(keys) == 0 {
		return states, nil
	}
	var rows []EmailDelivery
	if err := DB.Model(&EmailDelivery{}).Select("delivery_key, state").Where("delivery_key IN ?", keys).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		states[row.DeliveryKey] = row.State
	}
	return states, nil
}

// ListCyberGuardAdoptable lists users in the ban group that have no active or
// observed CyberGuard record (bans made by hand before CyberGuard existed).
func ListCyberGuardAdoptable(banGroup string, limit int) ([]*CyberGuardUser, error) {
	open := DB.Model(&CyberGuardBan{}).Select("user_id").
		Where("status IN ?", []string{CyberGuardBanStatusActive, CyberGuardBanStatusObserved})
	var users []User
	err := selectCyberGuardUser(DB).
		Where(map[string]any{"group": banGroup}).
		Where("id NOT IN (?)", open).
		Order("id asc").Limit(limit).Find(&users).Error
	if err != nil {
		return nil, err
	}
	result := make([]*CyberGuardUser, 0, len(users))
	for i := range users {
		result = append(result, cyberGuardUserFrom(&users[i]))
	}
	return result, nil
}

// SearchCyberGuardUserIds matches any user by id, username or email.
func SearchCyberGuardUserIds(keyword string, limit int) ([]int, error) {
	like := "%" + keyword + "%"
	query := DB.Model(&User{})
	if id, err := strconv.Atoi(keyword); err == nil {
		query = query.Where("id = ? OR username LIKE ? OR email LIKE ?", id, like, like)
	} else {
		query = query.Where("username LIKE ? OR email LIKE ?", like, like)
	}
	var ids []int
	err := query.Order("id desc").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

type CyberGuardUserActivity struct {
	UserId    int   `json:"user_id"`
	LastHitAt int64 `json:"last_hit_at"`
}

// ListCyberGuardUserActivity returns every user that has attributed events or
// a CyberGuard record, with the time of their latest non-void event.
func ListCyberGuardUserActivity() ([]CyberGuardUserActivity, error) {
	var rows []CyberGuardUserActivity
	if err := DB.Model(&CyberGuardEvent{}).
		Select("user_id, MAX(occurred_at) AS last_hit_at").
		Where("user_id > 0 AND state <> ?", CyberGuardEventStateVoid).
		Group("user_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	seen := make(map[int]bool, len(rows))
	for _, row := range rows {
		seen[row.UserId] = true
	}
	var banUsers []int
	if err := DB.Model(&CyberGuardBan{}).Distinct().Pluck("user_id", &banUsers).Error; err != nil {
		return nil, err
	}
	for _, userId := range banUsers {
		if !seen[userId] {
			rows = append(rows, CyberGuardUserActivity{UserId: userId})
			seen[userId] = true
		}
	}
	return rows, nil
}

// CyberGuardOpenBansByUser returns each user's open record; an active ban
// wins over an observed proposal.
func CyberGuardOpenBansByUser(userIds []int) (map[int]*CyberGuardBan, error) {
	result := make(map[int]*CyberGuardBan, len(userIds))
	if len(userIds) == 0 {
		return result, nil
	}
	var bans []*CyberGuardBan
	if err := DB.Where("user_id IN ? AND status IN ?", userIds,
		[]string{CyberGuardBanStatusActive, CyberGuardBanStatusObserved}).Order("id asc").Find(&bans).Error; err != nil {
		return nil, err
	}
	for _, ban := range bans {
		current, ok := result[ban.UserId]
		if !ok || current.Status != CyberGuardBanStatusActive {
			result[ban.UserId] = ban
		}
	}
	return result, nil
}

func GetCyberGuardUsers(userIds []int) (map[int]*CyberGuardUser, error) {
	result := make(map[int]*CyberGuardUser, len(userIds))
	if len(userIds) == 0 {
		return result, nil
	}
	var users []User
	if err := selectCyberGuardUser(DB).Where("id IN ?", userIds).Find(&users).Error; err != nil {
		return nil, err
	}
	for i := range users {
		result[users[i].Id] = cyberGuardUserFrom(&users[i])
	}
	return result, nil
}

type CyberGuardUnitRow struct {
	UserId     int
	UnitKey    string
	OccurredAt int64
}

func ListCyberGuardCountedUnits(userIds []int, since int64) ([]CyberGuardUnitRow, error) {
	var rows []CyberGuardUnitRow
	if len(userIds) == 0 {
		return rows, nil
	}
	err := DB.Model(&CyberGuardEvent{}).Select("user_id, unit_key, occurred_at").
		Where("user_id IN ? AND state = ? AND occurred_at > ?", userIds, CyberGuardEventStateCounted, since).
		Scan(&rows).Error
	return rows, err
}

func CyberGuardCountingFloors(userIds []int) (map[int]int64, error) {
	floors := make(map[int]int64, len(userIds))
	if len(userIds) == 0 {
		return floors, nil
	}
	var rows []struct {
		UserId  int
		FloorAt int64
	}
	err := DB.Model(&CyberGuardBan{}).Select("user_id, MAX(ended_at) AS floor_at").
		Where("user_id IN ? AND status IN ? AND end_reason <> ?", userIds,
			[]string{CyberGuardBanStatusEnded, CyberGuardBanStatusDismissed}, CyberGuardEndReasonPromoted).
		Group("user_id").Scan(&rows).Error
	for _, row := range rows {
		floors[row.UserId] = row.FloorAt
	}
	return floors, err
}

type CyberGuardUserHits struct {
	UserId    int
	Hits      int64
	LastHitAt int64
}

// CyberGuardHitsByUser counts non-void upstream refusals (session blocks
// excluded) per user since the given time.
func CyberGuardHitsByUser(userIds []int, since int64) (map[int]CyberGuardUserHits, error) {
	result := make(map[int]CyberGuardUserHits, len(userIds))
	if len(userIds) == 0 {
		return result, nil
	}
	var rows []CyberGuardUserHits
	err := DB.Model(&CyberGuardEvent{}).Select("user_id, COUNT(*) AS hits, MAX(occurred_at) AS last_hit_at").
		Where("user_id IN ? AND state <> ? AND category <> ? AND occurred_at > ?", userIds,
			CyberGuardEventStateVoid, "session_blocked", since).
		Group("user_id").Scan(&rows).Error
	for _, row := range rows {
		result[row.UserId] = row
	}
	return result, err
}

type CyberGuardOverviewCounts struct {
	HitsToday      int64 `json:"hits_today"`
	Hits7d         int64 `json:"hits_7d"`
	Units7d        int64 `json:"units_7d"`
	UsersFlagged7d int64 `json:"users_flagged_7d"`
	ActiveBans     int64 `json:"active_bans"`
	ObservedOpen   int64 `json:"observed_open"`
	Unattributed7d int64 `json:"unattributed_7d"`
	Pending        int64 `json:"pending"`
}

type CyberGuardReconciliation struct {
	Sub2apiRows  int64 `json:"sub2api_rows"`
	Attributed   int64 `json:"attributed"`
	Unattributed int64 `json:"unattributed"`
	Pending      int64 `json:"pending"`
}

func GetCyberGuardOverviewCounts(now int64, todayStart int64) (CyberGuardOverviewCounts, CyberGuardReconciliation, error) {
	var counts CyberGuardOverviewCounts
	var recon CyberGuardReconciliation
	week := now - 7*86400
	day := now - 86400
	hits := func() *gorm.DB {
		return DB.Model(&CyberGuardEvent{}).Where("state <> ? AND category <> ?", CyberGuardEventStateVoid, "session_blocked")
	}
	events := func() *gorm.DB { return DB.Model(&CyberGuardEvent{}) }
	var units []struct {
		UserId  int
		UnitKey string
	}
	steps := []error{
		hits().Where("occurred_at >= ?", todayStart).Count(&counts.HitsToday).Error,
		hits().Where("occurred_at > ?", week).Count(&counts.Hits7d).Error,
		events().Distinct("user_id", "unit_key").Where("state = ? AND occurred_at > ?", CyberGuardEventStateCounted, week).
			Scan(&units).Error,
		events().Where("state = ? AND occurred_at > ?", CyberGuardEventStateCounted, week).Distinct("user_id").Count(&counts.UsersFlagged7d).Error,
		DB.Model(&CyberGuardBan{}).Where("status = ?", CyberGuardBanStatusActive).Count(&counts.ActiveBans).Error,
		DB.Model(&CyberGuardBan{}).Where("status = ?", CyberGuardBanStatusObserved).Count(&counts.ObservedOpen).Error,
		events().Where("state = ? AND occurred_at > ?", CyberGuardEventStateUnattributed, week).Count(&counts.Unattributed7d).Error,
		events().Where("state = ?", CyberGuardEventStatePending).Count(&counts.Pending).Error,
		events().Where("occurred_at > ?", day).Count(&recon.Sub2apiRows).Error,
		events().Where("occurred_at > ? AND user_id > 0", day).Count(&recon.Attributed).Error,
		events().Where("occurred_at > ? AND state = ?", day, CyberGuardEventStateUnattributed).Count(&recon.Unattributed).Error,
		events().Where("occurred_at > ? AND state = ?", day, CyberGuardEventStatePending).Count(&recon.Pending).Error,
	}
	for _, err := range steps {
		if err != nil {
			return counts, recon, err
		}
	}
	counts.Units7d = int64(len(units))
	return counts, recon, nil
}

type CyberGuardAccountStat struct {
	AccountId   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	HitsCyber   int64  `json:"hits_cyber"`
	HitsBio     int64  `json:"hits_bio"`
	HitsUsage   int64  `json:"hits_usage"`
	HitsTotal   int64  `json:"-"`
	Users       int64  `json:"users"`
	LastHitAt   int64  `json:"last_hit_at"`
}

// CyberGuardAccountStats aggregates non-void upstream refusals per sub2api
// account (OpenAI 号). Session blocks have no account and are skipped.
func CyberGuardAccountStats(since int64, limit int) ([]CyberGuardAccountStat, error) {
	var rows []CyberGuardAccountStat
	err := DB.Model(&CyberGuardEvent{}).
		Select(`account_id, MAX(account_name) AS account_name,
			SUM(CASE WHEN category = 'cyber' THEN 1 ELSE 0 END) AS hits_cyber,
			SUM(CASE WHEN category = 'bio' THEN 1 ELSE 0 END) AS hits_bio,
			SUM(CASE WHEN category = 'usage' THEN 1 ELSE 0 END) AS hits_usage,
			COUNT(*) AS hits_total,
			COUNT(DISTINCT CASE WHEN user_id > 0 THEN user_id END) AS users,
			MAX(occurred_at) AS last_hit_at`).
		Where("account_id > 0 AND state <> ? AND category <> ? AND occurred_at > ?",
			CyberGuardEventStateVoid, "session_blocked", since).
		Group("account_id").Order("hits_total desc, last_hit_at desc").Limit(limit).Scan(&rows).Error
	return rows, err
}

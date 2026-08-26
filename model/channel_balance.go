package model

import (
	"database/sql/driver"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

const (
	ChannelBalanceUnitMoney    = "money"
	ChannelBalanceUnitTokens   = "tokens"
	ChannelBalanceUnitCredits  = "credits"
	ChannelBalanceUnitRequests = "requests"
)

func HasAutoRefreshBalanceChannels() bool {
	var channels []Channel
	if err := DB.Select("id", "settings").Where("status = ?", common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to check automatic channel balance refresh: %v", err))
		return false
	}
	for _, channel := range channels {
		settings := dto.ChannelOtherSettings{}
		if common.UnmarshalJsonStr(channel.OtherSettings, &settings) == nil && settings.BalanceQuery != nil && settings.BalanceQuery.AutoRefresh {
			return true
		}
	}
	return false
}

// ChannelBalanceInfo preserves the upstream unit and meaning. The legacy
// Channel.Balance field remains USD-only for compatibility with older clients.
type ChannelBalanceInfo struct {
	Remaining                string `json:"remaining,omitempty"`
	Total                    string `json:"total,omitempty"`
	Used                     string `json:"used,omitempty"`
	Unit                     string `json:"unit,omitempty"`
	Currency                 string `json:"currency,omitempty"`
	DisplayUnit              string `json:"display_unit,omitempty"`
	MetricKind               string `json:"metric_kind,omitempty"`
	Source                   string `json:"source,omitempty"`
	Unlimited                bool   `json:"unlimited"`
	UpdatedAt                int64  `json:"updated_at"`
	LowBalanceAlertActive    bool   `json:"low_balance_alert_active,omitempty"`
	LowBalanceAlertVersion   int64  `json:"low_balance_alert_version,omitempty"`
	LowBalanceAlertedAt      int64  `json:"low_balance_alerted_at,omitempty"`
	LowBalanceAlertThreshold string `json:"low_balance_alert_threshold,omitempty"`
}

func (info ChannelBalanceInfo) Value() (driver.Value, error) {
	return common.Marshal(&info)
}

func (info *ChannelBalanceInfo) Scan(value any) error {
	if value == nil {
		*info = ChannelBalanceInfo{}
		return nil
	}
	var data []byte
	switch typed := value.(type) {
	case []byte:
		data = typed
	case string:
		data = []byte(typed)
	default:
		return fmt.Errorf("unsupported channel balance info value type %T", value)
	}
	if len(data) == 0 {
		*info = ChannelBalanceInfo{}
		return nil
	}
	return common.Unmarshal(data, info)
}

func (channel *Channel) UpdateBalanceInfo(info ChannelBalanceInfo, legacyBalanceUSD *float64) error {
	if channel.BalanceInfo != nil {
		info.LowBalanceAlertActive = channel.BalanceInfo.LowBalanceAlertActive
		info.LowBalanceAlertVersion = channel.BalanceInfo.LowBalanceAlertVersion
		info.LowBalanceAlertedAt = channel.BalanceInfo.LowBalanceAlertedAt
		info.LowBalanceAlertThreshold = channel.BalanceInfo.LowBalanceAlertThreshold
	}
	updates := map[string]any{
		"balance_info":         info,
		"balance_updated_time": info.UpdatedAt,
	}
	if legacyBalanceUSD != nil {
		updates["balance"] = *legacyBalanceUSD
	}
	if err := DB.Model(channel).Updates(updates).Error; err != nil {
		return err
	}
	channel.BalanceInfo = &info
	channel.BalanceUpdatedTime = info.UpdatedAt
	if legacyBalanceUSD != nil {
		channel.Balance = *legacyBalanceUSD
	}
	return nil
}

func (channel *Channel) UpdateBalanceAlertState(active bool, version int64, alertedAt int64, threshold string) error {
	if channel.BalanceInfo == nil {
		return nil
	}
	info := *channel.BalanceInfo
	info.LowBalanceAlertActive = active
	info.LowBalanceAlertVersion = version
	info.LowBalanceAlertedAt = alertedAt
	info.LowBalanceAlertThreshold = threshold
	if err := DB.Model(channel).Update("balance_info", info).Error; err != nil {
		return err
	}
	channel.BalanceInfo = &info
	return nil
}

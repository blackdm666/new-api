package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/shopspring/decimal"
)

const channelBalanceRefreshTaskInterval = time.Minute

type channelBalanceRefreshHandler struct{}

func (channelBalanceRefreshHandler) Type() string { return model.SystemTaskTypeChannelBalance }

func (channelBalanceRefreshHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("CHANNEL_BALANCE_REFRESH_TASK_ENABLED", true) && model.HasAutoRefreshBalanceChannels()
}

func (channelBalanceRefreshHandler) Interval() time.Duration {
	return channelBalanceRefreshTaskInterval
}

func (channelBalanceRefreshHandler) NewPayload() any { return nil }

type channelBalanceRefreshSummary struct {
	Eligible    int   `json:"eligible"`
	Due         int   `json:"due"`
	Updated     int   `json:"updated"`
	Failed      int   `json:"failed"`
	Notified    int   `json:"notified"`
	AlertFailed int   `json:"alert_failed"`
	FailedIDs   []int `json:"failed_ids,omitempty"`
}

func (channelBalanceRefreshHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := runChannelBalanceRefreshTask(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	status := model.SystemTaskStatusSucceeded
	if err != nil {
		status = model.SystemTaskStatusFailed
	}
	finishSystemTaskHandler(task, runnerID, status, summary, err)
}

func runChannelBalanceRefreshTask(ctx context.Context, report func(processed, total int)) (channelBalanceRefreshSummary, error) {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return channelBalanceRefreshSummary{}, err
	}
	now := common.GetTimestamp()
	dueChannels := make([]*model.Channel, 0)
	eligible := 0
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled || channel.ChannelInfo.IsMultiKey {
			continue
		}
		config := channel.GetOtherSettings().BalanceQuery
		if config == nil || !config.AutoRefresh || strings.TrimSpace(config.Mode) == dto.ChannelBalanceQueryModeDisabled {
			continue
		}
		eligible++
		minutes := config.RefreshMinutes
		if minutes < 1 {
			minutes = 15
		}
		if channel.BalanceUpdatedTime > 0 && now-channel.BalanceUpdatedTime < int64(minutes*60) {
			continue
		}
		dueChannels = append(dueChannels, channel)
	}

	summary := channelBalanceRefreshSummary{Eligible: eligible, Due: len(dueChannels)}
	for index, channel := range dueChannels {
		if ctx.Err() != nil {
			return summary, ctx.Err()
		}
		result, queryErr := updateChannelBalance(channel)
		if queryErr != nil || result.RawResponse != "" || result.Info == nil {
			summary.Failed++
			summary.FailedIDs = append(summary.FailedIDs, channel.Id)
		} else {
			summary.Updated++
			config := channel.GetOtherSettings().BalanceQuery
			notified, alertErr := updateChannelLowBalanceAlert(channel, config, result.Info)
			if alertErr != nil {
				summary.AlertFailed++
			} else if notified {
				summary.Notified++
			}
		}
		if report != nil {
			report(index+1, len(dueChannels))
		}
		if index+1 < len(dueChannels) && common.RequestInterval > 0 {
			timer := time.NewTimer(common.RequestInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return summary, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return summary, nil
}

func updateChannelLowBalanceAlert(channel *model.Channel, config *dto.ChannelBalanceQueryConfig, info *model.ChannelBalanceInfo) (bool, error) {
	if channel == nil || config == nil || info == nil || channel.BalanceInfo == nil {
		return false, nil
	}
	current := channel.BalanceInfo
	if !config.LowBalanceAlert {
		if current.LowBalanceAlertActive || current.LowBalanceAlertThreshold != "" {
			return false, channel.UpdateBalanceAlertState(false, current.LowBalanceAlertVersion, current.LowBalanceAlertedAt, "")
		}
		return false, nil
	}
	thresholdText := strings.TrimSpace(config.LowBalanceThreshold)
	threshold, err := decimal.NewFromString(thresholdText)
	if err != nil || !threshold.IsPositive() {
		return false, errors.New("invalid low balance alert threshold")
	}
	remaining, err := decimal.NewFromString(strings.TrimSpace(info.Remaining))
	if err != nil {
		return false, fmt.Errorf("invalid channel remaining balance: %w", err)
	}
	thresholdChanged := current.LowBalanceAlertThreshold != thresholdText
	active := current.LowBalanceAlertActive && !thresholdChanged
	if info.Unlimited || !remaining.LessThan(threshold) {
		if active || thresholdChanged {
			return false, channel.UpdateBalanceAlertState(false, current.LowBalanceAlertVersion, current.LowBalanceAlertedAt, thresholdText)
		}
		return false, nil
	}
	if active {
		return false, nil
	}

	root := model.GetRootUser()
	if root == nil || root.Id <= 0 {
		return false, errors.New("root administrator is not configured")
	}
	nextVersion := current.LowBalanceAlertVersion + 1
	displayUnit := strings.TrimSpace(info.DisplayUnit)
	currentDisplay := displayUnit + remaining.String()
	thresholdDisplay := displayUnit + threshold.String()
	subject := fmt.Sprintf("渠道余额预警：%s（#%d）", channel.Name, channel.Id)
	content := fmt.Sprintf("渠道「%s」（#%d）的上游余额已低于预警阈值。\n当前余额：%s\n预警阈值：%s\n余额类型：%s\n请及时检查上游账户并补充余额。", channel.Name, channel.Id, currentDisplay, thresholdDisplay, info.MetricKind)
	baseUser := root.ToBaseUser()
	deliveryKey := fmt.Sprintf("channel-low-balance:%d:%d", channel.Id, nextVersion)
	if err := service.NotifyUserWithDeliveryKey(baseUser.Id, baseUser.Email, baseUser.GetSetting(), dto.NewNotify(dto.NotifyTypeInspectionAlert, subject, content, nil), deliveryKey); err != nil {
		_ = channel.UpdateBalanceAlertState(false, current.LowBalanceAlertVersion, current.LowBalanceAlertedAt, thresholdText)
		return false, err
	}
	now := common.GetTimestamp()
	if err := channel.UpdateBalanceAlertState(true, nextVersion, now, thresholdText); err != nil {
		return false, err
	}
	return true, nil
}

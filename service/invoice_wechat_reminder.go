package service

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/bytedance/gopkg/util/gopool"
)

// 88api: on every mainland working day the finance group gets one reminder
// listing the pending invoice requests submitted before that day.
const (
	invoiceWechatReminderInterval  = time.Minute
	invoiceWechatReminderStartHour = 9
	// A node that starts late still reminds in the morning, never at night.
	invoiceWechatReminderEndHour  = 12
	invoiceWechatReminderMaxLines = 20
)

var (
	// The holiday-cn data set follows the State Council holiday notices,
	// including the weekend days that become working days. A year that has
	// not been announced yet comes back with no days.
	chinaHolidayCalendarURL = "https://cdn.jsdelivr.net/gh/NateScarlet/holiday-cn@master/%d.json"
	chinaHolidayCalendar    = &holidayCalendarCache{years: map[int]holidayCalendarYear{}}
)

type holidayCalendarYear struct {
	// offDays maps a date (2006-01-02) to whether it is a day off; dates that
	// are not listed follow the normal Monday-Friday week.
	offDays   map[string]bool
	fetchedAt time.Time
}

type holidayCalendarCache struct {
	mu    sync.Mutex
	years map[int]holidayCalendarYear
}

type holidayCalendarFile struct {
	Days []struct {
		Date     string `json:"date"`
		IsOffDay bool   `json:"isOffDay"`
	} `json:"days"`
}

// isChinaWorkingDay reports whether day (in Asia/Shanghai) is a working day.
// Without calendar data it falls back to Monday-Friday.
func (cache *holidayCalendarCache) isChinaWorkingDay(day time.Time) bool {
	date := day.Format("2006-01-02")
	if offDay, listed := cache.yearDays(day.Year())[date]; listed {
		return !offDay
	}
	return day.Weekday() != time.Saturday && day.Weekday() != time.Sunday
}

func (cache *holidayCalendarCache) yearDays(year int) map[string]bool {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cached, ok := cache.years[year]
	// Refresh daily so a newly announced year, or a failed fetch, is picked up.
	if ok && time.Since(cached.fetchedAt) < 24*time.Hour {
		return cached.offDays
	}
	offDays, err := fetchHolidayCalendarYear(year)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to fetch the %d holiday calendar, using Monday-Friday: %s", year, err.Error()))
		if ok {
			offDays = cached.offDays
		}
		// Retry in an hour instead of on every tick.
		cache.years[year] = holidayCalendarYear{offDays: offDays, fetchedAt: time.Now().Add(-23 * time.Hour)}
		return offDays
	}
	cache.years[year] = holidayCalendarYear{offDays: offDays, fetchedAt: time.Now()}
	return offDays
}

func fetchHolidayCalendarYear(year int) (map[string]bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(chinaHolidayCalendarURL, year), nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 256*1024))
	if err != nil {
		return nil, err
	}
	var file holidayCalendarFile
	if err := common.Unmarshal(body, &file); err != nil {
		return nil, err
	}
	offDays := make(map[string]bool, len(file.Days))
	for _, day := range file.Days {
		offDays[day.Date] = day.IsOffDay
	}
	return offDays, nil
}

func StartInvoiceWechatReminder() {
	if !common.IsMasterNode {
		return
	}
	gopool.Go(func() {
		ticker := time.NewTicker(invoiceWechatReminderInterval)
		defer ticker.Stop()
		for range ticker.C {
			queueInvoiceWechatReminder(time.Now())
		}
	})
}

// queueInvoiceWechatReminder queues today's reminder once, keyed by date and
// group, between 09:00 and 12:00 on a working day when some pending request
// was submitted before today.
func queueInvoiceWechatReminder(now time.Time) {
	if !invoiceWechatNotifyReady() {
		return
	}
	local := now.In(invoiceWechatLocation)
	if local.Hour() < invoiceWechatReminderStartHour || local.Hour() >= invoiceWechatReminderEndHour {
		return
	}
	if !chinaHolidayCalendar.isChinaWorkingDay(local) {
		return
	}
	requests, err := model.ListPendingInvoiceRequests()
	if err != nil {
		common.SysError("failed to list pending invoice requests for the reminder: " + err.Error())
		return
	}
	startOfDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, invoiceWechatLocation).Unix()
	overdue := make([]*model.InvoiceRequest, 0, len(requests))
	for _, request := range requests {
		if request.CreatedTime > 0 && request.CreatedTime < startOfDay {
			overdue = append(overdue, request)
		}
	}
	if len(overdue) == 0 {
		return
	}
	delivery := newInvoiceWechatReminderDelivery(local.Format("2006-01-02"), overdue)
	if _, created, err := model.EnqueueInvoiceNotification(delivery); err != nil {
		common.SysError("failed to queue the invoice reminder: " + err.Error())
	} else if created {
		WakeInvoiceWechatDelivery()
	}
}

// newInvoiceWechatReminderDelivery files the reminder under the oldest
// pending request, where administrators can see and retry it.
func newInvoiceWechatReminderDelivery(date string, overdue []*model.InvoiceRequest) *model.InvoiceNotificationDelivery {
	groupID := strings.TrimSpace(setting.InvoiceWechatGroupId)
	seed := fmt.Sprintf("invoice-reminder:%s:wechat:%s", date, groupID)
	return &model.InvoiceNotificationDelivery{
		DeliveryKey:      hex.EncodeToString(common.Sha256Raw([]byte(seed))),
		InvoiceRequestId: overdue[0].Id,
		Kind:             model.InvoiceNotificationKindWechatGroup,
		UserId:           overdue[0].UserId,
		Recipient:        groupID,
		Subject:          fmt.Sprintf("开票提醒 %s", date),
		Body:             buildInvoiceWechatReminderText(overdue),
	}
}

func buildInvoiceWechatReminderText(overdue []*model.InvoiceRequest) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "【开票提醒】有 %d 张发票申请还未完成，请及时跟进：", len(overdue))
	for index, request := range overdue {
		if index == invoiceWechatReminderMaxLines {
			fmt.Fprintf(&builder, "\n……另有 %d 张，请到后台「发票管理」查看", len(overdue)-index)
			break
		}
		fmt.Fprintf(&builder, "\n#%d %s ¥%s（%s 申请）",
			request.Id, request.CompanyName, formatInvoiceAmountCNY(invoiceTotalCents(request)),
			formatInvoiceWechatTime(request.CreatedTime))
	}
	return builder.String()
}

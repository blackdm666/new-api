package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"html"
	"net/mail"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const UserNoticeCategory = "user_notice"
const UserNoticePerMinute = 5

var ErrUserNoticeInvalid = errors.New("invalid notification: select 1–100 existing users and provide a subject and message")
var ErrUserNoticeConflict = errors.New("notification changed or request key was reused; refresh before trying again")
var ErrUserNoticeNoRecipients = errors.New("no eligible recipients; check email addresses and delivery restrictions")

// UserNotice keeps the administrator's plain-text message and masked recipient
// snapshot, independently of the outbox's message-content retention policy.
type UserNotice struct {
	Id            int                   `json:"id"`
	Kind          string                `json:"kind" gorm:"type:varchar(24);not null"`
	Subject       string                `json:"subject" gorm:"type:varchar(512);not null"`
	Body          string                `json:"body" gorm:"type:text;not null"`
	Status        string                `json:"status" gorm:"type:varchar(24);not null;index"`
	RecipientData string                `json:"-" gorm:"type:text;not null"`
	CreatedBy     int                   `json:"created_by" gorm:"not null"`
	UpdatedBy     int                   `json:"updated_by" gorm:"not null"`
	CreatedTime   int64                 `json:"created_at" gorm:"bigint;autoCreateTime;index"`
	UpdatedTime   int64                 `json:"updated_at" gorm:"bigint;autoUpdateTime"`
	Recipients    []UserNoticeRecipient `json:"recipients" gorm:"-"`
	Operator      string                `json:"operator" gorm:"-"`
}

// Request receipts are transactional with drafts and outbox insertion. An HTTP
// retry with the same key cannot create a second send, even on another instance.
type UserNoticeRequest struct {
	RequestKey  string `gorm:"type:varchar(64);primaryKey"`
	Digest      string `gorm:"type:varchar(64);not null"`
	NoticeId    int    `gorm:"not null;index"`
	CreatedTime int64  `gorm:"bigint;autoCreateTime"`
}

type UserNoticeRecipient struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	EmailMasked string `json:"email_masked"`
	Group       string `json:"group"`
	Disabled    bool   `json:"disabled"`
	SkipReason  string `json:"skip_reason"`
	DeliveryID  int    `json:"delivery_id,omitempty"`
	State       string `json:"state,omitempty"`
	SMTPProfile string `json:"smtp_profile,omitempty"`
	SMTPChannel string `json:"smtp_channel,omitempty"`
	LastError   string `json:"last_error,omitempty"`
	Email       string `json:"-"`
}

type UserNoticeInput struct {
	Id         int    `json:"id"`
	Kind       string `json:"kind"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	UserIDs    []int  `json:"user_ids"`
	Action     string `json:"action"`
	RequestKey string `json:"request_key"`
}

func (input *UserNoticeInput) Validate() error {
	input.Subject, input.Body = strings.TrimSpace(input.Subject), strings.TrimSpace(input.Body)
	if input.Id < 0 || !slices.Contains([]string{"maintenance", "usage", "violation", "service"}, input.Kind) ||
		!slices.Contains([]string{"draft", "send"}, input.Action) ||
		input.Subject == "" || utf8.RuneCountInString(input.Subject) > 120 || strings.ContainsAny(input.Subject, "\r\n") ||
		input.Body == "" || utf8.RuneCountInString(input.Body) > 5000 ||
		len(input.UserIDs) == 0 || len(input.UserIDs) > 100 || len(input.RequestKey) < 8 || len(input.RequestKey) > 100 {
		return ErrUserNoticeInvalid
	}
	for _, id := range input.UserIDs {
		if id <= 0 {
			return ErrUserNoticeInvalid
		}
	}
	input.UserIDs = slices.Clone(input.UserIDs)
	slices.Sort(input.UserIDs)
	input.UserIDs = slices.Compact(input.UserIDs)
	return nil
}

func userNoticeRecipient(tx *gorm.DB, user *User) (UserNoticeRecipient, error) {
	email := strings.TrimSpace(user.Email)
	row := UserNoticeRecipient{Id: user.Id, Username: user.Username, DisplayName: user.DisplayName,
		EmailMasked: maskEmailAddress(email), Email: email, Group: user.Group, Disabled: user.Status != common.UserStatusEnabled}
	if email == "" {
		row.SkipReason = "missing_email"
		return row, nil
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || strings.ContainsAny(email, "\r\n") {
		row.SkipReason = "invalid_email"
		return row, nil
	}
	var count int64
	// Conservatively retain existing administrator/provider suppression rules.
	// This feature never removes suppressions or re-enables a blocked mailbox.
	err = tx.Model(&MarketingSuppression{}).
		Where("user_id = ? OR email_hash = ?", user.Id, hashMarketingValue(strings.ToLower(email))).Count(&count).Error
	if count > 0 {
		row.SkipReason = "blocked_email"
	}
	return row, err
}

func SearchUserNoticeRecipients(keyword string) ([]UserNoticeRecipient, error) {
	keyword = strings.TrimSpace(keyword)
	if utf8.RuneCountInString(keyword) > 100 {
		return nil, ErrUserNoticeInvalid
	}
	query := DB.Model(&User{}).Select("id", "username", "display_name", "email", "group", "status")
	if keyword != "" {
		id, _ := strconv.Atoi(keyword)
		like := "%" + strings.ToLower(keyword) + "%"
		query = query.Where("id = ? OR LOWER(username) LIKE ? OR LOWER(display_name) LIKE ? OR LOWER(email) LIKE ?", id, like, like, like)
	}
	var users []User
	if err := query.Order("id DESC").Limit(50).Find(&users).Error; err != nil {
		return nil, err
	}
	rows := make([]UserNoticeRecipient, 0, len(users))
	for i := range users {
		row, err := userNoticeRecipient(DB, &users[i])
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func SubmitUserNotice(input UserNoticeInput, actor int) (*UserNotice, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if actor <= 0 {
		return nil, ErrUserNoticeInvalid
	}
	payload, err := common.Marshal(input)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%s", actor, input.RequestKey))))
	digest := fmt.Sprintf("%x", sha256.Sum256(payload))
	var noticeID int
	err = DB.Transaction(func(tx *gorm.DB) error {
		receipt := UserNoticeRequest{RequestKey: key, Digest: digest}
		insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&receipt)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			if err := tx.First(&receipt, "request_key = ?", key).Error; err != nil {
				return err
			}
			if receipt.Digest != digest {
				return ErrUserNoticeConflict
			}
			noticeID = receipt.NoticeId
			return nil
		}
		notice := UserNotice{Kind: input.Kind, Subject: input.Subject, Body: input.Body,
			Status: "draft", CreatedBy: actor, UpdatedBy: actor}
		if input.Id > 0 {
			if err := lockForUpdate(tx).First(&notice, input.Id).Error; err != nil {
				return err
			}
			if notice.Status != "draft" {
				return ErrUserNoticeConflict
			}
			notice.Kind, notice.Subject, notice.Body, notice.UpdatedBy = input.Kind, input.Subject, input.Body, actor
		}
		var users []User
		if err := tx.Select("id", "username", "display_name", "email", "group", "status").Where("id IN ?", input.UserIDs).Order("id ASC").Find(&users).Error; err != nil {
			return err
		}
		if len(users) != len(input.UserIDs) {
			return ErrUserNoticeInvalid
		}
		recipients := make([]UserNoticeRecipient, 0, len(users))
		eligible := 0
		addresses := make(map[string]bool)
		for i := range users {
			row, err := userNoticeRecipient(tx, &users[i])
			if err != nil {
				return err
			}
			address := strings.ToLower(row.Email)
			if row.SkipReason == "" && addresses[address] {
				row.SkipReason = "duplicate_email"
			}
			if row.SkipReason == "" {
				eligible++
				addresses[address] = true
			}
			recipients = append(recipients, row)
		}
		if eligible == 0 {
			return ErrUserNoticeNoRecipients
		}
		snapshot, err := common.Marshal(recipients)
		if err != nil {
			return err
		}
		notice.RecipientData = string(snapshot)
		if input.Action == "send" {
			notice.Status = "queued"
		}
		if err := tx.Save(&notice).Error; err != nil {
			return err
		}
		noticeID = notice.Id
		if input.Action == "send" {
			// Plain text is always HTML-escaped. No arbitrary HTML, tracking pixel,
			// marketing action button or recipient list is inserted into the email.
			body := `<div style="white-space:pre-wrap;font-family:sans-serif;line-height:1.7">` + html.EscapeString(input.Body) + `</div>`
			for _, recipient := range recipients {
				if recipient.SkipReason != "" {
					continue
				}
				_, _, err := EnqueueEmailDeliveryTx(tx, &EmailDelivery{
					DeliveryKey: fmt.Sprintf("user-notice:%d:%d", notice.Id, recipient.Id),
					Category:    UserNoticeCategory, SMTPProfile: common.SMTPProfileSecurity,
					RelatedId: notice.Id, UserId: recipient.Id, Recipient: recipient.Email, RecipientMasked: recipient.EmailMasked,
					Subject: input.Subject, Body: body, Priority: 50,
				})
				if err != nil {
					return err
				}
			}
		}
		return tx.Model(&receipt).Update("notice_id", notice.Id).Error
	})
	if err != nil {
		return nil, err
	}
	rows, err := ListUserNotices(noticeID)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, gorm.ErrRecordNotFound
	}
	return &rows[0], nil
}

func ListUserNotices(id int) ([]UserNotice, error) {
	rows := []UserNotice{}
	query := DB.Order("id DESC").Limit(100)
	if id > 0 {
		query = query.Where("id = ?", id)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(rows))
	for i := range rows {
		if err := common.UnmarshalJsonStr(rows[i].RecipientData, &rows[i].Recipients); err != nil {
			return nil, err
		}
		rows[i].Operator = fmt.Sprintf("UID %d", rows[i].UpdatedBy)
		ids = append(ids, rows[i].Id)
	}
	if len(ids) == 0 {
		return rows, nil
	}
	var deliveries []EmailDelivery
	if err := DB.Select("id", "related_id", "user_id", "state", "smtp_profile", "smtp_channel", "last_error").
		Where("category = ? AND related_id IN ?", UserNoticeCategory, ids).Find(&deliveries).Error; err != nil {
		return nil, err
	}
	index := make(map[[2]int]EmailDelivery, len(deliveries))
	for _, delivery := range deliveries {
		index[[2]int{delivery.RelatedId, delivery.UserId}] = delivery
	}
	for i := range rows {
		for j := range rows[i].Recipients {
			recipient := &rows[i].Recipients[j]
			if delivery, ok := index[[2]int{rows[i].Id, recipient.Id}]; ok {
				recipient.DeliveryID, recipient.State = delivery.Id, delivery.State
				recipient.SMTPProfile, recipient.SMTPChannel, recipient.LastError = delivery.SMTPProfile, delivery.SMTPChannel, delivery.LastError
			}
		}
	}
	return rows, nil
}

// Recheck immediately before every SMTP attempt, including automatic/manual
// retries. Never send an old queued notice to a deleted or changed mailbox.
func UserNoticeDeliveryEligible(delivery *EmailDelivery) (bool, error) {
	var user User
	err := DB.Select("id", "username", "display_name", "email", "group", "status").First(&user, delivery.UserId).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	row, err := userNoticeRecipient(DB, &user)
	return err == nil && row.SkipReason == "" && strings.EqualFold(row.Email, delivery.Recipient), err
}

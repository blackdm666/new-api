package service

import (
	"errors"
	"fmt"
	"html"
	"net/mail"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
)

var (
	ErrNoUsableMarketingEmailAccount = errors.New("no verified marketing email account is available")
	ErrEmailReceiptEndpointDisabled  = errors.New("the EventBridge receipt interface must be enabled and verified before marketing delivery")
)

var marketingAccountScheduler = struct {
	sync.Mutex
	current map[int]int
}{current: map[int]int{}}

func SendMarketingEmailDelivery(delivery *model.EmailDelivery) error {
	if delivery == nil || delivery.Id <= 0 {
		return model.ErrEmailDeliveryIdInvalid
	}
	endpoint, err := model.GetEmailReceiptEndpoint()
	if err != nil {
		return err
	}
	if !endpoint.Enabled || !endpoint.TokenConfigured || endpoint.LastVerifiedTime <= 0 {
		return ErrEmailReceiptEndpointDisabled
	}
	accounts, err := model.ListUsableMarketingEmailSenderAccounts(common.GetTimestamp())
	if err != nil {
		return err
	}
	candidates, err := eligibleMarketingAccounts(accounts, delivery.Recipient)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return ErrNoUsableMarketingEmailAccount
	}

	var sendErrors []string
	accountRateLimited := false
	for len(candidates) > 0 {
		account := chooseWeightedMarketingAccount(candidates)
		candidates = removeMarketingAccount(candidates, account.Id)
		effectiveRPM, reserveErr := marketingAccountEffectiveRPM(account, delivery.Recipient)
		if reserveErr != nil {
			return reserveErr
		}
		reserved, reserveErr := reserveMarketingAccountMinute(account, effectiveRPM)
		if reserveErr != nil {
			return reserveErr
		}
		if !reserved {
			accountRateLimited = true
			continue
		}
		attempt, result, sendErr := sendEmailThroughMarketingAccount(delivery, account, model.EmailAttemptPurposeDelivery)
		if sendErr != nil {
			sendErrors = append(sendErrors, fmt.Sprintf("%s: %v", account.Name, sendErr))
			_ = model.MarkEmailDeliveryAttemptSubmissionFailure(attempt.Id, sendErr.Error(), common.GetTimestamp())
			continue
		}
		now := common.GetTimestamp()
		if err := model.MarkEmailDeliveryAttemptAccepted(attempt.Id, now); err != nil {
			return err
		}
		receiptDeadline := now + int64(setting.GetEmailDeliveryRules().ReceiptTimeoutHours)*3600
		if err := model.MarkEmailDeliveryAwaitingReceipt(delivery.Id, account.Id, attempt.Id, result.MessageID, now, receiptDeadline); err != nil {
			return err
		}
		return nil
	}
	if len(sendErrors) == 0 {
		if accountRateLimited {
			now := common.GetTimestamp()
			return model.DeferEmailDelivery(delivery.Id, (now/60+1)*60)
		}
		return ErrNoUsableMarketingEmailAccount
	}
	return errors.New(strings.Join(sendErrors, "; "))
}

func SendMarketingAccountTest(userId int, accountId int, requestedRecipient string) (string, *model.EmailDeliveryAttempt, error) {
	endpoint, err := model.GetEmailReceiptEndpoint()
	if err != nil {
		return "", nil, err
	}
	if !endpoint.Enabled || !endpoint.TokenConfigured {
		return "", nil, ErrEmailReceiptEndpointDisabled
	}
	account, err := model.GetEmailSenderAccount(accountId)
	if err != nil {
		return "", nil, err
	}
	if account.Profile != model.EmailSenderProfileMarketing || account.Provider != model.EmailSenderProviderAliyunEventBridge {
		return "", nil, model.ErrEmailSenderAccountInvalid
	}
	recipient, err := resolveMarketingTestRecipient(userId, requestedRecipient)
	if err != nil {
		return "", nil, err
	}
	systemName := common.SystemNameOrDefault()
	delivery := &model.EmailDelivery{
		Recipient: recipient,
		Subject:   fmt.Sprintf("[%s] marketing receipt test", systemName),
		Body: fmt.Sprintf(
			"<h2>Marketing sender test</h2><p>%s submitted this message through %s.</p><p>EventBridge must return the final delivery receipt before this account is enabled.</p>",
			html.EscapeString(systemName), html.EscapeString(account.Name),
		),
	}
	attempt, _, err := sendEmailThroughMarketingAccount(delivery, account, model.EmailAttemptPurposeAccountTest)
	if err != nil {
		if attempt != nil {
			_ = model.MarkEmailDeliveryAttemptSubmissionFailure(attempt.Id, err.Error(), common.GetTimestamp())
		}
		return "", attempt, err
	}
	now := common.GetTimestamp()
	if err := model.MarkEmailDeliveryAttemptAccepted(attempt.Id, now); err != nil {
		return "", attempt, err
	}
	if err := model.MarkEmailSenderAccountSMTPTestAccepted(account.Id, now); err != nil {
		return "", attempt, err
	}
	return recipient, attempt, nil
}

func sendEmailThroughMarketingAccount(delivery *model.EmailDelivery, account *model.EmailSenderAccount, purpose string) (*model.EmailDeliveryAttempt, common.SMTPDeliveryResult, error) {
	token, err := account.Token()
	if err != nil {
		return nil, common.SMTPDeliveryResult{}, err
	}
	messageID, err := common.GenerateEmailMessageID(account.From)
	if err != nil {
		return nil, common.SMTPDeliveryResult{}, err
	}
	notifyID := fmt.Sprintf("newapi-email-%s@%s", common.GetRandomString(32), senderDomain(account.From))
	attempt, err := model.CreateEmailDeliveryAttempt(delivery.Id, account, purpose, delivery.Recipient, messageID, notifyID)
	if err != nil {
		return nil, common.SMTPDeliveryResult{}, err
	}
	result, err := common.SendEmailViaAccount(common.SMTPAccountConfig{
		Name: common.SMTPChannelMarketing, Server: account.Server, Port: account.Port,
		Account: account.Account, From: account.From, Token: token,
		SSLEnabled: account.SSLEnabled, StartTLSEnabled: account.StartTLSEnabled,
		InsecureSkipVerify: account.InsecureSkipVerify, ForceAuthLogin: account.ForceAuthLogin,
	}, delivery.Subject, delivery.Recipient, delivery.Body, messageID, notifyID)
	return attempt, result, err
}

func eligibleMarketingAccounts(accounts []*model.EmailSenderAccount, recipient string) ([]*model.EmailSenderAccount, error) {
	now := common.GetTimestamp()
	recipientDomain := senderDomain(recipient)
	result := make([]*model.EmailSenderAccount, 0, len(accounts))
	for _, account := range accounts {
		if account == nil || account.Id <= 0 {
			continue
		}
		keys := []string{
			fmt.Sprintf("account:%d:%s", account.Id, recipientDomain),
			fmt.Sprintf("domain:%s:%s", senderDomain(account.From), recipientDomain),
			fmt.Sprintf("provider:%s:%s", strings.ToLower(account.Server), recipientDomain),
		}
		blocked := false
		for _, key := range keys {
			throttle, err := model.GetEmailDeliveryThrottle(key)
			if err == nil {
				if throttle.DisabledUntil > now {
					blocked = true
					break
				}
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		}
		if !blocked {
			result = append(result, account)
		}
	}
	return result, nil
}

func marketingAccountEffectiveRPM(account *model.EmailSenderAccount, recipient string) (int, error) {
	limit := account.RateLimitPerMinute
	if limit < 1 {
		limit = 1
	}
	recipientDomain := senderDomain(recipient)
	keys := []string{
		fmt.Sprintf("account:%d:%s", account.Id, recipientDomain),
		fmt.Sprintf("domain:%s:%s", senderDomain(account.From), recipientDomain),
		fmt.Sprintf("provider:%s:%s", strings.ToLower(account.Server), recipientDomain),
	}
	for _, key := range keys {
		throttle, err := model.GetEmailDeliveryThrottle(key)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if throttle.EffectiveRPM > 0 && throttle.EffectiveRPM < limit {
			limit = throttle.EffectiveRPM
		}
	}
	return limit, nil
}

func chooseWeightedMarketingAccount(accounts []*model.EmailSenderAccount) *model.EmailSenderAccount {
	marketingAccountScheduler.Lock()
	defer marketingAccountScheduler.Unlock()
	total := 0
	var selected *model.EmailSenderAccount
	selectedScore := 0
	for _, account := range accounts {
		weight := account.Weight
		if weight < 1 {
			weight = 1
		}
		total += weight
		marketingAccountScheduler.current[account.Id] += weight
		score := marketingAccountScheduler.current[account.Id]
		if selected == nil || score > selectedScore || score == selectedScore && account.Id < selected.Id {
			selected = account
			selectedScore = score
		}
	}
	marketingAccountScheduler.current[selected.Id] -= total
	return selected
}

func reserveMarketingAccountMinute(account *model.EmailSenderAccount, limit int) (bool, error) {
	if limit < 1 {
		limit = 1
	}
	now := common.GetTimestamp()
	return model.ReserveEmailDeliveryMinuteQuota(fmt.Sprintf("marketing-account-%d", account.Id), now/60*60, limit)
}

func removeMarketingAccount(accounts []*model.EmailSenderAccount, id int) []*model.EmailSenderAccount {
	result := make([]*model.EmailSenderAccount, 0, len(accounts)-1)
	for _, account := range accounts {
		if account.Id != id {
			result = append(result, account)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Id < result[j].Id })
	return result
}

func resolveMarketingTestRecipient(userId int, requestedRecipient string) (string, error) {
	recipient := strings.TrimSpace(requestedRecipient)
	if recipient == "" {
		user, err := model.GetUserById(userId, false)
		if err != nil {
			return "", err
		}
		recipient = strings.TrimSpace(user.Email)
	}
	if recipient == "" {
		return "", ErrSMTPTestRecipientRequired
	}
	parsed, err := mail.ParseAddress(recipient)
	if err != nil || !strings.EqualFold(parsed.Address, recipient) || strings.ContainsAny(recipient, ";\r\n") {
		return "", ErrSMTPTestRecipientInvalid
	}
	return recipient, nil
}

func senderDomain(address string) string {
	address = strings.TrimSpace(address)
	at := strings.LastIndex(address, "@")
	if at < 0 || at == len(address)-1 {
		return "invalid"
	}
	return strings.ToLower(address[at+1:])
}

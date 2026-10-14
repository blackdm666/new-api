package model

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TopUp struct {
	Id              int     `json:"id"`
	UserId          int     `json:"user_id" gorm:"index;index:idx_topups_marketing_audience,priority:3"`
	Amount          int64   `json:"amount"`
	Money           float64 `json:"money"`
	MoneyMinor      int64   `json:"money_minor" gorm:"type:bigint;default:0"`
	CreditedQuota   int     `json:"credited_quota" gorm:"type:int;default:0"`
	Currency        string  `json:"currency" gorm:"type:varchar(16);default:''"`
	TradeNo         string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string  `json:"payment_provider" gorm:"type:varchar(50);default:'';index:idx_topups_marketing_audience,priority:2"`
	CreateTime      int64   `json:"create_time"`
	CompleteTime    int64   `json:"complete_time" gorm:"index:idx_topups_marketing_audience,priority:4"`
	Status          string  `json:"status" gorm:"index:idx_topups_marketing_audience,priority:1"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodBalance      = "balance"
	PaymentMethodAntom        = "antom"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderBalance      = "balance"
	PaymentProviderAntom        = "antom"
)

func isPromotionalTopUpProvider(provider string) bool {
	return provider == PaymentProviderEpay || provider == PaymentProviderAntom
}

func validateAntomTopUpPayment(topUp *TopUp, paidAmountMinor int64, currency string) error {
	if topUp == nil {
		return ErrTopUpNotFound
	}
	if topUp.PaymentProvider != PaymentProviderAntom {
		return ErrPaymentMethodMismatch
	}
	if strings.ToUpper(strings.TrimSpace(topUp.Currency)) != strings.ToUpper(strings.TrimSpace(currency)) {
		return ErrTopUpCurrencyMismatch
	}
	if topUp.MoneyMinor <= 0 || topUp.MoneyMinor != paidAmountMinor {
		return ErrTopUpAmountMismatch
	}
	return nil
}

// ValidateAntomTopUpPayment verifies the immutable order identity, provider,
// amount and currency before a non-final notification is acknowledged. The
// final settlement path repeats these checks while holding the database lock.
func ValidateAntomTopUpPayment(tradeNo string, paidAmountMinor int64, currency string) error {
	if strings.TrimSpace(tradeNo) == "" {
		return ErrTopUpNotFound
	}
	if paidAmountMinor <= 0 {
		return ErrTopUpAmountMismatch
	}
	if strings.TrimSpace(currency) == "" {
		return ErrTopUpCurrencyMismatch
	}
	topUp := &TopUp{}
	if err := DB.Where("trade_no = ?", tradeNo).First(topUp).Error; err != nil {
		return ErrTopUpNotFound
	}
	return validateAntomTopUpPayment(topUp, paidAmountMinor, currency)
}

var (
	ErrPaymentMethodMismatch = errors.New("payment method mismatch")
	ErrTopUpNotFound         = errors.New("topup not found")
	ErrTopUpStatusInvalid    = errors.New("topup status invalid")
	ErrTopUpAmountMismatch   = errors.New("topup paid amount mismatch")
	ErrTopUpCurrencyMismatch = errors.New("topup paid currency mismatch")
	// The quota errors reach the web console, so they carry message keys.
	ErrInvalidTopUpQuota        error = common.NewMessage("Invalid top-up quota")
	ErrTopUpQuotaLimitExceeded  error = common.NewMessage("Top-up quota limit exceeded")
	ErrWalletQuotaLimitExceeded error = common.NewMessage("Wallet quota limit exceeded")
)

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func topUpQuotaMaxCurrent(creditedQuota int) (int, error) {
	if creditedQuota <= 0 || creditedQuota > common.MaxWalletQuota {
		return 0, ErrInvalidTopUpQuota
	}
	return common.MaxWalletQuota - creditedQuota, nil
}

// ValidateTopUpQuotaCapacity performs the user-facing pre-payment check. The
// settlement path repeats the same invariant with an atomic conditional
// update, because the wallet balance can change after checkout creation.
func ValidateTopUpQuotaCapacity(userId int, creditedQuota int) error {
	maxCurrentQuota, err := topUpQuotaMaxCurrent(creditedQuota)
	if err != nil {
		return err
	}

	var user User
	if err := DB.Select("quota").Where("id = ?", userId).First(&user).Error; err != nil {
		return err
	}
	if user.Quota > maxCurrentQuota {
		return ErrTopUpQuotaLimitExceeded
	}
	return nil
}

// creditTopUpQuota atomically enforces the wallet ceiling while adding quota.
// Keeping the predicate and increment in one UPDATE prevents two
// concurrent callbacks from both passing a separate read/check.
func creditTopUpQuota(tx *gorm.DB, userId int, creditedQuota int, updates map[string]any) error {
	maxCurrentQuota, err := topUpQuotaMaxCurrent(creditedQuota)
	if err != nil {
		return err
	}

	updateFields := make(map[string]any, len(updates)+1)
	maps.Copy(updateFields, updates)
	updateFields["quota"] = gorm.Expr("quota + ?", creditedQuota)

	result := tx.Model(&User{}).
		Where("id = ? AND quota <= ?", userId, maxCurrentQuota).
		Updates(updateFields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}

	var count int64
	if err := tx.Model(&User{}).Where("id = ?", userId).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return ErrTopUpQuotaLimitExceeded
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("payment order number not provided")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

// RechargeEpay 原子完成易支付订单：订单行锁、状态校验、成功更新与用户额度增加
// 在同一个事务内完成，因此同一订单的并发/重复回调（包括多实例部署下）最多充值一次。
// alreadyDone=true 表示订单此前已完成，本次为幂等重复回调。
// 进程内的 LockOrder 只是优化，正确性由本函数的数据库行锁保证。
func RechargeEpay(tradeNo string, actualPaymentMethod string, paidMoney float64, callerIp string) (alreadyDone bool, err error) {
	if tradeNo == "" {
		return false, errors.New("payment order number not provided")
	}
	if paidMoney <= 0 {
		return false, errors.New("invalid paid amount")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	var quotaToAdd int
	topUp := &TopUp{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if topUp.PaymentProvider != PaymentProviderEpay {
			return ErrPaymentMethodMismatch
		}
		expectedCents := decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromInt(100)).Round(0).IntPart()
		paidCents := decimal.NewFromFloat(paidMoney).Mul(decimal.NewFromInt(100)).Round(0).IntPart()
		if expectedCents != paidCents {
			return ErrTopUpAmountMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			alreadyDone = true
			tryCreateAffiliateCommissionForTopUpTx(tx, topUp)
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		if actualPaymentMethod != "" && topUp.PaymentMethod != actualPaymentMethod {
			topUp.PaymentMethod = actualPaymentMethod
		}
		topUp.Money = paidMoney
		var quotaErr error
		quotaToAdd, quotaErr = common.WalletQuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if quotaErr != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.CreditedQuota = quotaToAdd
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}
		if err := creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil); err != nil {
			return err
		}
		tryCreateAffiliateCommissionForTopUpTx(tx, topUp)
		return nil
	})
	if err != nil {
		if !errors.Is(err, ErrTopUpNotFound) && !errors.Is(err, ErrPaymentMethodMismatch) && !errors.Is(err, ErrTopUpStatusInvalid) && !errors.Is(err, ErrTopUpAmountMismatch) {
			common.SysError(common.LogText("epay topup failed: %s", err.Error()))
		}
		return false, err
	}
	// Marketing attribution is deliberately outside the payment transaction.
	// A reporting-table failure must never roll back a successful recharge;
	// the unique conversion event makes duplicate callbacks safe to retry.
	if attributionErr := AttributeMarketingConversion(topUp); attributionErr != nil {
		common.SysError(common.LogText("marketing conversion attribution skipped: %s", attributionErr.Error()))
	}
	if alreadyDone {
		return true, nil
	}
	syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, common.LogText("epay topup"))

	common.SysLog(common.LogText("Epay top-up succeeded trade_no=%s user_id=%d quota_to_add=%d money=%.2f", topUp.TradeNo, topUp.UserId, quotaToAdd, topUp.Money))
	RecordTopupLog(topUp.UserId, common.NewMessage("Online top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}", map[string]any{"quota": logger.FormatQuota(quotaToAdd), "amount": fmt.Sprintf("%f", topUp.Money)}), callerIp, topUp.PaymentMethod, PaymentProviderEpay)
	return false, nil
}

// RechargeAntom atomically completes an Antom order after a signed webhook or
// inquiry response confirms the exact amount and currency that NewAPI quoted.
func RechargeAntom(tradeNo string, actualPaymentMethod string, paidAmountMinor int64, currency string, callerIp string) (alreadyDone bool, err error) {
	if tradeNo == "" {
		return false, errors.New("payment order number not provided")
	}
	if paidAmountMinor <= 0 {
		return false, errors.New("invalid paid amount")
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return false, ErrTopUpCurrencyMismatch
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	var quotaToAdd int
	topUp := &TopUp{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if validationErr := validateAntomTopUpPayment(topUp, paidAmountMinor, currency); validationErr != nil {
			return validationErr
		}
		if topUp.Status == common.TopUpStatusSuccess {
			alreadyDone = true
			tryCreateAffiliateCommissionForTopUpTx(tx, topUp)
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		quotaToAdd = topUp.CreditedQuota
		if quotaToAdd <= 0 {
			var quotaErr error
			quotaToAdd, quotaErr = common.WalletQuotaFromDecimalStrict(
				decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
			)
			if quotaErr != nil || quotaToAdd <= 0 {
				return ErrInvalidTopUpQuota
			}
			topUp.CreditedQuota = quotaToAdd
		}
		if actualPaymentMethod != "" {
			topUp.PaymentMethod = actualPaymentMethod
		}
		topUp.Money = decimal.NewFromInt(paidAmountMinor).Div(decimal.NewFromInt(100)).InexactFloat64()
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}
		if err := creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil); err != nil {
			return err
		}
		tryCreateAffiliateCommissionForTopUpTx(tx, topUp)
		return nil
	})
	if err != nil {
		return false, err
	}
	if attributionErr := AttributeMarketingConversion(topUp); attributionErr != nil {
		common.SysError(common.LogText("marketing conversion attribution skipped: %s", attributionErr.Error()))
	}
	if alreadyDone {
		return true, nil
	}

	syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, common.LogText("Antom topup"))
	common.SysLog(common.LogText("Antom top-up succeeded trade_no=%s user_id=%d quota_to_add=%d money=%.2f currency=%s", topUp.TradeNo, topUp.UserId, quotaToAdd, topUp.Money, topUp.Currency))
	RecordTopupLog(topUp.UserId, common.NewMessage("{{provider}} top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}", map[string]any{"provider": "Antom", "quota": logger.FormatQuota(quotaToAdd), "amount": fmt.Sprintf("%.2f %s", topUp.Money, topUp.Currency)}), callerIp, topUp.PaymentMethod, PaymentProviderAntom)
	return false, nil
}

func Recharge(referenceId string, customerId string, paidMoney float64, currency string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("payment order number not provided")
	}
	if paidMoney <= 0 {
		return errors.New("invalid paid amount")
	}

	var quota int
	var alreadyDone bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("top-up order not found")
		}

		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			alreadyDone = true
			tryCreateAffiliateCommissionForTopUpTx(tx, topUp)
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return errors.New("invalid top-up order status")
		}

		legacyCreditMoney := topUp.Money
		topUp.Money = paidMoney
		topUp.Currency = currency
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		quota = topUp.CreditedQuota
		if quota <= 0 {
			quota, err = common.WalletQuotaFromDecimalStrict(
				decimal.NewFromFloat(legacyCreditMoney).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
			)
		}
		if err != nil || quota <= 0 {
			return ErrInvalidTopUpQuota
		}
		if topUp.CreditedQuota <= 0 {
			topUp.CreditedQuota = quota
			if err := tx.Model(topUp).Update("credited_quota", quota).Error; err != nil {
				return err
			}
		}
		if err := creditTopUpQuota(tx, topUp.UserId, quota, map[string]any{
			"stripe_customer": customerId,
		}); err != nil {
			return err
		}
		tryCreateAffiliateCommissionForTopUpTx(tx, topUp)
		return nil
	})

	if err != nil {
		common.SysError(common.LogText("topup failed: %s", err.Error()))
		return errors.New("top-up failed")
	}
	if alreadyDone {
		return nil
	}
	syncCreditUserQuotaCache(topUp.UserId, quota, common.LogText("stripe topup"))

	RecordTopupLog(topUp.UserId, common.NewMessage("Online top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}", map[string]any{"quota": logger.FormatQuota(quota), "amount": topUp.Amount}), callerIp, topUp.PaymentMethod, PaymentMethodStripe)

	return nil
}

// topUpQueryWindowSeconds 限制充值记录查询的时间窗口（秒）。
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

// topUpQueryCutoff 返回允许查询的最早 create_time（秒级 Unix 时间戳）。
func topUpQueryCutoff() int64 {
	return common.GetTimestamp() - topUpQueryWindowSeconds
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	cutoff := topUpQueryCutoff()

	// Get total count within transaction
	err = tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, cutoff).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = tx.Where("user_id = ? AND create_time >= ?", userId, cutoff).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&TopUp{}).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, topUpQueryCutoff())
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError(common.LogText("failed to count search topups: %s", err.Error()))
		return nil, 0, common.NewMessage("Failed to search top-up records")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError(common.LogText("failed to search topups: %s", err.Error()))
		return nil, 0, common.NewMessage("Failed to search top-up records")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用，不限制时间窗口）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{})
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError(common.LogText("failed to count search topups: %s", err.Error()))
		return nil, 0, common.NewMessage("Failed to search top-up records")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError(common.LogText("failed to search topups: %s", err.Error()))
		return nil, 0, common.NewMessage("Failed to search top-up records")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	if tradeNo == "" {
		return common.NewMessage("Order number not provided")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	var userId int
	var quotaToAdd int
	var payMoney float64
	var paymentMethod string
	var alreadyDone bool
	topUp := &TopUp{}

	err := DB.Transaction(func(tx *gorm.DB) error {
		// 行级锁，避免并发补单
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return common.NewMessage("Top-up order does not exist")
		}

		// 幂等处理：已成功直接返回
		if topUp.Status == common.TopUpStatusSuccess {
			alreadyDone = true
			tryCreateAffiliateCommissionForTopUpTx(tx, topUp)
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return common.NewMessage("Order is not pending payment and cannot be completed")
		}

		// 计算应充值额度：
		// - Stripe 订单：Money 代表经分组倍率换算后的美元数量，直接 * QuotaPerUnit
		// - 其他订单（如易支付）：Amount 为美元数量，* QuotaPerUnit
		var quotaErr error
		if topUp.PaymentProvider == PaymentProviderStripe {
			quotaToAdd = topUp.CreditedQuota
			if quotaToAdd <= 0 {
				quotaToAdd, quotaErr = common.WalletQuotaFromDecimalStrict(
					decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
				)
			}
		} else {
			quotaToAdd, quotaErr = common.WalletQuotaFromDecimalStrict(
				decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
			)
		}
		if quotaErr != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}

		// 标记完成
		topUp.CompleteTime = common.GetTimestamp()
		topUp.CreditedQuota = quotaToAdd
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		// 增加用户额度（立即写库，保持一致性）
		if err := creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil); err != nil {
			return err
		}

		userId = topUp.UserId
		payMoney = topUp.Money
		paymentMethod = topUp.PaymentMethod
		tryCreateAffiliateCommissionForTopUpTx(tx, topUp)
		return nil
	})

	if err != nil {
		return err
	}
	if alreadyDone {
		return nil
	}

	// 事务外记录日志，避免阻塞
	syncCreditUserQuotaCache(userId, quotaToAdd, common.LogText("manual topup"))
	RecordTopupLog(userId, common.NewMessage("Administrator completed the order, amount added: {{quota}}, payment amount: {{amount}}", map[string]any{"quota": logger.FormatQuota(quotaToAdd), "amount": fmt.Sprintf("%f", payMoney)}), callerIp, paymentMethod, "admin")
	return nil
}
func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("payment order number not provided")
	}

	var quota int
	var alreadyDone bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("top-up order not found")
		}

		if topUp.PaymentProvider != PaymentProviderCreem {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			alreadyDone = true
			tryCreateAffiliateCommissionForTopUpTx(tx, topUp)
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return errors.New("invalid top-up order status")
		}

		// Creem 直接使用 Amount 作为充值额度（整数）
		quota, err = common.WalletQuotaFromDecimalStrict(decimal.NewFromInt(topUp.Amount))
		if err != nil || quota <= 0 {
			return ErrInvalidTopUpQuota
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.CreditedQuota = quota
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		// 构建更新字段，优先使用邮箱，如果邮箱为空则使用用户名
		updateFields := map[string]any{}

		// 如果有客户邮箱，尝试更新用户邮箱（仅当用户邮箱为空时）
		if customerEmail != "" {
			// 先检查用户当前邮箱是否为空
			var user User
			err = tx.Where("id = ?", topUp.UserId).First(&user).Error
			if err != nil {
				return err
			}

			// 如果用户邮箱为空，则更新为支付时使用的邮箱
			if user.Email == "" {
				updateFields["email"] = customerEmail
			}
		}

		if err := creditTopUpQuota(tx, topUp.UserId, quota, updateFields); err != nil {
			return err
		}
		tryCreateAffiliateCommissionForTopUpTx(tx, topUp)
		return nil
	})

	if err != nil {
		common.SysError(common.LogText("creem topup failed: %s", err.Error()))
		return errors.New("top-up failed")
	}
	if alreadyDone {
		return nil
	}
	syncCreditUserQuotaCache(topUp.UserId, quota, common.LogText("creem topup"))

	RecordTopupLog(topUp.UserId, common.NewMessage("{{provider}} top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}", map[string]any{"provider": "Creem", "quota": quota, "amount": fmt.Sprintf("%.2f", topUp.Money)}), callerIp, topUp.PaymentMethod, PaymentMethodCreem)

	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("payment order number not provided")
	}

	var quotaToAdd int
	var alreadyDone bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("top-up order not found")
		}

		if topUp.PaymentProvider != PaymentProviderWaffo {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			alreadyDone = true
			tryCreateAffiliateCommissionForTopUpTx(tx, topUp)
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("invalid top-up order status")
		}

		quotaToAdd, err = common.WalletQuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if err != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.CreditedQuota = quotaToAdd
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil); err != nil {
			return err
		}
		tryCreateAffiliateCommissionForTopUpTx(tx, topUp)
		return nil
	})

	if err != nil {
		common.SysError(common.LogText("waffo topup failed: %s", err.Error()))
		return errors.New("top-up failed")
	}
	if alreadyDone {
		return nil
	}
	syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, common.LogText("waffo topup"))

	if quotaToAdd > 0 {
		RecordTopupLog(topUp.UserId, common.NewMessage("{{provider}} top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}", map[string]any{"provider": "Waffo", "quota": logger.FormatQuota(quotaToAdd), "amount": fmt.Sprintf("%.2f", topUp.Money)}), callerIp, topUp.PaymentMethod, PaymentMethodWaffo)
	}

	return nil
}

func RechargeWaffoPancake(tradeNo string) (err error) {
	if tradeNo == "" {
		return errors.New("payment order number not provided")
	}

	var quotaToAdd int
	var alreadyDone bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("top-up order not found")
		}

		if topUp.PaymentProvider != PaymentProviderWaffoPancake {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			alreadyDone = true
			tryCreateAffiliateCommissionForTopUpTx(tx, topUp)
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("invalid top-up order status")
		}

		quotaToAdd, err = common.WalletQuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if err != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.CreditedQuota = quotaToAdd
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil); err != nil {
			return err
		}
		tryCreateAffiliateCommissionForTopUpTx(tx, topUp)
		return nil
	})

	if err != nil {
		common.SysError(common.LogText("waffo pancake topup failed: %s", err.Error()))
		return errors.New("top-up failed")
	}
	if alreadyDone {
		return nil
	}
	syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, common.LogText("waffo pancake topup"))

	if quotaToAdd > 0 {
		RecordLog(topUp.UserId, LogTypeTopup, common.NewMessage("{{provider}} top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}", map[string]any{"provider": "Waffo Pancake", "quota": logger.FormatQuota(quotaToAdd), "amount": fmt.Sprintf("%.2f", topUp.Money)}))
	}

	return nil
}

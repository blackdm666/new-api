package model

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func seedInvoiceRequestUserAndOrder(t *testing.T, tradeNo string, money float64) (*User, *TopUp) {
	t.Helper()
	user := &User{
		Username: "invoice-request-user",
		Email:    "invoice-request@example.com",
		AffCode:  "invoice-request-code",
		Status:   common.UserStatusEnabled,
		Quota:    100_000_000,
	}
	require.NoError(t, DB.Create(user).Error)
	order := &TopUp{
		UserId:          user.Id,
		Money:           money,
		Amount:          100,
		TradeNo:         tradeNo,
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe,
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, DB.Create(order).Error)
	return user, order
}

func TestInvoiceRequestLifecycleUsesInvoiceStatusDirectly(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-request-order", 701.25)

	request, orders, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId:        user.Id,
		Username:      user.Username,
		CompanyName:   "Independent Invoice Co.",
		TaxNumber:     "91310000INVOICE",
		Email:         "billing@example.com",
		TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)
	require.Len(t, orders, 1)
	assert.Equal(t, InvoiceStatusPending, request.Status)

	items, total, err := ListInvoiceRequests(InvoiceRequestQueryOptions{
		Status:  InvoiceStatusPending,
		Keyword: "invoice-request-order",
	}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, InvoiceStatusPending, items[0].Status)

	updated, err := UpdateInvoiceRequestStatus(request.Id, InvoiceStatusRejected, 10, "开票资料不完整")
	require.NoError(t, err)
	assert.Equal(t, InvoiceStatusRejected, updated.Status)

	eligible, err := ListEligibleInvoiceOrders(user.Id)
	require.NoError(t, err)
	require.Len(t, eligible, 1)
	assert.Equal(t, order.Id, eligible[0].Id)
}

func TestInvoiceAdminSearchUsesNormalizedIndexedPrefixes(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "INV-SEARCH-20260814", 701.25)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId:        user.Id,
		Username:      "BillingAlice",
		CompanyName:   "未来科技有限公司",
		TaxNumber:     "91310000SEARCH",
		Email:         "search@example.com",
		TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)

	for _, keyword := range []string{"billing", "未来科技", "91310000", "inv-search", strconv.Itoa(request.Id)} {
		items, total, listErr := ListInvoiceRequests(InvoiceRequestQueryOptions{Keyword: keyword}, &common.PageInfo{Page: 1, PageSize: 10})
		require.NoError(t, listErr)
		assert.Equal(t, int64(1), total, keyword)
		require.Len(t, items, 1, keyword)
		assert.Equal(t, request.Id, items[0].Id, keyword)
	}

	items, total, err := ListInvoiceRequests(InvoiceRequestQueryOptions{Keyword: "%"}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, items)
}

func TestInvoiceSearchBackfillIsIdempotent(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "BACKFILL-ORDER-20260814", 700)
	request := &InvoiceRequest{
		UserId:          user.Id,
		Username:        "HistoricalUser",
		CompanyName:     "历史数据有限公司",
		TaxNumber:       "91310000HISTORY",
		Email:           "history@example.com",
		OrderNumbers:    order.TradeNo,
		TotalMoney:      700,
		TotalMoneyCents: 70000,
		Status:          InvoiceStatusPending,
	}
	require.NoError(t, request.SetTopUpOrderIDs([]int{order.Id}))
	require.NoError(t, DB.Create(request).Error)

	count, err := BackfillInvoiceSearchTerms(100)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	count, err = BackfillInvoiceSearchTerms(100)
	require.NoError(t, err)
	assert.Zero(t, count)

	items, total, err := ListInvoiceRequests(InvoiceRequestQueryOptions{Keyword: "backfill-order"}, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, request.Id, items[0].Id)
}

func TestInvoiceSearchPrefixUsesSQLiteIndex(t *testing.T) {
	type queryPlanRow struct {
		Detail string
	}
	var planRows []queryPlanRow
	require.NoError(t, DB.Raw("EXPLAIN QUERY PLAN SELECT id FROM invoice_search_terms WHERE value LIKE ? ESCAPE '!'", "invoice%").Scan(&planRows).Error)
	details := make([]string, 0, len(planRows))
	for _, row := range planRows {
		details = append(details, row.Detail)
	}
	assert.Contains(t, strings.Join(details, "\n"), "idx_invoice_search_terms_value_nocase")
}

func TestInvoiceRequestMinimumAmountIncludesExactlyFiveHundredYuan(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-minimum-order", 500)

	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId:        user.Id,
		Username:      user.Username,
		CompanyName:   "Minimum Invoice Co.",
		TaxNumber:     "91310000MINIMUM",
		Email:         "minimum@example.com",
		TopUpOrderIds: []int{order.Id},
	})

	require.NoError(t, err)
	assert.Equal(t, int64(50000), request.TotalMoneyCents)
}

func TestInvoiceRequestRejectsAmountBelowFiveHundredYuan(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-below-minimum-order", 499.99)

	_, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId:        user.Id,
		Username:      user.Username,
		CompanyName:   "Below Minimum Invoice Co.",
		TaxNumber:     "91310000BELOWMINIMUM",
		Email:         "below-minimum@example.com",
		TopUpOrderIds: []int{order.Id},
	})

	require.ErrorIs(t, err, ErrInvoiceAmountTooSmall)
}

func TestInvoiceRequestDeductsConfiguredTaxFeeAtomically(t *testing.T) {
	truncateTables(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	originalExchangeRate := operation_setting.USDExchangeRate
	originalTaxRate := setting.InvoiceTaxRateBasisPoints
	common.QuotaPerUnit = 500_000
	operation_setting.USDExchangeRate = 1
	setting.InvoiceTaxRateBasisPoints = 300
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		operation_setting.USDExchangeRate = originalExchangeRate
		setting.InvoiceTaxRateBasisPoints = originalTaxRate
	})

	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-tax-order", 500)
	quotaBefore := user.Quota
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId:        user.Id,
		Username:      user.Username,
		CompanyName:   "Tax Invoice Co.",
		TaxNumber:     "91310000TAXFEE",
		Email:         "tax@example.com",
		TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)
	assert.Equal(t, 300, request.TaxRateBasisPoints)
	assert.Equal(t, int64(1500), request.TaxFeeCents)
	assert.Equal(t, 7_500_000, request.TaxFeeQuota)

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	assert.Equal(t, quotaBefore-request.TaxFeeQuota, stored.Quota)

	_, _, err = CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId:        user.Id,
		Username:      user.Username,
		CompanyName:   "Duplicate Tax Invoice Co.",
		TaxNumber:     "91310000TAXDUP",
		Email:         "tax@example.com",
		TopUpOrderIds: []int{order.Id},
	})
	require.ErrorIs(t, err, ErrInvoiceOrderDuplicate)
	require.NoError(t, DB.First(&stored, user.Id).Error)
	assert.Equal(t, quotaBefore-request.TaxFeeQuota, stored.Quota)
}

func TestCalculateInvoiceTaxFeeSupportsWalletQuotaAboveInt32(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	originalExchangeRate := operation_setting.USDExchangeRate
	common.QuotaPerUnit = 500_000
	operation_setting.USDExchangeRate = 1
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		operation_setting.USDExchangeRate = originalExchangeRate
	})

	feeCents, feeQuota, err := CalculateInvoiceTaxFee(20_000_000, 300)
	require.NoError(t, err)
	assert.Equal(t, int64(600_000), feeCents)
	assert.Equal(t, 3_000_000_000, feeQuota)
}

func TestInvoiceRequestRejectsInsufficientTaxFeeBalanceWithoutCreatingRequest(t *testing.T) {
	truncateTables(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	originalExchangeRate := operation_setting.USDExchangeRate
	originalTaxRate := setting.InvoiceTaxRateBasisPoints
	common.QuotaPerUnit = 500_000
	operation_setting.USDExchangeRate = 1
	setting.InvoiceTaxRateBasisPoints = 300
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		operation_setting.USDExchangeRate = originalExchangeRate
		setting.InvoiceTaxRateBasisPoints = originalTaxRate
	})

	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-tax-insufficient-order", 500)
	require.NoError(t, DB.Model(user).Update("quota", 7_000_000).Error)

	_, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId:        user.Id,
		Username:      user.Username,
		CompanyName:   "Insufficient Tax Invoice Co.",
		TaxNumber:     "91310000TAXLOW",
		Email:         "tax@example.com",
		TopUpOrderIds: []int{order.Id},
	})
	require.ErrorIs(t, err, ErrInvoiceTaxFeeInsufficient)
	var insufficient *InvoiceTaxFeeInsufficientError
	require.ErrorAs(t, err, &insufficient)
	assert.Equal(t, int64(1500), insufficient.FeeCents)
	assert.Equal(t, int64(1400), insufficient.AvailableCents)

	var requestCount int64
	require.NoError(t, DB.Model(&InvoiceRequest{}).Count(&requestCount).Error)
	assert.Zero(t, requestCount)
	var claimCount int64
	require.NoError(t, DB.Model(&InvoiceOrderClaim{}).Count(&claimCount).Error)
	assert.Zero(t, claimCount)
}

func TestIssuedInvoiceRequiresDirectlyBoundInvoiceFile(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-file-order", 801.25)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId:        user.Id,
		Username:      user.Username,
		CompanyName:   "Invoice File Co.",
		TaxNumber:     "91310000FILE",
		Email:         "file@example.com",
		TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)

	_, err = UpdateInvoiceRequestStatus(request.Id, InvoiceStatusIssued, 10, "")
	require.ErrorIs(t, err, ErrInvoiceFileRequired)

	require.NoError(t, CreateInvoiceFile(&InvoiceFile{
		InvoiceRequestId: request.Id,
		UploaderId:       10,
		FileName:         "invoice.pdf",
		StoredName:       "invoice.pdf",
		MimeType:         "application/pdf",
		Size:             128,
		StorageType:      "local",
		StorageKey:       "2026/08/invoice.pdf",
	}))

	updated, err := UpdateInvoiceRequestStatus(request.Id, InvoiceStatusIssued, 10, "")
	require.NoError(t, err)
	assert.Equal(t, InvoiceStatusIssued, updated.Status)
	assert.NotZero(t, updated.IssuedTime)
}

func TestInvoiceRejectionRequiresReasonAndRecordsOperator(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-rejection-order", 620)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId:        user.Id,
		Username:      user.Username,
		CompanyName:   "Rejection Invoice Co.",
		TaxNumber:     "91310000REJECTION",
		Email:         "rejection@example.com",
		TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)

	_, err = UpdateInvoiceRequestStatus(request.Id, InvoiceStatusRejected, 10, "  ")
	require.ErrorIs(t, err, ErrInvoiceRejectionReasonRequired)

	updated, err := UpdateInvoiceRequestStatus(request.Id, InvoiceStatusRejected, 10, "税号与公司名称不匹配")
	require.NoError(t, err)
	assert.Equal(t, "税号与公司名称不匹配", updated.RejectionReason)

	events, err := ListInvoiceRequestEvents(request.Id)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, InvoiceStatusPending, events[0].ToStatus)
	assert.Equal(t, user.Id, events[0].OperatorId)
	assert.Equal(t, InvoiceStatusPending, events[1].FromStatus)
	assert.Equal(t, InvoiceStatusRejected, events[1].ToStatus)
	assert.Equal(t, 10, events[1].OperatorId)
	assert.Equal(t, "税号与公司名称不匹配", events[1].Reason)
}

func TestInvoiceFinalStatusesCannotTransition(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-final-order", 880)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId:        user.Id,
		Username:      user.Username,
		CompanyName:   "Final Invoice Co.",
		TaxNumber:     "91310000FINAL",
		Email:         "final@example.com",
		TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)
	require.NoError(t, CreateInvoiceFile(&InvoiceFile{
		InvoiceRequestId: request.Id,
		UploaderId:       10,
		FileName:         "final.pdf",
		StoredName:       "final.pdf",
		MimeType:         "application/pdf",
		Size:             128,
		StorageType:      "local",
		StorageKey:       "2026/08/final.pdf",
	}))

	_, err = UpdateInvoiceRequestStatus(request.Id, InvoiceStatusIssued, 10, "")
	require.NoError(t, err)
	_, err = UpdateInvoiceRequestStatus(request.Id, InvoiceStatusPending, 10, "")
	require.ErrorIs(t, err, ErrInvoiceStatusTransition)
	_, err = UpdateInvoiceRequestStatus(request.Id, InvoiceStatusRejected, 10, "开错了")
	require.ErrorIs(t, err, ErrInvoiceStatusTransition)
}

func TestInvoiceFileCreationEnforcesStatusAndLimitInTransaction(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-file-limit-order", 900)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId:        user.Id,
		Username:      user.Username,
		CompanyName:   "File Limit Invoice Co.",
		TaxNumber:     "91310000FILELIMIT",
		Email:         "file-limit@example.com",
		TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)

	newFile := func(name string) *InvoiceFile {
		return &InvoiceFile{
			InvoiceRequestId: request.Id,
			UploaderId:       10,
			FileName:         name,
			StoredName:       name,
			MimeType:         "application/pdf",
			Size:             128,
			StorageType:      "local",
			StorageKey:       "2026/08/" + name,
		}
	}
	require.NoError(t, CreateInvoiceFileWithinLimit(newFile("first.pdf"), 1))
	require.ErrorIs(t, CreateInvoiceFileWithinLimit(newFile("second.pdf"), 1), ErrInvoiceFileLimit)

	_, err = UpdateInvoiceRequestStatus(request.Id, InvoiceStatusRejected, 10, "资料错误")
	require.NoError(t, err)
	require.ErrorIs(t, CreateInvoiceFileWithinLimit(newFile("rejected.pdf"), 5), ErrInvoiceFileMutationRejected)
}

func TestInvoiceFileDeletionQueuesReliableCleanup(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-file-cleanup-order", 930)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId:        user.Id,
		Username:      user.Username,
		CompanyName:   "File Cleanup Invoice Co.",
		TaxNumber:     "91310000FILECLEANUP",
		Email:         "file-cleanup@example.com",
		TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)
	file := &InvoiceFile{
		InvoiceRequestId: request.Id,
		UploaderId:       10,
		FileName:         "cleanup.pdf",
		StoredName:       "cleanup.pdf",
		MimeType:         "application/pdf",
		Size:             128,
		StorageType:      "local",
		StorageKey:       "2026/08/cleanup.pdf",
	}
	require.NoError(t, CreateInvoiceFileWithinLimit(file, 5))

	cleanup, err := QueueInvoiceFileDeletion(request.Id, file.Id)
	require.NoError(t, err)
	assert.Equal(t, file.StorageKey, cleanup.StorageKey)
	_, err = GetInvoiceFileById(file.Id)
	require.ErrorIs(t, err, ErrInvoiceFileNotFound)
	pending, err := ListPendingInvoiceFileCleanups(10, common.GetTimestamp())
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, cleanup.Id, pending[0].Id)
	require.NoError(t, CompleteInvoiceFileCleanup(cleanup.Id))
	var metadataCount int64
	require.NoError(t, DB.Unscoped().Model(&InvoiceFile{}).Where("id = ?", file.Id).Count(&metadataCount).Error)
	assert.Zero(t, metadataCount)
}

func TestExpiredInvoiceDataIsRedactedWithoutReleasingOrderClaim(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-retention-order", 950)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId:         user.Id,
		Username:       user.Username,
		CompanyName:    "Retention Invoice Co.",
		TaxNumber:      "91310000RETENTION",
		BankName:       "Retention Bank",
		BankAccount:    "6222000000000000",
		CompanyAddress: "Retention Address",
		CompanyPhone:   "010-12345678",
		Email:          "retention@example.com",
		Remark:         "sensitive remark",
		TopUpOrderIds:  []int{order.Id},
	})
	require.NoError(t, err)
	file := &InvoiceFile{
		InvoiceRequestId: request.Id,
		UploaderId:       10,
		FileName:         "retention.pdf",
		StoredName:       "retention.pdf",
		MimeType:         "application/pdf",
		Size:             128,
		StorageType:      "local",
		StorageKey:       "2026/08/retention.pdf",
	}
	require.NoError(t, CreateInvoiceFileWithinLimit(file, 5))
	_, err = UpdateInvoiceRequestStatus(request.Id, InvoiceStatusIssued, 10, "")
	require.NoError(t, err)
	require.NoError(t, DB.Model(&InvoiceRequest{}).Where("id = ?", request.Id).Update("updated_time", 100).Error)

	count, err := RedactExpiredInvoiceRequests(200, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	redacted, err := GetInvoiceRequestById(request.Id)
	require.NoError(t, err)
	assert.NotZero(t, redacted.RedactedTime)
	assert.Empty(t, redacted.Username)
	assert.Empty(t, redacted.CompanyName)
	assert.Empty(t, redacted.TaxNumber)
	assert.Empty(t, redacted.BankAccount)
	assert.Empty(t, redacted.Email)
	_, err = GetInvoiceFileById(file.Id)
	require.ErrorIs(t, err, ErrInvoiceFileNotFound)
	cleanups, err := ListPendingInvoiceFileCleanups(10, common.GetTimestamp())
	require.NoError(t, err)
	require.Len(t, cleanups, 1)
	assert.Equal(t, file.StorageKey, cleanups[0].StorageKey)
	var claimCount int64
	require.NoError(t, DB.Model(&InvoiceOrderClaim{}).Where("top_up_id = ?", order.Id).Count(&claimCount).Error)
	assert.Equal(t, int64(1), claimCount)
	var searchTermCount int64
	require.NoError(t, DB.Model(&InvoiceSearchTerm{}).Where("invoice_request_id = ?", request.Id).Count(&searchTermCount).Error)
	assert.Zero(t, searchTermCount)
}

func TestPendingInvoiceCanBeWithdrawnAndOrderReused(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-withdraw-order", 680)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId: user.Id, Username: user.Username, CompanyName: "Withdraw Co.",
		TaxNumber: "91310000WITHDRAW", Email: "withdraw@example.com", TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)

	withdrawn, err := WithdrawInvoiceRequest(request.Id, user.Id, nil)
	require.NoError(t, err)
	assert.Equal(t, InvoiceStatusWithdrawn, withdrawn.Status)
	_, err = WithdrawInvoiceRequest(request.Id, user.Id, nil)
	require.ErrorIs(t, err, ErrInvoiceWithdrawForbidden)

	replacement, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId: user.Id, Username: user.Username, CompanyName: "Replacement Co.",
		TaxNumber: "91310000REPLACEMENT", Email: "replacement@example.com", TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)
	assert.NotEqual(t, request.Id, replacement.Id)
}

func TestStalePendingInvoiceExpiresAndReleasesOrder(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-expiry-order", 720)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId: user.Id, Username: user.Username, CompanyName: "Expiry Co.",
		TaxNumber: "91310000EXPIRY", Email: "expiry@example.com", TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)
	require.NoError(t, DB.Model(&InvoiceRequest{}).Where("id = ?", request.Id).Update("created_time", 100).Error)

	count, err := ExpirePendingInvoiceRequests(200, 100, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	expired, err := GetInvoiceRequestById(request.Id)
	require.NoError(t, err)
	assert.Equal(t, InvoiceStatusExpired, expired.Status)

	_, _, err = CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId: user.Id, Username: user.Username, CompanyName: "After Expiry Co.",
		TaxNumber: "91310000AFTEREXPIRY", Email: "after-expiry@example.com", TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)
}

func TestPendingInvoiceExpirySkipsRequestThatCannotBeClosed(t *testing.T) {
	truncateTables(t)
	withInvoiceTaxFeeSettings(t, 300)
	blockedUser, blockedOrder := seedInvoiceRequestUserAndOrder(t, "invoice-expiry-blocked", 500)
	otherUser := &User{Username: "invoice-expiry-other", Email: "other@example.com", AffCode: "invoice-expiry-other", Status: common.UserStatusEnabled, Quota: 100_000_000}
	require.NoError(t, DB.Create(otherUser).Error)
	submit := func(user *User, order *TopUp) *InvoiceRequest {
		request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
			UserId: user.Id, Username: user.Username, CompanyName: "Expiry Batch Co.",
			TaxNumber: "91310000EXPBATCH", Email: user.Email, TopUpOrderIds: []int{order.Id},
		})
		require.NoError(t, err)
		return request
	}
	blocked := submit(blockedUser, blockedOrder)
	later := make([]*InvoiceRequest, 0, 2)
	for _, tradeNo := range []string{"invoice-expiry-later-1", "invoice-expiry-later-2"} {
		order := &TopUp{UserId: otherUser.Id, Money: 500, Amount: 100, TradeNo: tradeNo,
			PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusSuccess}
		require.NoError(t, DB.Create(order).Error)
		later = append(later, submit(otherUser, order))
	}
	require.NoError(t, DB.Model(&InvoiceRequest{}).Where("id > 0").Update("created_time", 100).Error)
	// Refunding the tax fee would push this wallet past the limit, so closing
	// the oldest request fails on every run.
	require.NoError(t, DB.Model(&User{}).Where("id = ?", blockedUser.Id).Update("quota", common.MaxWalletQuota).Error)
	status := func(id int) int {
		stored, err := GetInvoiceRequestById(id)
		require.NoError(t, err)
		return stored.Status
	}

	// With a page size of one the failing request fills the first page; the
	// run must move past it instead of stopping or reading it again.
	count, err := ExpirePendingInvoiceRequests(200, 1, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, InvoiceStatusPending, status(blocked.Id))
	for _, request := range later {
		assert.Equal(t, InvoiceStatusExpired, status(request.Id))
	}
	var account User
	require.NoError(t, DB.First(&account, blockedUser.Id).Error)
	assert.Equal(t, int(common.MaxWalletQuota), account.Quota)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", blockedUser.Id).Update("quota", 0).Error)
	count, err = ExpirePendingInvoiceRequests(200, 1, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "the skipped request is retried on the next run")
	assert.Equal(t, InvoiceStatusExpired, status(blocked.Id))
}

func TestPendingInvoiceExpiryWarningIsQueuedOnlyOnce(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-expiry-warning-order", 730)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId: user.Id, Username: user.Username, CompanyName: "Expiry Warning Co.",
		TaxNumber: "91310000EXPIRYWARN", Email: "warning@example.com", TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)
	require.NoError(t, DB.Model(&InvoiceRequest{}).Where("id = ?", request.Id).Update("created_time", 100).Error)
	factory := func(current *InvoiceRequest) ([]*InvoiceNotificationDelivery, error) {
		return []*InvoiceNotificationDelivery{{
			DeliveryKey: "expiry-warning-key", InvoiceRequestId: current.Id,
			Kind: InvoiceNotificationKindAdminEmail, Recipient: "billing@example.com",
			Subject: "Expiring", Body: "Review now",
		}}, nil
	}

	count, err := QueuePendingInvoiceExpiryWarnings(150, 50, 100, factory)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	count, err = QueuePendingInvoiceExpiryWarnings(150, 50, 100, factory)
	require.NoError(t, err)
	assert.Zero(t, count)

	updated, err := GetInvoiceRequestById(request.Id)
	require.NoError(t, err)
	assert.NotZero(t, updated.ExpiryWarningTime)
	var deliveryCount int64
	require.NoError(t, DB.Model(&InvoiceNotificationDelivery{}).Where("invoice_request_id = ?", request.Id).Count(&deliveryCount).Error)
	assert.Equal(t, int64(1), deliveryCount)
}

func TestIssuedInvoiceFilesAreFullyImmutable(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-immutable-order", 840)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId: user.Id, Username: user.Username, CompanyName: "Immutable Co.",
		TaxNumber: "91310000IMMUTABLE", Email: "immutable@example.com", TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)
	file := &InvoiceFile{
		InvoiceRequestId: request.Id, UploaderId: 10, FileName: "issued.pdf", StoredName: "issued.pdf",
		MimeType: "application/pdf", Size: 128, StorageType: "local", StorageKey: "2026/08/issued.pdf",
	}
	require.NoError(t, CreateInvoiceFileWithinLimit(file, 5))
	_, err = UpdateInvoiceRequestStatus(request.Id, InvoiceStatusIssued, 10, "")
	require.NoError(t, err)

	extra := *file
	extra.Id = 0
	extra.FileName = "correction.pdf"
	extra.StoredName = "correction.pdf"
	extra.StorageKey = "2026/08/correction.pdf"
	require.ErrorIs(t, CreateInvoiceFileWithinLimit(&extra, 5), ErrInvoiceFileMutationFinal)
	_, err = QueueInvoiceFileDeletion(request.Id, file.Id)
	require.ErrorIs(t, err, ErrInvoiceFileMutationFinal)
}

func TestInvoiceBusinessAndNotificationOutboxCommitAtomically(t *testing.T) {
	truncateTables(t)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-outbox-order", 760)
	_, _, err := CreateInvoiceRequestWithNotifications(CreateInvoiceRequestParams{
		UserId: user.Id, Username: user.Username, CompanyName: "Outbox Co.",
		TaxNumber: "91310000OUTBOX", Email: "outbox@example.com", TopUpOrderIds: []int{order.Id},
	}, func(*InvoiceRequest) ([]*InvoiceNotificationDelivery, error) {
		return nil, errors.New("notification build failed")
	})
	require.EqualError(t, err, "notification build failed")

	var requestCount, claimCount int64
	require.NoError(t, DB.Model(&InvoiceRequest{}).Count(&requestCount).Error)
	require.NoError(t, DB.Model(&InvoiceOrderClaim{}).Count(&claimCount).Error)
	assert.Zero(t, requestCount)
	assert.Zero(t, claimCount)
}

func withInvoiceTaxFeeSettings(t *testing.T, rateBasisPoints int) {
	t.Helper()
	originalQuotaPerUnit := common.QuotaPerUnit
	originalExchangeRate := operation_setting.USDExchangeRate
	originalTaxRate := setting.InvoiceTaxRateBasisPoints
	common.QuotaPerUnit = 500_000
	operation_setting.USDExchangeRate = 1
	setting.InvoiceTaxRateBasisPoints = rateBasisPoints
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		operation_setting.USDExchangeRate = originalExchangeRate
		setting.InvoiceTaxRateBasisPoints = originalTaxRate
	})
}

func countInvoiceTaxFeeRefundLogs(t *testing.T, userId int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).
		Where("user_id = ? AND type = ? AND content LIKE ?", userId, LogTypeSystem, "发票申请税费退还%").
		Count(&count).Error)
	return count
}

func TestInvoiceTaxFeeIsRefundedExactlyOnceWhenClosedWithoutInvoice(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		closeRequest func(t *testing.T, request *InvoiceRequest, user *User)
	}{
		{
			name:   "rejected",
			status: InvoiceStatusRejected,
			closeRequest: func(t *testing.T, request *InvoiceRequest, user *User) {
				_, err := UpdateInvoiceRequestStatus(request.Id, InvoiceStatusRejected, 10, "资料不完整")
				require.NoError(t, err)
			},
		},
		{
			name:   "withdrawn",
			status: InvoiceStatusWithdrawn,
			closeRequest: func(t *testing.T, request *InvoiceRequest, user *User) {
				_, err := WithdrawInvoiceRequest(request.Id, user.Id, nil)
				require.NoError(t, err)
			},
		},
		{
			name:   "expired",
			status: InvoiceStatusExpired,
			closeRequest: func(t *testing.T, request *InvoiceRequest, user *User) {
				require.NoError(t, DB.Model(&InvoiceRequest{}).Where("id = ?", request.Id).Update("created_time", 100).Error)
				count, err := ExpirePendingInvoiceRequests(200, 100, nil)
				require.NoError(t, err)
				require.Equal(t, 1, count)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			truncateTables(t)
			withInvoiceTaxFeeSettings(t, 300)
			user, order := seedInvoiceRequestUserAndOrder(t, "invoice-refund-"+test.name, 500)
			quotaBefore := user.Quota
			request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
				UserId: user.Id, Username: user.Username, CompanyName: "Refund Co.",
				TaxNumber: "91310000REFUND", Email: "refund@example.com", TopUpOrderIds: []int{order.Id},
			})
			require.NoError(t, err)
			require.Equal(t, 7_500_000, request.TaxFeeQuota)
			stale := *request

			test.closeRequest(t, request, user)

			stored, err := GetInvoiceRequestById(request.Id)
			require.NoError(t, err)
			assert.Equal(t, test.status, stored.Status)
			assert.NotZero(t, stored.TaxFeeRefundedTime)
			var account User
			require.NoError(t, DB.First(&account, user.Id).Error)
			assert.Equal(t, quotaBefore, account.Quota)
			assert.Equal(t, int64(1), countInvoiceTaxFeeRefundLogs(t, user.Id))

			// Every later close attempt, including one that read the request
			// while it was still pending, must leave the wallet untouched.
			_, err = UpdateInvoiceRequestStatus(request.Id, InvoiceStatusRejected, 10, "重复驳回")
			if test.status != InvoiceStatusRejected {
				require.ErrorIs(t, err, ErrInvoiceStatusTransition)
			}
			_, err = WithdrawInvoiceRequest(request.Id, user.Id, nil)
			require.ErrorIs(t, err, ErrInvoiceWithdrawForbidden)
			count, err := ExpirePendingInvoiceRequests(common.GetTimestamp()+1, 100, nil)
			require.NoError(t, err)
			assert.Zero(t, count)
			err = DB.Transaction(func(tx *gorm.DB) error {
				_, closeErr := closePendingInvoiceRequestTx(tx, &stale, InvoiceStatusExpired, 0, "")
				return closeErr
			})
			require.ErrorIs(t, err, ErrInvoiceStatusTransition)

			require.NoError(t, DB.First(&account, user.Id).Error)
			assert.Equal(t, quotaBefore, account.Quota)
			assert.Equal(t, int64(1), countInvoiceTaxFeeRefundLogs(t, user.Id))
			again, err := GetInvoiceRequestById(request.Id)
			require.NoError(t, err)
			assert.Equal(t, stored.TaxFeeRefundedTime, again.TaxFeeRefundedTime)
		})
	}
}

func TestIssuedInvoiceKeepsTaxFee(t *testing.T) {
	truncateTables(t)
	withInvoiceTaxFeeSettings(t, 300)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-issued-fee-order", 500)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId: user.Id, Username: user.Username, CompanyName: "Issued Fee Co.",
		TaxNumber: "91310000ISSUEDFEE", Email: "issued-fee@example.com", TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)
	require.NoError(t, CreateInvoiceFile(&InvoiceFile{
		InvoiceRequestId: request.Id, UploaderId: 10, FileName: "fee.pdf", StoredName: "fee.pdf",
		MimeType: "application/pdf", Size: 128, StorageType: "local", StorageKey: "2026/08/fee.pdf",
	}))

	issued, err := UpdateInvoiceRequestStatus(request.Id, InvoiceStatusIssued, 10, "")
	require.NoError(t, err)
	assert.Zero(t, issued.TaxFeeRefundedTime)
	_, err = UpdateInvoiceRequestStatus(request.Id, InvoiceStatusRejected, 10, "开错了")
	require.ErrorIs(t, err, ErrInvoiceStatusTransition)

	var account User
	require.NoError(t, DB.First(&account, user.Id).Error)
	assert.Equal(t, user.Quota-request.TaxFeeQuota, account.Quota)
	assert.Zero(t, countInvoiceTaxFeeRefundLogs(t, user.Id))
}

func TestZeroTaxFeeInvoiceClosesWithoutRefund(t *testing.T) {
	truncateTables(t)
	withInvoiceTaxFeeSettings(t, 0)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-zero-fee-order", 500)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId: user.Id, Username: user.Username, CompanyName: "Zero Fee Co.",
		TaxNumber: "91310000ZEROFEE", Email: "zero-fee@example.com", TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)
	require.Zero(t, request.TaxFeeQuota)

	withdrawn, err := WithdrawInvoiceRequest(request.Id, user.Id, nil)
	require.NoError(t, err)
	assert.Equal(t, InvoiceStatusWithdrawn, withdrawn.Status)
	assert.Zero(t, withdrawn.TaxFeeRefundedTime)
	var account User
	require.NoError(t, DB.First(&account, user.Id).Error)
	assert.Equal(t, user.Quota, account.Quota)
	assert.Zero(t, countInvoiceTaxFeeRefundLogs(t, user.Id))
}

func TestExpiredInvoiceNotifiesApplicantInsideTransaction(t *testing.T) {
	truncateTables(t)
	withInvoiceTaxFeeSettings(t, 300)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-expiry-notify-order", 500)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId: user.Id, Username: user.Username, CompanyName: "Expiry Notify Co.",
		TaxNumber: "91310000EXPNOTIFY", Email: "expiry-notify@example.com", TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)
	require.NoError(t, DB.Model(&InvoiceRequest{}).Where("id = ?", request.Id).Update("created_time", 100).Error)

	var notifiedUser *User
	var notifiedRequest InvoiceRequest
	count, err := ExpirePendingInvoiceRequests(200, 100, func(current *InvoiceRequest, account *User) ([]*InvoiceNotificationDelivery, error) {
		notifiedRequest = *current
		notifiedUser = account
		return []*InvoiceNotificationDelivery{{
			DeliveryKey: "expired-user-notice", InvoiceRequestId: current.Id,
			Kind: InvoiceNotificationKindUserNotice, UserId: account.Id, Recipient: account.Email,
			Subject: "Expired", Body: "Refunded",
		}}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	require.NotNil(t, notifiedUser)
	assert.Equal(t, user.Email, notifiedUser.Email)
	assert.Equal(t, InvoiceStatusExpired, notifiedRequest.Status)
	assert.NotZero(t, notifiedRequest.TaxFeeRefundedTime)
	var delivery InvoiceNotificationDelivery
	require.NoError(t, DB.Where("invoice_request_id = ?", request.Id).First(&delivery).Error)
	queued, err := GetEmailDeliveryById(delivery.EmailDeliveryId)
	require.NoError(t, err)
	assert.Equal(t, "invoice_user_email", queued.Category)
	assert.Equal(t, user.Email, queued.Recipient)
}

func TestExpiryClosesRequestOfDeletedApplicantWithoutRefund(t *testing.T) {
	truncateTables(t)
	withInvoiceTaxFeeSettings(t, 300)
	user, order := seedInvoiceRequestUserAndOrder(t, "invoice-deleted-user-order", 500)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId: user.Id, Username: user.Username, CompanyName: "Deleted User Co.",
		TaxNumber: "91310000DELETED", Email: "deleted@example.com", TopUpOrderIds: []int{order.Id},
	})
	require.NoError(t, err)
	require.NoError(t, DB.Model(&InvoiceRequest{}).Where("id = ?", request.Id).Update("created_time", 100).Error)
	require.NoError(t, DB.Unscoped().Delete(&User{}, user.Id).Error)

	notified := false
	count, err := ExpirePendingInvoiceRequests(200, 100, func(_ *InvoiceRequest, account *User) ([]*InvoiceNotificationDelivery, error) {
		notified = account != nil
		return nil, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.False(t, notified)
	expired, err := GetInvoiceRequestById(request.Id)
	require.NoError(t, err)
	assert.Equal(t, InvoiceStatusExpired, expired.Status)
	assert.Zero(t, expired.TaxFeeRefundedTime)
}

func TestInvoiceOrdersMustBePaidInCNY(t *testing.T) {
	truncateTables(t)
	user, cnyOrder := seedInvoiceRequestUserAndOrder(t, "invoice-currency-empty", 600)
	newOrder := func(tradeNo string, currency string) *TopUp {
		order := &TopUp{
			UserId: user.Id, Money: 600, Amount: 100, TradeNo: tradeNo, Currency: currency,
			PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
			Status: common.TopUpStatusSuccess,
		}
		require.NoError(t, DB.Create(order).Error)
		return order
	}
	lowerCNY := newOrder("invoice-currency-cny", " cny ")
	usd := newOrder("invoice-currency-usd", "USD")

	eligible, err := ListEligibleInvoiceOrders(user.Id)
	require.NoError(t, err)
	ids := make([]int, 0, len(eligible))
	for _, order := range eligible {
		ids = append(ids, order.Id)
	}
	assert.ElementsMatch(t, []int{cnyOrder.Id, lowerCNY.Id}, ids)

	_, _, err = CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId: user.Id, Username: user.Username, CompanyName: "Currency Co.",
		TaxNumber: "91310000CURRENCY", Email: "currency@example.com", TopUpOrderIds: []int{cnyOrder.Id, usd.Id},
	})
	require.ErrorIs(t, err, ErrInvoiceOrderInvalid)
	request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
		UserId: user.Id, Username: user.Username, CompanyName: "Currency Co.",
		TaxNumber: "91310000CURRENCY", Email: "currency@example.com", TopUpOrderIds: []int{cnyOrder.Id, lowerCNY.Id},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(120000), request.TotalMoneyCents)
}

func TestInvoiceOrphanCleanupOnlyAcceptsUnreferencedInvoiceKeys(t *testing.T) {
	truncateTables(t)
	profile, err := GetOrCreateInvoiceStorageProfile("s3", "orphan-profile-fingerprint", "encrypted")
	require.NoError(t, err)
	uploadedAt := time.Date(2026, time.October, 12, 8, 0, 0, 0, time.UTC)
	orphanKey := InvoiceFileStorageKey(42, uploadedAt, "0f8fad5b-d9cb-469f-a165-70867728950e.pdf")
	liveKey := InvoiceFileStorageKey(42, uploadedAt, "7c9e6679-7425-40de-944b-e07fc1f90ae7.pdf")
	deletedKey := InvoiceFileStorageKey(43, uploadedAt, "16fd2706-8baf-433b-82eb-8c7fada847da.png")
	assert.Equal(t, "42/2026/10/0f8fad5b-d9cb-469f-a165-70867728950e.pdf", orphanKey)

	for _, key := range []string{liveKey, deletedKey} {
		require.NoError(t, DB.Create(&InvoiceFile{
			InvoiceRequestId: 42, UploaderId: 10, FileName: "invoice.pdf", StoredName: "invoice.pdf",
			MimeType: "application/pdf", Size: 128, StorageProfileId: profile.Id + 1, StorageType: "s3", StorageKey: key,
		}).Error)
	}
	require.NoError(t, DB.Where("storage_key = ?", deletedKey).Delete(&InvoiceFile{}).Error)

	tests := []struct {
		name string
		key  InvoiceStorageKey
		err  error
	}{
		{name: "foreign object", key: InvoiceStorageKey{StorageProfileId: profile.Id, StorageKey: "avatars/42.png"}, err: ErrInvoiceStorageKeyInvalid},
		{name: "health check object", key: InvoiceStorageKey{StorageProfileId: profile.Id, StorageKey: "_health/0f8fad5b-d9cb-469f-a165-70867728950e.txt"}, err: ErrInvoiceStorageKeyInvalid},
		{name: "path traversal", key: InvoiceStorageKey{StorageProfileId: profile.Id, StorageKey: "42/2026/10/../../../etc/passwd"}, err: ErrInvoiceStorageKeyInvalid},
		{name: "unknown profile", key: InvoiceStorageKey{StorageProfileId: profile.Id + 100, StorageKey: orphanKey}, err: ErrInvoiceStorageKeyInvalid},
		{name: "live file in another profile", key: InvoiceStorageKey{StorageProfileId: profile.Id, StorageKey: liveKey}, err: ErrInvoiceStorageKeyReferenced},
		{name: "soft-deleted file", key: InvoiceStorageKey{StorageProfileId: profile.Id, StorageKey: deletedKey}, err: ErrInvoiceStorageKeyReferenced},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := QueueInvoiceOrphanCleanups([]InvoiceStorageKey{{StorageProfileId: profile.Id, StorageKey: orphanKey}, test.key})
			require.ErrorIs(t, err, test.err)
			var cleanupCount int64
			require.NoError(t, DB.Model(&InvoiceFileCleanup{}).Count(&cleanupCount).Error)
			assert.Zero(t, cleanupCount, "a rejected batch must not queue any key")
		})
	}

	cleanups, err := QueueInvoiceOrphanCleanups([]InvoiceStorageKey{{StorageProfileId: profile.Id, StorageType: "local", StorageKey: orphanKey}})
	require.NoError(t, err)
	require.Len(t, cleanups, 1)
	assert.Equal(t, orphanKey, cleanups[0].StorageKey)
	assert.Equal(t, "s3", cleanups[0].StorageType)
}

// TestInvoiceRefundAndOrderRulesDatabaseMatrix upgrades a schema without the
// refund marker, then exercises the refund, currency, orphan and finance group
// auto-issue queries on every supported database.
func TestInvoiceRefundAndOrderRulesDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			kind := common.DatabaseTypeSQLite
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(filepath.Join(t.TempDir(), "invoice-refund.db"))
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN not configured")
				}
				driver = mysql.Open(dsn)
				kind = common.DatabaseTypeMySQL
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not configured")
				}
				driver = postgres.Open(dsn)
				kind = common.DatabaseTypePostgreSQL
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			oldDB, oldLog := DB, LOG_DB
			oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(kind, kind)
			initCol()
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() {
				DB, LOG_DB = oldDB, oldLog
				common.SetDatabaseTypes(oldMain, oldLogType)
				initCol()
				_ = sqlDB.Close()
			})
			tables := []any{
				&User{}, &Log{}, &TopUp{}, &InvoiceRequest{}, &InvoiceOrderClaim{}, &InvoiceStorageProfile{},
				&InvoiceFileUpload{}, &InvoiceFile{}, &InvoiceRequestEvent{}, &InvoiceSearchTerm{}, &InvoiceFileCleanup{},
				&InvoiceNotificationDelivery{}, &EmailDelivery{},
			}
			require.NoError(t, db.Migrator().DropTable(tables...))
			t.Cleanup(func() { _ = db.Migrator().DropTable(tables...) })
			require.NoError(t, db.AutoMigrate(tables...))
			require.NoError(t, db.Migrator().DropColumn(&InvoiceRequest{}, "tax_fee_refunded_time"))
			require.NoError(t, db.Migrator().DropColumn(&InvoiceNotificationDelivery{}, "failed_time"))
			require.NoError(t, db.Migrator().DropColumn(&InvoiceNotificationDelivery{}, "send_started_time"))
			withInvoiceTaxFeeSettings(t, 300)

			user := &User{Username: "matrix-refund", AffCode: "matrix-refund-code", Status: common.UserStatusEnabled, Quota: 92_500_000}
			require.NoError(t, db.Create(user).Error)
			now := common.GetTimestamp()
			require.NoError(t, db.Table("invoice_requests").Create(map[string]any{
				"user_id": user.Id, "username": user.Username, "company_name": "Legacy Co.", "tax_number": "91310000LEGACY",
				"bank_name": "", "bank_account": "", "company_address": "", "company_phone": "", "email": "legacy@example.com",
				"remark": "", "top_up_order_ids": "[]", "order_numbers": "", "total_money": 500, "total_money_cents": 50000,
				"tax_rate_basis_points": 300, "tax_fee_cents": 1500, "tax_fee_quota": 7_500_000, "status": InvoiceStatusPending,
				"rejection_reason": "", "issued_time": 0, "redacted_time": 0, "expiry_warning_time": 0,
				"created_time": now, "updated_time": now,
			}).Error)
			for range 2 {
				require.NoError(t, db.AutoMigrate(tables...))
			}
			require.True(t, db.Migrator().HasColumn(&InvoiceRequest{}, "tax_fee_refunded_time"))
			require.True(t, db.Migrator().HasColumn(&InvoiceNotificationDelivery{}, "failed_time"))
			require.True(t, db.Migrator().HasColumn(&InvoiceNotificationDelivery{}, "send_started_time"))

			var legacy InvoiceRequest
			require.NoError(t, db.Where("tax_number = ?", "91310000LEGACY").First(&legacy).Error)
			assert.Zero(t, legacy.TaxFeeRefundedTime)
			withdrawn, err := WithdrawInvoiceRequest(legacy.Id, user.Id, nil)
			require.NoError(t, err)
			assert.NotZero(t, withdrawn.TaxFeeRefundedTime)
			var account User
			require.NoError(t, db.First(&account, user.Id).Error)
			assert.Equal(t, 100_000_000, account.Quota)

			cny := &TopUp{UserId: user.Id, Money: 500, Amount: 100, TradeNo: "MATRIX-CNY", Currency: "cny",
				PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusSuccess}
			usd := &TopUp{UserId: user.Id, Money: 500, Amount: 100, TradeNo: "MATRIX-USD", Currency: "USD",
				PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusSuccess}
			require.NoError(t, db.Create(cny).Error)
			require.NoError(t, db.Create(usd).Error)
			eligible, err := ListEligibleInvoiceOrders(user.Id)
			require.NoError(t, err)
			require.Len(t, eligible, 1)
			assert.Equal(t, cny.Id, eligible[0].Id)

			request, _, err := CreateInvoiceRequest(CreateInvoiceRequestParams{
				UserId: user.Id, Username: user.Username, CompanyName: "Matrix Co.",
				TaxNumber: "91310000MATRIX", Email: "matrix@example.com", TopUpOrderIds: []int{cny.Id},
			})
			require.NoError(t, err)
			pendingRequests, err := ListPendingInvoiceRequests()
			require.NoError(t, err)
			require.Len(t, pendingRequests, 1, "the withdrawn legacy request is no longer pending")
			assert.Equal(t, request.Id, pendingRequests[0].Id)
			_, err = UpdateInvoiceRequestStatus(request.Id, InvoiceStatusRejected, user.Id, "matrix rejection")
			require.NoError(t, err)
			_, err = UpdateInvoiceRequestStatus(request.Id, InvoiceStatusRejected, user.Id, "matrix rejection")
			require.NoError(t, err)
			require.NoError(t, db.First(&account, user.Id).Error)
			assert.Equal(t, 100_000_000, account.Quota)
			assert.Equal(t, int64(2), countInvoiceTaxFeeRefundLogs(t, user.Id))
			events, err := ListInvoiceRequestEvents(request.Id)
			require.NoError(t, err)
			require.Len(t, events, 2)
			assert.Equal(t, "matrix-refund", events[1].OperatorName)

			profile, err := GetOrCreateInvoiceStorageProfile("s3", "matrix-profile", "encrypted")
			require.NoError(t, err)
			key := InvoiceFileStorageKey(request.Id, time.Now(), "0f8fad5b-d9cb-469f-a165-70867728950e.pdf")
			require.NoError(t, db.Create(&InvoiceFile{
				InvoiceRequestId: request.Id, UploaderId: user.Id, FileName: "invoice.pdf", StoredName: "invoice.pdf",
				MimeType: "application/pdf", Size: 1, StorageProfileId: profile.Id, StorageType: "s3", StorageKey: key,
				Sha256: "matrix-digest",
			}).Error)
			attached, err := InvoiceFileExists(request.Id, "matrix-digest")
			require.NoError(t, err)
			assert.True(t, attached)
			attached, err = InvoiceFileExists(legacy.Id, "matrix-digest")
			require.NoError(t, err)
			assert.False(t, attached)
			require.NoError(t, db.Where("storage_key = ?", key).Delete(&InvoiceFile{}).Error)
			_, err = QueueInvoiceOrphanCleanups([]InvoiceStorageKey{{StorageProfileId: profile.Id, StorageKey: key}})
			require.ErrorIs(t, err, ErrInvoiceStorageKeyReferenced)

			wechat, created, err := EnqueueInvoiceNotification(&InvoiceNotificationDelivery{
				DeliveryKey: "matrix-wechat", InvoiceRequestId: request.Id, Kind: InvoiceNotificationKindWechatGroup,
				Recipient: "finance@chatroom", Subject: "新开票申请", Body: "【新开票申请】",
			})
			require.NoError(t, err)
			require.True(t, created)
			assert.Zero(t, wechat.EmailDeliveryId)
			pending, err := ListPendingInvoiceWechatNotifications(10, common.GetTimestamp())
			require.NoError(t, err)
			require.Len(t, pending, 1)
			assert.Equal(t, InvoiceNotificationChannelWechat, pending[0].Channel)
			claimedAt := common.GetTimestamp()
			claimed, err := ClaimInvoiceWechatNotification(wechat.Id, claimedAt, claimedAt+60)
			require.NoError(t, err)
			require.True(t, claimed)
			// Even after the lock expires an in-flight message is not claimed again.
			claimed, err = ClaimInvoiceWechatNotification(wechat.Id, claimedAt+120, claimedAt+180)
			require.NoError(t, err)
			assert.False(t, claimed)
			require.Error(t, RecordInvoiceWechatNotificationResult(wechat.Id, claimedAt-1, 1, "result of another claim"))
			require.NoError(t, RecordInvoiceWechatNotificationResult(wechat.Id, claimedAt, 1, "wechat api rejected the message: group closed"))
			pending, err = ListPendingInvoiceWechatNotifications(10, claimedAt+120)
			require.NoError(t, err)
			assert.Empty(t, pending)

			interrupted, _, err := EnqueueInvoiceNotification(&InvoiceNotificationDelivery{
				DeliveryKey: "matrix-wechat-interrupted", InvoiceRequestId: request.Id, Kind: InvoiceNotificationKindWechatGroup,
				Recipient: "finance@chatroom", Subject: "新开票申请", Body: "【新开票申请】",
			})
			require.NoError(t, err)
			claimed, err = ClaimInvoiceWechatNotification(interrupted.Id, claimedAt-300, claimedAt-120)
			require.NoError(t, err)
			require.True(t, claimed)
			failedCount, err := FailInterruptedInvoiceWechatNotifications(claimedAt)
			require.NoError(t, err)
			assert.Equal(t, int64(1), failedCount)
			var unknown InvoiceNotificationDelivery
			require.NoError(t, db.First(&unknown, interrupted.Id).Error)
			assert.Equal(t, InvoiceWechatOutcomeUnknownError, unknown.LastError)
			assert.NotZero(t, unknown.FailedTime)
			retried, err := RetryInvoiceNotification(interrupted.Id)
			require.NoError(t, err)
			assert.Zero(t, retried.SendStartedTime)
			pending, err = ListPendingInvoiceWechatNotifications(10, common.GetTimestamp())
			require.NoError(t, err)
			require.Len(t, pending, 1)
			assert.Equal(t, interrupted.Id, pending[0].Id)
		})
	}
}

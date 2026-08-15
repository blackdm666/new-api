package model

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedInvoiceRequestUserAndOrder(t *testing.T, tradeNo string, money float64) (*User, *TopUp) {
	t.Helper()
	user := &User{
		Username: "invoice-request-user",
		Email:    "invoice-request@example.com",
		AffCode:  "invoice-request-code",
		Status:   common.UserStatusEnabled,
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

	withdrawn, err := WithdrawInvoiceRequest(request.Id, user.Id)
	require.NoError(t, err)
	assert.Equal(t, InvoiceStatusWithdrawn, withdrawn.Status)
	_, err = WithdrawInvoiceRequest(request.Id, user.Id)
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

	count, err := ExpirePendingInvoiceRequests(200, 100)
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

package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceErrorsUseAllFrontendLanguages(t *testing.T) {
	require.NoError(t, Init())
	assert.Equal(t, LangFr, ParseAcceptLanguage("fr-FR,fr;q=0.9"))
	assert.Equal(t, LangJa, ParseAcceptLanguage("ja-JP"))
	assert.Equal(t, LangRu, ParseAcceptLanguage("ru"))
	assert.Equal(t, LangVi, ParseAcceptLanguage("vi-VN"))
	assert.Equal(t, LangZhTW, ParseAcceptLanguage("zhTW"))
	assert.Equal(t, LangZhCN, ParseAcceptLanguage("zhCN"))

	translations := map[string]string{
		LangFr: "Le montant de la facture doit être d’au moins 500.00 CNY",
		LangJa: "請求金額は 500.00 人民元以上である必要があります",
		LangRu: "Сумма счёта должна быть не менее 500.00 CNY",
		LangVi: "Số tiền xuất hóa đơn phải đạt ít nhất 500.00 CNY",
	}
	for language, expected := range translations {
		assert.Equal(t, expected, Translate(language, MsgInvoiceAmountTooSmall, map[string]any{"Amount": "500.00"}))
	}
}

func TestInvoiceOperationalErrorsAreTranslatedInEveryLanguage(t *testing.T) {
	require.NoError(t, Init())
	keys := []string{
		MsgInvoiceTaxFeeInsufficient,
		MsgInvoiceOperationFailed,
		MsgInvoiceLoadFailed,
		MsgInvoiceSubmitFailed,
		MsgInvoiceUpdateFailed,
		MsgInvoiceUploadFailed,
		MsgInvoiceDownloadFailed,
		MsgInvoiceFileDeleteFailed,
		MsgInvoiceMaintenanceFailed,
		MsgInvoiceSettingsInvalid,
		MsgInvoiceSettingsSaveFailed,
		MsgInvoiceStorageTestFailed,
		MsgInvoiceFileNameInvalid,
		MsgInvoiceFileTypeNotAllowed,
		MsgInvoiceFileTypeMismatch,
		MsgInvoiceFileSVGForbidden,
	}
	for _, language := range SupportedLanguages() {
		for _, key := range keys {
			assert.NotEqualf(t, key, Translate(language, key), "missing %s translation for %s", key, language)
		}
	}
}

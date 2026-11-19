package invoicebill

import (
	"bytes"
	"testing"

	"github.com/signintech/gopdf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRenderer(t *testing.T) *renderer {
	t.Helper()
	coverage, err := fontCoverage()
	require.NoError(t, err)
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: gopdf.Rect{W: pageWidth, H: pageHeight}})
	require.NoError(t, pdf.AddTTFFontData(fontRegular, regularFont))
	require.NoError(t, pdf.AddTTFFontData(fontBold, boldFont))
	require.NoError(t, pdf.AddTTFFontData(fontRegularFallback, regularFallbackFont))
	require.NoError(t, pdf.AddTTFFontData(fontBoldFallback, boldFallbackFont))
	pdf.AddPage()
	return &renderer{pdf: pdf, coverage: coverage}
}

// Simplified characters Noto Sans TC lacks (盘, 区, 创) used to vanish from
// the bill; each must now be drawn by a font that has its glyph.
func TestRunsDrawEveryCharacterWithAFontThatHasIt(t *testing.T) {
	r := newTestRenderer(t)
	for _, family := range []string{fontRegular, fontBold} {
		require.NoError(t, r.setFont(family, 10, colorText))
		for _, value := range []string{
			"杭州盘子科技有限公司",
			"浙江省杭州市拱墅区上塘街道智慧网谷威新曦地科创中心16幢7层702室",
			"88 DIGITAL TECHNOLOGY LIMITED",
		} {
			var drawn string
			for _, run := range r.runs(value) {
				for _, char := range run.text {
					assert.True(t, r.hasGlyph(run.family, char), "%s: %q drawn in %s without a glyph", family, char, run.family)
				}
				drawn += run.text
			}
			assert.Equal(t, value, drawn)
		}
	}
}

func TestRunsKeepTextTheBillFontCoversInOneRun(t *testing.T) {
	r := newTestRenderer(t)
	require.NoError(t, r.setFont(fontRegular, 10, colorText))
	assert.Equal(t, []textRun{{family: fontRegular, text: "帳單編號 88API-HK-20261119-000109"}}, r.runs("帳單編號 88API-HK-20261119-000109"))
	assert.Equal(t, []textRun{
		{family: fontRegular, text: "杭州"},
		{family: fontRegularFallback, text: "盘"},
		{family: fontRegular, text: "子科技有限公司"},
	}, r.runs("杭州盘子科技有限公司"))
}

func TestRenderDrawsMainlandNamesInTheFallbackFont(t *testing.T) {
	bill := Bill{
		Number:        "88API-HK-20261119-000109",
		IssueDate:     "2026-11-19",
		DueDate:       "2026-11-19",
		Currency:      "CNY",
		IssuerHeading: "88API",
		Issuer:        []Field{{Label: "名稱", Value: "88 DIGITAL TECHNOLOGY LIMITED"}},
		Recipient:     []Field{{Label: "名稱", Value: "杭州盘子科技有限公司"}},
		Items:         []Item{{Description: "88API 平台 API 服務費（帳戶預充值）", Quantity: 1, UnitCents: 138600, AmountCents: 138600}},
		TotalCents:    138600,
	}
	content, err := Render(bill)
	require.NoError(t, err)
	assert.True(t, bytes.Contains(content, []byte("/BaseFont /"+fontRegularFallback)), "the recipient name needs the fallback font")
}

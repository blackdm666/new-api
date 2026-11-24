// Package invoicebill renders the INVOICE (帳單) PDF issued for Hong Kong
// invoice requests. The layout follows the sample shown to applicants: title
// and bill number block, issuer and recipient columns, the amount due, the
// line items and the totals.
package invoicebill

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/signintech/gopdf"
	"golang.org/x/image/font/sfnt"
)

// The bill is set in Noto Sans TC. It lacks about a quarter of the common
// Simplified Chinese characters (盘, 区, 创…) that Mainland company names and
// addresses use, and gopdf silently drops a character its font has no glyph
// for, so every character Noto Sans TC lacks is drawn in Noto Sans SC
// instead. The SC files hold only those characters (instances of the Google
// Fonts variable font at weights 400 and 700, subset to what TC lacks). Both
// are under the SIL Open Font License 1.1, see fonts/OFL.txt. Only the glyphs
// a bill uses are embedded in each PDF.
var (
	//go:embed fonts/NotoSansTC-Regular.ttf
	regularFont []byte
	//go:embed fonts/NotoSansTC-Bold.ttf
	boldFont []byte
	//go:embed fonts/NotoSansSC-Regular-Fallback.ttf
	regularFallbackFont []byte
	//go:embed fonts/NotoSansSC-Bold-Fallback.ttf
	boldFallbackFont []byte
)

// fallbackFamily is the family that draws what a family has no glyph for.
var fallbackFamily = map[string]string{
	fontRegular: fontRegularFallback,
	fontBold:    fontBoldFallback,
}

var (
	coverageOnce sync.Once
	coverage     map[string]*sfnt.Font
	coverageErr  error
)

// fontCoverage parses the embedded fonts once to tell which characters each
// one has a glyph for.
func fontCoverage() (map[string]*sfnt.Font, error) {
	coverageOnce.Do(func() {
		fonts := map[string][]byte{
			fontRegular:         regularFont,
			fontBold:            boldFont,
			fontRegularFallback: regularFallbackFont,
			fontBoldFallback:    boldFallbackFont,
		}
		coverage = make(map[string]*sfnt.Font, len(fonts))
		for family, data := range fonts {
			parsed, err := sfnt.Parse(data)
			if err != nil {
				coverageErr = fmt.Errorf("parse %s font: %w", family, err)
				return
			}
			coverage[family] = parsed
		}
	})
	return coverage, coverageErr
}

// Field is one labelled line of the issuer or recipient column.
type Field struct {
	Label string
	Value string
}

// Item is one line of the bill. Detail is printed in a smaller grey line
// under the description.
type Item struct {
	Description string
	Detail      string
	Quantity    int
	UnitCents   int64
	AmountCents int64
}

// Labels is the fixed wording of a document. Empty fields keep the wording of
// the Hong Kong bill (帳單 INVOICE), so a receipt only sets what differs.
type Labels struct {
	Title            string
	Subtitle         string
	NumberLabel      string
	RecipientHeading string
	// Amount labels the headline amount and the last line of the totals.
	Amount      string
	Subtotal    string
	Total       string
	Description string
	Quantity    string
	UnitPrice   string
	LineAmount  string
}

var invoiceLabels = Labels{
	Title:            "帳單",
	Subtitle:         "INVOICE",
	NumberLabel:      "帳單編號",
	RecipientHeading: "收件人",
	Amount:           "應付金額",
	Subtotal:         "小計",
	Total:            "合計",
	Description:      "描述",
	Quantity:         "數量",
	UnitPrice:        "單價",
	LineAmount:       "金額",
}

func (labels Labels) withDefaults() Labels {
	for _, pair := range []struct {
		value    *string
		fallback string
	}{
		{&labels.Title, invoiceLabels.Title},
		{&labels.Subtitle, invoiceLabels.Subtitle},
		{&labels.NumberLabel, invoiceLabels.NumberLabel},
		{&labels.RecipientHeading, invoiceLabels.RecipientHeading},
		{&labels.Amount, invoiceLabels.Amount},
		{&labels.Subtotal, invoiceLabels.Subtotal},
		{&labels.Total, invoiceLabels.Total},
		{&labels.Description, invoiceLabels.Description},
		{&labels.Quantity, invoiceLabels.Quantity},
		{&labels.UnitPrice, invoiceLabels.UnitPrice},
		{&labels.LineAmount, invoiceLabels.LineAmount},
	} {
		if strings.TrimSpace(*pair.value) == "" {
			*pair.value = pair.fallback
		}
	}
	return labels
}

type Bill struct {
	Labels Labels
	// HeaderFields replace the issue date and due date lines printed under
	// the number when set.
	HeaderFields []Field
	Number       string
	IssueDate    string
	DueDate      string
	// Currency is printed before every amount, e.g. "CNY".
	Currency      string
	IssuerHeading string
	Issuer        []Field
	Recipient     []Field
	Items         []Item
	TotalCents    int64
	Footer        string
	CreatedAt     time.Time
}

const (
	mm         = 72 / 25.4
	pageWidth  = 297 * mm
	pageHeight = 210 * mm
	marginX    = 13 * mm
	marginTop  = 13 * mm
	contentW   = pageWidth - 2*marginX
	// Content stops above the footer line.
	contentBottom = pageHeight - 12*mm
	footerY       = pageHeight - 8*mm

	fontRegular         = "regular"
	fontBold            = "bold"
	fontRegularFallback = "regular-sc"
	fontBoldFallback    = "bold-sc"

	rowHeight   = 7.8 * mm
	lineSpacing = 1.35
)

var (
	colorText  = [3]uint8{17, 17, 17}
	colorMuted = [3]uint8{85, 85, 85}
	colorFaint = [3]uint8{136, 136, 136}
	colorRule  = [3]uint8{214, 214, 214}
	colorLight = [3]uint8{226, 226, 226}
)

// Table columns, from the right edge of the content.
const (
	colAmountW = 42 * mm
	colUnitW   = 42 * mm
	colQtyW    = 30 * mm
)

type renderer struct {
	pdf *gopdf.GoPdf
	y   float64
	// family and size are the font set by setFont; text in another family
	// for missing glyphs switches back to it.
	family   string
	size     float64
	coverage map[string]*sfnt.Font
	glyphs   sfnt.Buffer
}

// textRun is a stretch of text drawn in one font family.
type textRun struct {
	family string
	text   string
}

func (r *renderer) hasGlyph(family string, char rune) bool {
	font := r.coverage[family]
	if font == nil {
		return false
	}
	index, err := font.GlyphIndex(&r.glyphs, char)
	return err == nil && index != 0
}

// runs splits value into stretches of the current family and of its
// fallback, which draws the characters the current family has no glyph for.
// A character neither font has stays in the current family.
func (r *renderer) runs(value string) []textRun {
	fallback := fallbackFamily[r.family]
	var runs []textRun
	for _, char := range value {
		family := r.family
		if fallback != "" && !r.hasGlyph(r.family, char) && r.hasGlyph(fallback, char) {
			family = fallback
		}
		if last := len(runs) - 1; last >= 0 && runs[last].family == family {
			runs[last].text += string(char)
			continue
		}
		runs = append(runs, textRun{family: family, text: string(char)})
	}
	return runs
}

// measure returns the width of value in the current font, counting the
// characters drawn in the fallback font at their own width.
func (r *renderer) measure(value string) (float64, error) {
	runs := r.runs(value)
	if len(runs) <= 1 {
		return r.pdf.MeasureTextWidth(value)
	}
	widths, err := r.runWidths(runs)
	if err != nil {
		return 0, err
	}
	var total float64
	for _, width := range widths {
		total += width
	}
	return total, nil
}

// runWidths measures each run in its family and leaves the current font set.
func (r *renderer) runWidths(runs []textRun) ([]float64, error) {
	widths := make([]float64, len(runs))
	for index, run := range runs {
		if err := r.pdf.SetFont(run.family, "", r.size); err != nil {
			return nil, err
		}
		width, err := r.pdf.MeasureTextWidth(run.text)
		if err != nil {
			return nil, err
		}
		widths[index] = width
	}
	return widths, r.pdf.SetFont(r.family, "", r.size)
}

// Render returns the bill as a PDF document.
func Render(bill Bill) ([]byte, error) {
	if strings.TrimSpace(bill.Number) == "" {
		return nil, errors.New("bill number is required")
	}
	if len(bill.Items) == 0 {
		return nil, errors.New("bill has no items")
	}
	coverage, err := fontCoverage()
	if err != nil {
		return nil, err
	}
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: gopdf.Rect{W: pageWidth, H: pageHeight}})
	for _, font := range []struct {
		family string
		data   []byte
	}{
		{fontRegular, regularFont},
		{fontBold, boldFont},
		{fontRegularFallback, regularFallbackFont},
		{fontBoldFallback, boldFallbackFont},
	} {
		if err := pdf.AddTTFFontData(font.family, font.data); err != nil {
			return nil, fmt.Errorf("load %s font: %w", font.family, err)
		}
	}
	createdAt := bill.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	bill.Labels = bill.Labels.withDefaults()
	pdf.SetInfo(gopdf.PdfInfo{
		Title:        bill.Labels.Title + " " + bill.Labels.Subtitle + " " + bill.Number,
		Author:       bill.IssuerHeading,
		Creator:      bill.IssuerHeading,
		Producer:     bill.IssuerHeading,
		CreationDate: createdAt,
	})
	r := &renderer{pdf: pdf, coverage: coverage}
	if err := r.render(bill); err != nil {
		return nil, err
	}
	return pdf.GetBytesPdfReturnErr()
}

func (r *renderer) render(bill Bill) error {
	r.pdf.AddPage()
	if err := r.header(bill); err != nil {
		return err
	}
	if err := r.parties(bill); err != nil {
		return err
	}
	if err := r.amountDue(bill); err != nil {
		return err
	}
	if err := r.tableHeader(bill.Labels); err != nil {
		return err
	}
	for _, item := range bill.Items {
		if err := r.item(bill, item); err != nil {
			return err
		}
	}
	if err := r.totals(bill); err != nil {
		return err
	}
	return r.footer(bill)
}

func (r *renderer) setFont(family string, size float64, color [3]uint8) error {
	r.pdf.SetTextColor(color[0], color[1], color[2])
	r.family, r.size = family, size
	return r.pdf.SetFont(family, "", size)
}

func (r *renderer) rule(x1, y, x2 float64, width float64, color [3]uint8) {
	r.pdf.SetStrokeColor(color[0], color[1], color[2])
	r.pdf.SetLineWidth(width)
	r.pdf.Line(x1, y, x2, y)
}

// text draws one line with its top at y. A line that needs the fallback font
// is drawn run by run, placed as the whole line would be.
func (r *renderer) text(x, y, w float64, value string, align int) error {
	runs := r.runs(value)
	if len(runs) <= 1 && (len(runs) == 0 || runs[0].family == r.family) {
		r.pdf.SetXY(x, y)
		return r.pdf.CellWithOption(&gopdf.Rect{W: w, H: 1}, value, gopdf.CellOption{Align: align | gopdf.Top})
	}
	widths, err := r.runWidths(runs)
	if err != nil {
		return err
	}
	var total float64
	for _, width := range widths {
		total += width
	}
	switch {
	case align&gopdf.Right != 0:
		x += w - total
	case align&gopdf.Center != 0:
		x += (w - total) / 2
	}
	for index, run := range runs {
		if err := r.pdf.SetFont(run.family, "", r.size); err != nil {
			return err
		}
		r.pdf.SetXY(x, y)
		if err := r.pdf.CellWithOption(&gopdf.Rect{W: widths[index], H: 1}, run.text, gopdf.CellOption{Align: gopdf.Left | gopdf.Top}); err != nil {
			return err
		}
		x += widths[index]
	}
	return r.pdf.SetFont(r.family, "", r.size)
}

// wrap splits value into lines no wider than width with the current font,
// breaking Latin text at spaces and CJK text between characters.
func (r *renderer) wrap(value string, width float64) ([]string, error) {
	var lines []string
	for paragraph := range strings.SplitSeq(strings.TrimSpace(value), "\n") {
		runes := []rune(strings.TrimSpace(paragraph))
		for len(runes) > 0 {
			end := 0
			lastSpace := -1
			for end < len(runes) {
				w, err := r.measure(string(runes[:end+1]))
				if err != nil {
					return nil, err
				}
				if w > width && end > 0 {
					break
				}
				if runes[end] == ' ' {
					lastSpace = end
				}
				end++
			}
			if end < len(runes) && lastSpace > 0 && runes[end] != ' ' {
				end = lastSpace
			}
			lines = append(lines, strings.TrimSpace(string(runes[:end])))
			runes = []rune(strings.TrimLeft(string(runes[end:]), " "))
		}
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines, nil
}

func (r *renderer) header(bill Bill) error {
	if err := r.setFont(fontBold, 30, colorText); err != nil {
		return err
	}
	if err := r.text(marginX, marginTop-2*mm, 60*mm, bill.Labels.Title, gopdf.Left); err != nil {
		return err
	}
	if err := r.setFont(fontRegular, 11, colorText); err != nil {
		return err
	}
	if err := r.text(marginX, marginTop+12*mm, 60*mm, bill.Labels.Subtitle, gopdf.Left); err != nil {
		return err
	}
	if err := r.setFont(fontBold, 46, colorText); err != nil {
		return err
	}
	if err := r.text(marginX, marginTop-4*mm, contentW, bill.IssuerHeading, gopdf.Right); err != nil {
		return err
	}

	r.y = marginTop + 20*mm
	fields := []Field{{Label: "發出日期", Value: bill.IssueDate}, {Label: "到期日", Value: bill.DueDate}}
	if len(bill.HeaderFields) > 0 {
		fields = bill.HeaderFields
	}
	fields = append([]Field{{Label: bill.Labels.NumberLabel, Value: bill.Number}}, fields...)
	if err := r.setFont(fontRegular, 10.5, colorText); err != nil {
		return err
	}
	// Values start after the widest label, and never before 24 mm.
	labelW := 24 * mm
	for _, field := range fields {
		width, err := r.measure(field.Label)
		if err != nil {
			return err
		}
		labelW = max(labelW, width+4*mm)
	}
	for _, field := range fields {
		if err := r.setFont(fontRegular, 10.5, colorText); err != nil {
			return err
		}
		if err := r.text(marginX, r.y+2.5*mm, labelW, field.Label, gopdf.Left); err != nil {
			return err
		}
		if err := r.text(marginX+labelW, r.y+2*mm, 68*mm, field.Value, gopdf.Left); err != nil {
			return err
		}
		r.y += rowHeight
		r.rule(marginX+labelW, r.y, marginX+labelW+68*mm, 0.6, colorRule)
	}
	return nil
}

// partyColumn draws a heading and its labelled lines, padding to three
// lines like the sample, and returns where the column ends.
func (r *renderer) partyColumn(x, top, width float64, heading string, fields []Field) (float64, error) {
	if err := r.setFont(fontBold, 13.5, colorText); err != nil {
		return 0, err
	}
	if err := r.text(x, top, width, heading, gopdf.Left); err != nil {
		return 0, err
	}
	y := top + 9*mm
	const labelW = 30 * mm
	const fontSize = 10
	for index := range max(3, len(fields)) {
		var lines []string
		var label string
		if index < len(fields) {
			label = fields[index].Label
			if err := r.setFont(fontRegular, fontSize, colorText); err != nil {
				return 0, err
			}
			wrapped, err := r.wrap(fields[index].Value, width-labelW)
			if err != nil {
				return 0, err
			}
			lines = wrapped
		}
		lineH := fontSize * lineSpacing
		height := max(rowHeight, float64(len(lines))*lineH+3.5*mm)
		textTop := y + height - 1.5*mm - float64(len(lines))*lineH
		if label != "" {
			if err := r.setFont(fontRegular, 9, colorMuted); err != nil {
				return 0, err
			}
			// The label sits on the first line of a wrapped value.
			if err := r.text(x, textTop+0.4*mm, labelW, label, gopdf.Left); err != nil {
				return 0, err
			}
		}
		if err := r.setFont(fontRegular, fontSize, colorText); err != nil {
			return 0, err
		}
		for lineIndex, line := range lines {
			if err := r.text(x+labelW, textTop+float64(lineIndex)*lineH, width-labelW, line, gopdf.Left); err != nil {
				return 0, err
			}
		}
		y += height
		r.rule(x, y, x+width, 0.6, colorRule)
	}
	return y, nil
}

func (r *renderer) parties(bill Bill) error {
	top := r.y + 9*mm
	issuerEnd, err := r.partyColumn(marginX, top, 100*mm, bill.IssuerHeading, bill.Issuer)
	if err != nil {
		return err
	}
	recipientEnd, err := r.partyColumn(marginX+125*mm, top, 100*mm, bill.Labels.RecipientHeading, bill.Recipient)
	if err != nil {
		return err
	}
	r.y = max(issuerEnd, recipientEnd)
	return nil
}

func (r *renderer) amountDue(bill Bill) error {
	top := r.y + 7*mm
	if err := r.setFont(fontBold, 21, colorText); err != nil {
		return err
	}
	labelW, err := r.measure(bill.Labels.Amount)
	if err != nil {
		return err
	}
	// The amount starts at 48 mm unless the label is wider.
	amountX := marginX + max(48*mm, labelW+6*mm)
	if err := r.text(marginX, top, amountX-marginX, bill.Labels.Amount, gopdf.Left); err != nil {
		return err
	}
	if err := r.setFont(fontBold, 19, colorText); err != nil {
		return err
	}
	if err := r.text(amountX, top+0.5*mm, 100*mm, formatAmount(bill.Currency, bill.TotalCents), gopdf.Left); err != nil {
		return err
	}
	r.rule(amountX, top+10*mm, amountX+100*mm, 0.8, colorRule)
	r.y = top + 10*mm
	return nil
}

func (r *renderer) tableHeader(labels Labels) error {
	top := r.y + 8*mm
	if err := r.setFont(fontRegular, 10.5, colorText); err != nil {
		return err
	}
	right := marginX + contentW
	if err := r.text(marginX, top, 100*mm, labels.Description, gopdf.Left); err != nil {
		return err
	}
	if err := r.text(right-colAmountW-colUnitW-colQtyW, top, colQtyW, labels.Quantity, gopdf.Right); err != nil {
		return err
	}
	if err := r.text(right-colAmountW-colUnitW, top, colUnitW, labels.UnitPrice, gopdf.Right); err != nil {
		return err
	}
	if err := r.text(right-colAmountW, top, colAmountW, labels.LineAmount, gopdf.Right); err != nil {
		return err
	}
	r.y = top + 6.5*mm
	r.rule(marginX, r.y, right, 1.6, colorText)
	return nil
}

// newPage continues the bill on another page: footer on the page being left,
// a short reminder of the bill number on the new one.
func (r *renderer) newPage(bill Bill, withTableHeader bool) error {
	if err := r.footer(bill); err != nil {
		return err
	}
	r.pdf.AddPage()
	if err := r.setFont(fontRegular, 10, colorMuted); err != nil {
		return err
	}
	if err := r.text(marginX, marginTop, contentW, fmt.Sprintf("%s %s %s（續）", bill.Labels.Title, bill.Labels.Subtitle, bill.Number), gopdf.Left); err != nil {
		return err
	}
	r.y = marginTop + 2*mm
	if withTableHeader {
		return r.tableHeader(bill.Labels)
	}
	r.y += 6 * mm
	return nil
}

func (r *renderer) item(bill Bill, item Item) error {
	right := marginX + contentW
	descW := contentW - colAmountW - colUnitW - colQtyW - 6*mm
	if err := r.setFont(fontRegular, 10.5, colorText); err != nil {
		return err
	}
	descLines, err := r.wrap(item.Description, descW)
	if err != nil {
		return err
	}
	var detailLines []string
	if strings.TrimSpace(item.Detail) != "" {
		if err := r.setFont(fontRegular, 8.5, colorFaint); err != nil {
			return err
		}
		detailLines, err = r.wrap(item.Detail, descW)
		if err != nil {
			return err
		}
	}
	const pad = 2.6 * mm
	descH := 10.5 * lineSpacing
	detailH := 8.5 * lineSpacing
	height := pad*2 + float64(len(descLines))*descH + float64(len(detailLines))*detailH
	if r.y+height > contentBottom {
		if err := r.newPage(bill, true); err != nil {
			return err
		}
	}
	top := r.y + pad
	if err := r.setFont(fontRegular, 10.5, colorText); err != nil {
		return err
	}
	for index, line := range descLines {
		if err := r.text(marginX, top+float64(index)*descH, descW, line, gopdf.Left); err != nil {
			return err
		}
	}
	if err := r.text(right-colAmountW-colUnitW-colQtyW, top, colQtyW, fmt.Sprintf("%d", item.Quantity), gopdf.Right); err != nil {
		return err
	}
	if err := r.text(right-colAmountW-colUnitW, top, colUnitW, formatAmount(bill.Currency, item.UnitCents), gopdf.Right); err != nil {
		return err
	}
	if err := r.text(right-colAmountW, top, colAmountW, formatAmount(bill.Currency, item.AmountCents), gopdf.Right); err != nil {
		return err
	}
	if len(detailLines) > 0 {
		if err := r.setFont(fontRegular, 8.5, colorFaint); err != nil {
			return err
		}
		detailTop := top + float64(len(descLines))*descH
		for index, line := range detailLines {
			if err := r.text(marginX, detailTop+float64(index)*detailH, descW, line, gopdf.Left); err != nil {
				return err
			}
		}
	}
	r.y += height
	r.rule(marginX, r.y, right, 0.6, colorLight)
	return nil
}

func (r *renderer) totals(bill Bill) error {
	const lineH = 7.5 * mm
	if r.y+4*mm+3*lineH > contentBottom {
		if err := r.newPage(bill, false); err != nil {
			return err
		}
	}
	right := marginX + contentW
	left := right - 172*mm
	y := r.y + 4*mm
	total := formatAmount(bill.Currency, bill.TotalCents)
	for index, label := range []string{bill.Labels.Subtotal, bill.Labels.Total, bill.Labels.Amount} {
		family, size := fontRegular, 10.5
		if index == 2 {
			family, size = fontBold, 12.5
		}
		if err := r.setFont(family, size, colorText); err != nil {
			return err
		}
		if err := r.text(left, y+1.6*mm, 60*mm, label, gopdf.Left); err != nil {
			return err
		}
		if err := r.text(left, y+1.6*mm, 172*mm, total, gopdf.Right); err != nil {
			return err
		}
		y += lineH
		r.rule(left, y, right, 0.6, colorRule)
	}
	r.y = y
	return nil
}

func (r *renderer) footer(bill Bill) error {
	if strings.TrimSpace(bill.Footer) == "" {
		return nil
	}
	if err := r.setFont(fontRegular, 8, colorFaint); err != nil {
		return err
	}
	return r.text(marginX, footerY, contentW, bill.Footer, gopdf.Center)
}

// formatAmount prints cents as "CNY 21,000.00".
func formatAmount(currency string, cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	whole := fmt.Sprintf("%d", cents/100)
	var grouped strings.Builder
	for index, digit := range whole {
		if index > 0 && (len(whole)-index)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	amount := fmt.Sprintf("%s%s.%02d", sign, grouped.String(), cents%100)
	if currency == "" {
		return amount
	}
	return currency + " " + amount
}

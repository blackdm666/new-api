package dto

type Notify struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Values  []any  `json:"values"`
}

const ContentValueParam = "{{value}}"

// NotifyLink is a notification value that email renders as a clickable link.
// Other channels receive the plain URL. Only code-built http(s) URLs belong here.
type NotifyLink string

const (
	NotifyTypeQuotaExceed     = "quota_exceed"
	NotifyTypeChannelUpdate   = "channel_update"
	NotifyTypeChannelTest     = "channel_test"
	NotifyTypeInspectionAlert = "inspection_alert"
	NotifyTypeInvoiceIssued   = "invoice_issued"
)

func NewNotify(t string, title string, content string, values []any) Notify {
	return Notify{
		Type:    t,
		Title:   title,
		Content: content,
		Values:  values,
	}
}

package setting

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
)

// DefaultInvoiceWechatApiBaseUrl is the hosted WeChat API used to push invoice
// events to the finance group.
const DefaultInvoiceWechatApiBaseUrl = "http://api.wechatapi.net/finder/v2/api"

var (
	InvoiceFileEnabled              = true
	InvoiceFileMaxSize        int64 = 5 * 1024 * 1024
	InvoiceFileMaxCount             = 5
	InvoiceMinimumAmountCents int64 = 50000
	InvoiceTaxRateBasisPoints       = 300
	InvoiceDataRetentionDays        = 0
	InvoicePendingExpiryDays        = 30

	InvoiceFileAllowedExts  = "jpg,jpeg,png,webp,pdf"
	InvoiceFileAllowedMimes = "image/*,application/pdf"

	InvoiceFileStorage   = "local"
	InvoiceFileLocalPath = "/data/invoice_files"

	InvoiceFileOSSEndpoint        = ""
	InvoiceFileOSSBucket          = ""
	InvoiceFileOSSRegion          = ""
	InvoiceFileOSSAccessKeyId     = ""
	InvoiceFileOSSAccessKeySecret = ""
	InvoiceFileOSSCustomDomain    = ""

	InvoiceFileS3Endpoint        = ""
	InvoiceFileS3Bucket          = ""
	InvoiceFileS3Region          = ""
	InvoiceFileS3AccessKeyId     = ""
	InvoiceFileS3AccessKeySecret = ""
	InvoiceFileS3CustomDomain    = ""

	InvoiceFileCOSEndpoint     = ""
	InvoiceFileCOSBucket       = ""
	InvoiceFileCOSRegion       = ""
	InvoiceFileCOSSecretId     = ""
	InvoiceFileCOSSecretKey    = ""
	InvoiceFileCOSCustomDomain = ""

	InvoiceFileSignedURLTTL int64 = 900

	InvoiceWechatNotifyEnabled = false
	InvoiceWechatApiBaseUrl    = DefaultInvoiceWechatApiBaseUrl
	InvoiceWechatApiToken      = ""
	InvoiceWechatAppId         = ""
	InvoiceWechatGroupId       = ""

	// InvoiceWechatAutoIssueEnabled lets the finance group bot hand over an
	// invoice PDF posted in the group: it is attached to the pending request
	// whose company name matches the file name, which is then marked issued.
	InvoiceWechatAutoIssueEnabled = false
	// InvoiceWechatInboundToken holds the SHA-256 hex digest of the token the
	// bot presents. The token itself is never stored.
	InvoiceWechatInboundToken = ""
)

// InvoiceWechatInboundTokenMinLength keeps bot tokens at 128 bits or more
// when they are hex encoded.
const InvoiceWechatInboundTokenMinLength = 32

// InvoiceWechatInboundTokenDigest is the stored form of a bot token.
func InvoiceWechatInboundTokenDigest(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

// MatchInvoiceWechatInboundToken reports whether token is the configured bot
// token, comparing digests in constant time.
func MatchInvoiceWechatInboundToken(token string) bool {
	stored := strings.TrimSpace(InvoiceWechatInboundToken)
	if stored == "" || strings.TrimSpace(token) == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(InvoiceWechatInboundTokenDigest(token)), []byte(stored)) == 1
}

// InvoiceWechatConfigured reports whether every value needed to post to the
// finance WeChat group is present, independent of the enabled switch.
func InvoiceWechatConfigured() bool {
	return strings.TrimSpace(InvoiceWechatApiBaseUrl) != "" &&
		strings.TrimSpace(InvoiceWechatApiToken) != "" &&
		strings.TrimSpace(InvoiceWechatAppId) != "" &&
		strings.TrimSpace(InvoiceWechatGroupId) != ""
}

func ValidateInvoiceWechatApiBaseUrl(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("WeChat API base URL must be a valid HTTP or HTTPS URL")
	}
	return nil
}

func ValidateInvoiceWechatGroupId(value string) error {
	value = strings.TrimSpace(value)
	if value != "" && !strings.HasSuffix(value, "@chatroom") {
		return errors.New("WeChat group ID must end with @chatroom")
	}
	return nil
}

func InvoiceFileAllowedExtList() []string {
	raw := strings.Split(InvoiceFileAllowedExts, ",")
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(item)), ".")
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func InvoiceFileAllowedMimeList() []string {
	raw := strings.Split(InvoiceFileAllowedMimes, ",")
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func IsInvoiceFileExtAllowed(ext string) bool {
	ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	if ext == "" {
		return false
	}
	for _, allowed := range InvoiceFileAllowedExtList() {
		if allowed == ext {
			return true
		}
	}
	return false
}

func IsInvoiceFileMimeAllowed(mime string) bool {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if idx := strings.Index(mime, ";"); idx >= 0 {
		mime = strings.TrimSpace(mime[:idx])
	}
	if mime == "" {
		return false
	}
	for _, allowed := range InvoiceFileAllowedMimeList() {
		if allowed == mime {
			return true
		}
		if strings.HasSuffix(allowed, "/*") {
			prefix := strings.TrimSuffix(allowed, "/*") + "/"
			if strings.HasPrefix(mime, prefix) {
				return true
			}
		}
	}
	return false
}

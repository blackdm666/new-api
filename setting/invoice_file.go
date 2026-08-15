package setting

import "strings"

var (
	InvoiceFileEnabled              = true
	InvoiceFileMaxSize        int64 = 5 * 1024 * 1024
	InvoiceFileMaxCount             = 5
	InvoiceMinimumAmountCents int64 = 50000
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
)

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

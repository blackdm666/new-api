package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicefile"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	minInvoiceFileSize      int64 = 1024 * 1024
	maxInvoiceFileSize      int64 = 5 * 1024 * 1024
	maxInvoiceFilesPerIssue       = 20
)

type InvoiceSettingsPayload struct {
	InvoiceApplicationNotifyAdminEnabled bool   `json:"InvoiceApplicationNotifyAdminEnabled"`
	InvoiceIssuedNotifyUserEnabled       bool   `json:"InvoiceIssuedNotifyUserEnabled"`
	InvoiceAdminEmail                    string `json:"InvoiceAdminEmail"`
	InvoiceMinimumAmountCents            int64  `json:"InvoiceMinimumAmountCents"`
	InvoiceDataRetentionDays             int    `json:"InvoiceDataRetentionDays"`
	InvoicePendingExpiryDays             int    `json:"InvoicePendingExpiryDays"`
	InvoiceFileEnabled                   bool   `json:"InvoiceFileEnabled"`
	InvoiceFileStorage                   string `json:"InvoiceFileStorage"`
	InvoiceFileMaxSize                   int64  `json:"InvoiceFileMaxSize"`
	InvoiceFileMaxCount                  int    `json:"InvoiceFileMaxCount"`
	InvoiceFileAllowedExts               string `json:"InvoiceFileAllowedExts"`
	InvoiceFileAllowedMimes              string `json:"-"`
	InvoiceFileLocalPath                 string `json:"InvoiceFileLocalPath"`
	InvoiceFileSignedURLTTL              int64  `json:"InvoiceFileSignedURLTTL"`

	InvoiceFileOSSEndpoint        string `json:"InvoiceFileOSSEndpoint"`
	InvoiceFileOSSBucket          string `json:"InvoiceFileOSSBucket"`
	InvoiceFileOSSRegion          string `json:"InvoiceFileOSSRegion"`
	InvoiceFileOSSAccessKeyId     string `json:"InvoiceFileOSSAccessKeyId"`
	InvoiceFileOSSAccessKeySecret string `json:"InvoiceFileOSSAccessKeySecret"`
	InvoiceFileOSSCustomDomain    string `json:"InvoiceFileOSSCustomDomain"`

	InvoiceFileS3Endpoint        string `json:"InvoiceFileS3Endpoint"`
	InvoiceFileS3Bucket          string `json:"InvoiceFileS3Bucket"`
	InvoiceFileS3Region          string `json:"InvoiceFileS3Region"`
	InvoiceFileS3AccessKeyId     string `json:"InvoiceFileS3AccessKeyId"`
	InvoiceFileS3AccessKeySecret string `json:"InvoiceFileS3AccessKeySecret"`
	InvoiceFileS3CustomDomain    string `json:"InvoiceFileS3CustomDomain"`

	InvoiceFileCOSEndpoint     string `json:"InvoiceFileCOSEndpoint"`
	InvoiceFileCOSBucket       string `json:"InvoiceFileCOSBucket"`
	InvoiceFileCOSRegion       string `json:"InvoiceFileCOSRegion"`
	InvoiceFileCOSSecretId     string `json:"InvoiceFileCOSSecretId"`
	InvoiceFileCOSSecretKey    string `json:"InvoiceFileCOSSecretKey"`
	InvoiceFileCOSCustomDomain string `json:"InvoiceFileCOSCustomDomain"`
}

func validateInvoiceSettingsPayload(payload *InvoiceSettingsPayload) error {
	if payload == nil {
		return errors.New("invoice settings are required")
	}
	payload.InvoiceAdminEmail = strings.TrimSpace(payload.InvoiceAdminEmail)
	if payload.InvoiceApplicationNotifyAdminEnabled || payload.InvoicePendingExpiryDays > 0 {
		if err := validateInvoiceNotificationEmails(payload.InvoiceAdminEmail); err != nil {
			return fmt.Errorf("invoice admin email: %w", err)
		}
	}
	if payload.InvoiceMinimumAmountCents <= 0 || payload.InvoiceMinimumAmountCents > 100_000_000 {
		return errors.New("minimum invoice amount must be between CNY 0.01 and CNY 1,000,000")
	}
	if payload.InvoiceDataRetentionDays != 0 && (payload.InvoiceDataRetentionDays < 30 || payload.InvoiceDataRetentionDays > 36500) {
		return errors.New("invoice data retention must be 0 or between 30 and 36500 days")
	}
	if payload.InvoicePendingExpiryDays != 0 && (payload.InvoicePendingExpiryDays < 1 || payload.InvoicePendingExpiryDays > 3650) {
		return errors.New("pending invoice expiry must be 0 or between 1 and 3650 days")
	}
	if payload.InvoiceFileMaxSize < minInvoiceFileSize || payload.InvoiceFileMaxSize > maxInvoiceFileSize {
		return errors.New("invoice file size limit must be between 1 MiB and 5 MiB")
	}
	if payload.InvoiceFileMaxCount < 1 || payload.InvoiceFileMaxCount > maxInvoiceFilesPerIssue {
		return fmt.Errorf("invoice file count must be between 1 and %d", maxInvoiceFilesPerIssue)
	}
	if payload.InvoiceFileSignedURLTTL < 60 || payload.InvoiceFileSignedURLTTL > 86400 {
		return errors.New("signed URL TTL must be between 60 and 86400 seconds")
	}
	if err := normalizeInvoiceFileTypes(payload); err != nil {
		return err
	}

	payload.InvoiceFileStorage = strings.ToLower(strings.TrimSpace(payload.InvoiceFileStorage))
	payload.InvoiceFileLocalPath = strings.TrimSpace(payload.InvoiceFileLocalPath)
	if payload.InvoiceFileStorage == "local" {
		if payload.InvoiceFileLocalPath == "" {
			return errors.New("local invoice storage path is required")
		}
		return nil
	}
	if payload.InvoiceFileStorage == "oss" {
		if strings.TrimSpace(payload.InvoiceFileOSSEndpoint) == "" || strings.TrimSpace(payload.InvoiceFileOSSBucket) == "" || strings.TrimSpace(payload.InvoiceFileOSSAccessKeyId) == "" || strings.TrimSpace(payload.InvoiceFileOSSAccessKeySecret) == "" {
			return errors.New("OSS endpoint, bucket, access key ID, and access key secret are required")
		}
		if err := validateInvoiceStorageURL(payload.InvoiceFileOSSEndpoint, "OSS endpoint"); err != nil {
			return err
		}
		return validateOptionalInvoiceStorageURL(payload.InvoiceFileOSSCustomDomain, "OSS custom domain")
	}
	if payload.InvoiceFileStorage == "s3" {
		if strings.TrimSpace(payload.InvoiceFileS3Bucket) == "" || strings.TrimSpace(payload.InvoiceFileS3Region) == "" || strings.TrimSpace(payload.InvoiceFileS3AccessKeyId) == "" || strings.TrimSpace(payload.InvoiceFileS3AccessKeySecret) == "" {
			return errors.New("S3 bucket, region, access key ID, and access key secret are required")
		}
		if strings.TrimSpace(payload.InvoiceFileS3CustomDomain) != "" {
			return errors.New("S3 custom domain cannot be used with signed private invoice files; configure the S3 endpoint instead")
		}
		return validateOptionalInvoiceStorageURL(payload.InvoiceFileS3Endpoint, "S3 endpoint")
	}
	if payload.InvoiceFileStorage == "cos" {
		if strings.TrimSpace(payload.InvoiceFileCOSSecretId) == "" || strings.TrimSpace(payload.InvoiceFileCOSSecretKey) == "" {
			return errors.New("COS secret ID and secret key are required")
		}
		if strings.TrimSpace(payload.InvoiceFileCOSEndpoint) == "" && (strings.TrimSpace(payload.InvoiceFileCOSBucket) == "" || strings.TrimSpace(payload.InvoiceFileCOSRegion) == "") {
			return errors.New("COS endpoint or bucket and region are required")
		}
		if err := validateOptionalInvoiceStorageURL(payload.InvoiceFileCOSEndpoint, "COS endpoint"); err != nil {
			return err
		}
		return validateOptionalInvoiceStorageURL(payload.InvoiceFileCOSCustomDomain, "COS custom domain")
	}
	return errors.New("unsupported invoice file storage")
}

func preserveInvoiceStorageSecrets(payload *InvoiceSettingsPayload) {
	if payload == nil {
		return
	}
	if strings.TrimSpace(payload.InvoiceFileOSSAccessKeySecret) == "" {
		payload.InvoiceFileOSSAccessKeySecret = setting.InvoiceFileOSSAccessKeySecret
	}
	if strings.TrimSpace(payload.InvoiceFileS3AccessKeySecret) == "" {
		payload.InvoiceFileS3AccessKeySecret = setting.InvoiceFileS3AccessKeySecret
	}
	if strings.TrimSpace(payload.InvoiceFileCOSSecretKey) == "" {
		payload.InvoiceFileCOSSecretKey = setting.InvoiceFileCOSSecretKey
	}
}

func (payload *InvoiceSettingsPayload) storageConfig() invoicefile.Config {
	config := invoicefile.Config{StorageType: payload.InvoiceFileStorage}
	switch payload.InvoiceFileStorage {
	case "local", "":
		config.StorageType = "local"
		config.LocalPath = payload.InvoiceFileLocalPath
	case "oss":
		config.Endpoint = payload.InvoiceFileOSSEndpoint
		config.Bucket = payload.InvoiceFileOSSBucket
		config.Region = payload.InvoiceFileOSSRegion
		config.AccessKeyId = payload.InvoiceFileOSSAccessKeyId
		config.AccessSecret = payload.InvoiceFileOSSAccessKeySecret
		config.CustomDomain = payload.InvoiceFileOSSCustomDomain
	case "s3":
		config.Endpoint = payload.InvoiceFileS3Endpoint
		config.Bucket = payload.InvoiceFileS3Bucket
		config.Region = payload.InvoiceFileS3Region
		config.AccessKeyId = payload.InvoiceFileS3AccessKeyId
		config.AccessSecret = payload.InvoiceFileS3AccessKeySecret
	case "cos":
		config.Endpoint = payload.InvoiceFileCOSEndpoint
		config.Bucket = payload.InvoiceFileCOSBucket
		config.Region = payload.InvoiceFileCOSRegion
		config.AccessKeyId = payload.InvoiceFileCOSSecretId
		config.AccessSecret = payload.InvoiceFileCOSSecretKey
		config.CustomDomain = payload.InvoiceFileCOSCustomDomain
	}
	return config
}

func validateInvoiceNotificationEmails(value string) error {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' })
	if len(parts) == 0 {
		return errors.New("at least one email address is required")
	}
	for _, part := range parts {
		email := strings.TrimSpace(part)
		parsed, err := mail.ParseAddress(email)
		if err != nil || !strings.EqualFold(parsed.Address, email) {
			return fmt.Errorf("invalid email address: %s", email)
		}
	}
	return nil
}

func normalizeInvoiceFileTypes(payload *InvoiceSettingsPayload) error {
	mimeByExt := map[string]string{
		"jpg":  "image/jpeg",
		"jpeg": "image/jpeg",
		"png":  "image/png",
		"webp": "image/webp",
		"pdf":  "application/pdf",
	}
	seen := make(map[string]struct{})
	extensions := make([]string, 0)
	mimes := make([]string, 0)
	seenMimes := make(map[string]struct{})
	for _, raw := range strings.Split(payload.InvoiceFileAllowedExts, ",") {
		ext := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(raw)), ".")
		if ext == "" {
			continue
		}
		mime, ok := mimeByExt[ext]
		if !ok {
			return fmt.Errorf("unsupported invoice file extension: %s", ext)
		}
		if _, ok := seen[ext]; ok {
			continue
		}
		seen[ext] = struct{}{}
		extensions = append(extensions, ext)
		if _, ok := seenMimes[mime]; !ok {
			seenMimes[mime] = struct{}{}
			mimes = append(mimes, mime)
		}
	}
	if len(extensions) == 0 {
		return errors.New("at least one invoice file extension is required")
	}
	payload.InvoiceFileAllowedExts = strings.Join(extensions, ",")
	payload.InvoiceFileAllowedMimes = strings.Join(mimes, ",")
	return nil
}

func validateOptionalInvoiceStorageURL(value string, label string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return validateInvoiceStorageURL(value, label)
}

func validateInvoiceStorageURL(value string, label string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be a valid HTTP or HTTPS URL", label)
	}
	return nil
}

func (payload *InvoiceSettingsPayload) optionValues() map[string]string {
	return map[string]string{
		"InvoiceApplicationNotifyAdminEnabled": strconv.FormatBool(payload.InvoiceApplicationNotifyAdminEnabled),
		"InvoiceIssuedNotifyUserEnabled":       strconv.FormatBool(payload.InvoiceIssuedNotifyUserEnabled),
		"InvoiceAdminEmail":                    payload.InvoiceAdminEmail,
		"InvoiceMinimumAmountCents":            strconv.FormatInt(payload.InvoiceMinimumAmountCents, 10),
		"InvoiceDataRetentionDays":             strconv.Itoa(payload.InvoiceDataRetentionDays),
		"InvoicePendingExpiryDays":             strconv.Itoa(payload.InvoicePendingExpiryDays),
		"InvoiceFileEnabled":                   strconv.FormatBool(payload.InvoiceFileEnabled),
		"InvoiceFileStorage":                   payload.InvoiceFileStorage,
		"InvoiceFileMaxSize":                   strconv.FormatInt(payload.InvoiceFileMaxSize, 10),
		"InvoiceFileMaxCount":                  strconv.Itoa(payload.InvoiceFileMaxCount),
		"InvoiceFileAllowedExts":               payload.InvoiceFileAllowedExts,
		"InvoiceFileAllowedMimes":              payload.InvoiceFileAllowedMimes,
		"InvoiceFileLocalPath":                 payload.InvoiceFileLocalPath,
		"InvoiceFileSignedURLTTL":              strconv.FormatInt(payload.InvoiceFileSignedURLTTL, 10),
		"InvoiceFileOSSEndpoint":               strings.TrimSpace(payload.InvoiceFileOSSEndpoint),
		"InvoiceFileOSSBucket":                 strings.TrimSpace(payload.InvoiceFileOSSBucket),
		"InvoiceFileOSSRegion":                 strings.TrimSpace(payload.InvoiceFileOSSRegion),
		"InvoiceFileOSSAccessKeyId":            strings.TrimSpace(payload.InvoiceFileOSSAccessKeyId),
		"InvoiceFileOSSAccessKeySecret":        strings.TrimSpace(payload.InvoiceFileOSSAccessKeySecret),
		"InvoiceFileOSSCustomDomain":           strings.TrimRight(strings.TrimSpace(payload.InvoiceFileOSSCustomDomain), "/"),
		"InvoiceFileS3Endpoint":                strings.TrimRight(strings.TrimSpace(payload.InvoiceFileS3Endpoint), "/"),
		"InvoiceFileS3Bucket":                  strings.TrimSpace(payload.InvoiceFileS3Bucket),
		"InvoiceFileS3Region":                  strings.TrimSpace(payload.InvoiceFileS3Region),
		"InvoiceFileS3AccessKeyId":             strings.TrimSpace(payload.InvoiceFileS3AccessKeyId),
		"InvoiceFileS3AccessKeySecret":         strings.TrimSpace(payload.InvoiceFileS3AccessKeySecret),
		"InvoiceFileS3CustomDomain":            "",
		"InvoiceFileCOSEndpoint":               strings.TrimRight(strings.TrimSpace(payload.InvoiceFileCOSEndpoint), "/"),
		"InvoiceFileCOSBucket":                 strings.TrimSpace(payload.InvoiceFileCOSBucket),
		"InvoiceFileCOSRegion":                 strings.TrimSpace(payload.InvoiceFileCOSRegion),
		"InvoiceFileCOSSecretId":               strings.TrimSpace(payload.InvoiceFileCOSSecretId),
		"InvoiceFileCOSSecretKey":              strings.TrimSpace(payload.InvoiceFileCOSSecretKey),
		"InvoiceFileCOSCustomDomain":           strings.TrimRight(strings.TrimSpace(payload.InvoiceFileCOSCustomDomain), "/"),
	}
}

func UpdateInvoiceSettings(c *gin.Context) {
	var payload InvoiceSettingsPayload
	if err := common.DecodeJson(c.Request.Body, &payload); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvoiceSettingsInvalid)
		return
	}
	preserveInvoiceStorageSecrets(&payload)
	if err := validateInvoiceSettingsPayload(&payload); err != nil {
		common.SysLog("invalid invoice settings: " + err.Error())
		common.ApiErrorI18n(c, i18n.MsgInvoiceSettingsInvalid)
		return
	}
	oldProfile, err := invoicefile.EnsureCurrentProfile()
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceSettingsSaveFailed, err)
		return
	}
	if err := model.BackfillLegacyInvoiceStorageProfile(oldProfile.Id); err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceSettingsSaveFailed, err)
		return
	}
	if _, err := invoicefile.EnsureProfile(payload.storageConfig()); err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceSettingsSaveFailed, err)
		return
	}
	if err := model.UpdateOptionsBulk(payload.optionValues()); err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceSettingsSaveFailed, err)
		return
	}
	recordManageAudit(c, "invoice.settings.update", map[string]interface{}{
		"storage": payload.InvoiceFileStorage,
	})
	common.ApiSuccess(c, gin.H{"storage": payload.InvoiceFileStorage})
}

func TestInvoiceStorage(c *gin.Context) {
	storage, err := invoicefile.Current()
	if err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceStorageTestFailed, err)
		return
	}
	key := path.Join("_health", uuid.NewString()+".txt")
	body := []byte("invoice storage health check")
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Minute)
	defer cancel()
	if err := storage.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "text/plain"); err != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceStorageTestFailed, err)
		return
	}
	cleanup := func() error {
		deleteErr := storage.Delete(ctx, key)
		if deleteErr != nil {
			profile, profileErr := invoicefile.EnsureCurrentProfile()
			if profileErr == nil {
				_ = model.EnqueueInvoiceFileCleanup(profile.Id, storage.Kind(), key, deleteErr.Error())
			}
		}
		return deleteErr
	}
	exists, err := storage.Exists(ctx, key)
	if err != nil || !exists {
		_ = cleanup()
		if err == nil {
			err = errors.New("uploaded health-check object was not found")
		}
		respondInvoiceInternalError(c, i18n.MsgInvoiceStorageTestFailed, err)
		return
	}
	reader, err := storage.Get(ctx, key)
	if err != nil {
		_ = cleanup()
		respondInvoiceInternalError(c, i18n.MsgInvoiceStorageTestFailed, err)
		return
	}
	if storage.Kind() != "local" {
		signedURL, signErr := storage.SignedURL(ctx, key, time.Minute, "invoice-storage-test.txt", false)
		if signErr != nil || signedURL == "" {
			_ = reader.Close()
			_ = cleanup()
			if signErr == nil {
				signErr = errors.New("invoice storage did not return a signed URL")
			}
			respondInvoiceInternalError(c, i18n.MsgInvoiceStorageTestFailed, signErr)
			return
		}
	}
	readBody, readErr := io.ReadAll(io.LimitReader(reader, int64(len(body)+1)))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(readBody, body) {
		_ = cleanup()
		if readErr == nil {
			readErr = closeErr
		}
		if readErr == nil {
			readErr = errors.New("invoice storage health-check content mismatch")
		}
		respondInvoiceInternalError(c, i18n.MsgInvoiceStorageTestFailed, readErr)
		return
	}
	if deleteErr := cleanup(); deleteErr != nil {
		respondInvoiceInternalError(c, i18n.MsgInvoiceStorageTestFailed, fmt.Errorf("delete invoice storage health-check object: %w", deleteErr))
		return
	}
	recordManageAudit(c, "invoice.storage.test", map[string]interface{}{"storage": storage.Kind()})
	common.ApiSuccess(c, gin.H{"storage": storage.Kind()})
}

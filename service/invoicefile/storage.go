// Package invoicefile provides validation and storage for issued invoice files.
//
// 设计原则：
//   - 业务层（controller/service）只依赖 Storage 接口，不关心底层是本地磁盘还是对象存储；
//   - 对象存储实现返回签名 URL 供前端直接访问；本地实现走鉴权下载接口，文件不暴露到静态路径；
//   - 所有实现必须保证 key 仅来自服务端生成（uuid.ext），拒绝任何来自客户端的路径片段。
package invoicefile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
)

const invoiceStorageProfilePurpose = "invoice-storage-profile"

type Config struct {
	StorageType  string `json:"storage_type"`
	LocalPath    string `json:"local_path,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
	Bucket       string `json:"bucket,omitempty"`
	Region       string `json:"region,omitempty"`
	AccessKeyId  string `json:"access_key_id,omitempty"`
	AccessSecret string `json:"access_secret,omitempty"`
	CustomDomain string `json:"custom_domain,omitempty"`
}

// Storage 抽象附件读写操作。所有方法必须是并发安全的。
type Storage interface {
	// Kind 返回存储类型标识（local / oss / s3 / cos），用于在 DB 中登记。
	Kind() string
	// Put 以给定 key 写入数据。实现需要对 key 做最终的安全检查，避免目录穿越。
	Put(ctx context.Context, key string, r io.Reader, size int64, mime string) error
	// Get 以只读方式返回文件内容。调用方负责关闭 reader。
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete 幂等删除，不存在不应报错。
	Delete(ctx context.Context, key string) error
	// Exists 检查 key 是否存在。
	Exists(ctx context.Context, key string) (bool, error)
	// ListKeysPage 按游标枚举对象键，用于全量存储对账。nextCursor 为空表示已到末页。
	ListKeysPage(ctx context.Context, prefix string, cursor string, limit int) (keys []string, nextCursor string, err error)
	// SignedURL 为对象存储生成短时签名地址；本地存储可返回 ("", ErrSignedURLNotSupported)。
	// filename 会作为下载时的显示文件名（Content-Disposition）。
	SignedURL(ctx context.Context, key string, ttl time.Duration, filename string, inline bool) (string, error)
}

// ErrSignedURLNotSupported 标识当前存储后端不支持生成签名 URL。
// 本地存储返回此错误时，controller 应退回到鉴权代理下载流程。
var ErrSignedURLNotSupported = errors.New("signed url not supported for this storage")

// ErrStorageNotConfigured 当对应云厂商的凭证/桶等必填项未配置时返回。
var ErrStorageNotConfigured = errors.New("storage backend is not fully configured")

var ErrS3CustomDomainUnsupported = errors.New("s3 custom domains cannot be used with signed private invoice files; configure the S3 endpoint instead")

// ErrNotFound 对象不存在时的通用错误。
var ErrNotFound = errors.New("object not found")

func ResponseContentDisposition(filename string, inline bool) string {
	if filename == "" {
		return ""
	}
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	return fmt.Sprintf("%s; filename*=UTF-8''%s", disposition, url.PathEscape(filename))
}

// Current returns the storage backend configured for electronic invoice files.
// 每次调用都构造新的实例：实现都是廉价的结构体包装，且设置可能在运行时被热更新。
func Current() (Storage, error) {
	return NewStorage(CurrentConfig())
}

// ForKind resolves the backend recorded on an invoice file. Downloads and
// cleanup use this so switching providers does not strand existing files.
func ForKind(kind string) (Storage, error) {
	switch kind {
	case "", "local":
		return NewLocalStorage(setting.InvoiceFileLocalPath)
	case "oss":
		return NewOSSStorage()
	case "s3":
		return NewS3Storage()
	case "cos":
		return NewCOSStorage()
	default:
		return nil, fmt.Errorf("unknown invoice file storage: %s", kind)
	}
}

func CurrentConfig() Config {
	kind := setting.InvoiceFileStorage
	switch kind {
	case "", "local":
		return Config{StorageType: "local", LocalPath: setting.InvoiceFileLocalPath}.normalized()
	case "oss":
		return currentOSSConfig()
	case "s3":
		return currentS3Config()
	case "cos":
		return currentCOSConfig()
	}
	return Config{StorageType: kind}.normalized()
}

func currentOSSConfig() Config {
	return Config{
		StorageType: "oss", Endpoint: setting.InvoiceFileOSSEndpoint,
		Bucket: setting.InvoiceFileOSSBucket, Region: setting.InvoiceFileOSSRegion,
		AccessKeyId: setting.InvoiceFileOSSAccessKeyId, AccessSecret: setting.InvoiceFileOSSAccessKeySecret,
		CustomDomain: setting.InvoiceFileOSSCustomDomain,
	}.normalized()
}

func currentS3Config() Config {
	return Config{
		StorageType: "s3", Endpoint: setting.InvoiceFileS3Endpoint,
		Bucket: setting.InvoiceFileS3Bucket, Region: setting.InvoiceFileS3Region,
		AccessKeyId: setting.InvoiceFileS3AccessKeyId, AccessSecret: setting.InvoiceFileS3AccessKeySecret,
		CustomDomain: setting.InvoiceFileS3CustomDomain,
	}.normalized()
}

func currentCOSConfig() Config {
	return Config{
		StorageType: "cos", Endpoint: setting.InvoiceFileCOSEndpoint,
		Bucket: setting.InvoiceFileCOSBucket, Region: setting.InvoiceFileCOSRegion,
		AccessKeyId: setting.InvoiceFileCOSSecretId, AccessSecret: setting.InvoiceFileCOSSecretKey,
		CustomDomain: setting.InvoiceFileCOSCustomDomain,
	}.normalized()
}

func (config Config) normalized() Config {
	config.StorageType = strings.ToLower(strings.TrimSpace(config.StorageType))
	if config.StorageType == "" {
		config.StorageType = "local"
	}
	config.LocalPath = strings.TrimSpace(config.LocalPath)
	config.Endpoint = strings.TrimRight(strings.TrimSpace(config.Endpoint), "/")
	config.Bucket = strings.TrimSpace(config.Bucket)
	config.Region = strings.TrimSpace(config.Region)
	config.AccessKeyId = strings.TrimSpace(config.AccessKeyId)
	config.AccessSecret = strings.TrimSpace(config.AccessSecret)
	config.CustomDomain = strings.TrimRight(strings.TrimSpace(config.CustomDomain), "/")
	return config
}

func NewStorage(config Config) (Storage, error) {
	config = config.normalized()
	switch config.StorageType {
	case "local":
		return NewLocalStorage(config.LocalPath)
	case "oss":
		return newOSSStorage(config)
	case "s3":
		return newS3Storage(config)
	case "cos":
		return newCOSStorage(config)
	default:
		return nil, fmt.Errorf("unknown invoice file storage: %s", config.StorageType)
	}
}

func EnsureProfile(config Config) (*model.InvoiceStorageProfile, error) {
	config = config.normalized()
	plain, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	fingerprintRaw := sha256.Sum256(plain)
	fingerprint := hex.EncodeToString(fingerprintRaw[:])
	if existing, err := findProfileByFingerprint(fingerprint); err == nil {
		if _, decryptErr := ConfigFromProfile(existing); decryptErr != nil {
			return nil, fmt.Errorf("decrypt existing invoice storage profile %d: %w", existing.Id, decryptErr)
		}
		return existing, nil
	}
	encrypted, err := common.EncryptSensitiveValue(invoiceStorageProfilePurpose, plain)
	if err != nil {
		return nil, err
	}
	return model.GetOrCreateInvoiceStorageProfile(config.StorageType, fingerprint, encrypted)
}

func findProfileByFingerprint(fingerprint string) (*model.InvoiceStorageProfile, error) {
	profiles, err := model.ListInvoiceStorageProfiles()
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if profile.Fingerprint == fingerprint {
			return profile, nil
		}
	}
	return nil, errors.New("invoice storage profile not found")
}

func EnsureCurrentProfile() (*model.InvoiceStorageProfile, error) {
	return EnsureProfile(CurrentConfig())
}

func ConfigFromProfile(profile *model.InvoiceStorageProfile) (Config, error) {
	if profile == nil || profile.Id <= 0 {
		return Config{}, errors.New("invoice storage profile is required")
	}
	plain, err := common.DecryptSensitiveValue(invoiceStorageProfilePurpose, profile.EncryptedConfig)
	if err != nil {
		return Config{}, err
	}
	var config Config
	if err := json.Unmarshal(plain, &config); err != nil {
		return Config{}, err
	}
	return config.normalized(), nil
}

func ForProfile(profileId int, legacyKind string) (Storage, error) {
	if profileId <= 0 {
		return ForKind(legacyKind)
	}
	profile, err := model.GetInvoiceStorageProfile(profileId)
	if err != nil {
		return nil, err
	}
	config, err := ConfigFromProfile(profile)
	if err != nil {
		return nil, err
	}
	return NewStorage(config)
}

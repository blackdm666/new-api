package invoicefile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/setting"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// OSSStorage 对接阿里云 OSS。
// 设计取舍：
//   - 每次调用 Put/Get 时才获取 Client，而不是在进程启动时初始化——便于管理员改配置立即生效；
//   - SignedURL 使用 OSS 的 SignURL，生效期由 setting.InvoiceFileSignedURLTTL 控制；
//   - 下载时通过 response-content-disposition 参数把前端显示文件名带进去，避免二次代理。
type OSSStorage struct {
	client        *oss.Client
	bucket        *oss.Bucket
	signingBucket *oss.Bucket

	customDomain string
}

// NewOSSStorage 按当前 setting 构造 OSS 客户端；任何必填项缺失都返回 ErrStorageNotConfigured。
func NewOSSStorage() (Storage, error) {
	return newOSSStorage(currentOSSConfig())
}

func newOSSStorage(config Config) (Storage, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	bucketName := strings.TrimSpace(config.Bucket)
	ak := strings.TrimSpace(config.AccessKeyId)
	sk := strings.TrimSpace(config.AccessSecret)
	if endpoint == "" || bucketName == "" || ak == "" || sk == "" {
		return nil, ErrStorageNotConfigured
	}
	client, err := oss.New(endpoint, ak, sk)
	if err != nil {
		return nil, fmt.Errorf("init oss client: %w", err)
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("open oss bucket: %w", err)
	}
	storage := &OSSStorage{
		client:       client,
		bucket:       bucket,
		customDomain: strings.TrimRight(strings.TrimSpace(config.CustomDomain), "/"),
	}
	if storage.customDomain != "" {
		customClient, customErr := oss.New(storage.customDomain, ak, sk, oss.UseCname(true))
		if customErr != nil {
			return nil, fmt.Errorf("init oss custom-domain signer: %w", customErr)
		}
		storage.signingBucket, customErr = customClient.Bucket(bucketName)
		if customErr != nil {
			return nil, fmt.Errorf("open oss custom-domain signing bucket: %w", customErr)
		}
	}
	return storage, nil
}

func (s *OSSStorage) Kind() string { return "oss" }

func (s *OSSStorage) Put(ctx context.Context, key string, r io.Reader, size int64, mime string) error {
	if key == "" {
		return errors.New("empty key")
	}
	opts := []oss.Option{}
	if mime != "" {
		opts = append(opts, oss.ContentType(mime))
	}
	if size > 0 {
		opts = append(opts, oss.ContentLength(size))
	}
	return s.bucket.PutObject(key, r, opts...)
}

func (s *OSSStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	rc, err := s.bucket.GetObject(key)
	if err != nil {
		if isOSSNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return rc, nil
}

func (s *OSSStorage) Delete(ctx context.Context, key string) error {
	if err := s.bucket.DeleteObject(key); err != nil {
		if isOSSNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

func (s *OSSStorage) Exists(ctx context.Context, key string) (bool, error) {
	ok, err := s.bucket.IsObjectExist(key)
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (s *OSSStorage) ListKeysPage(ctx context.Context, prefix string, cursor string, limit int) ([]string, string, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}
	keys := make([]string, 0, min(limit, 128))
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	result, err := s.bucket.ListObjects(oss.Prefix(prefix), oss.Marker(cursor), oss.MaxKeys(limit))
	if err != nil {
		return nil, "", err
	}
	for _, object := range result.Objects {
		keys = append(keys, object.Key)
	}
	nextCursor := ""
	if result.IsTruncated {
		nextCursor = result.NextMarker
	}
	return keys, nextCursor, nil
}

func (s *OSSStorage) SignedURL(ctx context.Context, key string, ttl time.Duration, filename string, inline bool) (string, error) {
	if ttl <= 0 {
		ttl = time.Duration(setting.InvoiceFileSignedURLTTL) * time.Second
	}
	var opts []oss.Option
	if filename != "" {
		// RFC 5987 友好的 UTF-8 文件名
		opts = append(opts, oss.ResponseContentDisposition(ResponseContentDisposition(filename, inline)))
	}
	signingBucket := s.bucket
	if s.signingBucket != nil {
		signingBucket = s.signingBucket
	}
	signed, err := signingBucket.SignURL(key, oss.HTTPGet, int64(ttl.Seconds()), opts...)
	if err != nil {
		return "", err
	}
	return signed, nil
}

func isOSSNotFound(err error) bool {
	if err == nil {
		return false
	}
	var srvErr oss.ServiceError
	if errors.As(err, &srvErr) {
		return srvErr.StatusCode == 404
	}
	return false
}

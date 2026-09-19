package invoicefile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/setting"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Storage 适配任何兼容 AWS S3 协议的对象存储（官方 AWS、MinIO、R2 等）。
//   - 需同时配置 endpoint（可选，默认走 AWS）、region、bucket、AK/SK；
//   - 自定义 endpoint 场景强制走 path-style（兼容 MinIO）。
type S3Storage struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func NewS3Storage() (Storage, error) {
	return newS3Storage(currentS3Config())
}

func newS3Storage(config Config) (Storage, error) {
	bucket := strings.TrimSpace(config.Bucket)
	region := strings.TrimSpace(config.Region)
	ak := strings.TrimSpace(config.AccessKeyId)
	sk := strings.TrimSpace(config.AccessSecret)
	endpoint := strings.TrimSpace(config.Endpoint)
	if strings.TrimSpace(config.CustomDomain) != "" {
		return nil, ErrS3CustomDomainUnsupported
	}
	if bucket == "" || region == "" || ak == "" || sk == "" {
		return nil, ErrStorageNotConfigured
	}
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(ak, sk, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	optFns := []func(*s3.Options){}
	if endpoint != "" {
		optFns = append(optFns, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		})
	}
	client := s3.NewFromConfig(cfg, optFns...)
	return &S3Storage{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  bucket,
	}, nil
}

func (s *S3Storage) Kind() string { return "s3" }

func (s *S3Storage) Put(ctx context.Context, key string, r io.Reader, size int64, mime string) error {
	if key == "" {
		return errors.New("empty key")
	}
	in := &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentLength: aws.Int64(size),
	}
	if mime != "" {
		in.ContentType = aws.String(mime)
	}
	if size <= 0 {
		// 没有声明长度时 S3 SDK 会 buffer 整个 body，允许但 put 进度未知。
		in.ContentLength = nil
	}
	_, err := s.client.PutObject(ctx, in)
	return err
}

func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return out.Body, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil && !isS3NotFound(err) {
		return err
	}
	return nil
}

func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *S3Storage) ListKeysPage(ctx context.Context, prefix string, cursor string, limit int) ([]string, string, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}
	input := &s3.ListObjectsV2Input{Bucket: aws.String(s.bucket), MaxKeys: aws.Int32(int32(limit))}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}
	if cursor != "" {
		input.ContinuationToken = aws.String(cursor)
	}
	page, err := s.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, "", err
	}
	keys := make([]string, 0, len(page.Contents))
	for _, object := range page.Contents {
		keys = append(keys, aws.ToString(object.Key))
	}
	return keys, aws.ToString(page.NextContinuationToken), nil
}

func (s *S3Storage) SignedURL(ctx context.Context, key string, ttl time.Duration, filename string, inline bool) (string, error) {
	if ttl <= 0 {
		ttl = time.Duration(setting.InvoiceFileSignedURLTTL) * time.Second
	}
	in := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}
	if filename != "" {
		in.ResponseContentDisposition = aws.String(ResponseContentDisposition(filename, inline))
	}
	signed, err := s.presign.PresignGetObject(ctx, in, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return signed.URL, nil
}

func isS3NotFound(err error) bool {
	if err == nil {
		return false
	}
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		if code == "NoSuchKey" || code == "NotFound" || code == "404" {
			return true
		}
	}
	return false
}

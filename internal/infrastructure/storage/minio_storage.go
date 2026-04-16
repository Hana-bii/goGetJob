package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOOptions struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	Region    string
	UseSSL    bool
}

type MinIOStorage struct {
	client *minio.Client
	bucket string
}

func NewMinIOStorage(opts MinIOOptions) (*MinIOStorage, error) {
	if opts.Endpoint == "" {
		return nil, fmt.Errorf("storage endpoint is required")
	}
	if opts.Bucket == "" {
		return nil, fmt.Errorf("storage bucket is required")
	}

	endpoint := strings.TrimPrefix(strings.TrimPrefix(opts.Endpoint, "https://"), "http://")
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(opts.AccessKey, opts.SecretKey, ""),
		Secure: opts.UseSSL || strings.HasPrefix(opts.Endpoint, "https://"),
		Region: opts.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create MinIO client: %w", err)
	}

	return &MinIOStorage{
		client: client,
		bucket: opts.Bucket,
	}, nil
}

func NewMinIOStorageWithClient(client *minio.Client, bucket string) *MinIOStorage {
	return &MinIOStorage{client: client, bucket: bucket}
}

func (s *MinIOStorage) PutObject(ctx context.Context, input PutObjectInput) (ObjectInfo, error) {
	if s.client == nil {
		return ObjectInfo{}, fmt.Errorf("MinIO client is required")
	}
	if input.Key == "" {
		return ObjectInfo{}, fmt.Errorf("object key is required")
	}

	info, err := s.client.PutObject(ctx, s.bucket, input.Key, input.Reader, input.Size, minio.PutObjectOptions{
		ContentType: input.ContentType,
	})
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("put object %q: %w", input.Key, err)
	}
	return ObjectInfo{
		Key:         input.Key,
		Size:        info.Size,
		ContentType: input.ContentType,
		ETag:        info.ETag,
	}, nil
}

func (s *MinIOStorage) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if s.client == nil {
		return nil, fmt.Errorf("MinIO client is required")
	}
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}
	return object, nil
}

func (s *MinIOStorage) DeleteObject(ctx context.Context, key string) error {
	if s.client == nil {
		return fmt.Errorf("MinIO client is required")
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete object %q: %w", key, err)
	}
	return nil
}

func (s *MinIOStorage) PresignedGetObject(ctx context.Context, key string, expires time.Duration) (*url.URL, error) {
	if s.client == nil {
		return nil, fmt.Errorf("MinIO client is required")
	}
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, expires, nil)
	if err != nil {
		return nil, fmt.Errorf("presign object %q: %w", key, err)
	}
	return u, nil
}

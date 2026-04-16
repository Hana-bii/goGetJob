package storage

import (
	"context"
	"io"
	"net/url"
	"time"
)

type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
	ETag        string
}

type PutObjectInput struct {
	Key         string
	Reader      io.Reader
	Size        int64
	ContentType string
}

type Storage interface {
	PutObject(ctx context.Context, input PutObjectInput) (ObjectInfo, error)
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, key string) error
	PresignedGetObject(ctx context.Context, key string, expires time.Duration) (*url.URL, error)
}

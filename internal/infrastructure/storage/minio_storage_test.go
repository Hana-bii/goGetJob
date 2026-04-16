package storage

import (
	"context"
	"errors"
	"io"
	"net/url"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

type fakeObjectClient struct {
	statErr   error
	statCalls int
	getCalls  int
}

func (f *fakeObjectClient) PutObject(context.Context, string, string, io.Reader, int64, minio.PutObjectOptions) (minio.UploadInfo, error) {
	return minio.UploadInfo{}, nil
}

func (f *fakeObjectClient) StatObject(context.Context, string, string, minio.StatObjectOptions) (minio.ObjectInfo, error) {
	f.statCalls++
	return minio.ObjectInfo{}, f.statErr
}

func (f *fakeObjectClient) GetObject(context.Context, string, string, minio.GetObjectOptions) (io.ReadCloser, error) {
	f.getCalls++
	return io.NopCloser(nil), nil
}

func (f *fakeObjectClient) RemoveObject(context.Context, string, string, minio.RemoveObjectOptions) error {
	return nil
}

func (f *fakeObjectClient) PresignedGetObject(context.Context, string, string, time.Duration, url.Values) (*url.URL, error) {
	return &url.URL{Scheme: "https", Host: "example.com"}, nil
}

func TestMinIOStorageGetObjectStatsBeforeGet(t *testing.T) {
	statErr := errors.New("missing object")
	client := &fakeObjectClient{statErr: statErr}
	store := newMinIOStorageWithObjectClient(client, "bucket")

	got, err := store.GetObject(context.Background(), "missing.txt")

	require.Nil(t, got)
	require.ErrorIs(t, err, statErr)
	require.Equal(t, 1, client.statCalls)
	require.Equal(t, 0, client.getCalls)
}

func TestMinIOStorageWithNilClientReturnsError(t *testing.T) {
	store := NewMinIOStorageWithClient(nil, "bucket")

	got, err := store.GetObject(context.Background(), "object.txt")

	require.Nil(t, got)
	require.ErrorContains(t, err, "MinIO client is required")
}

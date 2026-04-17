package resume

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	commonmodel "goGetJob/internal/common/model"
	docfile "goGetJob/internal/infrastructure/file"
	"goGetJob/internal/infrastructure/storage"
)

func TestUploadRejectsInvalidType(t *testing.T) {
	svc := newUploadTestService(t)

	_, err := svc.UploadBytes(context.Background(), UploadInput{
		Filename:    "resume.exe",
		ContentType: "application/octet-stream",
		Data:        []byte("not a resume"),
	})

	require.ErrorContains(t, err, "unsupported")
}

func TestUploadStoresResumePendingAndEnqueuesAnalyzeTask(t *testing.T) {
	svc := newUploadTestService(t)

	got, err := svc.UploadBytes(context.Background(), UploadInput{
		Filename:    "resume.txt",
		ContentType: "text/plain",
		Data:        []byte("Go backend engineer\nRedis and Gin"),
	})

	require.NoError(t, err)
	require.False(t, got.Duplicate)
	require.Equal(t, commonmodel.AsyncTaskStatusPending, got.Resume.AnalyzeStatus)
	require.Equal(t, "resume.txt", got.Resume.OriginalFilename)
	require.NotZero(t, got.Resume.ID)
	require.Len(t, svc.storage.puts, 1)
	require.Len(t, svc.producer.tasks, 1)
	require.Equal(t, got.Resume.ID, svc.producer.tasks[0].ResumeID)
	require.Contains(t, svc.producer.tasks[0].Content, "Redis")
}

func TestUploadDuplicateHashReturnsExistingWithoutStorageOrEnqueue(t *testing.T) {
	svc := newUploadTestService(t)
	ctx := context.Background()
	input := UploadInput{
		Filename:    "resume.txt",
		ContentType: "text/plain",
		Data:        []byte("same resume"),
	}
	first, err := svc.UploadBytes(ctx, input)
	require.NoError(t, err)

	second, err := svc.UploadBytes(ctx, input)

	require.NoError(t, err)
	require.True(t, second.Duplicate)
	require.Equal(t, first.Resume.ID, second.Resume.ID)
	require.Equal(t, 2, second.Resume.AccessCount)
	require.Len(t, svc.storage.puts, 1)
	require.Len(t, svc.producer.tasks, 1)
}

func TestUploadMarksFailedWhenStreamEnqueueFailsAndTruncatesError(t *testing.T) {
	svc := newUploadTestService(t)
	svc.producer.err = errors.New(string(bytes.Repeat([]byte("x"), 600)))

	_, err := svc.UploadBytes(context.Background(), UploadInput{
		Filename:    "resume.txt",
		ContentType: "text/plain",
		Data:        []byte("resume text"),
	})

	require.ErrorContains(t, err, "enqueue")
	stored, findErr := svc.repo.FindResumeByHash(context.Background(), docfile.HashBytes([]byte("resume text")))
	require.NoError(t, findErr)
	require.Equal(t, commonmodel.AsyncTaskStatusFailed, stored.AnalyzeStatus)
	require.Len(t, stored.AnalyzeError, maxAnalyzeErrorLength)
}

type uploadTestService struct {
	*UploadService
	repo     *MemoryRepository
	storage  *fakeStorage
	producer *fakeAnalyzeProducer
}

func newUploadTestService(t *testing.T) *uploadTestService {
	t.Helper()
	repo := NewMemoryRepository()
	store := &fakeStorage{}
	producer := &fakeAnalyzeProducer{}
	svc := NewUploadService(UploadServiceOptions{
		Repository: repo,
		Storage:    store,
		Producer:   producer,
	})
	return &uploadTestService{UploadService: svc, repo: repo, storage: store, producer: producer}
}

type fakeAnalyzeProducer struct {
	tasks []AnalyzeTask
	err   error
}

func (p *fakeAnalyzeProducer) SendAnalyzeTask(_ context.Context, task AnalyzeTask) error {
	if p.err != nil {
		return p.err
	}
	p.tasks = append(p.tasks, task)
	return nil
}

type fakeStorage struct {
	puts    []storage.PutObjectInput
	deleted []string
}

func (s *fakeStorage) PutObject(_ context.Context, input storage.PutObjectInput) (storage.ObjectInfo, error) {
	data, _ := io.ReadAll(input.Reader)
	input.Reader = bytes.NewReader(data)
	s.puts = append(s.puts, input)
	return storage.ObjectInfo{Key: input.Key, Size: input.Size, ContentType: input.ContentType}, nil
}

func (s *fakeStorage) GetObject(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func (s *fakeStorage) DeleteObject(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

func (s *fakeStorage) PresignedGetObject(context.Context, string, time.Duration) (*url.URL, error) {
	return url.Parse("https://files.example/resume")
}

package knowledgebase

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"time"

	"goGetJob/internal/infrastructure/storage"
	"goGetJob/internal/infrastructure/vector"
)

func NewServiceBundleForTest() ServiceBundle {
	repo := NewMemoryRepository()
	vectorStore := vector.NewMemoryStore()
	query := NewQueryService(QueryServiceOptions{Repository: repo, VectorService: NewVectorService(VectorServiceOptions{Store: vectorStore})})
	store := &memoryObjectStorage{objects: map[string][]byte{}}
	producer := VectorizeProducer(noopVectorizeProducer{})
	return ServiceBundle{
		List:    NewListService(repo),
		Upload:  NewUploadService(UploadServiceOptions{Repository: repo, Storage: store, Producer: producer}),
		Delete:  NewDeleteService(repo, store, vectorStore),
		Query:   query,
		RagChat: NewRagChatService(repo, repo, query),
		Storage: store,
	}
}

type noopVectorizeProducer struct{}

func (noopVectorizeProducer) SendVectorizeTask(context.Context, VectorizeTask) error {
	return nil
}

type memoryObjectStorage struct {
	objects map[string][]byte
}

func (s *memoryObjectStorage) PutObject(_ context.Context, input storage.PutObjectInput) (storage.ObjectInfo, error) {
	data, err := io.ReadAll(input.Reader)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	s.objects[input.Key] = data
	return storage.ObjectInfo{Key: input.Key, Size: int64(len(data)), ContentType: input.ContentType}, nil
}

func (s *memoryObjectStorage) GetObject(_ context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.objects[key])), nil
}

func (s *memoryObjectStorage) DeleteObject(_ context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

func (s *memoryObjectStorage) PresignedGetObject(_ context.Context, key string, _ time.Duration) (*url.URL, error) {
	return &url.URL{Scheme: "memory", Host: "knowledgebase", Path: "/" + key}, nil
}

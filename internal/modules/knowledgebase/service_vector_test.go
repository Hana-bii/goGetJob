package knowledgebase

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	commonmodel "goGetJob/internal/common/model"
	"goGetJob/internal/infrastructure/vector"
)

type recordingEmbedder struct {
	batches    [][]string
	queryTexts []string
	err        error
}

func (e *recordingEmbedder) EmbedDocuments(_ context.Context, texts []string) ([][]float32, error) {
	if e.err != nil {
		return nil, e.err
	}
	e.batches = append(e.batches, append([]string(nil), texts...))
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{float32(i + 1)}
	}
	return out, nil
}

func (e *recordingEmbedder) EmbedQuery(_ context.Context, text string) ([]float32, error) {
	e.queryTexts = append(e.queryTexts, text)
	return []float32{0.9, 0.1}, nil
}

type recordingVectorStore struct {
	deleted       []uint
	added         []vector.Document
	replacedKBIDs []uint
	requests      []vector.SearchRequest
}

func (s *recordingVectorStore) DeleteByKnowledgeBaseID(_ context.Context, kbID uint) error {
	s.deleted = append(s.deleted, kbID)
	return nil
}

func (s *recordingVectorStore) AddDocuments(_ context.Context, docs []vector.Document) error {
	s.added = append(s.added, docs...)
	return nil
}

func (s *recordingVectorStore) ReplaceDocuments(_ context.Context, kbID uint, docs []vector.Document) error {
	s.replacedKBIDs = append(s.replacedKBIDs, kbID)
	s.added = append(s.added, docs...)
	return nil
}

func (s *recordingVectorStore) SimilaritySearch(_ context.Context, req vector.SearchRequest) ([]vector.Document, error) {
	s.requests = append(s.requests, req)
	return nil, nil
}

func TestVectorizeDeletesOldVectorsBatchesEmbeddingsAndStoresKBMetadata(t *testing.T) {
	embedder := &recordingEmbedder{}
	store := &recordingVectorStore{}
	service := NewVectorService(VectorServiceOptions{Store: store, Embedder: embedder, BatchSize: 10, ChunkSize: 4})
	content := "one\ntwo\nthr\nfor\nfiv\nsix\nsev\neig\nnin\nten\nelf"

	chunks, err := service.VectorizeAndStore(context.Background(), 42, content)

	require.NoError(t, err)
	require.Equal(t, 11, chunks)
	require.Empty(t, store.deleted)
	require.Equal(t, []uint{42}, store.replacedKBIDs)
	require.Len(t, embedder.batches, 2)
	require.Len(t, embedder.batches[0], 10)
	require.Len(t, embedder.batches[1], 1)
	require.Len(t, store.added, 11)
	for _, doc := range store.added {
		require.Equal(t, "42", doc.Metadata["kb_id"])
	}
}

func TestVectorizeKeepsOldVectorsWhenEmbeddingFails(t *testing.T) {
	store := &recordingVectorStore{}
	service := NewVectorService(VectorServiceOptions{
		Store:     store,
		Embedder:  &recordingEmbedder{err: errors.New("embedding failed")},
		ChunkSize: 10,
	})

	_, err := service.VectorizeAndStore(context.Background(), 7, "redis stream")

	require.Error(t, err)
	require.Empty(t, store.deleted)
	require.Empty(t, store.added)
	require.Empty(t, store.replacedKBIDs)
}

func TestSimilaritySearchEmbedsQueryBeforeSearching(t *testing.T) {
	embedder := &recordingEmbedder{}
	store := &recordingVectorStore{}
	service := NewVectorService(VectorServiceOptions{Store: store, Embedder: embedder})

	_, err := service.SimilaritySearch(context.Background(), "redis pending", []uint{1}, 3, 0.2)

	require.NoError(t, err)
	require.Equal(t, []string{"redis pending"}, embedder.queryTexts)
	require.Len(t, store.requests, 1)
	require.Equal(t, []float32{0.9, 0.1}, store.requests[0].QueryEmbedding)
}

func TestVectorizeTaskHandlerStatusTransitions(t *testing.T) {
	repo := NewMemoryRepository()
	kb := &KnowledgeBase{Name: "kb", FileHash: "h", Content: "alpha beta gamma", VectorStatus: commonmodel.AsyncTaskStatusPending}
	require.NoError(t, repo.CreateKnowledgeBase(context.Background(), kb))
	handler := NewVectorizeTaskHandler(repo, NewVectorService(VectorServiceOptions{
		Store: &recordingVectorStore{}, Embedder: &recordingEmbedder{}, ChunkSize: 20,
	}))

	require.NoError(t, handler.MarkProcessing(context.Background(), VectorizeTask{KnowledgeBaseID: kb.ID, Content: kb.Content}))
	got, err := repo.FindKnowledgeBaseByID(context.Background(), kb.ID)
	require.NoError(t, err)
	require.Equal(t, commonmodel.AsyncTaskStatusProcessing, got.VectorStatus)

	require.NoError(t, handler.ProcessBusiness(context.Background(), VectorizeTask{KnowledgeBaseID: kb.ID, Content: kb.Content}))
	require.NoError(t, handler.MarkCompleted(context.Background(), VectorizeTask{KnowledgeBaseID: kb.ID, Content: kb.Content}))
	got, err = repo.FindKnowledgeBaseByID(context.Background(), kb.ID)
	require.NoError(t, err)
	require.Equal(t, commonmodel.AsyncTaskStatusCompleted, got.VectorStatus)
	require.Equal(t, 1, got.ChunkCount)

	require.NoError(t, handler.MarkFailed(context.Background(), VectorizeTask{KnowledgeBaseID: kb.ID}, fmt.Errorf("boom")))
	got, err = repo.FindKnowledgeBaseByID(context.Background(), kb.ID)
	require.NoError(t, err)
	require.Equal(t, commonmodel.AsyncTaskStatusFailed, got.VectorStatus)
	require.Contains(t, got.VectorError, "boom")
}

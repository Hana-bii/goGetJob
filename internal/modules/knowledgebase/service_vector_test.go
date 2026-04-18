package knowledgebase

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	commonmodel "goGetJob/internal/common/model"
	"goGetJob/internal/infrastructure/vector"
)

type recordingEmbedder struct {
	batches [][]string
}

func (e *recordingEmbedder) EmbedDocuments(_ context.Context, texts []string) ([][]float32, error) {
	e.batches = append(e.batches, append([]string(nil), texts...))
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{float32(i + 1)}
	}
	return out, nil
}

func (e *recordingEmbedder) EmbedQuery(context.Context, string) ([]float32, error) {
	return []float32{1}, nil
}

type recordingVectorStore struct {
	deleted []uint
	added   []vector.Document
}

func (s *recordingVectorStore) DeleteByKnowledgeBaseID(_ context.Context, kbID uint) error {
	s.deleted = append(s.deleted, kbID)
	return nil
}

func (s *recordingVectorStore) AddDocuments(_ context.Context, docs []vector.Document) error {
	s.added = append(s.added, docs...)
	return nil
}

func (s *recordingVectorStore) SimilaritySearch(context.Context, vector.SearchRequest) ([]vector.Document, error) {
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
	require.Equal(t, []uint{42}, store.deleted)
	require.Len(t, embedder.batches, 2)
	require.Len(t, embedder.batches[0], 10)
	require.Len(t, embedder.batches[1], 1)
	require.Len(t, store.added, 11)
	for _, doc := range store.added {
		require.Equal(t, "42", doc.Metadata["kb_id"])
	}
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

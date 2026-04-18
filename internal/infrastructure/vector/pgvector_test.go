package vector

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryStoreFiltersByKBIDAndScoreLocally(t *testing.T) {
	store := NewMemoryStore()
	err := store.AddDocuments(context.Background(), []Document{
		{ID: "1", Content: "redis stream consumer group", Metadata: map[string]any{"kb_id": "7"}, Score: 0.95},
		{ID: "2", Content: "mysql index", Metadata: map[string]any{"kb_id": uint(8)}, Score: 0.93},
		{ID: "3", Content: "redis list", Metadata: map[string]any{"kb_id": int64(7)}, Score: 0.20},
	})
	require.NoError(t, err)

	docs, err := store.SimilaritySearch(context.Background(), SearchRequest{
		Query:            "redis",
		KnowledgeBaseIDs: []uint{7},
		TopK:             5,
		MinScore:         0.3,
	})

	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, "1", docs[0].ID)
}

func TestMemoryStoreDeleteByKnowledgeBaseIDUsesKBIDMetadata(t *testing.T) {
	store := NewMemoryStore()
	require.NoError(t, store.AddDocuments(context.Background(), []Document{
		{ID: "a", Content: "a", Metadata: map[string]any{"kb_id": "1"}},
		{ID: "b", Content: "b", Metadata: map[string]any{"kb_id": uint(2)}},
	}))

	require.NoError(t, store.DeleteByKnowledgeBaseID(context.Background(), 1))

	docs, err := store.SimilaritySearch(context.Background(), SearchRequest{TopK: 10})
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, "b", docs[0].ID)
}

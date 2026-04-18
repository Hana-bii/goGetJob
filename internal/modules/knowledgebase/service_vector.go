package knowledgebase

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"goGetJob/internal/infrastructure/vector"
)

const (
	defaultVectorBatchSize = 10
	defaultChunkSize       = 800
)

type VectorServiceOptions struct {
	Store     vector.Store
	Embedder  vector.Embedder
	BatchSize int
	ChunkSize int
}

type VectorService struct {
	store     vector.Store
	embedder  vector.Embedder
	batchSize int
	chunkSize int
}

func NewVectorService(options VectorServiceOptions) *VectorService {
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = defaultVectorBatchSize
	}
	if batchSize > defaultVectorBatchSize {
		batchSize = defaultVectorBatchSize
	}
	chunkSize := options.ChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	return &VectorService{store: options.Store, embedder: options.Embedder, batchSize: batchSize, chunkSize: chunkSize}
}

func (s *VectorService) VectorizeAndStore(ctx context.Context, kbID uint, content string) (int, error) {
	if s == nil || s.store == nil || s.embedder == nil {
		return 0, fmt.Errorf("vector service dependencies are required")
	}
	_ = s.store.DeleteByKnowledgeBaseID(ctx, kbID)
	chunks := splitChunks(content, s.chunkSize)
	if len(chunks) == 0 {
		return 0, fmt.Errorf("knowledge base content is empty")
	}
	for start := 0; start < len(chunks); start += s.batchSize {
		end := start + s.batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		embeddings, err := s.embedder.EmbedDocuments(ctx, chunks[start:end])
		if err != nil {
			return 0, err
		}
		docs := make([]vector.Document, 0, end-start)
		for i, chunk := range chunks[start:end] {
			docIndex := start + i
			var embedding []float32
			if i < len(embeddings) {
				embedding = embeddings[i]
			}
			docs = append(docs, vector.Document{
				ID:        fmt.Sprintf("%d-%d", kbID, docIndex),
				Content:   chunk,
				Embedding: embedding,
				Metadata: map[string]any{
					"kb_id":       strconv.FormatUint(uint64(kbID), 10),
					"kb_id_long":  kbID,
					"chunk_index": docIndex,
				},
			})
		}
		if err := s.store.AddDocuments(ctx, docs); err != nil {
			return 0, err
		}
	}
	return len(chunks), nil
}

func (s *VectorService) SimilaritySearch(ctx context.Context, query string, kbIDs []uint, topK int, minScore float64) ([]vector.Document, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("vector store is required")
	}
	if topK < 0 {
		topK = 0
	}
	return s.store.SimilaritySearch(ctx, vector.SearchRequest{Query: query, KnowledgeBaseIDs: kbIDs, TopK: topK, MinScore: minScore})
}

func splitChunks(content string, chunkSize int) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	paragraphs := strings.Split(content, "\n")
	chunks := []string{}
	var current strings.Builder
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		if current.Len() > 0 && current.Len()+1+len(paragraph) > chunkSize {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		if len(paragraph) > chunkSize {
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
			}
			for len(paragraph) > chunkSize {
				chunks = append(chunks, paragraph[:chunkSize])
				paragraph = paragraph[chunkSize:]
			}
		}
		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		current.WriteString(paragraph)
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

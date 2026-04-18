package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

type Document struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata"`
	Embedding []float32      `json:"embedding,omitempty"`
	Score     float64        `json:"score,omitempty"`
}

type SearchRequest struct {
	Query            string
	KnowledgeBaseIDs []uint
	TopK             int
	MinScore         float64
}

type Store interface {
	DeleteByKnowledgeBaseID(ctx context.Context, kbID uint) error
	AddDocuments(ctx context.Context, docs []Document) error
	SimilaritySearch(ctx context.Context, req SearchRequest) ([]Document, error)
}

type Embedder interface {
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

type MemoryStore struct {
	mu   sync.Mutex
	docs []Document
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) DeleteByKnowledgeBaseID(_ context.Context, kbID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.docs[:0]
	for _, doc := range s.docs {
		if metadataKBID(doc.Metadata) == kbID {
			continue
		}
		out = append(out, doc)
	}
	s.docs = out
	return nil
}

func (s *MemoryStore) AddDocuments(_ context.Context, docs []Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, doc := range docs {
		if doc.Metadata == nil {
			doc.Metadata = map[string]any{}
		}
		s.docs = append(s.docs, doc)
	}
	return nil
}

func (s *MemoryStore) SimilaritySearch(_ context.Context, req SearchRequest) ([]Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filterIDs := map[uint]bool{}
	for _, id := range req.KnowledgeBaseIDs {
		filterIDs[id] = true
	}
	query := strings.ToLower(strings.TrimSpace(req.Query))
	result := []Document{}
	for _, doc := range s.docs {
		if len(filterIDs) > 0 && !filterIDs[metadataKBID(doc.Metadata)] {
			continue
		}
		score := doc.Score
		if score == 0 {
			score = lexicalScore(query, strings.ToLower(doc.Content))
		}
		if req.MinScore > 0 && score < req.MinScore {
			continue
		}
		copy := doc
		copy.Score = score
		result = append(result, copy)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})
	if req.TopK >= 0 && len(result) > req.TopK {
		result = result[:req.TopK]
	}
	return result, nil
}

type PGVectorStore struct {
	db *gorm.DB
}

func NewPGVectorStore(db *gorm.DB) *PGVectorStore {
	return &PGVectorStore{db: db}
}

func (s *PGVectorStore) DeleteByKnowledgeBaseID(ctx context.Context, kbID uint) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("gorm db is required")
	}
	return s.db.WithContext(ctx).
		Exec("DELETE FROM vector_store WHERE metadata->>'kb_id' = ? OR metadata->>'kb_id_long' = ?", strconv.FormatUint(uint64(kbID), 10), strconv.FormatUint(uint64(kbID), 10)).
		Error
}

func (s *PGVectorStore) AddDocuments(context.Context, []Document) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("gorm db is required")
	}
	return fmt.Errorf("pgvector add documents is not configured; use MemoryStore or implement vector table mapping")
}

func (s *PGVectorStore) SimilaritySearch(context.Context, SearchRequest) ([]Document, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("gorm db is required")
	}
	return nil, fmt.Errorf("pgvector similarity search is not configured; use MemoryStore or implement vector table mapping")
}

type OpenAIEmbedder struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenAIEmbedder(baseURL, apiKey, model string, client *http.Client) *OpenAIEmbedder {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &OpenAIEmbedder{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, model: model, client: client}
}

func (e *OpenAIEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embed(ctx, texts)
}

func (e *OpenAIEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vectors, err := e.embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("embedding provider returned no vector")
	}
	return vectors[0], nil
}

func (e *OpenAIEmbedder) embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e == nil || e.baseURL == "" || e.model == "" {
		return nil, fmt.Errorf("embedding provider is not configured")
	}
	payload := map[string]any{"model": e.model, "input": texts}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v1/embeddings", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("embedding provider returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	out := make([][]float32, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		out = append(out, item.Embedding)
	}
	return out, nil
}

func metadataKBID(metadata map[string]any) uint {
	if metadata == nil {
		return 0
	}
	for _, key := range []string{"kb_id", "kb_id_long"} {
		switch value := metadata[key].(type) {
		case uint:
			return value
		case uint64:
			return uint(value)
		case int:
			return uint(value)
		case int64:
			return uint(value)
		case float64:
			return uint(value)
		case string:
			parsed, _ := strconv.ParseUint(value, 10, 64)
			return uint(parsed)
		}
	}
	return 0
}

func lexicalScore(query, content string) float64 {
	if query == "" {
		return 1
	}
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return 1
	}
	matches := 0
	for _, term := range terms {
		if strings.Contains(content, term) {
			matches++
		}
	}
	return math.Min(1, float64(matches)/float64(len(terms)))
}

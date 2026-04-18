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
	QueryEmbedding   []float32
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

func (s *MemoryStore) ReplaceDocuments(_ context.Context, kbID uint, docs []Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.docs[:0]
	for _, doc := range s.docs {
		if metadataKBID(doc.Metadata) == kbID {
			continue
		}
		out = append(out, doc)
	}
	for _, doc := range docs {
		if doc.Metadata == nil {
			doc.Metadata = map[string]any{}
		}
		out = append(out, doc)
	}
	s.docs = out
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
		if len(req.QueryEmbedding) > 0 && len(doc.Embedding) > 0 {
			score = cosineSimilarity(req.QueryEmbedding, doc.Embedding)
		} else if score == 0 {
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
	db         *gorm.DB
	migrateMu  sync.Mutex
	migrated   bool
	migrateErr error
}

func NewPGVectorStore(db *gorm.DB) *PGVectorStore {
	return &PGVectorStore{db: db}
}

func (s *PGVectorStore) DeleteByKnowledgeBaseID(ctx context.Context, kbID uint) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("gorm db is required")
	}
	if err := s.ensureTable(); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Where("knowledge_base_id = ?", kbID).Delete(&VectorRecord{}).Error
}

func (s *PGVectorStore) AddDocuments(ctx context.Context, docs []Document) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("gorm db is required")
	}
	if err := s.ensureTable(); err != nil {
		return err
	}
	records := make([]VectorRecord, 0, len(docs))
	for _, doc := range docs {
		metadata, err := json.Marshal(doc.Metadata)
		if err != nil {
			return fmt.Errorf("marshal vector metadata: %w", err)
		}
		embedding, err := json.Marshal(doc.Embedding)
		if err != nil {
			return fmt.Errorf("marshal vector embedding: %w", err)
		}
		records = append(records, VectorRecord{
			ID:              doc.ID,
			KnowledgeBaseID: metadataKBID(doc.Metadata),
			Content:         doc.Content,
			Metadata:        string(metadata),
			Embedding:       string(embedding),
		})
	}
	if len(records) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Save(&records).Error
}

func (s *PGVectorStore) ReplaceDocuments(ctx context.Context, kbID uint, docs []Document) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("gorm db is required")
	}
	if err := s.ensureTable(); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("knowledge_base_id = ?", kbID).Delete(&VectorRecord{}).Error; err != nil {
			return err
		}
		records := make([]VectorRecord, 0, len(docs))
		for _, doc := range docs {
			metadata, err := json.Marshal(doc.Metadata)
			if err != nil {
				return fmt.Errorf("marshal vector metadata: %w", err)
			}
			embedding, err := json.Marshal(doc.Embedding)
			if err != nil {
				return fmt.Errorf("marshal vector embedding: %w", err)
			}
			records = append(records, VectorRecord{
				ID:              doc.ID,
				KnowledgeBaseID: metadataKBID(doc.Metadata),
				Content:         doc.Content,
				Metadata:        string(metadata),
				Embedding:       string(embedding),
			})
		}
		if len(records) == 0 {
			return nil
		}
		return tx.Save(&records).Error
	})
}

func (s *PGVectorStore) SimilaritySearch(ctx context.Context, req SearchRequest) ([]Document, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("gorm db is required")
	}
	if err := s.ensureTable(); err != nil {
		return nil, err
	}
	var records []VectorRecord
	query := s.db.WithContext(ctx)
	if len(req.KnowledgeBaseIDs) > 0 {
		query = query.Where("knowledge_base_id IN ?", req.KnowledgeBaseIDs)
	}
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	searchText := strings.ToLower(strings.TrimSpace(req.Query))
	docs := make([]Document, 0, len(records))
	for _, record := range records {
		var metadata map[string]any
		_ = json.Unmarshal([]byte(record.Metadata), &metadata)
		var embedding []float32
		_ = json.Unmarshal([]byte(record.Embedding), &embedding)
		score := lexicalScore(searchText, strings.ToLower(record.Content))
		if len(req.QueryEmbedding) > 0 && len(embedding) > 0 {
			score = cosineSimilarity(req.QueryEmbedding, embedding)
		}
		if req.MinScore > 0 && score < req.MinScore {
			continue
		}
		docs = append(docs, Document{
			ID:        record.ID,
			Content:   record.Content,
			Metadata:  metadata,
			Embedding: embedding,
			Score:     score,
		})
	}
	sort.SliceStable(docs, func(i, j int) bool {
		return docs[i].Score > docs[j].Score
	})
	if req.TopK >= 0 && len(docs) > req.TopK {
		docs = docs[:req.TopK]
	}
	return docs, nil
}

func (s *PGVectorStore) ensureTable() error {
	s.migrateMu.Lock()
	defer s.migrateMu.Unlock()
	if s.migrated || s.migrateErr != nil {
		return s.migrateErr
	}
	s.migrateErr = s.db.AutoMigrate(&VectorRecord{})
	s.migrated = s.migrateErr == nil
	return s.migrateErr
}

type VectorRecord struct {
	ID              string `gorm:"primaryKey;size:128"`
	KnowledgeBaseID uint   `gorm:"index;not null"`
	Content         string `gorm:"type:text"`
	Metadata        string `gorm:"type:text"`
	Embedding       string `gorm:"type:text"`
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

func cosineSimilarity(left, right []float32) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	var dot, leftNorm, rightNorm float64
	for i := 0; i < limit; i++ {
		l := float64(left[i])
		r := float64(right[i])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

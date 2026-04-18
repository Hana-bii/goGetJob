package knowledgebase

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"goGetJob/internal/common/ai"
	"goGetJob/internal/common/config"
	"goGetJob/internal/infrastructure/vector"
)

type scriptedChatModel struct {
	responses []string
	errors    []error
	prompts   []string
}

func (m *scriptedChatModel) Generate(_ context.Context, messages []ai.ChatMessage) (string, error) {
	m.prompts = append(m.prompts, messages[0].Content)
	if len(m.errors) > 0 {
		err := m.errors[0]
		m.errors = m.errors[1:]
		if err != nil {
			return "", err
		}
	}
	if len(m.responses) == 0 {
		return "", nil
	}
	next := m.responses[0]
	m.responses = m.responses[1:]
	return next, nil
}

type queryVectorService struct {
	requests []vector.SearchRequest
	docs     map[string][]vector.Document
}

func (s *queryVectorService) SimilaritySearch(_ context.Context, query string, kbIDs []uint, topK int, minScore float64) ([]vector.Document, error) {
	s.requests = append(s.requests, vector.SearchRequest{Query: query, KnowledgeBaseIDs: kbIDs, TopK: topK, MinScore: minScore})
	return append([]vector.Document(nil), s.docs[query]...), nil
}

func TestQueryRewriteFallbackCandidateOrderAndDynamicSearchTuning(t *testing.T) {
	rewrite := &scriptedChatModel{responses: []string{"Redis Stream pending list"}}
	answer := &scriptedChatModel{responses: []string{"Use XPENDING and XCLAIM."}}
	vectorSvc := &queryVectorService{docs: map[string][]vector.Document{
		"Redis Stream pending list": {{Content: "XPENDING shows pending messages.", Metadata: map[string]any{"kb_id": "1"}, Score: 0.9}},
	}}
	repo := NewMemoryRepository()
	require.NoError(t, repo.CreateKnowledgeBase(context.Background(), &KnowledgeBase{ID: 1, Name: "Redis", FileHash: "h"}))
	service := NewQueryService(QueryServiceOptions{
		Repository:    repo,
		VectorService: vectorSvc,
		Model:         answer,
		RewriteModel:  rewrite,
		PromptLoader:  ai.NewPromptLoader("../../../internal/prompts"),
		Config: config.RAGConfig{Rewrite: config.RAGRewriteConfig{Enabled: true}, Search: config.RAGSearchConfig{
			ShortQueryLength: 4, TopKShort: 20, TopKMedium: 12, TopKLong: 8, MinScoreShort: 0.25, MinScoreDefault: 0.28,
		}},
	})

	resp, err := service.Query(context.Background(), QueryRequest{KnowledgeBaseIDs: []uint{1}, Question: "how to inspect Redis Stream pending list"})

	require.NoError(t, err)
	require.Equal(t, "Use XPENDING and XCLAIM.", resp.Answer)
	require.Len(t, vectorSvc.requests, 1)
	require.Equal(t, "Redis Stream pending list", vectorSvc.requests[0].Query)
	require.Equal(t, 8, vectorSvc.requests[0].TopK)
	require.Equal(t, 0.28, vectorSvc.requests[0].MinScore)
	require.Contains(t, strings.Join(answer.prompts, "\n"), "XPENDING shows pending messages.")
}

func TestQueryFallsBackToOriginalWhenRewriteBlankAndNormalizesNoResult(t *testing.T) {
	rewrite := &scriptedChatModel{responses: []string{"   "}}
	answer := &scriptedChatModel{responses: []string{"没有找到相关信息"}}
	vectorSvc := &queryVectorService{docs: map[string][]vector.Document{
		"短": {{Content: "short doc", Metadata: map[string]any{"kb_id": "1"}, Score: 0.9}},
	}}
	repo := NewMemoryRepository()
	require.NoError(t, repo.CreateKnowledgeBase(context.Background(), &KnowledgeBase{ID: 1, Name: "KB", FileHash: "h"}))
	service := NewQueryService(QueryServiceOptions{
		Repository:    repo,
		VectorService: vectorSvc,
		Model:         answer,
		RewriteModel:  rewrite,
		PromptLoader:  ai.NewPromptLoader("../../../internal/prompts"),
		Config: config.RAGConfig{Rewrite: config.RAGRewriteConfig{Enabled: true}, Search: config.RAGSearchConfig{
			ShortQueryLength: 4, TopKShort: 20, TopKMedium: 12, TopKLong: 8, MinScoreShort: 0.25, MinScoreDefault: 0.28,
		}},
	})

	resp, err := service.Query(context.Background(), QueryRequest{KnowledgeBaseIDs: []uint{1}, Question: "短"})

	require.NoError(t, err)
	require.Contains(t, resp.Answer, "没有找到相关信息")
	require.Equal(t, "短", vectorSvc.requests[0].Query)
	require.Equal(t, 20, vectorSvc.requests[0].TopK)
	require.Equal(t, 0.25, vectorSvc.requests[0].MinScore)
}

func TestQueryReturnsNoResultForBlankQuestionOrNoDocs(t *testing.T) {
	service := NewQueryService(QueryServiceOptions{
		Repository:    NewMemoryRepository(),
		VectorService: &queryVectorService{docs: map[string][]vector.Document{}},
		Model:         &scriptedChatModel{},
		PromptLoader:  ai.NewPromptLoader("../../../internal/prompts"),
	})

	resp, err := service.Query(context.Background(), QueryRequest{KnowledgeBaseIDs: []uint{1}, Question: " "})

	require.NoError(t, err)
	require.Contains(t, resp.Answer, "没有找到相关信息")
}

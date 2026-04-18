package knowledgebase

import (
	"context"
	"fmt"
	"strings"

	"goGetJob/internal/common/ai"
	"goGetJob/internal/common/config"
	"goGetJob/internal/infrastructure/vector"
)

type QueryVectorService interface {
	SimilaritySearch(ctx context.Context, query string, kbIDs []uint, topK int, minScore float64) ([]vector.Document, error)
}

type QueryServiceOptions struct {
	Repository    Repository
	VectorService QueryVectorService
	Model         ai.ChatModel
	RewriteModel  ai.ChatModel
	PromptLoader  *ai.PromptLoader
	Config        config.RAGConfig
}

type QueryService struct {
	repo     Repository
	vectors  QueryVectorService
	model    ai.ChatModel
	rewrite  ai.ChatModel
	loader   *ai.PromptLoader
	cfg      config.RAGConfig
	noResult string
}

func NewQueryService(options QueryServiceOptions) *QueryService {
	loader := options.PromptLoader
	if loader == nil {
		loader = ai.NewPromptLoader("internal/prompts")
	}
	cfg := options.Config
	if cfg.Search.ShortQueryLength == 0 {
		cfg.Search.ShortQueryLength = 4
	}
	if cfg.Search.TopKShort == 0 {
		cfg.Search.TopKShort = 20
	}
	if cfg.Search.TopKMedium == 0 {
		cfg.Search.TopKMedium = 12
	}
	if cfg.Search.TopKLong == 0 {
		cfg.Search.TopKLong = 8
	}
	if cfg.Search.MinScoreShort == 0 {
		cfg.Search.MinScoreShort = 0.25
	}
	if cfg.Search.MinScoreDefault == 0 {
		cfg.Search.MinScoreDefault = 0.28
	}
	return &QueryService{
		repo: options.Repository, vectors: options.VectorService, model: options.Model,
		rewrite: options.RewriteModel, loader: loader, cfg: cfg, noResult: defaultNoResult,
	}
}

func (s *QueryService) Query(ctx context.Context, request QueryRequest) (QueryResponse, error) {
	question := strings.TrimSpace(request.Question)
	if question == "" || len(request.KnowledgeBaseIDs) == 0 {
		return QueryResponse{Answer: s.noResult}, nil
	}
	kbs, err := s.loadKnowledgeBases(ctx, request.KnowledgeBaseIDs)
	if err != nil {
		return QueryResponse{}, err
	}
	if len(kbs) == 0 {
		return QueryResponse{Answer: s.noResult}, nil
	}
	_ = s.repo.IncrementQuestionCount(ctx, request.KnowledgeBaseIDs)
	docs, err := s.retrieve(ctx, question, request.KnowledgeBaseIDs)
	if err != nil {
		return QueryResponse{}, err
	}
	if len(docs) == 0 {
		return QueryResponse{Answer: s.noResult, KnowledgeBaseID: kbs[0].ID, KnowledgeBaseName: joinKBNames(kbs)}, nil
	}
	answer, err := s.answer(ctx, question, docs)
	if err != nil {
		return QueryResponse{}, err
	}
	if isNoResultLike(answer) {
		answer = s.noResult
	}
	return QueryResponse{Answer: answer, KnowledgeBaseID: kbs[0].ID, KnowledgeBaseName: joinKBNames(kbs)}, nil
}

func (s *QueryService) StreamAnswer(ctx context.Context, request QueryRequest) (<-chan string, error) {
	out := make(chan string, 1)
	go func() {
		defer close(out)
		resp, err := s.Query(ctx, request)
		if err != nil {
			out <- "回答生成失败，请稍后重试。"
			return
		}
		out <- resp.Answer
	}()
	return out, nil
}

func (s *QueryService) retrieve(ctx context.Context, question string, kbIDs []uint) ([]vector.Document, error) {
	if s.vectors == nil {
		return nil, fmt.Errorf("vector service is required")
	}
	topK, minScore := s.searchTuning(question)
	for _, candidate := range s.candidateQueries(ctx, question) {
		docs, err := s.vectors.SimilaritySearch(ctx, candidate, kbIDs, topK, minScore)
		if err != nil {
			return nil, err
		}
		if len(docs) > 0 {
			return docs, nil
		}
	}
	return nil, nil
}

func (s *QueryService) candidateQueries(ctx context.Context, question string) []string {
	candidates := []string{}
	if s.cfg.Rewrite.Enabled && s.rewrite != nil {
		if rewritten, err := s.rewriteQuestion(ctx, question); err == nil && strings.TrimSpace(rewritten) != "" {
			candidates = append(candidates, strings.TrimSpace(rewritten))
		}
	}
	candidates = append(candidates, question)
	seen := map[string]bool{}
	out := []string{}
	for _, candidate := range candidates {
		key := strings.TrimSpace(candidate)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func (s *QueryService) rewriteQuestion(ctx context.Context, question string) (string, error) {
	if s.loader == nil {
		return "", fmt.Errorf("prompt loader is required")
	}
	prompt, err := s.loader.Render("knowledgebase-query-rewrite.st", map[string]string{
		"question": question,
		"history":  "",
	})
	if err != nil {
		return "", err
	}
	return s.rewrite.Generate(ctx, []ai.ChatMessage{{Role: "user", Content: prompt}})
}

func (s *QueryService) answer(ctx context.Context, question string, docs []vector.Document) (string, error) {
	if s.model == nil {
		return "", fmt.Errorf("chat model is required")
	}
	contextText := buildContext(docs)
	system, err := s.loader.Load("knowledgebase-query-system.st")
	if err != nil {
		return "", err
	}
	user, err := s.loader.Render("knowledgebase-query-user.st", map[string]string{
		"context":  contextText,
		"question": question,
	})
	if err != nil {
		return "", err
	}
	return s.model.Generate(ctx, []ai.ChatMessage{{Role: "user", Content: system + "\n\n" + user}})
}

func (s *QueryService) searchTuning(question string) (int, float64) {
	length := len([]rune(strings.TrimSpace(question)))
	if length <= s.cfg.Search.ShortQueryLength {
		return s.cfg.Search.TopKShort, s.cfg.Search.MinScoreShort
	}
	if length <= 12 {
		return s.cfg.Search.TopKMedium, s.cfg.Search.MinScoreDefault
	}
	return s.cfg.Search.TopKLong, s.cfg.Search.MinScoreDefault
}

func (s *QueryService) loadKnowledgeBases(ctx context.Context, ids []uint) ([]KnowledgeBase, error) {
	out := []KnowledgeBase{}
	for _, id := range ids {
		kb, err := s.repo.FindKnowledgeBaseByID(ctx, id)
		if err != nil {
			if err == ErrNotFound {
				continue
			}
			return nil, err
		}
		out = append(out, *kb)
	}
	return out, nil
}

func buildContext(docs []vector.Document) string {
	parts := make([]string, 0, len(docs))
	for _, doc := range docs {
		parts = append(parts, doc.Content)
	}
	return strings.Join(parts, "\n\n")
}

func joinKBNames(kbs []KnowledgeBase) string {
	names := make([]string, 0, len(kbs))
	for _, kb := range kbs {
		names = append(names, kb.Name)
	}
	return strings.Join(names, "、")
}

func isNoResultLike(answer string) bool {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		return true
	}
	phrases := []string{"没有找到相关信息", "未检索到相关信息", "信息不足", "超出知识库范围", "无法根据提供内容回答"}
	for _, phrase := range phrases {
		if strings.Contains(trimmed, phrase) {
			return true
		}
	}
	return false
}

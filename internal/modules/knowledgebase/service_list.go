package knowledgebase

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	commonmodel "goGetJob/internal/common/model"
)

type ListService struct {
	repo Repository
}

func NewListService(repo Repository) *ListService {
	return &ListService{repo: repo}
}

func (s *ListService) List(ctx context.Context, status string, sortBy string) ([]KnowledgeBaseListItem, error) {
	vectorStatus, err := parseVectorStatus(status)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListKnowledgeBases(ctx, vectorStatus)
	if err != nil {
		return nil, err
	}
	sortKnowledgeBases(items, sortBy)
	out := make([]KnowledgeBaseListItem, 0, len(items))
	for _, item := range items {
		out = append(out, toListItem(item))
	}
	return out, nil
}

func (s *ListService) Detail(ctx context.Context, id uint) (KnowledgeBaseListItem, error) {
	kb, err := s.repo.FindKnowledgeBaseByID(ctx, id)
	if err != nil {
		return KnowledgeBaseListItem{}, err
	}
	return toListItem(*kb), nil
}

func (s *ListService) Categories(ctx context.Context) ([]string, error) {
	items, err := s.repo.ListKnowledgeBases(ctx, "")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []string{}
	for _, item := range items {
		category := strings.TrimSpace(item.Category)
		if category != "" && !seen[category] {
			seen[category] = true
			out = append(out, category)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s *ListService) ByCategory(ctx context.Context, category string) ([]KnowledgeBaseListItem, error) {
	items, err := s.repo.ListKnowledgeBases(ctx, "")
	if err != nil {
		return nil, err
	}
	out := []KnowledgeBaseListItem{}
	for _, item := range items {
		if item.Category == category {
			out = append(out, toListItem(item))
		}
	}
	return out, nil
}

func (s *ListService) Uncategorized(ctx context.Context) ([]KnowledgeBaseListItem, error) {
	items, err := s.repo.ListKnowledgeBases(ctx, "")
	if err != nil {
		return nil, err
	}
	out := []KnowledgeBaseListItem{}
	for _, item := range items {
		if strings.TrimSpace(item.Category) == "" {
			out = append(out, toListItem(item))
		}
	}
	return out, nil
}

func (s *ListService) UpdateCategory(ctx context.Context, id uint, category string) error {
	kb, err := s.repo.FindKnowledgeBaseByID(ctx, id)
	if err != nil {
		return err
	}
	kb.Category = strings.TrimSpace(category)
	return s.repo.UpdateKnowledgeBase(ctx, kb)
}

func (s *ListService) Search(ctx context.Context, keyword string) ([]KnowledgeBaseListItem, error) {
	keyword = normalizeKeyword(keyword)
	items, err := s.repo.ListKnowledgeBases(ctx, "")
	if err != nil {
		return nil, err
	}
	out := []KnowledgeBaseListItem{}
	for _, item := range items {
		if keyword == "" || strings.Contains(normalizeKeyword(item.Name), keyword) || strings.Contains(normalizeKeyword(item.Content), keyword) {
			out = append(out, toListItem(item))
		}
	}
	return out, nil
}

func (s *ListService) Stats(ctx context.Context) (KnowledgeBaseStats, error) {
	return s.repo.Stats(ctx)
}

type objectGetter interface {
	GetObject(context.Context, string) (io.ReadCloser, error)
}

func (s *ListService) Download(ctx context.Context, store objectGetter, id uint) ([]byte, string, string, error) {
	kb, err := s.repo.FindKnowledgeBaseByID(ctx, id)
	if err != nil {
		return nil, "", "", err
	}
	if store == nil || kb.StorageKey == "" {
		return nil, "", "", fmt.Errorf("knowledge base storage object not found")
	}
	rc, err := store.GetObject(ctx, kb.StorageKey)
	if err != nil {
		return nil, "", "", err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	return data, kb.OriginalFilename, kb.ContentType, err
}

func sortKnowledgeBases(items []KnowledgeBase, sortBy string) {
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "size":
		sort.Slice(items, func(i, j int) bool { return items[i].FileSize > items[j].FileSize })
	case "access":
		sort.Slice(items, func(i, j int) bool { return items[i].AccessCount > items[j].AccessCount })
	case "question":
		sort.Slice(items, func(i, j int) bool { return items[i].QuestionCount > items[j].QuestionCount })
	}
}

func parseVectorStatus(status string) (commonmodel.AsyncTaskStatus, error) {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "":
		return "", nil
	case string(commonmodel.AsyncTaskStatusPending):
		return commonmodel.AsyncTaskStatusPending, nil
	case string(commonmodel.AsyncTaskStatusProcessing):
		return commonmodel.AsyncTaskStatusProcessing, nil
	case string(commonmodel.AsyncTaskStatusCompleted):
		return commonmodel.AsyncTaskStatusCompleted, nil
	case string(commonmodel.AsyncTaskStatusFailed):
		return commonmodel.AsyncTaskStatusFailed, nil
	default:
		return "", fmt.Errorf("invalid vector status")
	}
}

package knowledgebase

import (
	"context"

	"goGetJob/internal/infrastructure/storage"
	"goGetJob/internal/infrastructure/vector"
)

type DeleteService struct {
	repo    Repository
	storage storage.Storage
	vectors vector.Store
}

func NewDeleteService(repo Repository, objectStorage storage.Storage, vectors vector.Store) *DeleteService {
	return &DeleteService{repo: repo, storage: objectStorage, vectors: vectors}
}

func (s *DeleteService) Delete(ctx context.Context, id uint) error {
	kb, err := s.repo.FindKnowledgeBaseByID(ctx, id)
	if err != nil {
		return err
	}
	if s.vectors != nil {
		_ = s.vectors.DeleteByKnowledgeBaseID(ctx, id)
	}
	if s.storage != nil && kb.StorageKey != "" {
		_ = s.storage.DeleteObject(ctx, kb.StorageKey)
	}
	return s.repo.DeleteKnowledgeBase(ctx, id)
}

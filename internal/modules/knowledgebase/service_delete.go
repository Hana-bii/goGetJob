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
	if err := s.repo.DeleteKnowledgeBase(ctx, id); err != nil {
		return err
	}
	if s.vectors != nil {
		if err := s.vectors.DeleteByKnowledgeBaseID(ctx, id); err != nil {
			return err
		}
	}
	if s.storage != nil && kb.StorageKey != "" {
		if err := s.storage.DeleteObject(ctx, kb.StorageKey); err != nil {
			return err
		}
	}
	return nil
}

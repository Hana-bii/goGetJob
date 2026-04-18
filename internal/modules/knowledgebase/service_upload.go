package knowledgebase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	commonmodel "goGetJob/internal/common/model"
	"goGetJob/internal/infrastructure/file"
	"goGetJob/internal/infrastructure/storage"
)

const defaultKnowledgeBaseMaxFileSize = 20 * 1024 * 1024

type UploadServiceOptions struct {
	Repository    Repository
	Storage       storage.Storage
	Producer      VectorizeProducer
	Parser        *file.Parser
	MaxFileSize   int64
	StoragePrefix string
}

type UploadService struct {
	repo          Repository
	storage       storage.Storage
	producer      VectorizeProducer
	parser        *file.Parser
	maxFileSize   int64
	storagePrefix string
}

func NewUploadService(options UploadServiceOptions) *UploadService {
	maxFileSize := options.MaxFileSize
	if maxFileSize <= 0 {
		maxFileSize = defaultKnowledgeBaseMaxFileSize
	}
	parser := options.Parser
	if parser == nil {
		parser = file.NewParser(file.ParserOptions{
			Validation: file.ValidationOptions{MaxSizeBytes: maxFileSize},
		})
	}
	prefix := strings.Trim(options.StoragePrefix, "/")
	if prefix == "" {
		prefix = "knowledgebase"
	}
	return &UploadService{
		repo:          options.Repository,
		storage:       options.Storage,
		producer:      options.Producer,
		parser:        parser,
		maxFileSize:   maxFileSize,
		storagePrefix: prefix,
	}
}

func (s *UploadService) UploadBytes(ctx context.Context, input UploadInput) (UploadResult, error) {
	if err := s.require(); err != nil {
		return UploadResult{}, err
	}
	if len(input.Data) == 0 {
		return UploadResult{}, fmt.Errorf("knowledge base file is empty")
	}
	if int64(len(input.Data)) > s.maxFileSize {
		return UploadResult{}, fmt.Errorf("knowledge base file exceeds %d bytes", s.maxFileSize)
	}

	hash := file.HashBytes(input.Data)
	if existing, err := s.repo.FindKnowledgeBaseByHash(ctx, hash); err == nil {
		existing.Touch()
		if input.Category != "" {
			existing.Category = strings.TrimSpace(input.Category)
		}
		if err := s.repo.UpdateKnowledgeBase(ctx, existing); err != nil {
			return UploadResult{}, err
		}
		return UploadResult{
			KnowledgeBase: uploadSummary(*existing),
			Storage:       StorageSummary{FileKey: existing.StorageKey, FileURL: existing.StorageURL},
			Duplicate:     true,
		}, nil
	} else if !errors.Is(err, ErrNotFound) {
		return UploadResult{}, err
	}

	content, err := s.parser.ParseBytes(ctx, input.Filename, input.Data)
	if err != nil {
		return UploadResult{}, err
	}
	if strings.TrimSpace(content) == "" {
		return UploadResult{}, fmt.Errorf("knowledge base text is empty")
	}

	key := s.storageKey(hash, input.Filename)
	if _, err := s.storage.PutObject(ctx, storage.PutObjectInput{
		Key:         key,
		Reader:      bytes.NewReader(input.Data),
		Size:        int64(len(input.Data)),
		ContentType: input.ContentType,
	}); err != nil {
		return UploadResult{}, fmt.Errorf("store knowledge base: %w", err)
	}
	fileURL := ""
	if u, err := s.storage.PresignedGetObject(ctx, key, 24*time.Hour); err == nil && u != nil {
		fileURL = u.String()
	}

	kb := &KnowledgeBase{
		Name:             chooseName(input.Name, input.Filename),
		Category:         strings.TrimSpace(input.Category),
		FileHash:         hash,
		OriginalFilename: input.Filename,
		FileSize:         int64(len(input.Data)),
		ContentType:      input.ContentType,
		StorageKey:       key,
		StorageURL:       fileURL,
		Content:          content,
		VectorStatus:     commonmodel.AsyncTaskStatusPending,
	}
	if err := s.repo.CreateKnowledgeBase(ctx, kb); err != nil {
		return UploadResult{}, err
	}
	if err := s.producer.SendVectorizeTask(ctx, VectorizeTask{KnowledgeBaseID: kb.ID, Content: content}); err != nil {
		kb.VectorStatus = commonmodel.AsyncTaskStatusFailed
		kb.VectorError = truncateError("enqueue vectorize task: " + err.Error())
		_ = s.repo.UpdateKnowledgeBase(ctx, kb)
		return UploadResult{}, fmt.Errorf("enqueue vectorize task: %w", err)
	}

	return UploadResult{
		KnowledgeBase: uploadSummary(*kb),
		Storage:       StorageSummary{FileKey: key, FileURL: fileURL},
		Duplicate:     false,
	}, nil
}

func (s *UploadService) Revectorize(ctx context.Context, id uint) error {
	if err := s.require(); err != nil {
		return err
	}
	kb, err := s.repo.FindKnowledgeBaseByID(ctx, id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(kb.Content) == "" {
		return fmt.Errorf("knowledge base text is empty")
	}
	kb.VectorStatus = commonmodel.AsyncTaskStatusPending
	kb.VectorError = ""
	if err := s.repo.UpdateKnowledgeBase(ctx, kb); err != nil {
		return err
	}
	if err := s.producer.SendVectorizeTask(ctx, VectorizeTask{KnowledgeBaseID: kb.ID, Content: kb.Content}); err != nil {
		kb.VectorStatus = commonmodel.AsyncTaskStatusFailed
		kb.VectorError = truncateError("enqueue vectorize task: " + err.Error())
		_ = s.repo.UpdateKnowledgeBase(ctx, kb)
		return fmt.Errorf("enqueue vectorize task: %w", err)
	}
	return nil
}

func (s *UploadService) require() error {
	if s == nil || s.repo == nil || s.storage == nil || s.producer == nil || s.parser == nil {
		return fmt.Errorf("upload service dependencies are required")
	}
	return nil
}

func (s *UploadService) storageKey(hash, name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	return s.storagePrefix + "/" + hash + ext
}

func chooseName(name, filename string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if base == "" || base == "." {
		return filename
	}
	return base
}

func uploadSummary(kb KnowledgeBase) KnowledgeBaseUploadSummary {
	return KnowledgeBaseUploadSummary{
		ID:            kb.ID,
		Name:          kb.Name,
		Category:      kb.Category,
		FileSize:      kb.FileSize,
		ContentLength: len(kb.Content),
		VectorStatus:  kb.VectorStatus,
	}
}

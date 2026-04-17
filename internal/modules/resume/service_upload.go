package resume

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

const defaultResumeMaxFileSize = 10 * 1024 * 1024

type UploadServiceOptions struct {
	Repository    Repository
	Storage       storage.Storage
	Producer      AnalyzeProducer
	Parser        *file.Parser
	MaxFileSize   int64
	StoragePrefix string
}

type UploadService struct {
	repo          Repository
	storage       storage.Storage
	producer      AnalyzeProducer
	parser        *file.Parser
	maxFileSize   int64
	storagePrefix string
}

func NewUploadService(options UploadServiceOptions) *UploadService {
	maxFileSize := options.MaxFileSize
	if maxFileSize <= 0 {
		maxFileSize = defaultResumeMaxFileSize
	}
	parser := options.Parser
	if parser == nil {
		parser = file.NewParser(file.ParserOptions{
			Validation: file.ValidationOptions{MaxSizeBytes: maxFileSize},
		})
	}
	prefix := strings.Trim(options.StoragePrefix, "/")
	if prefix == "" {
		prefix = "resumes"
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
		return UploadResult{}, fmt.Errorf("resume file is empty")
	}
	if int64(len(input.Data)) > s.maxFileSize {
		return UploadResult{}, fmt.Errorf("resume file exceeds %d bytes", s.maxFileSize)
	}
	hash := file.HashBytes(input.Data)
	if existing, err := s.repo.FindResumeByHash(ctx, hash); err == nil {
		existing.Touch()
		analysis, err := latestAnalysisResult(ctx, s.repo, existing)
		if err != nil {
			return UploadResult{}, err
		}
		if analysis == nil && existing.AnalyzeStatus == commonmodel.AsyncTaskStatusFailed {
			existing.AnalyzeStatus = commonmodel.AsyncTaskStatusPending
			existing.AnalyzeError = ""
			if updateErr := s.repo.UpdateResume(ctx, existing); updateErr != nil {
				return UploadResult{}, updateErr
			}
			if err := s.producer.SendAnalyzeTask(ctx, AnalyzeTask{ResumeID: existing.ID, Content: existing.ResumeText}); err != nil {
				existing.AnalyzeStatus = commonmodel.AsyncTaskStatusFailed
				existing.AnalyzeError = truncateError("enqueue analyze task: " + err.Error())
				_ = s.repo.UpdateResume(ctx, existing)
				return UploadResult{}, fmt.Errorf("enqueue analyze task: %w", err)
			}
		}
		if updateErr := s.repo.UpdateResume(ctx, existing); updateErr != nil {
			return UploadResult{}, updateErr
		}
		return UploadResult{
			Resume:    *existing,
			Storage:   StorageSummary{FileKey: existing.StorageKey, FileURL: existing.StorageURL, ResumeID: existing.ID},
			Analysis:  analysis,
			Duplicate: true,
		}, nil
	} else if !errors.Is(err, ErrNotFound) {
		return UploadResult{}, err
	}

	resumeText, err := s.parser.ParseBytes(ctx, input.Filename, input.Data)
	if err != nil {
		return UploadResult{}, err
	}
	if strings.TrimSpace(resumeText) == "" {
		return UploadResult{}, fmt.Errorf("resume text is empty")
	}

	key := s.storageKey(hash, input.Filename)
	if _, err := s.storage.PutObject(ctx, storage.PutObjectInput{
		Key:         key,
		Reader:      bytes.NewReader(input.Data),
		Size:        int64(len(input.Data)),
		ContentType: input.ContentType,
	}); err != nil {
		return UploadResult{}, fmt.Errorf("store resume: %w", err)
	}
	fileURL := ""
	if u, err := s.storage.PresignedGetObject(ctx, key, 24*time.Hour); err == nil && u != nil {
		fileURL = u.String()
	}

	resume := &Resume{
		FileHash:         hash,
		OriginalFilename: input.Filename,
		FileSize:         int64(len(input.Data)),
		ContentType:      input.ContentType,
		StorageKey:       key,
		StorageURL:       fileURL,
		ResumeText:       resumeText,
	}
	if err := s.repo.CreateResume(ctx, resume); err != nil {
		return UploadResult{}, err
	}
	if err := s.producer.SendAnalyzeTask(ctx, AnalyzeTask{ResumeID: resume.ID, Content: resumeText}); err != nil {
		resume.AnalyzeStatus = commonmodel.AsyncTaskStatusFailed
		resume.AnalyzeError = truncateError("enqueue analyze task: " + err.Error())
		_ = s.repo.UpdateResume(ctx, resume)
		return UploadResult{}, fmt.Errorf("enqueue analyze task: %w", err)
	}

	return UploadResult{
		Resume:    *resume,
		Storage:   StorageSummary{FileKey: key, FileURL: fileURL, ResumeID: resume.ID},
		Duplicate: false,
	}, nil
}

func (s *UploadService) Reanalyze(ctx context.Context, id uint) error {
	if err := s.require(); err != nil {
		return err
	}
	resume, err := s.repo.FindResumeByID(ctx, id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(resume.ResumeText) == "" {
		return fmt.Errorf("resume text is empty")
	}
	resume.AnalyzeStatus = commonmodel.AsyncTaskStatusPending
	resume.AnalyzeError = ""
	if err := s.repo.UpdateResume(ctx, resume); err != nil {
		return err
	}
	if err := s.producer.SendAnalyzeTask(ctx, AnalyzeTask{ResumeID: resume.ID, Content: resume.ResumeText, Force: true}); err != nil {
		resume.AnalyzeStatus = commonmodel.AsyncTaskStatusFailed
		resume.AnalyzeError = truncateError("enqueue analyze task: " + err.Error())
		_ = s.repo.UpdateResume(ctx, resume)
		return fmt.Errorf("enqueue analyze task: %w", err)
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

package resume

import (
	"context"
	"errors"
	"fmt"

	"goGetJob/internal/infrastructure/export"
)

type HistoryService struct {
	repo        Repository
	pdfExporter export.PDFExporter
}

func NewHistoryService(repo Repository, pdfExporter export.PDFExporter) *HistoryService {
	return &HistoryService{repo: repo, pdfExporter: pdfExporter}
}

func (s *HistoryService) List(ctx context.Context) ([]ResumeListItem, error) {
	resumes, err := s.repo.ListResumes(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]ResumeListItem, 0, len(resumes))
	for _, resume := range resumes {
		item := ResumeListItem{
			ID:               resume.ID,
			OriginalFilename: resume.OriginalFilename,
			FileSize:         resume.FileSize,
			UploadedAt:       resume.UploadedAt,
			AccessCount:      resume.AccessCount,
			AnalyzeStatus:    resume.AnalyzeStatus,
			AnalyzeError:     resume.AnalyzeError,
		}
		if analysis, err := s.repo.LatestAnalysis(ctx, resume.ID); err == nil {
			score := analysis.OverallScore
			analyzedAt := analysis.AnalyzedAt
			item.LatestScore = &score
			item.LastAnalyzedAt = &analyzedAt
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *HistoryService) Detail(ctx context.Context, id uint) (ResumeDetail, error) {
	resume, err := s.repo.FindResumeByID(ctx, id)
	if err != nil {
		return ResumeDetail{}, err
	}
	entities, err := s.repo.ListAnalyses(ctx, id)
	if err != nil {
		return ResumeDetail{}, err
	}
	history := make([]AnalysisResult, 0, len(entities))
	for _, entity := range entities {
		result, err := analysisFromEntity(&entity, resume.ResumeText)
		if err != nil {
			return ResumeDetail{}, err
		}
		history = append(history, result)
	}
	return ResumeDetail{Resume: *resume, AnalysisHistory: history}, nil
}

func (s *HistoryService) Delete(ctx context.Context, id uint) error {
	return s.repo.DeleteResume(ctx, id)
}

func (s *HistoryService) Export(ctx context.Context, id uint) ([]byte, string, error) {
	if s == nil || s.pdfExporter == nil {
		return nil, "", fmt.Errorf("pdf exporter is required")
	}
	resume, err := s.repo.FindResumeByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	analysis, err := latestAnalysisResult(ctx, s.repo, resume)
	if errors.Is(err, ErrNotFound) || analysis == nil {
		return nil, "", fmt.Errorf("resume analysis not found")
	}
	if err != nil {
		return nil, "", err
	}
	report := export.Report{
		Title: "Resume Analysis Report",
		Sections: []export.ReportSection{
			{Heading: "File", Body: resume.OriginalFilename},
			{Heading: "Score", Body: fmt.Sprintf("%d / 100", analysis.OverallScore)},
			{Heading: "Summary", Body: analysis.Summary},
		},
	}
	pdf, err := s.pdfExporter.ExportReport(ctx, report)
	if err != nil {
		return nil, "", err
	}
	return pdf, resume.OriginalFilename + ".pdf", nil
}

package resume

import (
	"context"
	"errors"
	"fmt"

	"goGetJob/internal/common/async"
	commonmodel "goGetJob/internal/common/model"
)

const (
	AnalyzeStreamKey      = "resume:analyze:stream"
	AnalyzeStreamGroup    = "resume:analyze:group"
	AnalyzeConsumerPrefix = "resume-analyze"
)

type AnalyzeTask struct {
	ResumeID uint   `json:"resumeId"`
	Content  string `json:"content"`
}

type AnalyzeProducer interface {
	SendAnalyzeTask(ctx context.Context, task AnalyzeTask) error
}

type StreamAnalyzeProducer struct {
	producer *async.Producer[AnalyzeTask]
}

func NewStreamAnalyzeProducer(client async.StreamClient) *StreamAnalyzeProducer {
	return &StreamAnalyzeProducer{producer: async.NewProducer[AnalyzeTask](client, AnalyzeStreamKey)}
}

func (p *StreamAnalyzeProducer) SendAnalyzeTask(ctx context.Context, task AnalyzeTask) error {
	if p == nil || p.producer == nil {
		return fmt.Errorf("analyze producer is required")
	}
	_, err := p.producer.Send(ctx, task)
	return err
}

type Analyzer interface {
	Analyze(ctx context.Context, resumeText string) (AnalysisResult, error)
}

type AnalyzeTaskHandler struct {
	repo     Repository
	analyzer Analyzer
}

func NewAnalyzeTaskHandler(repo Repository, analyzer Analyzer) *AnalyzeTaskHandler {
	return &AnalyzeTaskHandler{repo: repo, analyzer: analyzer}
}

func (h *AnalyzeTaskHandler) AsyncHandler() async.Handler[AnalyzeTask] {
	return async.Handler[AnalyzeTask]{
		MarkProcessing:  h.MarkProcessing,
		ProcessBusiness: h.ProcessBusiness,
		MarkCompleted:   h.MarkCompleted,
		MarkFailed:      h.MarkFailed,
	}
}

func (h *AnalyzeTaskHandler) MarkProcessing(ctx context.Context, task AnalyzeTask) error {
	return h.updateStatus(ctx, task.ResumeID, commonmodel.AsyncTaskStatusProcessing, "")
}

func (h *AnalyzeTaskHandler) ProcessBusiness(ctx context.Context, task AnalyzeTask) error {
	if h == nil || h.repo == nil || h.analyzer == nil {
		return fmt.Errorf("analyze handler dependencies are required")
	}
	resume, err := h.repo.FindResumeByID(ctx, task.ResumeID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if latest, err := h.repo.LatestAnalysis(ctx, resume.ID); err == nil && latest != nil {
		return nil
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	result, err := h.analyzer.Analyze(ctx, task.Content)
	if err != nil {
		return err
	}
	result.OriginalText = task.Content
	return h.repo.CreateAnalysis(ctx, analysisToEntity(resume.ID, result))
}

func (h *AnalyzeTaskHandler) MarkCompleted(ctx context.Context, task AnalyzeTask) error {
	return h.updateStatus(ctx, task.ResumeID, commonmodel.AsyncTaskStatusCompleted, "")
}

func (h *AnalyzeTaskHandler) MarkFailed(ctx context.Context, task AnalyzeTask, cause error) error {
	return h.updateStatus(ctx, task.ResumeID, commonmodel.AsyncTaskStatusFailed, truncateError(errorString(cause)))
}

func (h *AnalyzeTaskHandler) updateStatus(ctx context.Context, resumeID uint, status commonmodel.AsyncTaskStatus, analyzeError string) error {
	if h == nil || h.repo == nil {
		return fmt.Errorf("resume repository is required")
	}
	resume, err := h.repo.FindResumeByID(ctx, resumeID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	resume.AnalyzeStatus = status
	resume.AnalyzeError = analyzeError
	return h.repo.UpdateResume(ctx, resume)
}

func NewAnalyzeConsumer(client async.StreamClient, repo Repository, analyzer Analyzer, consumerName string) *async.Consumer[AnalyzeTask] {
	if consumerName == "" {
		consumerName = AnalyzeConsumerPrefix
	}
	handler := NewAnalyzeTaskHandler(repo, analyzer)
	return async.NewConsumer(client, async.ConsumerOptions{
		Stream:     AnalyzeStreamKey,
		Group:      AnalyzeStreamGroup,
		Consumer:   consumerName,
		MaxRetries: 3,
	}, handler.AsyncHandler())
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func truncateError(value string) string {
	if len(value) <= maxAnalyzeErrorLength {
		return value
	}
	return value[:maxAnalyzeErrorLength]
}

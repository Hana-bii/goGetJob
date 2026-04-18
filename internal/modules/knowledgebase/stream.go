package knowledgebase

import (
	"context"
	"errors"
	"fmt"

	"goGetJob/internal/common/async"
	commonmodel "goGetJob/internal/common/model"
)

const (
	VectorizeStreamKey      = "knowledgebase:vectorize:stream"
	VectorizeStreamGroup    = "vectorize-group"
	VectorizeConsumerPrefix = "vectorize-consumer"
)

type VectorizeTask struct {
	KnowledgeBaseID uint   `json:"kbId"`
	Content         string `json:"content"`
}

type VectorizeProducer interface {
	SendVectorizeTask(ctx context.Context, task VectorizeTask) error
}

type StreamVectorizeProducer struct {
	producer *async.Producer[VectorizeTask]
}

func NewStreamVectorizeProducer(client async.StreamClient) *StreamVectorizeProducer {
	return &StreamVectorizeProducer{producer: async.NewProducer[VectorizeTask](client, VectorizeStreamKey)}
}

func (p *StreamVectorizeProducer) SendVectorizeTask(ctx context.Context, task VectorizeTask) error {
	if p == nil || p.producer == nil {
		return nil
	}
	_, err := p.producer.Send(ctx, task)
	return err
}

type VectorizeTaskHandler struct {
	repo    Repository
	vectors *VectorService
}

func NewVectorizeTaskHandler(repo Repository, vectors *VectorService) *VectorizeTaskHandler {
	return &VectorizeTaskHandler{repo: repo, vectors: vectors}
}

func (h *VectorizeTaskHandler) AsyncHandler() async.Handler[VectorizeTask] {
	return async.Handler[VectorizeTask]{
		MarkProcessing:  h.MarkProcessing,
		ProcessBusiness: h.ProcessBusiness,
		MarkCompleted:   h.MarkCompleted,
		MarkFailed:      h.MarkFailed,
	}
}

func (h *VectorizeTaskHandler) MarkProcessing(ctx context.Context, task VectorizeTask) error {
	return h.updateStatus(ctx, task.KnowledgeBaseID, commonmodel.AsyncTaskStatusProcessing, "")
}

func (h *VectorizeTaskHandler) ProcessBusiness(ctx context.Context, task VectorizeTask) error {
	if h == nil || h.repo == nil || h.vectors == nil {
		return fmt.Errorf("vectorize handler dependencies are required")
	}
	kb, err := h.repo.FindKnowledgeBaseByID(ctx, task.KnowledgeBaseID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	content := task.Content
	if content == "" {
		content = kb.Content
	}
	chunks, err := h.vectors.VectorizeAndStore(ctx, kb.ID, content)
	if err != nil {
		return err
	}
	kb.ChunkCount = chunks
	return h.repo.UpdateKnowledgeBase(ctx, kb)
}

func (h *VectorizeTaskHandler) MarkCompleted(ctx context.Context, task VectorizeTask) error {
	return h.updateStatus(ctx, task.KnowledgeBaseID, commonmodel.AsyncTaskStatusCompleted, "")
}

func (h *VectorizeTaskHandler) MarkFailed(ctx context.Context, task VectorizeTask, cause error) error {
	return h.updateStatus(ctx, task.KnowledgeBaseID, commonmodel.AsyncTaskStatusFailed, truncateError(errorString(cause)))
}

func (h *VectorizeTaskHandler) updateStatus(ctx context.Context, kbID uint, status commonmodel.AsyncTaskStatus, errText string) error {
	kb, err := h.repo.FindKnowledgeBaseByID(ctx, kbID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	kb.VectorStatus = status
	kb.VectorError = errText
	return h.repo.UpdateKnowledgeBase(ctx, kb)
}

func NewVectorizeConsumer(client async.StreamClient, repo Repository, vectors *VectorService, consumerName string) *async.Consumer[VectorizeTask] {
	if consumerName == "" {
		consumerName = VectorizeConsumerPrefix
	}
	handler := NewVectorizeTaskHandler(repo, vectors)
	return async.NewConsumer(client, async.ConsumerOptions{
		Stream:     VectorizeStreamKey,
		Group:      VectorizeStreamGroup,
		Consumer:   consumerName,
		MaxRetries: 3,
		Count:      10,
	}, handler.AsyncHandler())
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func truncateError(value string) string {
	if len(value) <= maxVectorErrorLength {
		return value
	}
	return value[:maxVectorErrorLength]
}

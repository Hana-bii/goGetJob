package voiceinterview

import (
	"context"
	"errors"
	"fmt"

	"goGetJob/internal/common/async"
	commonmodel "goGetJob/internal/common/model"
)

const (
	EvaluationStreamKey      = "voice-interview:evaluate:stream"
	EvaluationStreamGroup    = "voice-interview-evaluate-group"
	EvaluationConsumerPrefix = "voice-interview-evaluate-consumer"
)

type EvaluationTask struct {
	SessionID string `json:"sessionId"`
}

type StreamEvaluationProducer struct {
	producer *async.Producer[EvaluationTask]
}

func NewStreamEvaluationProducer(client async.StreamClient) *StreamEvaluationProducer {
	return &StreamEvaluationProducer{producer: async.NewProducer[EvaluationTask](client, EvaluationStreamKey)}
}

func (p *StreamEvaluationProducer) SendEvaluationTask(ctx context.Context, task EvaluationTask) error {
	if p == nil || p.producer == nil {
		return nil
	}
	_, err := p.producer.Send(ctx, task)
	return err
}

type EvaluationTaskHandler struct {
	repo    Repository
	service *EvaluationService
}

func NewEvaluationTaskHandler(repo Repository, service *EvaluationService) *EvaluationTaskHandler {
	return &EvaluationTaskHandler{repo: repo, service: service}
}

func (h *EvaluationTaskHandler) AsyncHandler() async.Handler[EvaluationTask] {
	return async.Handler[EvaluationTask]{MarkProcessing: h.MarkProcessing, ProcessBusiness: h.ProcessBusiness, MarkCompleted: h.MarkCompleted, MarkFailed: h.MarkFailed}
}

func (h *EvaluationTaskHandler) MarkProcessing(ctx context.Context, task EvaluationTask) error {
	return h.updateStatus(ctx, task.SessionID, commonmodel.AsyncTaskStatusProcessing, "")
}

func (h *EvaluationTaskHandler) ProcessBusiness(ctx context.Context, task EvaluationTask) error {
	if h == nil || h.service == nil {
		return fmt.Errorf("voice evaluation service is required")
	}
	_, err := h.service.EvaluateSession(ctx, task.SessionID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func (h *EvaluationTaskHandler) MarkCompleted(ctx context.Context, task EvaluationTask) error {
	return h.updateStatus(ctx, task.SessionID, commonmodel.AsyncTaskStatusCompleted, "")
}

func (h *EvaluationTaskHandler) MarkFailed(ctx context.Context, task EvaluationTask, cause error) error {
	return h.updateStatus(ctx, task.SessionID, commonmodel.AsyncTaskStatusFailed, truncateError(errorString(cause)))
}

func (h *EvaluationTaskHandler) updateStatus(ctx context.Context, sessionID string, status commonmodel.AsyncTaskStatus, errText string) error {
	if h == nil || h.repo == nil {
		return fmt.Errorf("voice evaluation repository is required")
	}
	session, err := h.repo.FindSessionByID(ctx, sessionID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	session.EvaluateStatus = status
	session.EvaluateError = errText
	return h.repo.UpdateSession(ctx, session)
}

func NewEvaluationConsumer(client async.StreamClient, repo Repository, service *EvaluationService, consumerName string) *async.Consumer[EvaluationTask] {
	if consumerName == "" {
		consumerName = EvaluationConsumerPrefix
	}
	handler := NewEvaluationTaskHandler(repo, service)
	return async.NewConsumer(client, async.ConsumerOptions{Stream: EvaluationStreamKey, Group: EvaluationStreamGroup, Consumer: consumerName, MaxRetries: 3}, handler.AsyncHandler())
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func truncateError(value string) string {
	if len(value) <= maxEvaluateErrorLen {
		return value
	}
	return value[:maxEvaluateErrorLen]
}

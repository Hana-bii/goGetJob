package interview

import (
	"context"
	"errors"
	"fmt"

	"goGetJob/internal/common/async"
	"goGetJob/internal/common/evaluation"
	commonmodel "goGetJob/internal/common/model"
	interviewskill "goGetJob/internal/modules/interview/skill"
)

const (
	EvaluateStreamKey      = "interview:evaluate:stream"
	EvaluateStreamGroup    = "evaluate-group"
	EvaluateConsumerPrefix = "evaluate-consumer"
)

type EvaluateTask struct {
	SessionID string `json:"sessionId"`
}

type EvaluationRunner interface {
	Evaluate(ctx context.Context, sessionID string, qaRecords []evaluation.QaRecord, resumeText, referenceContext string) (evaluation.Report, error)
}

type EvaluationService struct {
	evaluator    *evaluation.Service
	skillService *interviewskill.Service
}

func NewEvaluationService(evaluator *evaluation.Service, skillService *interviewskill.Service) *EvaluationService {
	return &EvaluationService{evaluator: evaluator, skillService: skillService}
}

func (s *EvaluationService) Evaluate(ctx context.Context, sessionID string, qaRecords []evaluation.QaRecord, resumeText, referenceContext string) (evaluation.Report, error) {
	if s == nil || s.evaluator == nil {
		return evaluation.Report{}, fmt.Errorf("evaluation service is required")
	}
	return s.evaluator.Evaluate(ctx, sessionID, qaRecords, resumeText, referenceContext)
}

func (s *EvaluationService) ReferenceContext(skillID string) string {
	if s == nil || s.skillService == nil {
		return ""
	}
	return s.skillService.BuildReferenceSectionSafe(skillID, nil)
}

type StreamEvaluateProducer struct {
	producer *async.Producer[EvaluateTask]
}

func NewStreamEvaluateProducer(client async.StreamClient) *StreamEvaluateProducer {
	return &StreamEvaluateProducer{producer: async.NewProducer[EvaluateTask](client, EvaluateStreamKey)}
}

func (p *StreamEvaluateProducer) SendEvaluateTask(ctx context.Context, task EvaluateTask) error {
	if p == nil || p.producer == nil {
		return nil
	}
	_, err := p.producer.Send(ctx, task)
	return err
}

type referenceProvider interface {
	ReferenceContext(skillID string) string
}

type EvaluateTaskHandler struct {
	repo      Repository
	evaluator EvaluationRunner
	refs      referenceProvider
}

func NewEvaluateTaskHandler(repo Repository, evaluator EvaluationRunner, refs referenceProvider) *EvaluateTaskHandler {
	return &EvaluateTaskHandler{repo: repo, evaluator: evaluator, refs: refs}
}

func (h *EvaluateTaskHandler) AsyncHandler() async.Handler[EvaluateTask] {
	return async.Handler[EvaluateTask]{
		MarkProcessing:  h.MarkProcessing,
		ProcessBusiness: h.ProcessBusiness,
		MarkCompleted:   h.MarkCompleted,
		MarkFailed:      h.MarkFailed,
	}
}

func (h *EvaluateTaskHandler) MarkProcessing(ctx context.Context, task EvaluateTask) error {
	return h.updateStatus(ctx, task.SessionID, commonmodel.AsyncTaskStatusProcessing, "")
}

func (h *EvaluateTaskHandler) ProcessBusiness(ctx context.Context, task EvaluateTask) error {
	if h == nil || h.repo == nil || h.evaluator == nil {
		return fmt.Errorf("evaluate handler dependencies are required")
	}
	session, err := h.repo.FindSessionByID(ctx, task.SessionID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	questions, err := parseQuestions(session.QuestionsJSON)
	if err != nil {
		return err
	}
	answers, err := h.repo.ListAnswers(ctx, task.SessionID)
	if err != nil {
		return err
	}
	for _, answer := range answers {
		for i := range questions {
			if questions[i].QuestionIndex == answer.QuestionIndex {
				questions[i].UserAnswer = answer.UserAnswer
			}
		}
	}
	referenceContext := ""
	if h.refs != nil {
		referenceContext = h.refs.ReferenceContext(session.SkillID)
	}
	report, err := h.evaluator.Evaluate(ctx, task.SessionID, qaRecords(questions), session.ResumeText, referenceContext)
	if err != nil {
		return err
	}
	return saveReport(ctx, h.repo, session, questions, report)
}

func (h *EvaluateTaskHandler) MarkCompleted(ctx context.Context, task EvaluateTask) error {
	return h.updateStatus(ctx, task.SessionID, commonmodel.AsyncTaskStatusCompleted, "")
}

func (h *EvaluateTaskHandler) MarkFailed(ctx context.Context, task EvaluateTask, cause error) error {
	return h.updateStatus(ctx, task.SessionID, commonmodel.AsyncTaskStatusFailed, truncateError(errorString(cause)))
}

func (h *EvaluateTaskHandler) updateStatus(ctx context.Context, sessionID string, status commonmodel.AsyncTaskStatus, errText string) error {
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

func NewEvaluateConsumer(client async.StreamClient, repo Repository, evaluator EvaluationRunner, refs referenceProvider, consumerName string) *async.Consumer[EvaluateTask] {
	if consumerName == "" {
		consumerName = EvaluateConsumerPrefix
	}
	handler := NewEvaluateTaskHandler(repo, evaluator, refs)
	return async.NewConsumer(client, async.ConsumerOptions{
		Stream:     EvaluateStreamKey,
		Group:      EvaluateStreamGroup,
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
	if len(value) <= maxEvaluateErrorLen {
		return value
	}
	return value[:maxEvaluateErrorLen]
}

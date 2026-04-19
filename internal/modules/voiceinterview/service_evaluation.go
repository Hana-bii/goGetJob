package voiceinterview

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"goGetJob/internal/common/evaluation"
	commonmodel "goGetJob/internal/common/model"
)

type EvaluationRunner interface {
	Evaluate(ctx context.Context, sessionID string, qaRecords []evaluation.QaRecord, resumeText, referenceContext string) (evaluation.Report, error)
}

type EvaluationService struct {
	repo      Repository
	evaluator EvaluationRunner
	refs      ReferenceProvider
}

type ReferenceProvider interface {
	ReferenceContext(skillID string) string
}

func NewEvaluationService(repo Repository, evaluator EvaluationRunner, refs ...ReferenceProvider) *EvaluationService {
	var ref ReferenceProvider
	if len(refs) > 0 {
		ref = refs[0]
	}
	return &EvaluationService{repo: repo, evaluator: evaluator, refs: ref}
}

func (s *EvaluationService) EvaluateSession(ctx context.Context, sessionID string) (evaluation.Report, error) {
	if s == nil || s.repo == nil || s.evaluator == nil {
		return evaluation.Report{}, fmt.Errorf("voice evaluation dependencies are required")
	}
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return evaluation.Report{}, err
	}
	questions, err := parseQuestions(session.QuestionsJSON)
	if err != nil {
		return evaluation.Report{}, err
	}
	messages, err := s.repo.ListMessages(ctx, sessionID)
	if err != nil {
		return evaluation.Report{}, err
	}
	records := DialogueToQARecords(questions, messages)
	referenceContext := ""
	if s.refs != nil {
		referenceContext = s.refs.ReferenceContext(session.SkillID)
	}
	report, err := s.evaluator.Evaluate(ctx, sessionID, records, session.ResumeText, referenceContext)
	if err != nil {
		return evaluation.Report{}, err
	}
	if err := saveEvaluationReport(ctx, s.repo, session, report); err != nil {
		return evaluation.Report{}, err
	}
	return report, nil
}

func (s *EvaluationService) GetEvaluation(ctx context.Context, sessionID string) (EvaluationDTO, error) {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return EvaluationDTO{}, err
	}
	dto := EvaluationDTO{SessionID: session.SessionID, Status: session.Status, EvaluateStatus: session.EvaluateStatus, EvaluateError: session.EvaluateError}
	if strings.TrimSpace(session.EvaluationJSON) != "" {
		var report evaluation.Report
		if err := json.Unmarshal([]byte(session.EvaluationJSON), &report); err == nil {
			dto.Report = &report
		}
	}
	return dto, nil
}

func DialogueToQARecords(questions []PromptQuestion, messages []VoiceMessage) []evaluation.QaRecord {
	userAnswers := []string{}
	for _, message := range messages {
		if message.Role != MessageRoleUser || strings.TrimSpace(message.Content) == "" {
			continue
		}
		userAnswers = append(userAnswers, message.Content)
	}
	if len(questions) == 0 {
		records := make([]evaluation.QaRecord, 0, len(userAnswers))
		for i, answer := range userAnswers {
			records = append(records, evaluation.QaRecord{QuestionIndex: i, Question: fmt.Sprintf("Voice interview answer %d", i+1), Category: "Voice", UserAnswer: answer})
		}
		return records
	}
	records := make([]evaluation.QaRecord, 0, len(questions))
	for i, question := range questions {
		answer := ""
		if i < len(userAnswers) {
			answer = userAnswers[i]
		}
		records = append(records, evaluation.QaRecord{QuestionIndex: question.Index, Question: question.Question, Category: question.Category, UserAnswer: answer})
	}
	return records
}

func saveEvaluationReport(ctx context.Context, repo Repository, session *VoiceSession, report evaluation.Report) error {
	encoded, err := json.Marshal(report)
	if err != nil {
		return err
	}
	session.EvaluationJSON = string(encoded)
	session.OverallScore = report.OverallScore
	session.Status = VoiceSessionStatusEvaluated
	session.EvaluateStatus = commonmodel.AsyncTaskStatusCompleted
	session.EvaluateError = ""
	return repo.UpdateSession(ctx, session)
}

package interview

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"goGetJob/internal/common/evaluation"
	commonmodel "goGetJob/internal/common/model"
)

type QuestionGenerator interface {
	Generate(context.Context, GenerateQuestionsInput) ([]Question, error)
}

type EvaluateProducer interface {
	SendEvaluateTask(context.Context, EvaluateTask) error
}

type SessionServiceOptions struct {
	Repository        Repository
	QuestionGenerator QuestionGenerator
	EvaluateProducer  EvaluateProducer
}

type SessionService struct {
	repo              Repository
	questionGenerator QuestionGenerator
	evaluateProducer  EvaluateProducer
}

func NewSessionService(options SessionServiceOptions) *SessionService {
	return &SessionService{
		repo:              options.Repository,
		questionGenerator: options.QuestionGenerator,
		evaluateProducer:  options.EvaluateProducer,
	}
}

func (s *SessionService) Create(ctx context.Context, request CreateSessionRequest) (SessionDTO, error) {
	if s == nil || s.repo == nil || s.questionGenerator == nil {
		return SessionDTO{}, fmt.Errorf("interview session dependencies are required")
	}
	skillID := nonEmpty(request.SkillID, DefaultSkillID)
	difficulty := nonEmpty(request.Difficulty, DefaultDifficulty)
	count := request.QuestionCount
	if count <= 0 {
		count = DefaultQuestionCount
	}
	if request.ResumeID != nil && !request.ForceCreate {
		if existing, err := s.repo.FindUnfinishedSession(ctx, *request.ResumeID, ""); err == nil {
			return s.sessionDTO(existing)
		} else if err != nil && !errors.Is(err, ErrNotFound) {
			return SessionDTO{}, err
		}
	}
	history, err := s.repo.HistoricalQuestions(ctx, skillID, request.ResumeID, 60)
	if err != nil {
		return SessionDTO{}, err
	}
	questions, err := s.questionGenerator.Generate(ctx, GenerateQuestionsInput{
		SkillID:             skillID,
		Difficulty:          difficulty,
		ResumeText:          request.ResumeText,
		QuestionCount:       count,
		HistoricalQuestions: history,
		CustomCategories:    request.CustomCategories,
		JDText:              request.JDText,
	})
	if err != nil {
		return SessionDTO{}, err
	}
	encoded, err := json.Marshal(questions)
	if err != nil {
		return SessionDTO{}, err
	}
	session := &Session{
		SessionID:            newSessionID(),
		SkillID:              skillID,
		Difficulty:           difficulty,
		ResumeID:             request.ResumeID,
		ResumeText:           request.ResumeText,
		TotalQuestions:       len(questions),
		CurrentQuestionIndex: 0,
		Status:               SessionStatusCreated,
		QuestionsJSON:        string(encoded),
		LLMProvider:          request.LLMProvider,
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return SessionDTO{}, err
	}
	return s.sessionDTO(session)
}

func (s *SessionService) List(ctx context.Context) ([]SessionListItem, error) {
	sessions, err := s.repo.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]SessionListItem, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, SessionListItem{
			SessionID:      session.SessionID,
			SkillID:        session.SkillID,
			Difficulty:     session.Difficulty,
			ResumeID:       session.ResumeID,
			TotalQuestions: session.TotalQuestions,
			Status:         session.Status,
			EvaluateStatus: session.EvaluateStatus,
			EvaluateError:  session.EvaluateError,
			OverallScore:   session.OverallScore,
			CreatedAt:      session.CreatedAt,
			CompletedAt:    session.CompletedAt,
		})
	}
	return items, nil
}

func (s *SessionService) Get(ctx context.Context, sessionID string) (SessionDTO, error) {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return SessionDTO{}, err
	}
	return s.sessionDTO(session)
}

func (s *SessionService) CurrentQuestion(ctx context.Context, sessionID string) (map[string]any, error) {
	session, questions, err := s.sessionWithQuestions(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status == SessionStatusCreated {
		session.Status = SessionStatusInProgress
		if err := s.repo.UpdateSession(ctx, session); err != nil {
			return nil, err
		}
	}
	if session.CurrentQuestionIndex >= len(questions) {
		return map[string]any{"completed": true, "message": "all questions completed"}, nil
	}
	return map[string]any{"completed": false, "question": questions[session.CurrentQuestionIndex]}, nil
}

func (s *SessionService) SaveAnswer(ctx context.Context, request SubmitAnswerRequest) error {
	session, questions, err := s.sessionWithQuestions(ctx, request.SessionID)
	if err != nil {
		return err
	}
	if request.QuestionIndex < 0 || request.QuestionIndex >= len(questions) {
		return fmt.Errorf("invalid question index")
	}
	question := questions[request.QuestionIndex]
	questions[request.QuestionIndex] = question.WithAnswer(request.Answer)
	if session.Status == SessionStatusCreated {
		session.Status = SessionStatusInProgress
	}
	return s.persistAnswerAndQuestions(ctx, session, questions, question, request.Answer)
}

func (s *SessionService) SubmitAnswer(ctx context.Context, request SubmitAnswerRequest) (SubmitAnswerResponse, error) {
	session, questions, err := s.sessionWithQuestions(ctx, request.SessionID)
	if err != nil {
		return SubmitAnswerResponse{}, err
	}
	if request.QuestionIndex < 0 || request.QuestionIndex >= len(questions) {
		return SubmitAnswerResponse{}, fmt.Errorf("invalid question index")
	}
	question := questions[request.QuestionIndex]
	questions[request.QuestionIndex] = question.WithAnswer(request.Answer)
	newIndex := request.QuestionIndex + 1
	hasNext := newIndex < len(questions)
	session.CurrentQuestionIndex = newIndex
	if hasNext {
		session.Status = SessionStatusInProgress
	} else {
		session.MarkCompleted(SessionStatusCompleted)
		session.EvaluateStatus = commonmodel.AsyncTaskStatusPending
		session.EvaluateError = ""
	}
	if err := s.persistAnswerAndQuestions(ctx, session, questions, question, request.Answer); err != nil {
		return SubmitAnswerResponse{}, err
	}
	if !hasNext {
		if err := s.enqueueEvaluate(ctx, session.SessionID); err != nil {
			return SubmitAnswerResponse{}, err
		}
	}
	var next *Question
	if hasNext {
		copy := questions[newIndex]
		next = &copy
	}
	return SubmitAnswerResponse{HasNextQuestion: hasNext, NextQuestion: next, CurrentIndex: newIndex, TotalQuestions: len(questions)}, nil
}

func (s *SessionService) Complete(ctx context.Context, sessionID string) error {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.Status == SessionStatusCompleted || session.Status == SessionStatusEvaluated {
		return fmt.Errorf("interview already completed")
	}
	session.MarkCompleted(SessionStatusCompleted)
	session.EvaluateStatus = commonmodel.AsyncTaskStatusPending
	session.EvaluateError = ""
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return err
	}
	return s.enqueueEvaluate(ctx, sessionID)
}

func (s *SessionService) Delete(ctx context.Context, sessionID string) error {
	return s.repo.DeleteSession(ctx, sessionID)
}

func (s *SessionService) GenerateReport(ctx context.Context, evaluator EvaluationRunner, sessionID string, referenceContext string) (evaluation.Report, error) {
	session, questions, err := s.sessionWithQuestions(ctx, sessionID)
	if err != nil {
		return evaluation.Report{}, err
	}
	if session.Status != SessionStatusCompleted && session.Status != SessionStatusEvaluated {
		return evaluation.Report{}, fmt.Errorf("interview not completed")
	}
	if err := applyAnswers(ctx, s.repo, sessionID, questions); err != nil {
		return evaluation.Report{}, err
	}
	report, err := evaluator.Evaluate(ctx, sessionID, qaRecords(questions), session.ResumeText, referenceContext)
	if err != nil {
		return evaluation.Report{}, err
	}
	return report, saveReport(ctx, s.repo, session, questions, report)
}

func (s *SessionService) sessionWithQuestions(ctx context.Context, sessionID string) (*Session, []Question, error) {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	questions, err := parseQuestions(session.QuestionsJSON)
	if err != nil {
		return nil, nil, err
	}
	return session, questions, nil
}

func (s *SessionService) sessionDTO(session *Session) (SessionDTO, error) {
	questions, err := parseQuestions(session.QuestionsJSON)
	if err != nil {
		return SessionDTO{}, err
	}
	return SessionDTO{
		SessionID:            session.SessionID,
		ResumeText:           session.ResumeText,
		TotalQuestions:       session.TotalQuestions,
		CurrentQuestionIndex: session.CurrentQuestionIndex,
		Questions:            questions,
		Status:               session.Status,
		EvaluateStatus:       session.EvaluateStatus,
		EvaluateError:        session.EvaluateError,
	}, nil
}

func (s *SessionService) persistAnswerAndQuestions(ctx context.Context, session *Session, questions []Question, question Question, answerText string) error {
	encoded, err := json.Marshal(questions)
	if err != nil {
		return err
	}
	session.QuestionsJSON = string(encoded)
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return err
	}
	return s.repo.UpsertAnswer(ctx, &Answer{
		SessionID:     session.SessionID,
		QuestionIndex: question.QuestionIndex,
		Question:      question.Question,
		Category:      question.Category,
		UserAnswer:    answerText,
	})
}

func (s *SessionService) enqueueEvaluate(ctx context.Context, sessionID string) error {
	if s.evaluateProducer == nil {
		return nil
	}
	return s.evaluateProducer.SendEvaluateTask(ctx, EvaluateTask{SessionID: sessionID})
}

func parseQuestions(encoded string) ([]Question, error) {
	if strings.TrimSpace(encoded) == "" {
		return nil, nil
	}
	var questions []Question
	if err := json.Unmarshal([]byte(encoded), &questions); err != nil {
		return nil, err
	}
	return questions, nil
}

func qaRecords(questions []Question) []evaluation.QaRecord {
	records := make([]evaluation.QaRecord, 0, len(questions))
	for _, question := range questions {
		records = append(records, evaluation.QaRecord{
			QuestionIndex: question.QuestionIndex,
			Question:      question.Question,
			Category:      question.Category,
			UserAnswer:    question.UserAnswer,
		})
	}
	return records
}

func applyAnswers(ctx context.Context, repo Repository, sessionID string, questions []Question) error {
	answers, err := repo.ListAnswers(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, answer := range answers {
		for i := range questions {
			if questions[i].QuestionIndex == answer.QuestionIndex {
				questions[i].UserAnswer = answer.UserAnswer
				break
			}
		}
	}
	return nil
}

func saveReport(ctx context.Context, repo Repository, session *Session, questions []Question, report evaluation.Report) error {
	strengths, _ := json.Marshal(nilSlice(report.Strengths))
	improvements, _ := json.Marshal(nilSlice(report.Improvements))
	references, _ := json.Marshal(nilSlice(report.ReferenceAnswers))
	session.OverallScore = report.OverallScore
	session.OverallFeedback = report.OverallFeedback
	session.StrengthsJSON = string(strengths)
	session.ImprovementsJSON = string(improvements)
	session.ReferenceAnswersJSON = string(references)
	session.MarkCompleted(SessionStatusEvaluated)
	if err := repo.UpdateSession(ctx, session); err != nil {
		return err
	}
	refByIndex := map[int]evaluation.ReferenceAnswer{}
	for _, ref := range report.ReferenceAnswers {
		refByIndex[ref.QuestionIndex] = ref
	}
	for _, detail := range report.QuestionDetails {
		ref := refByIndex[detail.QuestionIndex]
		keyPoints, _ := json.Marshal(nilSlice(ref.KeyPoints))
		answer := &Answer{
			SessionID:       session.SessionID,
			QuestionIndex:   detail.QuestionIndex,
			Question:        detail.Question,
			Category:        detail.Category,
			UserAnswer:      detail.UserAnswer,
			Score:           detail.Score,
			Feedback:        detail.Feedback,
			ReferenceAnswer: ref.ReferenceAnswer,
			KeyPointsJSON:   string(keyPoints),
		}
		if answer.Question == "" {
			answer.Question = questionText(questions, detail.QuestionIndex)
		}
		if err := repo.UpsertAnswer(ctx, answer); err != nil {
			return err
		}
	}
	return nil
}

func questionText(questions []Question, index int) string {
	for _, question := range questions {
		if question.QuestionIndex == index {
			return question.Question
		}
	}
	return ""
}

func newSessionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprint(len(b))))
	}
	return hex.EncodeToString(b[:])
}

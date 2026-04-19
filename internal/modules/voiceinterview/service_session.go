package voiceinterview

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	commonmodel "goGetJob/internal/common/model"
)

type PromptGenerator interface {
	GeneratePrompts(context.Context, PromptInput) ([]PromptQuestion, error)
}

type EvaluationProducer interface {
	SendEvaluationTask(context.Context, EvaluationTask) error
}

type SessionServiceOptions struct {
	Repository         Repository
	PromptGenerator    PromptGenerator
	EvaluationProducer EvaluationProducer
}

type SessionService struct {
	repo               Repository
	promptGenerator    PromptGenerator
	evaluationProducer EvaluationProducer
}

func NewSessionService(options SessionServiceOptions) *SessionService {
	return &SessionService{repo: options.Repository, promptGenerator: options.PromptGenerator, evaluationProducer: options.EvaluationProducer}
}

func (s *SessionService) Create(ctx context.Context, request CreateSessionRequest) (SessionDTO, error) {
	if s == nil || s.repo == nil {
		return SessionDTO{}, fmt.Errorf("voice session repository is required")
	}
	count := request.QuestionCount
	if count <= 0 {
		count = DefaultQuestionCount
	}
	phase := nonEmpty(request.InitialPhase, DefaultPhase)
	questions := []PromptQuestion{}
	if s.promptGenerator != nil {
		generated, err := s.promptGenerator.GeneratePrompts(ctx, PromptInput{
			ResumeText:    request.ResumeText,
			JDText:        request.JDText,
			SkillID:       request.SkillID,
			Difficulty:    nonEmpty(request.Difficulty, DefaultDifficulty),
			QuestionCount: count,
			Phase:         phase,
		})
		if err != nil {
			return SessionDTO{}, err
		}
		questions = generated
	}
	if len(questions) == 0 {
		questions = fallbackPromptQuestions(count, phase)
	}
	encoded, err := encodeQuestions(questions)
	if err != nil {
		return SessionDTO{}, err
	}
	session := &VoiceSession{
		SessionID:            newSessionID(),
		ResumeID:             request.ResumeID,
		ResumeText:           request.ResumeText,
		JDText:               request.JDText,
		SkillID:              request.SkillID,
		Difficulty:           nonEmpty(request.Difficulty, DefaultDifficulty),
		TotalQuestions:       len(questions),
		CurrentQuestionIndex: 0,
		CurrentPhase:         phase,
		Status:               VoiceSessionStatusCreated,
		QuestionsJSON:        encoded,
		LLMProvider:          request.LLMProvider,
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return SessionDTO{}, err
	}
	if strings.TrimSpace(request.WelcomeMessage) != "" {
		if _, err := s.AppendMessage(ctx, session.SessionID, MessageRoleAssistant, request.WelcomeMessage, phase); err != nil {
			return SessionDTO{}, err
		}
	}
	return s.sessionDTO(session)
}

func (s *SessionService) Get(ctx context.Context, sessionID string) (SessionDTO, error) {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return SessionDTO{}, err
	}
	return s.sessionDTO(session)
}

func (s *SessionService) List(ctx context.Context) ([]SessionDTO, error) {
	sessions, err := s.repo.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]SessionDTO, 0, len(sessions))
	for i := range sessions {
		item, err := s.sessionDTO(&sessions[i])
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *SessionService) Pause(ctx context.Context, sessionID string) error {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.Status == VoiceSessionStatusCompleted || session.Status == VoiceSessionStatusEvaluated {
		return fmt.Errorf("voice interview already completed")
	}
	session.Status = VoiceSessionStatusPaused
	return s.repo.UpdateSession(ctx, session)
}

func (s *SessionService) Resume(ctx context.Context, sessionID string) (SessionDTO, error) {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return SessionDTO{}, err
	}
	if session.Status == VoiceSessionStatusCompleted || session.Status == VoiceSessionStatusEvaluated {
		return SessionDTO{}, fmt.Errorf("voice interview already completed")
	}
	session.Status = VoiceSessionStatusInProgress
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return SessionDTO{}, err
	}
	return s.sessionDTO(session)
}

func (s *SessionService) StartPhase(ctx context.Context, sessionID, phase string) error {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	session.CurrentPhase = nonEmpty(phase, session.CurrentPhase)
	if session.Status == VoiceSessionStatusCreated {
		session.Status = VoiceSessionStatusInProgress
	}
	return s.repo.UpdateSession(ctx, session)
}

func (s *SessionService) End(ctx context.Context, sessionID string) error {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.Status == VoiceSessionStatusEvaluated {
		return nil
	}
	if session.Status != VoiceSessionStatusCompleted {
		session.MarkCompleted()
	}
	session.EvaluateStatus = commonmodel.AsyncTaskStatusPending
	session.EvaluateError = ""
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return err
	}
	return s.sendEvaluationTask(ctx, sessionID)
}

func (s *SessionService) RequestEvaluation(ctx context.Context, sessionID string) (SessionDTO, error) {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return SessionDTO{}, err
	}
	if session.Status != VoiceSessionStatusCompleted && session.Status != VoiceSessionStatusEvaluated {
		session.MarkCompleted()
	}
	session.EvaluateStatus = commonmodel.AsyncTaskStatusPending
	session.EvaluateError = ""
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return SessionDTO{}, err
	}
	if err := s.sendEvaluationTask(ctx, sessionID); err != nil {
		session.EvaluateStatus = commonmodel.AsyncTaskStatusFailed
		session.EvaluateError = truncateError(errorString(err))
		_ = s.repo.UpdateSession(ctx, session)
		return SessionDTO{}, err
	}
	return s.sessionDTO(session)
}

func (s *SessionService) Delete(ctx context.Context, sessionID string) error {
	return s.repo.DeleteSession(ctx, sessionID)
}

func (s *SessionService) AppendMessage(ctx context.Context, sessionID string, role MessageRole, content string, phase string) (VoiceMessage, error) {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return VoiceMessage{}, err
	}
	if session.Status == VoiceSessionStatusCreated && role == MessageRoleUser {
		session.Status = VoiceSessionStatusInProgress
		if err := s.repo.UpdateSession(ctx, session); err != nil {
			return VoiceMessage{}, err
		}
	}
	message := &VoiceMessage{SessionID: sessionID, Role: role, Content: content, Phase: nonEmpty(phase, session.CurrentPhase)}
	if err := s.repo.AppendMessage(ctx, message); err != nil {
		return VoiceMessage{}, err
	}
	return *message, nil
}

func (s *SessionService) AppendAudioMessage(ctx context.Context, sessionID string, role MessageRole, content string, audio []byte, format string, sampleRate int, phase string) (VoiceMessage, error) {
	message, err := s.AppendMessage(ctx, sessionID, role, content, phase)
	if err != nil {
		return VoiceMessage{}, err
	}
	if len(audio) == 0 {
		return message, nil
	}
	message.AudioBytes = append([]byte(nil), audio...)
	message.AudioFormat = format
	message.SampleRate = sampleRate
	return message, nil
}

func (s *SessionService) ListMessages(ctx context.Context, sessionID string) ([]VoiceMessage, error) {
	return s.repo.ListMessages(ctx, sessionID)
}

func (s *SessionService) sessionDTO(session *VoiceSession) (SessionDTO, error) {
	questions, err := parseQuestions(session.QuestionsJSON)
	if err != nil {
		return SessionDTO{}, err
	}
	return SessionDTO{
		SessionID:            session.SessionID,
		ResumeID:             session.ResumeID,
		ResumeText:           session.ResumeText,
		JDText:               session.JDText,
		SkillID:              session.SkillID,
		Difficulty:           session.Difficulty,
		TotalQuestions:       session.TotalQuestions,
		CurrentQuestionIndex: session.CurrentQuestionIndex,
		CurrentPhase:         session.CurrentPhase,
		Status:               session.Status,
		Questions:            questions,
		EvaluateStatus:       session.EvaluateStatus,
		EvaluateError:        session.EvaluateError,
		OverallScore:         session.OverallScore,
		CreatedAt:            session.CreatedAt,
		UpdatedAt:            session.UpdatedAt,
		EndedAt:              session.EndedAt,
	}, nil
}

func (s *SessionService) sendEvaluationTask(ctx context.Context, sessionID string) error {
	if s.evaluationProducer == nil {
		return nil
	}
	return s.evaluationProducer.SendEvaluationTask(ctx, EvaluationTask{SessionID: sessionID})
}

func newSessionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprint(len(b))))
	}
	return hex.EncodeToString(b[:])
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

package voiceinterview

import (
	"time"

	"goGetJob/internal/common/evaluation"
	commonmodel "goGetJob/internal/common/model"
)

const (
	DefaultQuestionCount = 5
	DefaultDifficulty    = "mid"
	DefaultPhase         = "opening"
	maxEvaluateErrorLen  = 500
)

type VoiceSessionStatus string

const (
	VoiceSessionStatusCreated    VoiceSessionStatus = "CREATED"
	VoiceSessionStatusInProgress VoiceSessionStatus = "IN_PROGRESS"
	VoiceSessionStatusPaused     VoiceSessionStatus = "PAUSED"
	VoiceSessionStatusCompleted  VoiceSessionStatus = "COMPLETED"
	VoiceSessionStatusEvaluated  VoiceSessionStatus = "EVALUATED"
)

type MessageRole string

const (
	MessageRoleSystem    MessageRole = "system"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleUser      MessageRole = "user"
)

type VoiceSession struct {
	ID                   uint                        `json:"id" gorm:"primaryKey"`
	SessionID            string                      `json:"sessionId" gorm:"size:36;uniqueIndex;not null"`
	ResumeID             *uint                       `json:"resumeId,omitempty" gorm:"index"`
	ResumeText           string                      `json:"resumeText" gorm:"type:text"`
	JDText               string                      `json:"jdText" gorm:"type:text"`
	SkillID              string                      `json:"skillId" gorm:"size:64;index"`
	Difficulty           string                      `json:"difficulty" gorm:"size:16"`
	TotalQuestions       int                         `json:"totalQuestions"`
	CurrentQuestionIndex int                         `json:"currentQuestionIndex"`
	CurrentPhase         string                      `json:"currentPhase" gorm:"size:64"`
	Status               VoiceSessionStatus          `json:"status" gorm:"size:20;index"`
	QuestionsJSON        string                      `json:"questionsJson" gorm:"type:text"`
	EvaluationJSON       string                      `json:"evaluationJson" gorm:"type:text"`
	EvaluateStatus       commonmodel.AsyncTaskStatus `json:"evaluateStatus" gorm:"size:20"`
	EvaluateError        string                      `json:"evaluateError" gorm:"size:500"`
	OverallScore         int                         `json:"overallScore"`
	LLMProvider          string                      `json:"llmProvider" gorm:"size:50"`
	CreatedAt            time.Time                   `json:"createdAt"`
	UpdatedAt            time.Time                   `json:"updatedAt"`
	EndedAt              *time.Time                  `json:"endedAt,omitempty"`
	Messages             []VoiceMessage              `json:"-" gorm:"foreignKey:SessionID;references:SessionID"`
}

func (s *VoiceSession) BeforeCreate() error {
	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}
	if s.Status == "" {
		s.Status = VoiceSessionStatusCreated
	}
	if s.Difficulty == "" {
		s.Difficulty = DefaultDifficulty
	}
	if s.CurrentPhase == "" {
		s.CurrentPhase = DefaultPhase
	}
	return nil
}

func (s *VoiceSession) BeforeUpdate() error {
	s.UpdatedAt = time.Now()
	return nil
}

func (s *VoiceSession) MarkCompleted() {
	now := time.Now()
	s.Status = VoiceSessionStatusCompleted
	s.EndedAt = &now
	s.UpdatedAt = now
}

type VoiceMessage struct {
	ID          uint        `json:"id" gorm:"primaryKey"`
	SessionID   string      `json:"sessionId" gorm:"size:36;not null;index:idx_voice_message_session_sequence,priority:1"`
	Sequence    int         `json:"sequence" gorm:"index:idx_voice_message_session_sequence,priority:2"`
	Role        MessageRole `json:"role" gorm:"size:20;index"`
	Content     string      `json:"content" gorm:"type:text"`
	Phase       string      `json:"phase" gorm:"size:64"`
	AudioFormat string      `json:"audioFormat" gorm:"size:20"`
	SampleRate  int         `json:"sampleRate"`
	AudioBytes  []byte      `json:"-" gorm:"type:bytea"`
	CreatedAt   time.Time   `json:"createdAt"`
}

func (m *VoiceMessage) BeforeCreate() error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	return nil
}

type PromptQuestion struct {
	Index    int    `json:"index"`
	Question string `json:"question"`
	Category string `json:"category"`
	Phase    string `json:"phase,omitempty"`
}

type CreateSessionRequest struct {
	ResumeID       *uint  `json:"resumeId"`
	ResumeText     string `json:"resumeText"`
	JDText         string `json:"jdText"`
	SkillID        string `json:"skillId"`
	Difficulty     string `json:"difficulty"`
	QuestionCount  int    `json:"questionCount"`
	LLMProvider    string `json:"llmProvider"`
	InitialPhase   string `json:"initialPhase"`
	WelcomeMessage string `json:"welcomeMessage"`
}

type SessionDTO struct {
	SessionID            string                      `json:"sessionId"`
	WebSocketURL         string                      `json:"webSocketUrl,omitempty"`
	ResumeID             *uint                       `json:"resumeId,omitempty"`
	ResumeText           string                      `json:"resumeText"`
	JDText               string                      `json:"jdText"`
	SkillID              string                      `json:"skillId"`
	Difficulty           string                      `json:"difficulty"`
	TotalQuestions       int                         `json:"totalQuestions"`
	CurrentQuestionIndex int                         `json:"currentQuestionIndex"`
	CurrentPhase         string                      `json:"currentPhase"`
	Status               VoiceSessionStatus          `json:"status"`
	Questions            []PromptQuestion            `json:"questions"`
	EvaluateStatus       commonmodel.AsyncTaskStatus `json:"evaluateStatus,omitempty"`
	EvaluateError        string                      `json:"evaluateError,omitempty"`
	OverallScore         int                         `json:"overallScore"`
	CreatedAt            time.Time                   `json:"createdAt"`
	UpdatedAt            time.Time                   `json:"updatedAt"`
	EndedAt              *time.Time                  `json:"endedAt,omitempty"`
}

type EvaluationDTO struct {
	SessionID      string                      `json:"sessionId"`
	Status         VoiceSessionStatus          `json:"status"`
	EvaluateStatus commonmodel.AsyncTaskStatus `json:"evaluateStatus,omitempty"`
	EvaluateError  string                      `json:"evaluateError,omitempty"`
	Report         *evaluation.Report          `json:"report,omitempty"`
}

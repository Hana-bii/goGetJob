package interview

import (
	"time"

	"goGetJob/internal/common/evaluation"
	commonmodel "goGetJob/internal/common/model"
	"goGetJob/internal/modules/interview/skill"
)

const (
	DefaultSkillID       = "java-backend"
	DefaultDifficulty    = "mid"
	DefaultQuestionCount = 5
	maxEvaluateErrorLen  = 500
)

type SessionStatus string

const (
	SessionStatusCreated    SessionStatus = "CREATED"
	SessionStatusInProgress SessionStatus = "IN_PROGRESS"
	SessionStatusCompleted  SessionStatus = "COMPLETED"
	SessionStatusEvaluated  SessionStatus = "EVALUATED"
)

type Session struct {
	ID                   uint                        `json:"id" gorm:"primaryKey"`
	SessionID            string                      `json:"sessionId" gorm:"size:36;uniqueIndex;not null"`
	SkillID              string                      `json:"skillId" gorm:"size:64;index"`
	Difficulty           string                      `json:"difficulty" gorm:"size:16"`
	ResumeID             *uint                       `json:"resumeId" gorm:"index"`
	ResumeText           string                      `json:"resumeText" gorm:"type:text"`
	TotalQuestions       int                         `json:"totalQuestions"`
	CurrentQuestionIndex int                         `json:"currentQuestionIndex"`
	Status               SessionStatus               `json:"status" gorm:"size:20;index"`
	QuestionsJSON        string                      `json:"questionsJson" gorm:"type:text"`
	LLMProvider          string                      `json:"llmProvider" gorm:"size:50"`
	EvaluateStatus       commonmodel.AsyncTaskStatus `json:"evaluateStatus" gorm:"size:20"`
	EvaluateError        string                      `json:"evaluateError" gorm:"size:500"`
	OverallScore         int                         `json:"overallScore"`
	OverallFeedback      string                      `json:"overallFeedback" gorm:"type:text"`
	StrengthsJSON        string                      `json:"strengthsJson" gorm:"type:text"`
	ImprovementsJSON     string                      `json:"improvementsJson" gorm:"type:text"`
	ReferenceAnswersJSON string                      `json:"referenceAnswersJson" gorm:"type:text"`
	CreatedAt            time.Time                   `json:"createdAt"`
	CompletedAt          *time.Time                  `json:"completedAt"`
	Answers              []Answer                    `json:"-" gorm:"foreignKey:SessionID;references:SessionID"`
}

func (s *Session) BeforeCreate() error {
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}
	if s.Status == "" {
		s.Status = SessionStatusCreated
	}
	if s.SkillID == "" {
		s.SkillID = DefaultSkillID
	}
	if s.Difficulty == "" {
		s.Difficulty = DefaultDifficulty
	}
	return nil
}

func (s *Session) MarkCompleted(status SessionStatus) {
	now := time.Now()
	s.Status = status
	s.CompletedAt = &now
}

type Answer struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	SessionID       string    `json:"sessionId" gorm:"size:36;not null;uniqueIndex:uk_interview_answer_session_question,priority:1;index"`
	QuestionIndex   int       `json:"questionIndex" gorm:"uniqueIndex:uk_interview_answer_session_question,priority:2;index"`
	Question        string    `json:"question" gorm:"type:text"`
	Category        string    `json:"category"`
	UserAnswer      string    `json:"userAnswer" gorm:"type:text"`
	Score           int       `json:"score"`
	Feedback        string    `json:"feedback" gorm:"type:text"`
	ReferenceAnswer string    `json:"referenceAnswer" gorm:"type:text"`
	KeyPointsJSON   string    `json:"keyPointsJson" gorm:"type:text"`
	AnsweredAt      time.Time `json:"answeredAt"`
}

func (a *Answer) BeforeCreate() error {
	if a.AnsweredAt.IsZero() {
		a.AnsweredAt = time.Now()
	}
	return nil
}

type Question struct {
	QuestionIndex       int    `json:"questionIndex"`
	Question            string `json:"question"`
	Type                string `json:"type"`
	Category            string `json:"category"`
	TopicSummary        string `json:"topicSummary,omitempty"`
	UserAnswer          string `json:"userAnswer,omitempty"`
	Score               *int   `json:"score,omitempty"`
	Feedback            string `json:"feedback,omitempty"`
	IsFollowUp          bool   `json:"isFollowUp"`
	ParentQuestionIndex *int   `json:"parentQuestionIndex,omitempty"`
}

func NewQuestion(index int, question, qtype, category, topicSummary string, isFollowUp bool, parent *int) Question {
	return Question{
		QuestionIndex:       index,
		Question:            question,
		Type:                qtype,
		Category:            category,
		TopicSummary:        topicSummary,
		IsFollowUp:          isFollowUp,
		ParentQuestionIndex: parent,
	}
}

func (q Question) WithAnswer(answer string) Question {
	q.UserAnswer = answer
	return q
}

func (q Question) ParentQuestionIndexValue() int {
	if q.ParentQuestionIndex == nil {
		return -1
	}
	return *q.ParentQuestionIndex
}

type HistoricalQuestion struct {
	Question     string `json:"question"`
	Type         string `json:"type"`
	TopicSummary string `json:"topicSummary"`
}

type CreateSessionRequest struct {
	ResumeText       string           `json:"resumeText"`
	QuestionCount    int              `json:"questionCount"`
	ResumeID         *uint            `json:"resumeId"`
	ForceCreate      bool             `json:"forceCreate"`
	LLMProvider      string           `json:"llmProvider"`
	SkillID          string           `json:"skillId"`
	Difficulty       string           `json:"difficulty"`
	CustomCategories []skill.Category `json:"customCategories"`
	JDText           string           `json:"jdText"`
}

type SessionDTO struct {
	SessionID            string                      `json:"sessionId"`
	ResumeText           string                      `json:"resumeText"`
	TotalQuestions       int                         `json:"totalQuestions"`
	CurrentQuestionIndex int                         `json:"currentQuestionIndex"`
	Questions            []Question                  `json:"questions"`
	Status               SessionStatus               `json:"status"`
	EvaluateStatus       commonmodel.AsyncTaskStatus `json:"evaluateStatus,omitempty"`
	EvaluateError        string                      `json:"evaluateError,omitempty"`
}

type SessionListItem struct {
	SessionID      string                      `json:"sessionId"`
	SkillID        string                      `json:"skillId"`
	Difficulty     string                      `json:"difficulty"`
	ResumeID       *uint                       `json:"resumeId,omitempty"`
	TotalQuestions int                         `json:"totalQuestions"`
	Status         SessionStatus               `json:"status"`
	EvaluateStatus commonmodel.AsyncTaskStatus `json:"evaluateStatus,omitempty"`
	EvaluateError  string                      `json:"evaluateError,omitempty"`
	OverallScore   int                         `json:"overallScore"`
	CreatedAt      time.Time                   `json:"createdAt"`
	CompletedAt    *time.Time                  `json:"completedAt,omitempty"`
}

type SubmitAnswerRequest struct {
	SessionID     string `json:"sessionId"`
	QuestionIndex int    `json:"questionIndex"`
	Answer        string `json:"answer"`
}

type SubmitAnswerResponse struct {
	HasNextQuestion bool      `json:"hasNextQuestion"`
	NextQuestion    *Question `json:"nextQuestion"`
	CurrentIndex    int       `json:"currentIndex"`
	TotalQuestions  int       `json:"totalQuestions"`
}

type AnswerDetail struct {
	QuestionIndex   int       `json:"questionIndex"`
	Question        string    `json:"question"`
	Category        string    `json:"category"`
	UserAnswer      string    `json:"userAnswer"`
	Score           int       `json:"score"`
	Feedback        string    `json:"feedback"`
	ReferenceAnswer string    `json:"referenceAnswer"`
	KeyPoints       []string  `json:"keyPoints"`
	AnsweredAt      time.Time `json:"answeredAt"`
}

type InterviewDetail struct {
	ID               uint                         `json:"id"`
	SessionID        string                       `json:"sessionId"`
	TotalQuestions   int                          `json:"totalQuestions"`
	Status           SessionStatus                `json:"status"`
	EvaluateStatus   commonmodel.AsyncTaskStatus  `json:"evaluateStatus,omitempty"`
	EvaluateError    string                       `json:"evaluateError,omitempty"`
	OverallScore     int                          `json:"overallScore"`
	OverallFeedback  string                       `json:"overallFeedback"`
	CreatedAt        time.Time                    `json:"createdAt"`
	CompletedAt      *time.Time                   `json:"completedAt,omitempty"`
	Questions        []Question                   `json:"questions"`
	Strengths        []string                     `json:"strengths"`
	Improvements     []string                     `json:"improvements"`
	ReferenceAnswers []evaluation.ReferenceAnswer `json:"referenceAnswers"`
	Answers          []AnswerDetail               `json:"answers"`
}

package interview

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"goGetJob/internal/common/evaluation"
	commonmodel "goGetJob/internal/common/model"
	"goGetJob/internal/infrastructure/export"
)

type staticQuestionGenerator struct {
	questions []Question
}

func (g staticQuestionGenerator) Generate(context.Context, GenerateQuestionsInput) ([]Question, error) {
	return append([]Question(nil), g.questions...), nil
}

type recordingProducer struct {
	tasks []EvaluateTask
}

func (p *recordingProducer) SendEvaluateTask(_ context.Context, task EvaluateTask) error {
	p.tasks = append(p.tasks, task)
	return nil
}

type staticEvaluator struct {
	report evaluation.Report
}

func (e staticEvaluator) Evaluate(context.Context, string, []evaluation.QaRecord, string, string) (evaluation.Report, error) {
	return e.report, nil
}

func TestSessionServiceSaveSubmitCompleteAndEnqueueEvaluation(t *testing.T) {
	repo := NewMemoryRepository()
	producer := &recordingProducer{}
	service := NewSessionService(SessionServiceOptions{
		Repository: repo,
		QuestionGenerator: staticQuestionGenerator{questions: []Question{
			NewQuestion(0, "q1", "JAVA", "Java", "", false, nil),
			NewQuestion(1, "q2", "MYSQL", "MySQL", "", false, nil),
		}},
		EvaluateProducer: producer,
	})

	session, err := service.Create(context.Background(), CreateSessionRequest{
		ResumeID:      uintPtr(7),
		ResumeText:    "resume",
		QuestionCount: 2,
		SkillID:       "java-backend",
		Difficulty:    "mid",
	})
	require.NoError(t, err)
	require.Equal(t, SessionStatusCreated, session.Status)

	err = service.SaveAnswer(context.Background(), SubmitAnswerRequest{
		SessionID:     session.SessionID,
		QuestionIndex: 0,
		Answer:        "draft",
	})
	require.NoError(t, err)
	got, err := service.Get(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Equal(t, 0, got.CurrentQuestionIndex)
	require.Equal(t, "draft", got.Questions[0].UserAnswer)

	response, err := service.SubmitAnswer(context.Background(), SubmitAnswerRequest{
		SessionID:     session.SessionID,
		QuestionIndex: 0,
		Answer:        "final",
	})
	require.NoError(t, err)
	require.True(t, response.HasNextQuestion)
	require.Equal(t, 1, response.CurrentIndex)
	require.Equal(t, "q2", response.NextQuestion.Question)

	response, err = service.SubmitAnswer(context.Background(), SubmitAnswerRequest{
		SessionID:     session.SessionID,
		QuestionIndex: 1,
		Answer:        "last",
	})
	require.NoError(t, err)
	require.False(t, response.HasNextQuestion)
	require.Len(t, producer.tasks, 1)
	require.Equal(t, session.SessionID, producer.tasks[0].SessionID)

	detail, err := service.Get(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Equal(t, SessionStatusCompleted, detail.Status)
	require.Equal(t, commonmodel.AsyncTaskStatusPending, detail.EvaluateStatus)
}

func TestCreateReturnsUnfinishedSessionUnlessForceCreate(t *testing.T) {
	repo := NewMemoryRepository()
	service := NewSessionService(SessionServiceOptions{
		Repository: repo,
		QuestionGenerator: staticQuestionGenerator{questions: []Question{
			NewQuestion(0, "q", "JAVA", "Java", "", false, nil),
		}},
	})

	first, err := service.Create(context.Background(), CreateSessionRequest{
		ResumeID:      uintPtr(11),
		QuestionCount: 1,
		SkillID:       "java-backend",
	})
	require.NoError(t, err)

	second, err := service.Create(context.Background(), CreateSessionRequest{
		ResumeID:      uintPtr(11),
		QuestionCount: 1,
		SkillID:       "java-backend",
	})
	require.NoError(t, err)
	require.Equal(t, first.SessionID, second.SessionID)

	forced, err := service.Create(context.Background(), CreateSessionRequest{
		ResumeID:      uintPtr(11),
		ForceCreate:   true,
		QuestionCount: 1,
		SkillID:       "java-backend",
	})
	require.NoError(t, err)
	require.NotEqual(t, first.SessionID, forced.SessionID)
}

func TestEvaluationTaskHandlerPersistsReportAndHistoryExportIncludesAnswers(t *testing.T) {
	repo := NewMemoryRepository()
	sessionService := NewSessionService(SessionServiceOptions{
		Repository: repo,
		QuestionGenerator: staticQuestionGenerator{questions: []Question{
			NewQuestion(0, "q1", "JAVA", "Java", "", false, nil),
		}},
	})
	session, err := sessionService.Create(context.Background(), CreateSessionRequest{
		QuestionCount: 1,
		SkillID:       "java-backend",
	})
	require.NoError(t, err)
	_, err = sessionService.SubmitAnswer(context.Background(), SubmitAnswerRequest{
		SessionID:     session.SessionID,
		QuestionIndex: 0,
		Answer:        "answer",
	})
	require.NoError(t, err)

	handler := NewEvaluateTaskHandler(repo, staticEvaluator{report: evaluation.Report{
		SessionID:       session.SessionID,
		TotalQuestions:  1,
		OverallScore:    88,
		OverallFeedback: "good",
		Strengths:       []string{"clear"},
		Improvements:    []string{"deeper"},
		QuestionDetails: []evaluation.QuestionEvaluation{
			{QuestionIndex: 0, Question: "q1", Category: "Java", UserAnswer: "answer", Score: 88, Feedback: "ok"},
		},
		ReferenceAnswers: []evaluation.ReferenceAnswer{
			{QuestionIndex: 0, Question: "q1", ReferenceAnswer: "ref", KeyPoints: []string{"point"}},
		},
	}}, nil)

	require.NoError(t, handler.ProcessBusiness(context.Background(), EvaluateTask{SessionID: session.SessionID}))
	require.NoError(t, handler.MarkCompleted(context.Background(), EvaluateTask{SessionID: session.SessionID}))

	history := NewHistoryService(repo, export.NewPDFExporter(export.PDFOptions{}))
	detail, err := history.Detail(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Equal(t, 88, detail.OverallScore)
	require.Len(t, detail.Answers, 1)
	require.Equal(t, "ref", detail.Answers[0].ReferenceAnswer)

	pdf, filename, err := history.Export(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.NotEmpty(t, pdf)
	require.Contains(t, filename, session.SessionID)
}

func uintPtr(value uint) *uint {
	return &value
}

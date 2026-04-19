package voiceinterview

import (
	"context"
	"testing"

	commonmodel "goGetJob/internal/common/model"

	"github.com/stretchr/testify/require"
)

type fakePromptGenerator struct {
	questions []PromptQuestion
}

func (f fakePromptGenerator) GeneratePrompts(context.Context, PromptInput) ([]PromptQuestion, error) {
	return f.questions, nil
}

type recordingEvaluationProducer struct {
	tasks []EvaluationTask
}

func (p *recordingEvaluationProducer) SendEvaluationTask(_ context.Context, task EvaluationTask) error {
	p.tasks = append(p.tasks, task)
	return nil
}

func TestSessionServiceLifecyclePersistsSessionAndMessages(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	producer := &recordingEvaluationProducer{}
	service := NewSessionService(SessionServiceOptions{
		Repository:         repo,
		PromptGenerator:    fakePromptGenerator{questions: []PromptQuestion{{Index: 0, Question: "Tell me about Go.", Category: "Go"}}},
		EvaluationProducer: producer,
	})

	created, err := service.Create(ctx, CreateSessionRequest{ResumeText: "built services", QuestionCount: 1})
	require.NoError(t, err)
	require.NotEmpty(t, created.SessionID)
	require.Equal(t, VoiceSessionStatusCreated, created.Status)
	require.Equal(t, 1, created.TotalQuestions)

	msg, err := service.AppendMessage(ctx, created.SessionID, MessageRoleUser, "I used goroutines.", "answer")
	require.NoError(t, err)
	require.Equal(t, 1, msg.Sequence)

	require.NoError(t, service.Pause(ctx, created.SessionID))
	paused, err := service.Get(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, VoiceSessionStatusPaused, paused.Status)

	_, err = service.Resume(ctx, created.SessionID)
	require.NoError(t, err)
	resumed, err := service.Get(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, VoiceSessionStatusInProgress, resumed.Status)

	require.NoError(t, service.End(ctx, created.SessionID))
	ended, err := service.Get(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, VoiceSessionStatusCompleted, ended.Status)
	require.Equal(t, commonmodel.AsyncTaskStatusPending, ended.EvaluateStatus)
	require.Len(t, producer.tasks, 1)
	require.Equal(t, created.SessionID, producer.tasks[0].SessionID)

	messages, err := service.ListMessages(ctx, created.SessionID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "I used goroutines.", messages[0].Content)
}

func TestSessionServicePreservesErrNotFound(t *testing.T) {
	service := NewSessionService(SessionServiceOptions{Repository: NewMemoryRepository()})

	_, err := service.Get(context.Background(), "missing")

	require.ErrorIs(t, err, ErrNotFound)
}

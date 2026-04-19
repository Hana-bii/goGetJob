package voiceinterview

import (
	"context"
	"testing"

	"goGetJob/internal/common/evaluation"

	"github.com/stretchr/testify/require"
)

type fakeEvaluator struct {
	records []evaluation.QaRecord
}

func (f *fakeEvaluator) Evaluate(_ context.Context, sessionID string, records []evaluation.QaRecord, _ string, _ string) (evaluation.Report, error) {
	f.records = records
	return evaluation.Report{
		SessionID:      sessionID,
		TotalQuestions: len(records),
		OverallScore:   88,
		QuestionDetails: []evaluation.QuestionEvaluation{{
			QuestionIndex: records[0].QuestionIndex,
			Question:      records[0].Question,
			Category:      records[0].Category,
			UserAnswer:    records[0].UserAnswer,
			Score:         88,
			Feedback:      "solid",
		}},
		OverallFeedback: "good",
	}, nil
}

func TestEvaluationServiceConvertsDialogueToQARecords(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	session := &VoiceSession{SessionID: "s1", Status: VoiceSessionStatusCompleted, QuestionsJSON: mustJSONQuestions(t, []PromptQuestion{{Index: 0, Question: "What is Go?", Category: "Go"}})}
	require.NoError(t, repo.CreateSession(ctx, session))
	require.NoError(t, repo.AppendMessage(ctx, &VoiceMessage{SessionID: "s1", Role: MessageRoleAssistant, Content: "What is Go?", Phase: "question"}))
	require.NoError(t, repo.AppendMessage(ctx, &VoiceMessage{SessionID: "s1", Role: MessageRoleUser, Content: "A programming language.", Phase: "answer"}))
	evaluator := &fakeEvaluator{}
	service := NewEvaluationService(repo, evaluator)

	report, err := service.EvaluateSession(ctx, "s1")

	require.NoError(t, err)
	require.Equal(t, 88, report.OverallScore)
	require.Len(t, evaluator.records, 1)
	require.Equal(t, "What is Go?", evaluator.records[0].Question)
	require.Equal(t, "A programming language.", evaluator.records[0].UserAnswer)

	stored, err := repo.FindSessionByID(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, VoiceSessionStatusEvaluated, stored.Status)
	require.Contains(t, stored.EvaluationJSON, "good")
}

func mustJSONQuestions(t *testing.T, questions []PromptQuestion) string {
	t.Helper()
	encoded, err := encodeQuestions(questions)
	require.NoError(t, err)
	return encoded
}

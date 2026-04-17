package evaluation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"goGetJob/internal/common/ai"
)

func TestEvaluateSplitsBatchesMergesSummaryAndReferenceAnswers(t *testing.T) {
	model := &sequenceModel{responses: []string{
		`{"overallScore":80,"overallFeedback":"batch one","strengths":["clear"],"improvements":["depth"],"questionEvaluations":[{"questionIndex":0,"score":80,"feedback":"ok","referenceAnswer":"ref0","keyPoints":["k0"]}]}`,
		`{"overallScore":60,"overallFeedback":"batch two","strengths":["basic"],"improvements":["practice"],"questionEvaluations":[{"questionIndex":1,"score":60,"feedback":"fine","referenceAnswer":"ref1","keyPoints":["k1"]}]}`,
		`{"overallFeedback":"summary","strengths":["merged strength"],"improvements":["merged improvement"]}`,
	}}
	service := NewService(Options{Model: model, PromptLoader: ai.NewPromptLoader("../../prompts"), BatchSize: 1})

	got, err := service.Evaluate(context.Background(), "s1", []QaRecord{
		{QuestionIndex: 0, Question: "Q1", Category: "Go", UserAnswer: "A1"},
		{QuestionIndex: 1, Question: "Q2", Category: "Redis", UserAnswer: "A2"},
	}, "resume", "reference baseline")

	require.NoError(t, err)
	require.Equal(t, 70, got.OverallScore)
	require.Equal(t, "summary", got.OverallFeedback)
	require.Equal(t, []string{"merged strength"}, got.Strengths)
	require.Len(t, got.ReferenceAnswers, 2)
	require.Equal(t, "ref1", got.ReferenceAnswers[1].ReferenceAnswer)
	require.Len(t, model.prompts, 3)
	require.Contains(t, model.prompts[0], "reference baseline")
}

func TestEvaluateUsesZeroScoreFallbackAndSummaryFallback(t *testing.T) {
	model := &sequenceModel{errs: []error{errors.New("batch down"), errors.New("summary down")}}
	service := NewService(Options{Model: model, PromptLoader: ai.NewPromptLoader("../../prompts"), BatchSize: 10})

	got, err := service.Evaluate(context.Background(), "s2", []QaRecord{
		{QuestionIndex: 0, Question: "Q1", Category: "Go", UserAnswer: "A1"},
	}, "", "")

	require.NoError(t, err)
	require.Equal(t, 0, got.OverallScore)
	require.Equal(t, 0, got.QuestionDetails[0].Score)
	require.NotEmpty(t, got.OverallFeedback)
	require.NotEmpty(t, got.Improvements)
}

func TestEvaluateCategoryAveragesAndUnansweredZero(t *testing.T) {
	model := &sequenceModel{responses: []string{
		`{"overallScore":100,"overallFeedback":"ok","strengths":[],"improvements":[],"questionEvaluations":[{"questionIndex":0,"score":100,"feedback":"ok","referenceAnswer":"ref","keyPoints":[]},{"questionIndex":1,"score":100,"feedback":"ok","referenceAnswer":"ref","keyPoints":[]}]}`,
		`{"overallFeedback":"ok","strengths":[],"improvements":[]}`,
	}}
	service := NewService(Options{Model: model, PromptLoader: ai.NewPromptLoader("../../prompts"), BatchSize: 10})

	got, err := service.Evaluate(context.Background(), "s3", []QaRecord{
		{QuestionIndex: 0, Question: "Q1", Category: "Go", UserAnswer: "A1"},
		{QuestionIndex: 1, Question: "Q2", Category: "Go"},
	}, "", "")

	require.NoError(t, err)
	require.Equal(t, 50, got.OverallScore)
	require.Equal(t, 50, got.CategoryScores[0].Score)
}

type sequenceModel struct {
	responses []string
	errs      []error
	prompts   []string
	calls     int
}

func (m *sequenceModel) Generate(_ context.Context, messages []ai.ChatMessage) (string, error) {
	if len(messages) > 0 {
		m.prompts = append(m.prompts, messages[0].Content)
	}
	call := m.calls
	m.calls++
	if call < len(m.errs) && m.errs[call] != nil {
		return "", m.errs[call]
	}
	if call < len(m.responses) {
		return m.responses[call], nil
	}
	return `{}`, nil
}

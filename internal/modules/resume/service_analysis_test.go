package resume

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"goGetJob/internal/common/ai"
	commonmodel "goGetJob/internal/common/model"
)

func TestAIAnalysisServiceParsesStructuredOutput(t *testing.T) {
	model := &fakeChatModel{response: `{
		"overallScore": 88,
		"scoreDetail": {
			"contentScore": 20,
			"structureScore": 18,
			"skillMatchScore": 22,
			"expressionScore": 13,
			"projectScore": 15
		},
		"summary": "solid backend resume",
		"strengths": ["Go", "Redis"],
		"suggestions": [{"category":"project","priority":"medium","issue":"detail","recommendation":"add metrics"}]
	}`}
	svc := NewAIAnalysisService(AIAnalysisOptions{
		Model:        model,
		PromptLoader: ai.NewPromptLoader("../../prompts"),
		MaxAttempts:  1,
	})

	got, err := svc.Analyze(context.Background(), "resume body")

	require.NoError(t, err)
	require.Equal(t, 88, got.OverallScore)
	require.Equal(t, 22, got.ScoreDetail.SkillMatchScore)
	require.Equal(t, []string{"Go", "Redis"}, got.Strengths)
	require.Len(t, got.Suggestions, 1)
	require.Equal(t, "resume body", got.OriginalText)
	require.Contains(t, model.prompt, "resume body")
}

func TestAnalyzeTaskHandlerStatusTransitions(t *testing.T) {
	repo := NewMemoryRepository()
	resume := &Resume{OriginalFilename: "resume.txt", FileHash: "hash", AnalyzeStatus: commonmodel.AsyncTaskStatusPending}
	require.NoError(t, repo.CreateResume(context.Background(), resume))
	handler := NewAnalyzeTaskHandler(repo, fakeAnalyzer{result: AnalysisResult{OverallScore: 90, Summary: "ok"}})
	task := AnalyzeTask{ResumeID: resume.ID, Content: "resume text"}

	require.NoError(t, handler.MarkProcessing(context.Background(), task))
	require.NoError(t, handler.ProcessBusiness(context.Background(), task))
	require.NoError(t, handler.MarkCompleted(context.Background(), task))

	updated, err := repo.FindResumeByID(context.Background(), resume.ID)
	require.NoError(t, err)
	require.Equal(t, commonmodel.AsyncTaskStatusCompleted, updated.AnalyzeStatus)
	require.Empty(t, updated.AnalyzeError)
	latest, err := repo.LatestAnalysis(context.Background(), resume.ID)
	require.NoError(t, err)
	require.Equal(t, 90, latest.OverallScore)
}

func TestAnalyzeTaskHandlerMarksFailedWithTruncatedError(t *testing.T) {
	repo := NewMemoryRepository()
	resume := &Resume{OriginalFilename: "resume.txt", FileHash: "hash", AnalyzeStatus: commonmodel.AsyncTaskStatusProcessing}
	require.NoError(t, repo.CreateResume(context.Background(), resume))
	handler := NewAnalyzeTaskHandler(repo, fakeAnalyzer{})

	require.NoError(t, handler.MarkFailed(context.Background(), AnalyzeTask{ResumeID: resume.ID}, errors.New(strings.Repeat("x", 700))))

	updated, err := repo.FindResumeByID(context.Background(), resume.ID)
	require.NoError(t, err)
	require.Equal(t, commonmodel.AsyncTaskStatusFailed, updated.AnalyzeStatus)
	require.Len(t, updated.AnalyzeError, maxAnalyzeErrorLength)
}

type fakeChatModel struct {
	response string
	prompt   string
}

func (m *fakeChatModel) Generate(_ context.Context, messages []ai.ChatMessage) (string, error) {
	if len(messages) > 0 {
		m.prompt = messages[0].Content
	}
	return m.response, nil
}

type fakeAnalyzer struct {
	result AnalysisResult
	err    error
}

func (a fakeAnalyzer) Analyze(context.Context, string) (AnalysisResult, error) {
	return a.result, a.err
}

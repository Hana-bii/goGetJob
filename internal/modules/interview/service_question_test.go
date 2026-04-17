package interview

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"goGetJob/internal/common/ai"
	"goGetJob/internal/modules/interview/skill"
)

type promptAwareModel struct {
	mu      sync.Mutex
	prompts []string
}

func (m *promptAwareModel) Generate(_ context.Context, messages []ai.ChatMessage) (string, error) {
	prompt := messages[0].Content
	m.mu.Lock()
	m.prompts = append(m.prompts, prompt)
	m.mu.Unlock()

	if strings.Contains(prompt, "resume text for candidate") {
		return `{"questions":[
			{"question":"resume question 1","type":"PROJECT","category":"Project","topicSummary":"resume-1","followUps":["r1-a","r1-b","r1-c"]},
			{"question":"resume question 2","type":"PROJECT","category":"Project","topicSummary":"resume-2"},
			{"question":"resume question 3","type":"PROJECT","category":"Project","topicSummary":"resume-3"}
		]}`, nil
	}
	return `{"questions":[
		{"question":"skill question 1","type":"JAVA","category":"Java","topicSummary":"skill-1","followUps":["s1-a","s1-b","s1-c"]},
		{"question":"skill question 2","type":"MYSQL","category":"MySQL","topicSummary":"skill-2","followUps":["s2-a"]}
	]}`, nil
}

func (m *promptAwareModel) joinedPrompts() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return strings.Join(m.prompts, "\n---\n")
}

type failingModel struct{}

func (failingModel) Generate(context.Context, []ai.ChatMessage) (string, error) {
	return "", fmt.Errorf("model unavailable")
}

func TestGenerateQuestionsWithoutResumeUsesSkillPromptHistoryAndCapsFollowUps(t *testing.T) {
	model := &promptAwareModel{}
	service := newTestQuestionService(t, model, 5)

	questions, err := service.Generate(context.Background(), GenerateQuestionsInput{
		SkillID:       "java-backend",
		Difficulty:    "mid",
		QuestionCount: 2,
		HistoricalQuestions: []HistoricalQuestion{
			{Question: "old redis question", Type: "REDIS", TopicSummary: "Redis persistence"},
		},
	})

	require.NoError(t, err)
	require.Len(t, questions, 5)
	require.Equal(t, "skill question 1", questions[0].Question)
	require.False(t, questions[0].IsFollowUp)
	require.Equal(t, 0, questions[1].ParentQuestionIndexValue())
	require.Equal(t, 0, questions[2].ParentQuestionIndexValue())
	require.Equal(t, "skill question 2", questions[3].Question)
	require.Equal(t, "s2-a", questions[4].Question)
	require.Contains(t, model.joinedPrompts(), "Redis persistence")
	require.Contains(t, model.joinedPrompts(), "java-backend")
}

func TestGenerateQuestionsWithResumeUsesResumeSixtyDirectionFortyAndOffsetsDirectionIndexes(t *testing.T) {
	model := &promptAwareModel{}
	service := newTestQuestionService(t, model, 2)

	questions, err := service.Generate(context.Background(), GenerateQuestionsInput{
		SkillID:       "java-backend",
		Difficulty:    "senior",
		QuestionCount: 5,
		ResumeText:    "resume text for candidate",
	})

	require.NoError(t, err)
	require.Len(t, questions, 10)
	require.Equal(t, "resume question 1", questions[0].Question)
	require.Equal(t, "resume question 3", questions[4].Question)
	require.Equal(t, "skill question 1", questions[5].Question)
	require.Equal(t, 5, questions[5].QuestionIndex)
	require.Equal(t, 5, questions[6].ParentQuestionIndexValue())
	require.Equal(t, "skill question 2", questions[8].Question)
	require.Contains(t, model.joinedPrompts(), "resume text for candidate")
	require.Contains(t, model.joinedPrompts(), "3+ years")
}

func TestGenerateQuestionsFallsBackToSkillCategoriesWhenDirectionModelFails(t *testing.T) {
	service := newTestQuestionService(t, failingModel{}, 1)

	questions, err := service.Generate(context.Background(), GenerateQuestionsInput{
		SkillID:       "java-backend",
		Difficulty:    "mid",
		QuestionCount: 3,
	})

	require.NoError(t, err)
	require.Len(t, questions, 6)
	require.False(t, questions[0].IsFollowUp)
	require.True(t, questions[1].IsFollowUp)
	require.NotEmpty(t, questions[0].Category)
	require.NotEmpty(t, questions[0].Question)
}

func newTestQuestionService(t *testing.T, model ai.ChatModel, followUps int) *QuestionService {
	t.Helper()
	skillService, err := skill.NewService(skill.Options{Root: "../../../internal/skills"})
	require.NoError(t, err)
	return NewQuestionService(QuestionServiceOptions{
		Model:         model,
		ResumeModel:   model,
		SkillService:  skillService,
		PromptLoader:  ai.NewPromptLoader("../../../internal/prompts"),
		FollowUpCount: followUps,
	})
}

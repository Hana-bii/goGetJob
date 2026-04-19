package voiceinterview

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPromptServiceUsesFallbackQuestionsWithoutModel(t *testing.T) {
	service := NewPromptService(PromptServiceOptions{})

	questions, err := service.GeneratePrompts(context.Background(), PromptInput{QuestionCount: 2, ResumeText: "Go backend"})

	require.NoError(t, err)
	require.Len(t, questions, 2)
	require.Equal(t, 0, questions[0].Index)
	require.NotEmpty(t, questions[0].Question)
	require.NotEmpty(t, questions[0].Category)
}

func TestPromptServiceBuildsDialoguePrompt(t *testing.T) {
	service := NewPromptService(PromptServiceOptions{})
	messages := []VoiceMessage{
		{Role: MessageRoleAssistant, Content: "Please introduce yourself."},
		{Role: MessageRoleUser, Content: "I am a Go engineer."},
	}

	prompt := service.BuildReplyPrompt(VoiceSession{SessionID: "s1", CurrentPhase: "technical"}, messages)

	require.Contains(t, prompt, "technical")
	require.Contains(t, prompt, "Please introduce yourself.")
	require.Contains(t, prompt, "I am a Go engineer.")
}

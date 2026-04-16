package ai_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"goGetJob/internal/common/ai"
)

func TestPromptLoaderLoadsCopiedPrompt(t *testing.T) {
	loader := ai.NewPromptLoader("../../../internal/prompts")

	got, err := loader.Load("knowledgebase-query-user.st")

	require.NoError(t, err)
	require.Contains(t, got, "{context}")
	require.Contains(t, got, "{question}")
}

func TestPromptLoaderRelativeRootSurvivesWorkingDirectoryChange(t *testing.T) {
	root := filepath.Join("..", "..", "..", "internal", "prompts")
	loader := ai.NewPromptLoader(root)
	t.Chdir(t.TempDir())

	got, err := loader.Load("knowledgebase-query-user.st")

	require.NoError(t, err)
	require.Contains(t, got, "{context}")
	require.Contains(t, got, "{question}")
}

func TestPromptLoaderReturnsErrorForMissingPrompt(t *testing.T) {
	loader := ai.NewPromptLoader("../../../internal/prompts")

	_, err := loader.Load("does-not-exist.st")

	require.Error(t, err)
	require.Contains(t, err.Error(), "prompt not found")
	require.Contains(t, err.Error(), "does-not-exist.st")
}

func TestPromptLoaderRendersVariables(t *testing.T) {
	loader := ai.NewPromptLoader("../../../internal/prompts")

	got, err := loader.Render("knowledgebase-query-user.st", map[string]string{
		"context":  "Redis Streams",
		"question": "How do consumer groups work?",
	})

	require.NoError(t, err)
	require.Contains(t, got, "Redis Streams")
	require.Contains(t, got, "How do consumer groups work?")
	require.False(t, strings.Contains(got, "{context}"))
	require.False(t, strings.Contains(got, "{question}"))
}

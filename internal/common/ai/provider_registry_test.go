package ai_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"goGetJob/internal/common/ai"
)

func TestOpenAICompatibleChatModelReturnsHTTPStatusAndBodyForNonJSONError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "plain text auth failure", http.StatusUnauthorized)
	}))
	defer server.Close()

	model := ai.NewOpenAICompatibleChatModel(server.URL, "bad-key", "qwen-plus", server.Client())

	_, err := model.Generate(context.Background(), []ai.ChatMessage{
		{Role: "user", Content: "hello"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
	require.Contains(t, err.Error(), "plain text auth failure")
	require.NotContains(t, strings.ToLower(err.Error()), "decode")
}

package knowledgebase

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterRoutesExposesKnowledgeBaseAndRagChatEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterRoutes(engine, NewHandler(NewServiceBundleForTest()))

	registered := map[string]bool{}
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /api/knowledgebase/list",
		"GET /api/knowledgebase/:id",
		"DELETE /api/knowledgebase/:id",
		"POST /api/knowledgebase/query",
		"POST /api/knowledgebase/query/stream",
		"GET /api/knowledgebase/categories",
		"GET /api/knowledgebase/category/:category",
		"GET /api/knowledgebase/uncategorized",
		"PUT /api/knowledgebase/:id/category",
		"POST /api/knowledgebase/upload",
		"GET /api/knowledgebase/:id/download",
		"GET /api/knowledgebase/search",
		"GET /api/knowledgebase/stats",
		"POST /api/knowledgebase/:id/revectorize",
		"POST /api/rag-chat/sessions",
		"GET /api/rag-chat/sessions",
		"GET /api/rag-chat/sessions/:sessionId",
		"PUT /api/rag-chat/sessions/:sessionId/title",
		"PUT /api/rag-chat/sessions/:sessionId/pin",
		"PUT /api/rag-chat/sessions/:sessionId/knowledge-bases",
		"DELETE /api/rag-chat/sessions/:sessionId",
		"POST /api/rag-chat/sessions/:sessionId/messages/stream",
	} {
		require.True(t, registered[route], route)
	}
}

func TestKnowledgeBaseQueryUploadAndRevectorizeRateLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := &recordingRateLimitRedis{}
	engine := gin.New()
	RegisterRoutes(engine, NewHandler(NewServiceBundleForTest()), limiter)

	requests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/knowledgebase/query", `{"knowledgeBaseIds":[1],"question":"q"}`},
		{http.MethodPost, "/api/knowledgebase/query/stream", `{"knowledgeBaseIds":[1],"question":"q"}`},
		{http.MethodPost, "/api/knowledgebase/1/revectorize", `{}`},
	}
	for _, item := range requests {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(item.method, item.path, bytes.NewBufferString(item.body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
	}

	joined := strings.Join(limiter.keys, "\n")
	require.Contains(t, joined, "ratelimit:{knowledgebase:query}:global")
	require.Contains(t, joined, "ratelimit:{knowledgebase:query}:ip:")
	require.Contains(t, joined, "ratelimit:{knowledgebase:query-stream}:global")
	require.Contains(t, joined, "ratelimit:{knowledgebase:query-stream}:ip:")
	require.Contains(t, joined, "ratelimit:{knowledgebase:revectorize}:global")
	require.Contains(t, joined, "ratelimit:{knowledgebase:revectorize}:ip:")
}

type recordingRateLimitRedis struct {
	keys []string
}

func (r *recordingRateLimitRedis) ScriptLoad(context.Context, string) (string, error) {
	return "sha", nil
}

func (r *recordingRateLimitRedis) EvalSHA(_ context.Context, _ string, keys []string, _ ...any) (any, error) {
	r.keys = append(r.keys, keys...)
	return int64(1), nil
}

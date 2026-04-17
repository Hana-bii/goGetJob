package interview

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

func TestRegisterRoutesKeepsUnfinishedRouteReachable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(
		NewSessionService(SessionServiceOptions{Repository: NewMemoryRepository(), QuestionGenerator: staticQuestionGenerator{}}),
		NewHistoryService(NewMemoryRepository(), nil),
		nil,
		nil,
	)

	require.NotPanics(t, func() {
		RegisterRoutes(gin.New(), handler)
	})
}

func TestRegisterRoutesExposesTaskSevenEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterRoutes(engine, NewHandler(
		NewSessionService(SessionServiceOptions{Repository: NewMemoryRepository(), QuestionGenerator: staticQuestionGenerator{}}),
		NewHistoryService(NewMemoryRepository(), nil),
		nil,
		nil,
	))

	registered := map[string]bool{}
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = true
	}

	for _, route := range []string{
		"GET /api/interview/sessions",
		"POST /api/interview/sessions",
		"GET /api/interview/sessions/:sessionId",
		"GET /api/interview/sessions/:sessionId/question",
		"POST /api/interview/sessions/:sessionId/answers",
		"PUT /api/interview/sessions/:sessionId/answers",
		"POST /api/interview/sessions/:sessionId/complete",
		"GET /api/interview/sessions/:sessionId/report",
		"GET /api/interview/sessions/:sessionId/details",
		"GET /api/interview/sessions/:sessionId/export",
		"DELETE /api/interview/sessions/:sessionId",
	} {
		require.True(t, registered[route], route)
	}
}

func TestInterviewRoutesApplyCreateAndSubmitRateLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := NewMemoryRepository()
	limiter := &recordingRateLimitRedis{}
	engine := gin.New()
	service := NewSessionService(SessionServiceOptions{
		Repository: repo,
		QuestionGenerator: staticQuestionGenerator{questions: []Question{
			NewQuestion(0, "q", "JAVA", "Java", "", false, nil),
		}},
	})
	RegisterRoutes(engine, NewHandler(service, NewHistoryService(repo, nil), nil, nil), limiter)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/interview/sessions", bytes.NewBufferString(`{"questionCount":1,"skillId":"java-backend"}`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var sessionID string
	for _, session := range repo.sessions {
		sessionID = session.SessionID
	}
	require.NotEmpty(t, sessionID)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/interview/sessions/"+sessionID+"/answers", bytes.NewBufferString(`{"questionIndex":0,"answer":"ok"}`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	joined := strings.Join(limiter.keys, "\n")
	require.Contains(t, joined, "ratelimit:{interview:create}:global")
	require.Contains(t, joined, "ratelimit:{interview:create}:ip:")
	require.Contains(t, joined, "ratelimit:{interview:submit-answer}:global")
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

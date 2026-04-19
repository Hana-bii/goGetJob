package interviewschedule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestService_CRUDAndStatusTransitions(t *testing.T) {
	repo := NewMemoryRepository()
	service := NewService(repo)
	updater := NewStatusUpdater(repo)
	ctx := context.Background()

	created, err := service.Create(ctx, CreateInterviewRequest{
		CompanyName:   "深蓝科技",
		Position:      "Go 开发工程师",
		InterviewTime: "2026-04-20T14:30:00",
	})
	require.NoError(t, err)
	require.Equal(t, "深蓝科技", created.CompanyName)
	require.Equal(t, "Go 开发工程师", created.Position)
	require.Equal(t, "2026-04-20T14:30:00", created.InterviewTime)
	require.Equal(t, "VIDEO", created.InterviewType)
	require.Equal(t, 1, created.RoundNumber)
	require.Equal(t, InterviewStatusPending, created.Status)
	require.NotZero(t, created.ID)

	got, err := service.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)

	updated, err := service.Update(ctx, created.ID, CreateInterviewRequest{
		CompanyName:   "深蓝科技有限公司",
		Position:      "高级 Go 开发工程师",
		InterviewTime: "2026-04-21T10:00:00",
		InterviewType: "PHONE",
		RoundNumber:   2,
		Interviewer:   "王五",
		Notes:         "注意带上作品集",
	})
	require.NoError(t, err)
	require.Equal(t, "深蓝科技有限公司", updated.CompanyName)
	require.Equal(t, "PHONE", updated.InterviewType)
	require.Equal(t, 2, updated.RoundNumber)
	require.Equal(t, "王五", updated.Interviewer)
	require.Equal(t, "注意带上作品集", updated.Notes)
	require.Equal(t, InterviewStatusPending, updated.Status)

	statusUpdated, err := service.UpdateStatus(ctx, created.ID, InterviewStatusCompleted)
	require.NoError(t, err)
	require.Equal(t, InterviewStatusCompleted, statusUpdated.Status)

	filtered, err := service.List(ctx, string(InterviewStatusCompleted), nil, nil)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, created.ID, filtered[0].ID)

	err = service.Delete(ctx, created.ID)
	require.NoError(t, err)

	_, err = service.GetByID(ctx, created.ID)
	require.ErrorIs(t, err, ErrNotFound)

	past, err := service.Create(ctx, CreateInterviewRequest{
		CompanyName:   "过期公司",
		Position:      "测试工程师",
		InterviewTime: "2024-01-01T09:00:00",
	})
	require.NoError(t, err)

	future, err := service.Create(ctx, CreateInterviewRequest{
		CompanyName:   "未来公司",
		Position:      "测试工程师",
		InterviewTime: "2026-12-31T09:00:00",
	})
	require.NoError(t, err)

	completedPast, err := service.Create(ctx, CreateInterviewRequest{
		CompanyName:   "完成公司",
		Position:      "测试工程师",
		InterviewTime: "2024-01-01T09:00:00",
	})
	require.NoError(t, err)
	_, err = service.UpdateStatus(ctx, completedPast.ID, InterviewStatusCompleted)
	require.NoError(t, err)

	updatedCount, err := updater.UpdateExpired(ctx, time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local))
	require.NoError(t, err)
	require.Equal(t, 1, updatedCount)

	expired, err := service.GetByID(ctx, past.ID)
	require.NoError(t, err)
	require.Equal(t, InterviewStatusCancelled, expired.Status)

	stillFuture, err := service.GetByID(ctx, future.ID)
	require.NoError(t, err)
	require.Equal(t, InterviewStatusPending, stillFuture.Status)

	stillCompleted, err := service.GetByID(ctx, completedPast.ID)
	require.NoError(t, err)
	require.Equal(t, InterviewStatusCompleted, stillCompleted.Status)
}

func TestServiceRejectsUnsupportedInterviewType(t *testing.T) {
	service := NewService(NewMemoryRepository())

	_, err := service.Create(context.Background(), CreateInterviewRequest{
		CompanyName:   "Test Co",
		Position:      "Go Engineer",
		InterviewTime: "2026-04-20T14:30:00",
		InterviewType: "WEBEX",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidInput)
	require.Contains(t, err.Error(), "unsupported interview type")
}

func TestRegisterRoutesExposesFrontendPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterRoutes(engine, NewHandler(NewService(NewMemoryRepository()), NewParseService(nil)))

	routes := map[string]bool{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	for _, route := range []string{
		"POST /api/interview-schedule/parse",
		"POST /api/interview-schedule",
		"GET /api/interview-schedule",
		"GET /api/interview-schedule/:id",
		"PUT /api/interview-schedule/:id",
		"DELETE /api/interview-schedule/:id",
		"PATCH /api/interview-schedule/:id/status",
		"PUT /api/interview-schedule/:id/status",
	} {
		require.True(t, routes[route], route)
	}
}

func TestHandlerReturns404ForMissingSchedule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterRoutes(engine, NewHandler(NewService(NewMemoryRepository()), NewParseService(nil)))

	w := performRequest(engine, http.MethodGet, "/api/interview-schedule/1", "")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func performRequest(engine *gin.Engine, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

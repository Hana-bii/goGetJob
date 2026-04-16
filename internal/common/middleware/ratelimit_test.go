package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"goGetJob/internal/common/middleware"
)

type fakeRateLimitRedis struct {
	sha        string
	loads      int
	evalErrs   []error
	evalValues []any
	calls      []rateLimitCall
}

type rateLimitCall struct {
	sha  string
	keys []string
	args []any
}

func (f *fakeRateLimitRedis) ScriptLoad(ctx context.Context, script string) (string, error) {
	f.loads++
	if f.sha == "" {
		f.sha = "sha-1"
	}
	return f.sha, nil
}

func (f *fakeRateLimitRedis) EvalSHA(ctx context.Context, sha string, keys []string, args ...any) (any, error) {
	f.calls = append(f.calls, rateLimitCall{sha: sha, keys: append([]string(nil), keys...), args: append([]any(nil), args...)})
	if len(f.evalErrs) > 0 {
		err := f.evalErrs[0]
		f.evalErrs = f.evalErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	if len(f.evalValues) > 0 {
		value := f.evalValues[0]
		f.evalValues = f.evalValues[1:]
		return value, nil
	}
	return int64(1), nil
}

func TestRateLimitTrustedForwardedHeadersBuildsGlobalIPAndUserKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redis := &fakeRateLimitRedis{sha: "loaded-sha"}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "ctx-user")
		c.Next()
	})
	router.Use(middleware.RateLimitWithOptions("KnowledgeBaseController.Query", redis, middleware.RateLimitOptions{TrustForwardedHeaders: true},
		middleware.Rule{Dimension: middleware.DimensionGlobal, Limit: 10, Window: time.Minute},
		middleware.Rule{Dimension: middleware.DimensionIP, Limit: 5, Window: time.Minute},
		middleware.Rule{Dimension: middleware.DimensionUser, Limit: 3, Window: time.Minute},
	))
	router.GET("/query", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.2")
	req.Header.Set("X-Real-IP", "198.51.100.7")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Len(t, redis.calls, 3)
	require.Equal(t, []string{"ratelimit:{KnowledgeBaseController.Query}:global"}, redis.calls[0].keys)
	require.Equal(t, []string{"ratelimit:{KnowledgeBaseController.Query}:ip:203.0.113.9"}, redis.calls[1].keys)
	require.Equal(t, []string{"ratelimit:{KnowledgeBaseController.Query}:user:ctx-user"}, redis.calls[2].keys)
}

func TestRateLimitTrustedForwardedHeadersUsesRealIPWhenForwardedForMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redis := &fakeRateLimitRedis{sha: "loaded-sha"}
	router := gin.New()
	router.Use(middleware.RateLimitWithOptions("KnowledgeBaseController.Query", redis, middleware.RateLimitOptions{TrustForwardedHeaders: true},
		middleware.Rule{Dimension: middleware.DimensionIP, Limit: 5, Window: time.Minute},
	))
	router.GET("/query", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	req.RemoteAddr = "192.0.2.55:4321"
	req.Header.Set("X-Real-IP", "198.51.100.7")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Len(t, redis.calls, 1)
	require.Equal(t, []string{"ratelimit:{KnowledgeBaseController.Query}:ip:198.51.100.7"}, redis.calls[0].keys)
}

func TestRateLimitUsesClientIPByDefaultWhenForwardedHeaderIsSpoofed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redis := &fakeRateLimitRedis{sha: "loaded-sha"}
	router := gin.New()
	router.Use(middleware.RateLimit("KnowledgeBaseController.Query", redis,
		middleware.Rule{Dimension: middleware.DimensionIP, Limit: 5, Window: time.Minute},
	))
	router.GET("/query", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	req.RemoteAddr = "192.0.2.55:4321"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.Header.Set("X-Real-IP", "198.51.100.7")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Len(t, redis.calls, 1)
	require.Equal(t, []string{"ratelimit:{KnowledgeBaseController.Query}:ip:192.0.2.55"}, redis.calls[0].keys)
}

func TestRateLimitRejectsWhenAnyRuleIsExceeded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redis := &fakeRateLimitRedis{sha: "loaded-sha", evalValues: []any{int64(1), int64(1), int64(0)}}
	router := gin.New()
	router.Use(middleware.RateLimit("InterviewController.Submit", redis,
		middleware.Rule{Dimension: middleware.DimensionGlobal, Limit: 10, Window: time.Minute},
		middleware.Rule{Dimension: middleware.DimensionIP, Limit: 5, Window: time.Minute},
		middleware.Rule{Dimension: middleware.DimensionUser, Limit: 3, Window: time.Minute},
	))
	handlerCalled := false
	router.POST("/submit", func(c *gin.Context) {
		handlerCalled = true
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set("X-User-Id", "header-user")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.JSONEq(t, `{"code":429,"message":"RATE_LIMIT_EXCEEDED","data":null}`, w.Body.String())
	require.Len(t, redis.calls, 3)
	require.Equal(t, []string{"ratelimit:{InterviewController.Submit}:user:header-user"}, redis.calls[2].keys)
	require.False(t, handlerCalled)
}

func TestRateLimitReloadsScriptOnNoScript(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redis := &fakeRateLimitRedis{
		sha:        "loaded-sha",
		evalErrs:   []error{errors.New("NOSCRIPT No matching script. Please use EVAL."), nil},
		evalValues: []any{int64(1)},
	}
	router := gin.New()
	router.Use(middleware.RateLimit("KnowledgeBaseController.Upload", redis,
		middleware.Rule{Dimension: middleware.DimensionGlobal, Limit: 1, Window: time.Minute},
	))
	router.POST("/upload", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/upload", nil))

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, 2, redis.loads)
	require.Len(t, redis.calls, 2)
}

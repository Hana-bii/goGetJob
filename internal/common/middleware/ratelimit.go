package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"goGetJob/internal/common/response"
)

const (
	rateLimitExceededCode    = 429
	rateLimitExceededMessage = "RATE_LIMIT_EXCEEDED"
	defaultPermits           = 1
)

type Dimension string

const (
	DimensionGlobal Dimension = "GLOBAL"
	DimensionIP     Dimension = "IP"
	DimensionUser   Dimension = "USER"
)

type Rule struct {
	Dimension Dimension
	Limit     int
	Window    time.Duration
	Permits   int
}

type RateLimitRedis interface {
	ScriptLoad(ctx context.Context, script string) (string, error)
	EvalSHA(ctx context.Context, sha string, keys []string, args ...any) (any, error)
}

type RateLimitOptions struct {
	TrustForwardedHeaders bool
}

func RateLimit(action string, redis RateLimitRedis, rules ...Rule) gin.HandlerFunc {
	return RateLimitWithOptions(action, redis, RateLimitOptions{}, rules...)
}

func RateLimitWithOptions(action string, redis RateLimitRedis, options RateLimitOptions, rules ...Rule) gin.HandlerFunc {
	limiter := &rateLimiter{
		action:                action,
		redis:                 redis,
		rules:                 append([]Rule(nil), rules...),
		trustForwardedHeaders: options.TrustForwardedHeaders,
	}

	return limiter.handle
}

type rateLimiter struct {
	action                string
	redis                 RateLimitRedis
	rules                 []Rule
	trustForwardedHeaders bool

	mu  sync.Mutex
	sha string
}

func (l *rateLimiter) handle(c *gin.Context) {
	if l.redis == nil || len(l.rules) == 0 {
		c.Next()
		return
	}

	for _, rule := range l.rules {
		allowed, err := l.allow(c, rule)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, "rate limit unavailable"))
			return
		}
		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, response.Error(rateLimitExceededCode, rateLimitExceededMessage))
			return
		}
	}

	c.Next()
}

func (l *rateLimiter) allow(c *gin.Context, rule Rule) (bool, error) {
	sha, err := l.ensureScript(c.Request.Context(), false)
	if err != nil {
		return false, err
	}

	value, err := l.redis.EvalSHA(c.Request.Context(), sha, []string{l.key(c, rule)}, l.args(rule)...)
	if isNoScript(err) {
		sha, err = l.ensureScript(c.Request.Context(), true)
		if err != nil {
			return false, err
		}
		value, err = l.redis.EvalSHA(c.Request.Context(), sha, []string{l.key(c, rule)}, l.args(rule)...)
	}
	if err != nil {
		return false, err
	}

	return redisAllowed(value), nil
}

func (l *rateLimiter) ensureScript(ctx context.Context, reload bool) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.sha != "" && !reload {
		return l.sha, nil
	}

	script, err := loadRateLimitScript()
	if err != nil {
		return "", err
	}

	sha, err := l.redis.ScriptLoad(ctx, script)
	if err != nil {
		return "", err
	}
	l.sha = sha
	return sha, nil
}

func (l *rateLimiter) key(c *gin.Context, rule Rule) string {
	base := "ratelimit:{" + l.action + "}:"
	switch rule.Dimension {
	case DimensionIP:
		return base + "ip:" + clientIP(c, l.trustForwardedHeaders)
	case DimensionUser:
		return base + "user:" + userID(c)
	default:
		return base + "global"
	}
}

func (l *rateLimiter) args(rule Rule) []any {
	permits := rule.Permits
	if permits <= 0 {
		permits = defaultPermits
	}
	window := rule.Window
	if window <= 0 {
		window = time.Second
	}

	return []any{
		time.Now().UnixMilli(),
		permits,
		window.Milliseconds(),
		rule.Limit,
		requestID(),
	}
}

func clientIP(c *gin.Context, trustForwardedHeaders bool) string {
	if trustForwardedHeaders {
		if forwarded := c.GetHeader("X-Forwarded-For"); forwarded != "" {
			if first := strings.TrimSpace(strings.Split(forwarded, ",")[0]); first != "" {
				return first
			}
		}
		if realIP := strings.TrimSpace(c.GetHeader("X-Real-IP")); realIP != "" {
			return realIP
		}
	}
	if ip, _, err := net.SplitHostPort(c.Request.RemoteAddr); err == nil && ip != "" {
		return ip
	}
	if c.Request.RemoteAddr != "" {
		return c.Request.RemoteAddr
	}
	return "unknown"
}

func userID(c *gin.Context) string {
	if value, ok := c.Get("userId"); ok {
		switch typed := value.(type) {
		case string:
			if typed != "" {
				return typed
			}
		case int:
			return strconv.Itoa(typed)
		case int64:
			return strconv.FormatInt(typed, 10)
		}
	}
	if header := strings.TrimSpace(c.GetHeader("X-User-Id")); header != "" {
		return header
	}
	return "anonymous"
}

func redisAllowed(value any) bool {
	switch typed := value.(type) {
	case int64:
		return typed == 1
	case int:
		return typed == 1
	case string:
		return typed == "1"
	default:
		return false
	}
}

func isNoScript(err error) bool {
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "NOSCRIPT")
}

func requestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(b[:])
}

func loadRateLimitScript() (string, error) {
	relative := filepath.Join("internal", "infrastructure", "redis", "scripts", "rate_limit_single.lua")
	if data, err := os.ReadFile(relative); err == nil {
		return string(data), nil
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", os.ErrNotExist
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	data, err := os.ReadFile(filepath.Join(root, relative))
	return string(data), err
}

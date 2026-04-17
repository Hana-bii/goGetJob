package app

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"goGetJob/internal/common/config"
	"goGetJob/internal/common/middleware"
	"goGetJob/internal/common/response"
)

type Option func(*gin.Engine)

func WithRoutes(register func(*gin.Engine)) Option {
	return func(engine *gin.Engine) {
		if register != nil {
			register(engine)
		}
	}
}

func New(cfg *config.Config, log *slog.Logger, options ...Option) *gin.Engine {
	if cfg == nil {
		defaultCfg := config.Default()
		cfg = &defaultCfg
	}
	if log == nil {
		log = slog.Default()
	}

	switch strings.ToLower(cfg.App.Env) {
	case "prod", "production":
		gin.SetMode(gin.ReleaseMode)
	default:
		gin.SetMode(gin.DebugMode)
	}

	engine := gin.New()
	engine.Use(middleware.Recovery(log))
	engine.Use(middleware.CORS(cfg.CORS.AllowedOrigins))

	engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, response.Success(gin.H{
			"status": "ok",
		}))
	})
	for _, option := range options {
		if option != nil {
			option(engine)
		}
	}

	return engine
}

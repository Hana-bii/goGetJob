package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"goGetJob/internal/app"
	"goGetJob/internal/common/ai"
	"goGetJob/internal/common/config"
	"goGetJob/internal/common/logger"
	"goGetJob/internal/infrastructure/db"
	"goGetJob/internal/infrastructure/export"
	redisinfra "goGetJob/internal/infrastructure/redis"
	"goGetJob/internal/infrastructure/storage"
	"goGetJob/internal/modules/resume"
)

func main() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = filepath.Join("configs", "config.example.yaml")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(cfg.App.Env)
	resumeOption, cleanup, err := buildResumeModule(cfg, log)
	if err != nil {
		log.Error("initialize resume module", "error", err)
		os.Exit(1)
	}
	defer cleanup()

	engine := app.New(cfg, log, resumeOption)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Info("starting server", "addr", addr, "config_path", configPath)

	if err := engine.Run(addr); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func buildResumeModule(cfg *config.Config, log *slog.Logger) (app.Option, func(), error) {
	ctx := context.Background()
	var cleanup []func()

	var repo resume.Repository
	if cfg.Database.DSN != "" {
		database, err := db.Open(db.Options{DSN: cfg.Database.DSN})
		if err != nil {
			return nil, nil, err
		}
		sqlDB, err := database.DB()
		if err == nil {
			cleanup = append(cleanup, func() { _ = sqlDB.Close() })
		}
		gormRepo := resume.NewGormRepository(database)
		if err := gormRepo.AutoMigrate(); err != nil {
			return nil, nil, err
		}
		repo = gormRepo
	} else {
		log.Warn("DATABASE_DSN is empty; resume module uses in-memory repository")
		repo = resume.NewMemoryRepository()
	}

	objectStorage, err := storage.NewMinIOStorage(storage.MinIOOptions{
		Endpoint:  cfg.Storage.Endpoint,
		Bucket:    cfg.Storage.Bucket,
		AccessKey: cfg.Storage.AccessKey,
		SecretKey: cfg.Storage.SecretKey,
		Region:    cfg.Storage.Region,
	})
	if err != nil {
		return nil, nil, err
	}

	redisClient := redisinfra.New(redisinfra.Options{
		Addr: cfg.Redis.Addr,
		DB:   cfg.Redis.DB,
	})
	cleanup = append(cleanup, func() { _ = redisClient.Close() })

	model, err := ai.NewProviderRegistry(cfg.AI).Default()
	if err != nil {
		return nil, nil, err
	}
	analyzer := resume.NewAIAnalysisService(resume.AIAnalysisOptions{
		Model:        model,
		PromptLoader: ai.NewPromptLoader("internal/prompts"),
	})
	producer := resume.NewStreamAnalyzeProducer(redisClient)
	uploadService := resume.NewUploadService(resume.UploadServiceOptions{
		Repository: repo,
		Storage:    objectStorage,
		Producer:   producer,
	})
	historyService := resume.NewHistoryService(repo, export.NewPDFExporter(export.PDFOptions{}), objectStorage)
	handler := resume.NewHandler(uploadService, historyService)

	consumer := resume.NewAnalyzeConsumer(redisClient, repo, analyzer, "")
	go runResumeConsumer(ctx, log, consumer)

	return app.WithRoutes(func(engine *gin.Engine) {
			resume.RegisterRoutes(engine, handler)
		}), func() {
			for i := len(cleanup) - 1; i >= 0; i-- {
				cleanup[i]()
			}
		}, nil
}

type resumeConsumer interface {
	Run(context.Context) error
}

func runResumeConsumer(ctx context.Context, log *slog.Logger, consumer resumeConsumer) {
	backoff := time.Second
	for {
		if err := consumer.Run(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error("resume analyze consumer stopped; restarting", "error", err, "backoff", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		return
	}
}

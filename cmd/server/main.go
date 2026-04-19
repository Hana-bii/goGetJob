package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"goGetJob/internal/app"
	"goGetJob/internal/common/ai"
	"goGetJob/internal/common/config"
	"goGetJob/internal/common/evaluation"
	"goGetJob/internal/common/logger"
	"goGetJob/internal/infrastructure/db"
	"goGetJob/internal/infrastructure/export"
	redisinfra "goGetJob/internal/infrastructure/redis"
	"goGetJob/internal/infrastructure/storage"
	"goGetJob/internal/infrastructure/vector"
	"goGetJob/internal/modules/interview"
	interviewskill "goGetJob/internal/modules/interview/skill"
	"goGetJob/internal/modules/interviewschedule"
	"goGetJob/internal/modules/knowledgebase"
	"goGetJob/internal/modules/resume"
	"goGetJob/internal/modules/voiceinterview"
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
	interviewOption, interviewCleanup, err := buildInterviewModule(cfg, log)
	if err != nil {
		log.Error("initialize interview module", "error", err)
		os.Exit(1)
	}
	defer interviewCleanup()
	voiceOption, voiceCleanup, err := buildVoiceInterviewModule(cfg, log)
	if err != nil {
		log.Error("initialize voice interview module", "error", err)
		os.Exit(1)
	}
	defer voiceCleanup()
	scheduleOption, scheduleCleanup, err := buildInterviewScheduleModule(cfg, log)
	if err != nil {
		log.Error("initialize interview schedule module", "error", err)
		os.Exit(1)
	}
	defer scheduleCleanup()
	knowledgeOption, knowledgeCleanup, err := buildKnowledgeBaseModule(cfg, log)
	if err != nil {
		log.Error("initialize knowledge base module", "error", err)
		os.Exit(1)
	}
	defer knowledgeCleanup()

	engine := app.New(cfg, log, resumeOption, interviewOption, voiceOption, scheduleOption, knowledgeOption)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Info("starting server", "addr", addr, "config_path", configPath)

	if err := engine.Run(addr); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func buildKnowledgeBaseModule(cfg *config.Config, log *slog.Logger) (app.Option, func(), error) {
	ctx := context.Background()
	var cleanup []func()

	var repo knowledgebase.Repository
	var chatRepo knowledgebase.RagChatRepository
	var vectorStore vector.Store
	if cfg.Database.DSN != "" {
		database, err := db.Open(db.Options{DSN: cfg.Database.DSN})
		if err != nil {
			return nil, nil, err
		}
		sqlDB, err := database.DB()
		if err == nil {
			cleanup = append(cleanup, func() { _ = sqlDB.Close() })
		}
		gormRepo := knowledgebase.NewGormRepository(database)
		if err := gormRepo.AutoMigrate(); err != nil {
			return nil, nil, err
		}
		repo = gormRepo
		chatRepo = gormRepo
		vectorStore = vector.NewPGVectorStore(database)
	} else {
		log.Warn("DATABASE_DSN is empty; knowledge base module uses in-memory repository")
		memoryRepo := knowledgebase.NewMemoryRepository()
		repo = memoryRepo
		chatRepo = memoryRepo
		vectorStore = vector.NewMemoryStore()
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
	provider := cfg.AI.Providers[cfg.AI.DefaultProvider]
	vectorService := knowledgebase.NewVectorService(knowledgebase.VectorServiceOptions{
		Store:    vectorStore,
		Embedder: vector.NewOpenAIEmbedder(provider.BaseURL, provider.APIKey, provider.Model, nil),
	})
	queryService := knowledgebase.NewQueryService(knowledgebase.QueryServiceOptions{
		Repository:    repo,
		VectorService: vectorService,
		Model:         model,
		RewriteModel:  model,
		PromptLoader:  ai.NewPromptLoader("internal/prompts"),
		Config:        cfg.RAG,
	})
	producer := knowledgebase.NewStreamVectorizeProducer(redisClient)
	uploadService := knowledgebase.NewUploadService(knowledgebase.UploadServiceOptions{
		Repository: repo,
		Storage:    objectStorage,
		Producer:   producer,
	})
	services := knowledgebase.ServiceBundle{
		List:    knowledgebase.NewListService(repo),
		Upload:  uploadService,
		Delete:  knowledgebase.NewDeleteService(repo, objectStorage, vectorStore),
		Query:   queryService,
		RagChat: knowledgebase.NewRagChatService(repo, chatRepo, queryService),
		Storage: objectStorage,
	}
	handler := knowledgebase.NewHandler(services)

	consumer := knowledgebase.NewVectorizeConsumer(redisClient, repo, vectorService, "")
	go runConsumer(ctx, log, "knowledge base vectorize consumer", consumer)

	return app.WithRoutes(func(engine *gin.Engine) {
			knowledgebase.RegisterRoutes(engine, handler, redisClient)
		}), func() {
			for i := len(cleanup) - 1; i >= 0; i-- {
				cleanup[i]()
			}
		}, nil
}

func buildInterviewModule(cfg *config.Config, log *slog.Logger) (app.Option, func(), error) {
	ctx := context.Background()
	var cleanup []func()

	var repo interview.Repository
	if cfg.Database.DSN != "" {
		database, err := db.Open(db.Options{DSN: cfg.Database.DSN})
		if err != nil {
			return nil, nil, err
		}
		sqlDB, err := database.DB()
		if err == nil {
			cleanup = append(cleanup, func() { _ = sqlDB.Close() })
		}
		gormRepo := interview.NewGormRepository(database)
		if err := gormRepo.AutoMigrate(); err != nil {
			return nil, nil, err
		}
		repo = gormRepo
	} else {
		log.Warn("DATABASE_DSN is empty; interview module uses in-memory repository")
		repo = interview.NewMemoryRepository()
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
	skillService, err := interviewskill.NewService(interviewskill.Options{Root: "internal/skills"})
	if err != nil {
		return nil, nil, err
	}
	questionService := interview.NewQuestionService(interview.QuestionServiceOptions{
		Model:        model,
		ResumeModel:  model,
		SkillService: skillService,
		PromptLoader: ai.NewPromptLoader("internal/prompts"),
	})
	evaluator := interview.NewEvaluationService(evaluation.NewService(evaluation.Options{
		Model:        model,
		PromptLoader: ai.NewPromptLoader("internal/prompts"),
	}), skillService)
	producer := interview.NewStreamEvaluateProducer(redisClient)
	sessionService := interview.NewSessionService(interview.SessionServiceOptions{
		Repository:        repo,
		QuestionGenerator: questionService,
		EvaluateProducer:  producer,
	})
	historyService := interview.NewHistoryService(repo, export.NewPDFExporter(export.PDFOptions{}))
	handler := interview.NewHandler(sessionService, historyService, evaluator, evaluator)

	consumer := interview.NewEvaluateConsumer(redisClient, repo, evaluator, evaluator, "")
	go runConsumer(ctx, log, "interview evaluate consumer", consumer)

	return app.WithRoutes(func(engine *gin.Engine) {
			interview.RegisterRoutes(engine, handler, redisClient)
		}), func() {
			for i := len(cleanup) - 1; i >= 0; i-- {
				cleanup[i]()
			}
		}, nil
}

func buildVoiceInterviewModule(cfg *config.Config, log *slog.Logger) (app.Option, func(), error) {
	ctx := context.Background()
	var cleanup []func()

	var repo voiceinterview.Repository
	if cfg.Database.DSN != "" {
		database, err := db.Open(db.Options{DSN: cfg.Database.DSN})
		if err != nil {
			return nil, nil, err
		}
		sqlDB, err := database.DB()
		if err == nil {
			cleanup = append(cleanup, func() { _ = sqlDB.Close() })
		}
		gormRepo := voiceinterview.NewGormRepository(database)
		if err := gormRepo.AutoMigrate(); err != nil {
			return nil, nil, err
		}
		repo = gormRepo
	} else {
		log.Warn("DATABASE_DSN is empty; voice interview module uses in-memory repository")
		repo = voiceinterview.NewMemoryRepository()
	}

	redisClient := redisinfra.New(redisinfra.Options{
		Addr: cfg.Redis.Addr,
		DB:   cfg.Redis.DB,
	})
	cleanup = append(cleanup, func() { _ = redisClient.Close() })

	registry := ai.NewProviderRegistry(cfg.AI)
	model, err := registry.Get(cfg.Voice.LLMProvider)
	if err != nil {
		model, err = registry.Default()
		if err != nil {
			return nil, nil, err
		}
	}

	promptService := voiceinterview.NewPromptService(voiceinterview.PromptServiceOptions{
		Model:        model,
		PromptLoader: ai.NewPromptLoader("internal/prompts"),
	})
	evaluationService := voiceinterview.NewEvaluationService(repo, evaluation.NewService(evaluation.Options{
		Model:        model,
		PromptLoader: ai.NewPromptLoader("internal/prompts"),
		BatchSize:    cfg.Interview.Evaluation.BatchSize,
	}))
	producer := voiceinterview.NewStreamEvaluationProducer(redisClient)
	sessionService := voiceinterview.NewSessionService(voiceinterview.SessionServiceOptions{
		Repository:         repo,
		PromptGenerator:    promptService,
		EvaluationProducer: producer,
	})
	handler := voiceinterview.NewHandler(sessionService, evaluationService)
	wsHandler := voiceinterview.NewWebSocketHandler(
		sessionService,
		voiceinterview.NewDashScopeASR(cfg.Voice.ASR),
		voiceinterview.NewDashScopeTTS(cfg.Voice.TTS, nil),
		voiceinterview.NewLLMService(model, promptService),
	)

	consumer := voiceinterview.NewEvaluationConsumer(redisClient, repo, evaluationService, "")
	go runConsumer(ctx, log, "voice interview evaluate consumer", consumer)

	return app.WithRoutes(func(engine *gin.Engine) {
			voiceinterview.RegisterRoutes(engine, handler, wsHandler)
		}), func() {
			for i := len(cleanup) - 1; i >= 0; i-- {
				cleanup[i]()
			}
		}, nil
}

func buildInterviewScheduleModule(cfg *config.Config, log *slog.Logger) (app.Option, func(), error) {
	var wg sync.WaitGroup
	var closeDB func()

	var repo interviewschedule.Repository
	if cfg.Database.DSN != "" {
		database, err := db.Open(db.Options{DSN: cfg.Database.DSN})
		if err != nil {
			return nil, nil, err
		}
		sqlDB, err := database.DB()
		if err != nil {
			return nil, nil, err
		}
		gormRepo := interviewschedule.NewGormRepository(database)
		if err := gormRepo.AutoMigrate(); err != nil {
			return nil, nil, err
		}
		repo = gormRepo
		closeDB = func() { _ = sqlDB.Close() }
	} else {
		log.Warn("DATABASE_DSN is empty; interview schedule module uses in-memory repository")
		repo = interviewschedule.NewMemoryRepository()
	}

	model, err := ai.NewProviderRegistry(cfg.AI).Default()
	if err != nil {
		return nil, nil, err
	}

	service := interviewschedule.NewService(repo)
	parser := interviewschedule.NewParseService(model)
	handler := interviewschedule.NewHandler(service, parser)
	updater := interviewschedule.NewStatusUpdater(repo)
	ctx, cancel := context.WithCancel(context.Background())
	wg.Add(1)
	go func() {
		defer wg.Done()
		runScheduleUpdater(ctx, log, updater)
	}()
	lifecycle := scheduleModuleLifecycle{
		cancel:  cancel,
		wait:    wg.Wait,
		closeDB: closeDB,
	}

	return app.WithRoutes(func(engine *gin.Engine) {
		interviewschedule.RegisterRoutes(engine, handler)
	}), lifecycle.Shutdown, nil
}

type scheduleModuleLifecycle struct {
	cancel  context.CancelFunc
	wait    func()
	closeDB func()
}

func (l scheduleModuleLifecycle) Shutdown() {
	if l.cancel != nil {
		l.cancel()
	}
	if l.wait != nil {
		l.wait()
	}
	if l.closeDB != nil {
		l.closeDB()
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
	runConsumer(ctx, log, "resume analyze consumer", consumer)
}

type scheduleUpdater interface {
	Run(context.Context, time.Duration) error
}

func runScheduleUpdater(ctx context.Context, log *slog.Logger, updater scheduleUpdater) {
	if updater == nil {
		return
	}
	if err := updater.Run(ctx, time.Hour); err != nil && ctx.Err() == nil {
		log.Error("interview schedule updater stopped", "error", err)
	}
}

func runConsumer(ctx context.Context, log *slog.Logger, name string, consumer resumeConsumer) {
	backoff := time.Second
	for {
		if err := consumer.Run(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error(name+" stopped; restarting", "error", err, "backoff", backoff)
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

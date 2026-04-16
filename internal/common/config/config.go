package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	App       AppConfig       `yaml:"app"`
	Database  DatabaseConfig  `yaml:"database"`
	Redis     RedisConfig     `yaml:"redis"`
	Storage   StorageConfig   `yaml:"storage"`
	AI        AIConfig        `yaml:"ai"`
	RAG       RAGConfig       `yaml:"rag"`
	Interview InterviewConfig `yaml:"interview"`
	Voice     VoiceConfig     `yaml:"voice"`
	CORS      CORSConfig      `yaml:"cors"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type AppConfig struct {
	Env  string `yaml:"env"`
	Name string `yaml:"name"`
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

type RedisConfig struct {
	Addr string `yaml:"addr"`
	DB   int    `yaml:"db"`
}

type StorageConfig struct {
	Endpoint  string `yaml:"endpoint"`
	Bucket    string `yaml:"bucket"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	Region    string `yaml:"region"`
}

type AIConfig struct {
	DefaultProvider string                      `yaml:"default_provider"`
	Providers       map[string]AIProviderConfig `yaml:"providers"`
}

type AIProviderConfig struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
}

type RAGConfig struct {
	Rewrite RAGRewriteConfig `yaml:"rewrite"`
	Search  RAGSearchConfig  `yaml:"search"`
}

type RAGRewriteConfig struct {
	Enabled bool `yaml:"enabled"`
}

type RAGSearchConfig struct {
	ShortQueryLength int     `yaml:"short_query_length"`
	TopKShort        int     `yaml:"topk_short"`
	TopKMedium       int     `yaml:"topk_medium"`
	TopKLong         int     `yaml:"topk_long"`
	MinScoreShort    float64 `yaml:"min_score_short"`
	MinScoreDefault  float64 `yaml:"min_score_default"`
}

type InterviewConfig struct {
	FollowUpCount int                       `yaml:"follow_up_count"`
	Evaluation    InterviewEvaluationConfig `yaml:"evaluation"`
}

type InterviewEvaluationConfig struct {
	BatchSize int `yaml:"batch_size"`
}

type VoiceConfig struct {
	LLMProvider string         `yaml:"llm_provider"`
	TTS         VoiceTTSConfig `yaml:"tts"`
	ASR         VoiceASRConfig `yaml:"asr"`
}

type VoiceTTSConfig struct {
	DefaultVoice string  `yaml:"default_voice"`
	URL          string  `yaml:"url"`
	Model        string  `yaml:"model"`
	APIKey       string  `yaml:"api_key"`
	Voice        string  `yaml:"voice"`
	Format       string  `yaml:"format"`
	SampleRate   int     `yaml:"sample_rate"`
	Mode         string  `yaml:"mode"`
	LanguageType string  `yaml:"language_type"`
	SpeechRate   float64 `yaml:"speech_rate"`
	Volume       int     `yaml:"volume"`
}

type VoiceASRConfig struct {
	URL                        string  `yaml:"url"`
	Model                      string  `yaml:"model"`
	APIKey                     string  `yaml:"api_key"`
	Language                   string  `yaml:"language"`
	Format                     string  `yaml:"format"`
	SampleRate                 int     `yaml:"sample_rate"`
	EnableTurnDetection        bool    `yaml:"enable_turn_detection"`
	TurnDetectionType          string  `yaml:"turn_detection_type"`
	TurnDetectionThreshold     float64 `yaml:"turn_detection_threshold"`
	TurnDetectionSilenceMillis int     `yaml:"turn_detection_silence_duration_ms"`
}

type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
}

func (c *CORSConfig) UnmarshalYAML(value *yaml.Node) error {
	type rawCORSConfig struct {
		AllowedOrigins any `yaml:"allowed_origins"`
	}

	var raw rawCORSConfig
	if err := value.Decode(&raw); err != nil {
		return err
	}

	switch typed := raw.AllowedOrigins.(type) {
	case nil:
		c.AllowedOrigins = nil
	case string:
		c.AllowedOrigins = splitCSV(typed)
	case []any:
		origins := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				origins = append(origins, text)
			}
		}
		c.AllowedOrigins = origins
	case []string:
		c.AllowedOrigins = append([]string(nil), typed...)
	default:
		c.AllowedOrigins = splitCSV(fmt.Sprint(typed))
	}

	return nil
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Port: 8080,
		},
		App: AppConfig{
			Env:  "dev",
			Name: "goGetJob",
		},
		Database: DatabaseConfig{},
		Redis: RedisConfig{
			Addr: "localhost:6379",
			DB:   0,
		},
		Storage: StorageConfig{
			Endpoint:  "http://localhost:9000",
			Bucket:    "interview-guide",
			AccessKey: "minioadmin",
			SecretKey: "minioadmin",
			Region:    "us-east-1",
		},
		AI: AIConfig{
			DefaultProvider: "dashscope",
			Providers: map[string]AIProviderConfig{
				"dashscope": {
					BaseURL: "https://dashscope.aliyuncs.com/compatible-mode",
					APIKey:  "",
					Model:   "qwen-plus",
				},
				"lmstudio": {
					BaseURL: "http://localhost:1234",
					APIKey:  "lm-studio",
					Model:   "qwen2.5-7b-instruct",
				},
			},
		},
		RAG: RAGConfig{
			Rewrite: RAGRewriteConfig{
				Enabled: true,
			},
			Search: RAGSearchConfig{
				ShortQueryLength: 4,
				TopKShort:        20,
				TopKMedium:       12,
				TopKLong:         8,
				MinScoreShort:    0.18,
				MinScoreDefault:  0.28,
			},
		},
		Interview: InterviewConfig{
			FollowUpCount: 1,
			Evaluation: InterviewEvaluationConfig{
				BatchSize: 8,
			},
		},
		Voice: VoiceConfig{
			LLMProvider: "dashscope",
			TTS: VoiceTTSConfig{
				DefaultVoice: "siyue",
				URL:          "wss://dashscope.aliyuncs.com/api-ws/v1/realtime",
				Model:        "qwen3-tts-flash-realtime",
				Format:       "pcm",
				SampleRate:   24000,
				Mode:         "commit",
				LanguageType: "Chinese",
				SpeechRate:   1.0,
				Volume:       60,
				Voice:        "Cherry",
			},
			ASR: VoiceASRConfig{
				URL:                        "wss://dashscope.aliyuncs.com/api-ws/v1/realtime",
				Model:                      "qwen3-asr-flash-realtime",
				Language:                   "zh",
				Format:                     "pcm",
				SampleRate:                 16000,
				EnableTurnDetection:        true,
				TurnDetectionType:          "server_vad",
				TurnDetectionThreshold:     0,
				TurnDetectionSilenceMillis: 2000,
			},
		},
		CORS: CORSConfig{
			AllowedOrigins: []string{
				"http://localhost:5173",
				"http://localhost:5174",
				"http://localhost:80",
			},
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()

	if strings.TrimSpace(path) != "" {
		raw, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read config: %w", err)
		}
		if err == nil {
			expanded := expandPlaceholders(raw)
			if err := yaml.Unmarshal(expanded, &cfg); err != nil {
				return nil, fmt.Errorf("parse config: %w", err)
			}
		}
	}

	applyEnvOverrides(&cfg)
	normalize(&cfg)

	return &cfg, nil
}

var envPattern = regexp.MustCompile(`\$\{([A-Z0-9_]+)(?::([^}]*))?\}`)

func expandPlaceholders(raw []byte) []byte {
	return envPattern.ReplaceAllFunc(raw, func(match []byte) []byte {
		parts := envPattern.FindSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		key := string(parts[1])
		def := ""
		if len(parts[2]) > 0 {
			def = string(parts[2])
		}
		if value, ok := os.LookupEnv(key); ok {
			return []byte(value)
		}
		return []byte(def)
	})
}

func applyEnvOverrides(cfg *Config) {
	overrideInt("SERVER_PORT", &cfg.Server.Port)
	overrideString("APP_ENV", &cfg.App.Env)
	overrideString("APP_NAME", &cfg.App.Name)
	overrideString("DATABASE_DSN", &cfg.Database.DSN)
	overrideString("REDIS_ADDR", &cfg.Redis.Addr)
	overrideInt("REDIS_DB", &cfg.Redis.DB)
	overrideString("STORAGE_ENDPOINT", &cfg.Storage.Endpoint)
	overrideString("STORAGE_BUCKET", &cfg.Storage.Bucket)
	overrideString("STORAGE_ACCESS_KEY", &cfg.Storage.AccessKey)
	overrideString("STORAGE_SECRET_KEY", &cfg.Storage.SecretKey)
	overrideString("STORAGE_REGION", &cfg.Storage.Region)
	overrideString("AI_DEFAULT_PROVIDER", &cfg.AI.DefaultProvider)
	dashscope := cfg.AI.Providers["dashscope"]
	overrideString("AI_PROVIDER_DASHSCOPE_BASE_URL", &dashscope.BaseURL)
	overrideString("AI_PROVIDER_DASHSCOPE_API_KEY", &dashscope.APIKey)
	overrideString("AI_PROVIDER_DASHSCOPE_MODEL", &dashscope.Model)
	cfg.AI.Providers["dashscope"] = dashscope

	lmstudio := cfg.AI.Providers["lmstudio"]
	overrideString("AI_PROVIDER_LMSTUDIO_BASE_URL", &lmstudio.BaseURL)
	overrideString("AI_PROVIDER_LMSTUDIO_API_KEY", &lmstudio.APIKey)
	overrideString("AI_PROVIDER_LMSTUDIO_MODEL", &lmstudio.Model)
	cfg.AI.Providers["lmstudio"] = lmstudio
	overrideBool("RAG_REWRITE_ENABLED", &cfg.RAG.Rewrite.Enabled)
	overrideInt("RAG_SHORT_QUERY_LENGTH", &cfg.RAG.Search.ShortQueryLength)
	overrideInt("RAG_TOPK_SHORT", &cfg.RAG.Search.TopKShort)
	overrideInt("RAG_TOPK_MEDIUM", &cfg.RAG.Search.TopKMedium)
	overrideInt("RAG_TOPK_LONG", &cfg.RAG.Search.TopKLong)
	overrideFloat("RAG_MIN_SCORE_SHORT", &cfg.RAG.Search.MinScoreShort)
	overrideFloat("RAG_MIN_SCORE_DEFAULT", &cfg.RAG.Search.MinScoreDefault)
	overrideInt("INTERVIEW_FOLLOW_UP_COUNT", &cfg.Interview.FollowUpCount)
	overrideInt("INTERVIEW_EVALUATION_BATCH_SIZE", &cfg.Interview.Evaluation.BatchSize)
	overrideString("VOICE_LLM_PROVIDER", &cfg.Voice.LLMProvider)
	overrideString("VOICE_TTS_DEFAULT_VOICE", &cfg.Voice.TTS.DefaultVoice)
	overrideString("VOICE_ASR_URL", &cfg.Voice.ASR.URL)
	overrideString("VOICE_ASR_MODEL", &cfg.Voice.ASR.Model)
	overrideString("VOICE_ASR_API_KEY", &cfg.Voice.ASR.APIKey)
	overrideString("VOICE_ASR_LANGUAGE", &cfg.Voice.ASR.Language)
	overrideString("VOICE_ASR_FORMAT", &cfg.Voice.ASR.Format)
	overrideInt("VOICE_ASR_SAMPLE_RATE", &cfg.Voice.ASR.SampleRate)
	overrideBool("VOICE_ASR_ENABLE_TURN_DETECTION", &cfg.Voice.ASR.EnableTurnDetection)
	overrideString("VOICE_ASR_TURN_DETECTION_TYPE", &cfg.Voice.ASR.TurnDetectionType)
	overrideFloat("VOICE_ASR_TURN_DETECTION_THRESHOLD", &cfg.Voice.ASR.TurnDetectionThreshold)
	overrideInt("VOICE_ASR_TURN_DETECTION_SILENCE_MS", &cfg.Voice.ASR.TurnDetectionSilenceMillis)
	overrideString("VOICE_TTS_URL", &cfg.Voice.TTS.URL)
	overrideString("VOICE_TTS_MODEL", &cfg.Voice.TTS.Model)
	overrideString("VOICE_TTS_API_KEY", &cfg.Voice.TTS.APIKey)
	overrideString("VOICE_TTS_VOICE", &cfg.Voice.TTS.Voice)
	overrideString("VOICE_TTS_FORMAT", &cfg.Voice.TTS.Format)
	overrideInt("VOICE_TTS_SAMPLE_RATE", &cfg.Voice.TTS.SampleRate)
	overrideString("VOICE_TTS_MODE", &cfg.Voice.TTS.Mode)
	overrideString("VOICE_TTS_LANGUAGE_TYPE", &cfg.Voice.TTS.LanguageType)
	overrideFloat("VOICE_TTS_SPEECH_RATE", &cfg.Voice.TTS.SpeechRate)
	overrideInt("VOICE_TTS_VOLUME", &cfg.Voice.TTS.Volume)
	overrideStringSlice("CORS_ALLOWED_ORIGINS", &cfg.CORS.AllowedOrigins)
}

func normalize(cfg *Config) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.App.Env == "" {
		cfg.App.Env = "dev"
	}
	if cfg.App.Name == "" {
		cfg.App.Name = "goGetJob"
	}
	if cfg.AI.DefaultProvider == "" {
		cfg.AI.DefaultProvider = "dashscope"
	}
	if cfg.AI.Providers == nil {
		cfg.AI.Providers = map[string]AIProviderConfig{}
	}
	if _, ok := cfg.AI.Providers["dashscope"]; !ok {
		cfg.AI.Providers["dashscope"] = Default().AI.Providers["dashscope"]
	}
	if _, ok := cfg.AI.Providers["lmstudio"]; !ok {
		cfg.AI.Providers["lmstudio"] = Default().AI.Providers["lmstudio"]
	}
	if cfg.RAG.Search.ShortQueryLength == 0 {
		cfg.RAG.Search.ShortQueryLength = 4
	}
	if cfg.RAG.Search.TopKShort == 0 {
		cfg.RAG.Search.TopKShort = 20
	}
	if cfg.RAG.Search.TopKMedium == 0 {
		cfg.RAG.Search.TopKMedium = 12
	}
	if cfg.RAG.Search.TopKLong == 0 {
		cfg.RAG.Search.TopKLong = 8
	}
	if cfg.RAG.Search.MinScoreShort == 0 {
		cfg.RAG.Search.MinScoreShort = 0.18
	}
	if cfg.RAG.Search.MinScoreDefault == 0 {
		cfg.RAG.Search.MinScoreDefault = 0.28
	}
	if cfg.Interview.FollowUpCount == 0 {
		cfg.Interview.FollowUpCount = 1
	}
	if cfg.Interview.Evaluation.BatchSize == 0 {
		cfg.Interview.Evaluation.BatchSize = 8
	}
	if cfg.Voice.LLMProvider == "" {
		cfg.Voice.LLMProvider = "dashscope"
	}
	if cfg.Voice.TTS.DefaultVoice == "" {
		cfg.Voice.TTS.DefaultVoice = "siyue"
	}
	if cfg.Voice.ASR.URL == "" {
		cfg.Voice.ASR.URL = Default().Voice.ASR.URL
	}
	if cfg.Voice.ASR.Model == "" {
		cfg.Voice.ASR.Model = Default().Voice.ASR.Model
	}
	if cfg.Voice.ASR.Language == "" {
		cfg.Voice.ASR.Language = "zh"
	}
	if cfg.Voice.ASR.Format == "" {
		cfg.Voice.ASR.Format = "pcm"
	}
	if cfg.Voice.ASR.SampleRate == 0 {
		cfg.Voice.ASR.SampleRate = 16000
	}
	if cfg.Voice.ASR.TurnDetectionType == "" {
		cfg.Voice.ASR.TurnDetectionType = "server_vad"
	}
	if cfg.Voice.ASR.TurnDetectionSilenceMillis == 0 {
		cfg.Voice.ASR.TurnDetectionSilenceMillis = 2000
	}
	if cfg.Voice.TTS.URL == "" {
		cfg.Voice.TTS.URL = Default().Voice.TTS.URL
	}
	if cfg.Voice.TTS.Model == "" {
		cfg.Voice.TTS.Model = Default().Voice.TTS.Model
	}
	if cfg.Voice.TTS.Voice == "" {
		cfg.Voice.TTS.Voice = "Cherry"
	}
	if cfg.Voice.TTS.Format == "" {
		cfg.Voice.TTS.Format = "pcm"
	}
	if cfg.Voice.TTS.SampleRate == 0 {
		cfg.Voice.TTS.SampleRate = 24000
	}
	if cfg.Voice.TTS.Mode == "" {
		cfg.Voice.TTS.Mode = "commit"
	}
	if cfg.Voice.TTS.LanguageType == "" {
		cfg.Voice.TTS.LanguageType = "Chinese"
	}
	if cfg.Voice.TTS.SpeechRate == 0 {
		cfg.Voice.TTS.SpeechRate = 1.0
	}
	if cfg.Voice.TTS.Volume == 0 {
		cfg.Voice.TTS.Volume = 60
	}
	if len(cfg.CORS.AllowedOrigins) == 0 {
		cfg.CORS.AllowedOrigins = append([]string(nil), Default().CORS.AllowedOrigins...)
	}
}

func overrideString(key string, target *string) {
	if value, ok := os.LookupEnv(key); ok {
		*target = value
	}
}

func overrideInt(key string, target *int) {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.Atoi(value); err == nil {
			*target = parsed
		}
	}
}

func overrideBool(key string, target *bool) {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			*target = parsed
		}
	}
}

func overrideFloat(key string, target *float64) {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			*target = parsed
		}
	}
}

func overrideStringSlice(key string, target *[]string) {
	if value, ok := os.LookupEnv(key); ok {
		if strings.TrimSpace(value) == "" {
			*target = nil
			return
		}
		parts := strings.Split(value, ",")
		orig := (*target)[:0]
		for _, part := range parts {
			item := strings.TrimSpace(part)
			if item != "" {
				orig = append(orig, item)
			}
		}
		*target = orig
	}
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			origins = append(origins, item)
		}
	}
	return origins
}

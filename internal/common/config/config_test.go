package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"goGetJob/internal/common/config"
)

func TestLoadConfigUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("{}\n"), 0o600))

	cfg, err := config.Load(path)
	require.NoError(t, err)

	require.Equal(t, 8080, cfg.Server.Port)
	require.Equal(t, "dev", cfg.App.Env)
	require.Equal(t, "goGetJob", cfg.App.Name)
	require.Equal(t, "dashscope", cfg.AI.DefaultProvider)
	require.Equal(t, []string{
		"http://localhost:5173",
		"http://localhost:5174",
		"http://localhost:80",
	}, cfg.CORS.AllowedOrigins)
	require.Equal(t, 4, cfg.RAG.Search.ShortQueryLength)
	require.Equal(t, 1, cfg.Interview.FollowUpCount)
	require.Equal(t, 8, cfg.Interview.Evaluation.BatchSize)
	require.Equal(t, "dashscope", cfg.Voice.LLMProvider)
	require.Equal(t, "siyue", cfg.Voice.TTS.DefaultVoice)
}

func TestLoadConfigReturnsErrorForMissingExplicitFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")

	cfg, err := config.Load(path)

	require.Error(t, err)
	require.Nil(t, cfg)
}

func TestLoadConfigReturnsErrorForInvalidTypedEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  port: 8080\n"), 0o600))

	t.Setenv("SERVER_PORT", "abc")

	cfg, err := config.Load(path)

	require.Error(t, err)
	require.Nil(t, cfg)
}

func TestLoadConfigExpandsEnvironmentOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
server:
  port: 9090
app:
  env: yaml
  name: yaml-name
database:
  dsn: postgres://yaml
redis:
  addr: 127.0.0.1:6379
  db: 1
storage:
  endpoint: ${STORAGE_ENDPOINT:http://localhost:9000}
  bucket: yaml-bucket
  access_key: yaml-access
  secret_key: yaml-secret
ai:
  default_provider: dashscope
  providers:
    dashscope:
      base_url: https://dashscope.aliyuncs.com/compatible-mode
      api_key: yaml-api-key
      model: ${AI_PROVIDER_DASHSCOPE_MODEL:qwen-plus}
rag:
  rewrite:
    enabled: true
  search:
    short_query_length: 4
    topk_short: 20
    topk_medium: 12
    topk_long: 8
    min_score_short: 0.18
    min_score_default: 0.28
interview:
  follow_up_count: 1
  evaluation:
    batch_size: 8
voice:
  llm_provider: dashscope
  tts:
    default_voice: siyue
  asr:
    model: qwen3-asr-flash-realtime
cors:
  allowed_origins: http://localhost:5173
`), 0o600))

	t.Setenv("SERVER_PORT", "18080")
	t.Setenv("APP_ENV", "prod")
	t.Setenv("APP_NAME", "env-name")
	t.Setenv("DATABASE_DSN", "postgres://env")
	t.Setenv("REDIS_ADDR", "redis.example:6380")
	t.Setenv("REDIS_DB", "9")
	t.Setenv("STORAGE_BUCKET", "env-bucket")
	t.Setenv("AI_DEFAULT_PROVIDER", "custom")
	t.Setenv("AI_PROVIDER_DASHSCOPE_MODEL", "env-model")
	t.Setenv("INTERVIEW_FOLLOW_UP_COUNT", "3")
	t.Setenv("INTERVIEW_EVALUATION_BATCH_SIZE", "16")
	t.Setenv("VOICE_LLM_PROVIDER", "openai")
	t.Setenv("VOICE_TTS_DEFAULT_VOICE", "echo")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173")

	cfg, err := config.Load(path)
	require.NoError(t, err)

	require.Equal(t, 18080, cfg.Server.Port)
	require.Equal(t, "prod", cfg.App.Env)
	require.Equal(t, "env-name", cfg.App.Name)
	require.Equal(t, "postgres://env", cfg.Database.DSN)
	require.Equal(t, "redis.example:6380", cfg.Redis.Addr)
	require.Equal(t, 9, cfg.Redis.DB)
	require.Equal(t, "http://localhost:9000", cfg.Storage.Endpoint)
	require.Equal(t, "env-bucket", cfg.Storage.Bucket)
	require.Equal(t, "custom", cfg.AI.DefaultProvider)
	require.Equal(t, "env-model", cfg.AI.Providers["dashscope"].Model)
	require.Equal(t, 3, cfg.Interview.FollowUpCount)
	require.Equal(t, 16, cfg.Interview.Evaluation.BatchSize)
	require.Equal(t, "openai", cfg.Voice.LLMProvider)
	require.Equal(t, "echo", cfg.Voice.TTS.DefaultVoice)
	require.Equal(t, []string{"http://localhost:3000", "http://localhost:5173"}, cfg.CORS.AllowedOrigins)
}

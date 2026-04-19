# goGetJob

goGetJob is a practice interview platform for resumes, mock interviews, interview schedules, knowledge-base Q&A, and real-time voice interviews. The Go backend powers the data model, background jobs, object storage, RAG search, and voice session orchestration, while the React frontend provides the user experience.

## Structure

- `cmd/server`: application entrypoint and service wiring
- `internal/app`: Gin engine setup and route registration
- `internal/common`: shared config, AI helpers, evaluation, middleware, logging, and response helpers
- `internal/infrastructure`: Postgres, Redis, vector store, MinIO storage, PDF export, and file utilities
- `internal/modules`: feature modules for resume analysis, text interviews, interview scheduling, knowledge base / RAG, and real-time voice interviews
- `internal/prompts`: prompt templates loaded at runtime
- `internal/skills`: filesystem skill catalog used by the interview question generator
- `frontend`: Vite + React UI
- `docker`: container bootstrap assets such as Postgres init scripts

## Stack And Tradeoffs

- Go + Gin keeps the API layer simple and predictable.
- GORM + PostgreSQL + pgvector gives one persistence layer for both relational data and embeddings.
- Redis Streams decouple slow analysis and evaluation work from the HTTP request path.
- MinIO-compatible object storage keeps resume and knowledge-base file handling portable.
- Prompt and skill files live on disk so interview behavior can be tuned without touching code.
- The voice interview flow is fully implemented in Go with REST session APIs, WebSocket turn handling, ASR/TTS adapters, and async evaluation.

## Quick Start

1. Copy `.env.example` to `.env` and adjust keys or local service ports if needed.
2. Start the full stack:

```bash
docker compose up --build
```

3. Open the app:
- Frontend: `http://localhost:3000`
- Backend health: `http://localhost:8080/health`
- MinIO console: `http://localhost:9001`

For local backend development without Docker:

```bash
go run ./cmd/server
```

For local frontend development:

```bash
cd frontend
pnpm install
pnpm dev
```

## Environment Variables

The most important variables are listed below. The full sample set lives in `.env.example` and `configs/config.example.yaml`.

| Variable | Purpose |
| --- | --- |
| `CONFIG_PATH` | Path to the YAML config file used by the backend |
| `SERVER_PORT` | HTTP port for the Go server |
| `APP_ENV` | Runtime mode used for Gin and logging |
| `APP_NAME` | Application name in config |
| `DATABASE_DSN` | PostgreSQL connection string for pgvector and GORM |
| `REDIS_ADDR` / `REDIS_DB` | Redis connection settings for rate limits and async jobs |
| `STORAGE_ENDPOINT` / `STORAGE_BUCKET` | MinIO or other S3-compatible object storage settings |
| `STORAGE_ACCESS_KEY` / `STORAGE_SECRET_KEY` | Storage credentials |
| `AI_DEFAULT_PROVIDER` | Default AI provider selection |
| `AI_PROVIDER_DASHSCOPE_*` | DashScope-compatible text model settings |
| `AI_PROVIDER_LMSTUDIO_*` | Local LM Studio fallback settings |
| `VOICE_TTS_API_KEY` / `VOICE_ASR_API_KEY` | Realtime voice interview keys used by the voice section |
| `CORS_ALLOWED_ORIGINS` | Allowed browser origins for the API |
| `POSTGRES_*`, `REDIS_PORT`, `MINIO_*`, `BACKEND_PORT`, `FRONTEND_PORT` | Docker Compose runtime settings |

Frontend runtime variables live in `frontend/.env.example`:

- `VITE_API_BASE_URL`
- `VITE_WS_BASE_URL`

## Module Overview

### Resume

Implemented features:
- Resume upload
- Resume history and detail pages
- PDF export
- Reanalysis pipeline through Redis Streams

Status note:
- The backend route prefix is `/api/resume`.
- Some frontend callers still reference legacy pluralized `/api/resumes` paths, so that integration should be kept in mind when wiring the UI.

### Text Interview

Implemented features:
- Session creation and listing
- Question generation from resume and skill catalog data
- Answer submission, save/continue, completion, report generation, details, export, and delete
- Redis-backed rate limiting on the high-traffic actions when Redis is configured
- Async evaluation via Redis Streams

### Interview Schedule

Implemented features:
- JD parsing
- Schedule CRUD
- Status updates
- Periodic status updater loop

### Knowledge Base And RAG

Implemented features:
- Knowledge-base file upload and listing
- Category management
- Search and stats
- RAG query and streaming query
- RAG chat sessions and message streaming
- Async vectorization via Redis Streams
- pgvector-backed embeddings when PostgreSQL is configured

### Voice Interview

Implemented features:
- Voice interview session create/list/get/pause/resume/end/delete APIs
- Voice dialogue history and evaluation polling APIs
- Real-time `/ws/voice-interview/:sessionId` message loop for subtitles, text streaming, and audio responses
- ASR append-retry behavior for closed upstream sessions
- Sentence-level TTS synthesis, ordered `audio_chunk` events, and merged `audio` payload
- Redis Stream based async voice evaluation pipeline

## Engineering Stories

The full write-up for the five required topics lives in [`docs/engineering-stories.md`](docs/engineering-stories.md).

- Mock interview: skill-catalog-driven question allocation and prompt tuning
- RAG: pgvector-backed retrieval with query rewrite and MinIO storage
- Redis Stream async tasks: non-blocking analysis, evaluation, and vectorization
- Redis + Lua rate limiting: atomic protection for expensive endpoints
- Real-time voice interview: full backend implementation with WebSocket orchestration, ASR/TTS integration, and async evaluation

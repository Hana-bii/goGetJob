# Go Eino Gin Interview Guide Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go + Eino + Gin version of the original Java interview guide, including resume analysis, mock interview, RAG knowledge base, Redis Stream async tasks, Redis + Lua rate limiting, schedule management, and full real-time voice interview.

**Architecture:** Use a modular Go monolith: Gin handlers call services, services call repositories and infrastructure interfaces, GORM owns PostgreSQL persistence, go-redis owns Redis/Lua/Streams, Eino owns LLM/RAG orchestration, pgvector stores embeddings, S3-compatible storage keeps uploads, and React/Vite remains the frontend with stable layout.

**Tech Stack:** Go 1.22+, Gin, Eino, GORM, PostgreSQL + pgvector, go-redis, MinIO Go SDK, WebSocket, React, TypeScript, Vite, Docker Compose.

---

## Global Rules

- Work branch-by-branch from `master`, commit each completed task, and push the branch.
- Write failing tests before production code for every behavior-bearing package.
- Do not edit the original Java project under `../interview-guide`.
- Copy prompt and skill content verbatim unless a path or renderer adaptation is required.
- Keep frontend layout stable; only adjust styling and API/WebSocket integration.
- Production paths must use API-based LLM, embedding, ASR, and TTS services.
- Voice interview must be real and usable: REST lifecycle, WebSocket audio, ASR, TTS, LLM streaming, echo protection, pause/resume, persistence, and async evaluation.

## Target Structure

```text
cmd/server/
configs/
internal/app/
internal/common/{ai,async,config,errors,evaluation,logger,middleware,model,response}
internal/infrastructure/{db,export,file,redis,storage,vector}
internal/modules/{resume,interview,knowledgebase,interviewschedule,voiceinterview}
internal/prompts/
internal/skills/
frontend/
docker/
docs/
```

## Standard Verification

```powershell
go fmt ./...
go test ./...
go vet ./...
docker compose config
```

Frontend verification:

```powershell
Set-Location frontend
npm install
npm run lint
npm run build
Set-Location ..
```

## Task 1: Bootstrap Go Gin Service

**Branch:** `feature/bootstrap-go-gin`

**Files:**
- Create: `go.mod`, `.gitignore`, `.env.example`, `configs/config.example.yaml`
- Create: `cmd/server/main.go`, `internal/app/app.go`
- Create: `internal/common/config/config.go`
- Create: `internal/common/logger/logger.go`
- Create: `internal/common/response/result.go`
- Create: `internal/common/errors/error_code.go`
- Create: `internal/common/errors/business_error.go`
- Create: `internal/common/middleware/recovery.go`
- Create: `internal/common/middleware/cors.go`
- Test: `internal/common/config/config_test.go`
- Test: `internal/common/response/result_test.go`
- Test: `internal/common/errors/business_error_test.go`

- [ ] **Step 1: Create branch**

```powershell
git switch master
git pull --ff-only
git switch -c feature/bootstrap-go-gin
```

- [ ] **Step 2: Write failing tests**

Tests must assert config defaults and env overrides, `Result` JSON shape, and business error code/message behavior.

```go
func TestSuccessResultUsesJavaCompatibleShape(t *testing.T) {
  got := response.Success(map[string]string{"id": "1"})
  require.Equal(t, 0, got.Code)
  require.Equal(t, "success", got.Message)
}
```

- [ ] **Step 3: Run tests and confirm RED**

```powershell
go test ./internal/common/... -v
```

Expected: fail because packages do not exist yet.

- [ ] **Step 4: Implement minimal bootstrap**

Implement typed config loading, logger setup, `Result[T]`, `BusinessError`, global recovery middleware, CORS middleware, `/health`, and Gin startup.

- [ ] **Step 5: Verify GREEN**

```powershell
go fmt ./...
go test ./...
go vet ./...
```

- [ ] **Step 6: Commit and push**

```powershell
git add .
git commit -m "feat: bootstrap Go Gin service"
git push -u origin feature/bootstrap-go-gin
```

## Task 2: Common Infrastructure

**Branch:** `feature/common-infra`

**Files:**
- Create: `internal/infrastructure/db/db.go`, `internal/infrastructure/db/transaction.go`
- Create: `internal/infrastructure/redis/client.go`
- Create: `internal/infrastructure/storage/storage.go`, `internal/infrastructure/storage/minio_storage.go`
- Create: `internal/common/ai/provider_registry.go`
- Create: `internal/common/ai/prompt_loader.go`
- Create: `internal/common/ai/structured_invoker.go`
- Test: `internal/common/ai/prompt_loader_test.go`
- Test: `internal/common/ai/structured_invoker_test.go`

- [ ] **Step 1: Create branch**

```powershell
git switch master
git pull --ff-only
git switch -c feature/common-infra
```

- [ ] **Step 2: Write failing tests**

Test prompt loading from `internal/prompts`, missing prompt errors, structured output retry after invalid JSON, and repair prompt content.

- [ ] **Step 3: Copy prompts and skills**

Copy Java resources:

```text
../interview-guide/app/src/main/resources/prompts -> internal/prompts
../interview-guide/app/src/main/resources/skills -> internal/skills
```

- [ ] **Step 4: Implement infrastructure**

Implement GORM connection, transaction helper, go-redis wrapper, S3-compatible storage interface, Eino/OpenAI-compatible provider registry, prompt renderer, and structured JSON output invoker.

- [ ] **Step 5: Verify and commit**

```powershell
go fmt ./...
go test ./internal/common/... ./internal/infrastructure/...
go vet ./...
git add .
git commit -m "feat: add common infrastructure"
git push -u origin feature/common-infra
```

## Task 3: Redis Lua Rate Limit and Redis Stream Templates

**Branch:** `feature/rate-limit-stream`

**Files:**
- Create: `internal/infrastructure/redis/scripts/rate_limit_single.lua`
- Create: `internal/common/middleware/ratelimit.go`
- Create: `internal/common/async/constants.go`
- Create: `internal/common/async/producer.go`
- Create: `internal/common/async/consumer.go`
- Test: `internal/common/middleware/ratelimit_test.go`
- Test: `internal/common/async/consumer_test.go`

- [ ] **Step 1: Create branch**

```powershell
git switch master
git pull --ff-only
git switch -c feature/rate-limit-stream
```

- [ ] **Step 2: Write failing tests**

Test GLOBAL/IP/USER key generation, secure default IP extraction from Gin/client remote IP, trusted forwarded-header parsing (`X-Forwarded-For` first IP then `X-Real-IP` when explicitly enabled), multi-rule rejection, malformed Stream message ACK, success transition, retry transition, and failed-after-max transition.

- [ ] **Step 3: Implement**

Copy the original Lua sliding-window script unchanged. Implement Gin rate-limit middleware with EvalSha and NOSCRIPT reload. IP dimensions must use Gin/client remote IP by default; Java-compatible `X-Forwarded-For`/`X-Real-IP` extraction is enabled only through explicit trusted-forwarded-header configuration. Implement generic Stream producer/consumer with `markProcessing`, `processBusiness`, `markCompleted`, retry, `markFailed`, and ACK.

- [ ] **Step 4: Verify and commit**

```powershell
go fmt ./...
go test ./internal/common/middleware ./internal/common/async ./internal/infrastructure/redis
go vet ./...
git add .
git commit -m "feat: add Redis rate limiting and stream templates"
git push -u origin feature/rate-limit-stream
```

## Task 4: File Parsing and PDF Export Foundation

**Branch:** `feature/file-export-foundation`

**Files:**
- Create: `internal/infrastructure/file/hash.go`
- Create: `internal/infrastructure/file/validation.go`
- Create: `internal/infrastructure/file/cleaning.go`
- Create: `internal/infrastructure/file/parser.go`
- Create: `internal/infrastructure/export/pdf.go`
- Create: `internal/common/model/async_status.go`
- Test: `internal/infrastructure/file/parser_test.go`
- Test: `internal/infrastructure/export/pdf_test.go`

- [ ] **Step 1: Create branch**

```powershell
git switch master
git pull --ff-only
git switch -c feature/file-export-foundation
```

- [ ] **Step 2: Write failing tests**

Test TXT/MD/DOCX/PDF extraction, unsupported legacy DOC error, MIME validation, file size validation, stable SHA-256 hash, and PDF bytes beginning with `%PDF`.

- [ ] **Step 3: Implement**

Implement parser chain, text cleaning, file validation, hash service, async status enum, and Chinese-capable PDF export interface.

- [ ] **Step 4: Verify and commit**

```powershell
go fmt ./...
go test ./internal/infrastructure/file ./internal/infrastructure/export
go vet ./...
git add .
git commit -m "feat: add file parsing and PDF export foundation"
git push -u origin feature/file-export-foundation
```

## Task 5: Resume Module

**Branch:** `feature/resume-module`

**Files:**
- Create: `internal/modules/resume/model.go`
- Create: `internal/modules/resume/repository.go`
- Create: `internal/modules/resume/service_upload.go`
- Create: `internal/modules/resume/service_analysis.go`
- Create: `internal/modules/resume/service_history.go`
- Create: `internal/modules/resume/handler.go`
- Create: `internal/modules/resume/stream.go`
- Test: `internal/modules/resume/service_upload_test.go`
- Test: `internal/modules/resume/service_analysis_test.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Create branch**

```powershell
git switch master
git pull --ff-only
git switch -c feature/resume-module
```

- [ ] **Step 2: Write failing tests**

Test invalid type, duplicate hash, new upload status `PENDING`, storage upload, Stream enqueue, analysis status transitions, structured output parsing, and failure status/error truncation.

- [ ] **Step 3: Implement**

Implement resume entity, repository, upload service, AI analysis service, history/detail/delete/export APIs, and Redis Stream analyze producer/consumer.

Routes:

```text
POST /api/resume/upload
GET /api/resume/history
GET /api/resume/{id}
DELETE /api/resume/{id}
POST /api/resume/{id}/reanalyze
GET /api/resume/{id}/export
```

- [ ] **Step 4: Verify and commit**

```powershell
go fmt ./...
go test ./internal/modules/resume ./internal/common/async
go vet ./...
git add .
git commit -m "feat: add resume upload and async analysis"
git push -u origin feature/resume-module
```

## Task 6: Interview Skill and Unified Evaluation

**Branch:** `feature/interview-evaluation-foundation`

**Files:**
- Create: `internal/modules/interview/skill/model.go`
- Create: `internal/modules/interview/skill/service.go`
- Create: `internal/common/evaluation/model.go`
- Create: `internal/common/evaluation/service.go`
- Test: `internal/modules/interview/skill/service_test.go`
- Test: `internal/common/evaluation/service_test.go`

- [ ] **Step 1: Create branch**

```powershell
git switch master
git pull --ff-only
git switch -c feature/interview-evaluation-foundation
```

- [ ] **Step 2: Write failing tests**

Test skill metadata loading, category allocation, custom skill creation, safe reference section, batch evaluation split, zero-score fallback, summary fallback, category averages, and reference answer preservation.

- [ ] **Step 3: Implement**

Implement copied skill loader and the shared evaluation service used by text and voice interview. Preserve Java logic: batch evaluation, structured output, merge, second summary, and fallback.

- [ ] **Step 4: Verify and commit**

```powershell
go fmt ./...
go test ./internal/modules/interview/skill ./internal/common/evaluation
go vet ./...
git add .
git commit -m "feat: add interview skill and evaluation foundation"
git push -u origin feature/interview-evaluation-foundation
```

## Task 7: Text Mock Interview Module

**Branch:** `feature/interview-module`

**Files:**
- Create: `internal/modules/interview/model.go`
- Create: `internal/modules/interview/repository.go`
- Create: `internal/modules/interview/service_question.go`
- Create: `internal/modules/interview/service_session.go`
- Create: `internal/modules/interview/service_history.go`
- Create: `internal/modules/interview/handler.go`
- Create: `internal/modules/interview/stream.go`
- Test: `internal/modules/interview/service_question_test.go`
- Test: `internal/modules/interview/service_session_test.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Create branch**

```powershell
git switch master
git pull --ff-only
git switch -c feature/interview-module
```

- [ ] **Step 2: Write failing tests**

Test skill-only generation, resume+direction parallel generation, 60/40 merge, historical dedup prompt section, follow-up cap, fallback questions, answer save vs submit, complete, details, report, and PDF export.

- [ ] **Step 3: Implement**

Implement session, question, answer, history, report, export, and async evaluation flows.

Routes:

```text
GET /api/interview/sessions
POST /api/interview/sessions
GET /api/interview/sessions/{sessionId}
GET /api/interview/sessions/{sessionId}/question
POST /api/interview/sessions/{sessionId}/answers
PUT /api/interview/sessions/{sessionId}/answers
POST /api/interview/sessions/{sessionId}/complete
GET /api/interview/sessions/{sessionId}/report
GET /api/interview/sessions/{sessionId}/details
GET /api/interview/sessions/{sessionId}/export
DELETE /api/interview/sessions/{sessionId}
```

- [ ] **Step 4: Add rate limits**

Create session: GLOBAL 5 and IP 5. Submit answer: GLOBAL 10.

- [ ] **Step 5: Verify and commit**

```powershell
go fmt ./...
go test ./internal/modules/interview ./internal/common/evaluation
go vet ./...
git add .
git commit -m "feat: add text mock interview module"
git push -u origin feature/interview-module
```

## Task 8: Knowledge Base RAG Module

**Branch:** `feature/knowledgebase-rag`

**Files:**
- Create: `internal/modules/knowledgebase/model.go`
- Create: `internal/modules/knowledgebase/repository.go`
- Create: `internal/modules/knowledgebase/service_upload.go`
- Create: `internal/modules/knowledgebase/service_vector.go`
- Create: `internal/modules/knowledgebase/service_query.go`
- Create: `internal/modules/knowledgebase/service_list.go`
- Create: `internal/modules/knowledgebase/service_delete.go`
- Create: `internal/modules/knowledgebase/service_rag_chat.go`
- Create: `internal/modules/knowledgebase/handler.go`
- Create: `internal/modules/knowledgebase/stream.go`
- Create: `internal/infrastructure/vector/pgvector.go`
- Test: `internal/modules/knowledgebase/service_query_test.go`
- Test: `internal/modules/knowledgebase/service_vector_test.go`
- Test: `internal/infrastructure/vector/pgvector_test.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Create branch**

```powershell
git switch master
git pull --ff-only
git switch -c feature/knowledgebase-rag
```

- [ ] **Step 2: Write failing tests**

Test query rewrite fallback, candidate query order, dynamic topK/minScore, no-result normalization, vector delete-before-insert, batch embedding limit, `kb_id` metadata, DB filter, local filter fallback, Stream status transitions, and SSE output.

- [ ] **Step 3: Implement**

Implement upload, duplicate detection, parsing, storage, vectorization Stream, pgvector retrieval, RAG query, RAG chat sessions/messages, categories, stats, download, delete, search, and manual revectorization.

- [ ] **Step 4: Add rate limits**

Query: GLOBAL 10/IP 10. Stream query: GLOBAL 5/IP 5. Upload: GLOBAL 3/IP 3. Revectorize: GLOBAL 2/IP 2.

- [ ] **Step 5: Verify and commit**

```powershell
go fmt ./...
go test ./internal/modules/knowledgebase ./internal/infrastructure/vector
go vet ./...
git add .
git commit -m "feat: add knowledge base RAG module"
git push -u origin feature/knowledgebase-rag
```

## Task 9: Full Voice Interview Module

**Branch:** `feature/voice-interview`

**Files:**
- Create: `internal/modules/voiceinterview/model.go`
- Create: `internal/modules/voiceinterview/repository.go`
- Create: `internal/modules/voiceinterview/service_session.go`
- Create: `internal/modules/voiceinterview/service_prompt.go`
- Create: `internal/modules/voiceinterview/service_llm.go`
- Create: `internal/modules/voiceinterview/service_evaluation.go`
- Create: `internal/modules/voiceinterview/asr.go`
- Create: `internal/modules/voiceinterview/asr_dashscope.go`
- Create: `internal/modules/voiceinterview/tts.go`
- Create: `internal/modules/voiceinterview/tts_dashscope.go`
- Create: `internal/modules/voiceinterview/websocket.go`
- Create: `internal/modules/voiceinterview/stream.go`
- Create: `internal/modules/voiceinterview/handler.go`
- Create: `internal/modules/voiceinterview/audio.go`
- Test: `internal/modules/voiceinterview/audio_test.go`
- Test: `internal/modules/voiceinterview/websocket_test.go`
- Test: `internal/modules/voiceinterview/service_session_test.go`
- Test: `internal/modules/voiceinterview/service_prompt_test.go`
- Test: `internal/modules/voiceinterview/service_evaluation_test.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Create branch**

```powershell
git switch master
git pull --ff-only
git switch -c feature/voice-interview
```

- [ ] **Step 2: Write failing tests**

Test PCM-to-WAV header, create/pause/resume/end/delete session, prompt generation with skill and resume, ASR partial/final handling, merge-buffer submit behavior, echo protection, ASR restart after append failure, LLM stream text push, sentence-level TTS, message persistence, timeout pause, disconnect cleanup, empty evaluation, and Stream evaluation status changes.

- [ ] **Step 3: Implement session and REST APIs**

Routes:

```text
POST /api/voice-interview/sessions
GET /api/voice-interview/sessions/{sessionId}
POST /api/voice-interview/sessions/{sessionId}/end
PUT /api/voice-interview/sessions/{sessionId}/pause
PUT /api/voice-interview/sessions/{sessionId}/resume
GET /api/voice-interview/sessions
DELETE /api/voice-interview/sessions/{sessionId}
GET /api/voice-interview/sessions/{sessionId}/messages
GET /api/voice-interview/sessions/{sessionId}/evaluation
POST /api/voice-interview/sessions/{sessionId}/evaluation
```

- [ ] **Step 4: Implement real-time WebSocket**

Register `GET /ws/voice-interview/{sessionId}`. Support client `audio`, `control submit`, `end_interview`, and `start_phase`. Send server `welcome`, `subtitle`, `text`, `audio`, `audio_chunk`, and `error`.

- [ ] **Step 5: Implement ASR/TTS API adapters**

ASR: DashScope Qwen realtime WebSocket, PCM 16kHz, Chinese, server VAD, partial/final/error callbacks, restart support.

TTS: DashScope Qwen realtime WebSocket, PCM 24kHz, commit mode, delta collection, timeout, close cleanup.

- [ ] **Step 6: Implement LLM voice pipeline**

Use Eino provider registry voice model. Stream tokens, throttle frontend text, detect sentence boundaries, run per-sentence TTS with concurrency limit, merge or chunk WAV audio, save dialogue, and set AI speaking cooldown.

- [ ] **Step 7: Implement async evaluation**

Convert saved dialogue to QA records, infer categories, reuse unified evaluation, save JSON report fields, and update Stream status.

- [ ] **Step 8: Verify and commit**

```powershell
go fmt ./...
go test ./internal/modules/voiceinterview ./internal/common/evaluation
go test -race ./internal/modules/voiceinterview
go vet ./...
git add .
git commit -m "feat: add real-time voice interview"
git push -u origin feature/voice-interview
```

## Task 10: Interview Schedule Module

**Branch:** `feature/schedule-module`

**Files:**
- Create: `internal/modules/interviewschedule/model.go`
- Create: `internal/modules/interviewschedule/repository.go`
- Create: `internal/modules/interviewschedule/service.go`
- Create: `internal/modules/interviewschedule/service_parse.go`
- Create: `internal/modules/interviewschedule/status_updater.go`
- Create: `internal/modules/interviewschedule/handler.go`
- Test: `internal/modules/interviewschedule/service_parse_test.go`
- Test: `internal/modules/interviewschedule/service_test.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Create branch**

```powershell
git switch master
git pull --ff-only
git switch -c feature/schedule-module
```

- [ ] **Step 2: Write failing tests**

Test CRUD, invite parsing by regex, AI structured fallback, company/position/time/link extraction, status update, and expired schedule transition.

- [ ] **Step 3: Implement**

Implement schedule repository, service, parse service, status updater, and routes matching original frontend API.

- [ ] **Step 4: Verify and commit**

```powershell
go fmt ./...
go test ./internal/modules/interviewschedule
go vet ./...
git add .
git commit -m "feat: add interview schedule module"
git push -u origin feature/schedule-module
```

## Task 11: Frontend Migration

**Branch:** `feature/frontend-migration`

**Files:**
- Copy/Create: `frontend/**`
- Modify: `frontend/src/api/request.ts`
- Modify: `frontend/src/api/voiceInterview.ts`
- Modify: `frontend/src/index.css`
- Modify: `frontend/vite.config.ts`
- Create: `frontend/.env.example`

- [ ] **Step 1: Create branch**

```powershell
git switch master
git pull --ff-only
git switch -c feature/frontend-migration
```

- [ ] **Step 2: Copy frontend and adapt API config**

Copy original frontend. Set `VITE_API_BASE_URL` and `VITE_WS_BASE_URL`. Use returned `webSocketUrl` for voice interview when available.

- [ ] **Step 3: Style refresh without layout rewrite**

Keep existing pages and component hierarchy. Adjust spacing, color, typography, and borders only.

- [ ] **Step 4: Verify and commit**

```powershell
Set-Location frontend
npm install
npm run lint
npm run build
Set-Location ..
git add .
git commit -m "feat: migrate frontend"
git push -u origin feature/frontend-migration
```

## Task 12: Docker and Final Documentation

**Branch:** `feature/docs-and-deployment`

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`
- Create: `docker/postgres/init.sql`
- Create: `README.md`
- Create: `docs/architecture.md`
- Create: `docs/engineering-stories.md`
- Create: `docs/api.md`
- Modify: `.env.example`
- Modify: `configs/config.example.yaml`

- [ ] **Step 1: Create branch**

```powershell
git switch master
git pull --ff-only
git switch -c feature/docs-and-deployment
```

- [ ] **Step 2: Implement deployment files**

Compose services: backend, frontend, PostgreSQL with pgvector, Redis, MinIO/RustFS-compatible storage, and bucket initializer. PostgreSQL init must run `CREATE EXTENSION IF NOT EXISTS vector;`.

- [ ] **Step 3: Write project documentation**

README must include project introduction, structure, tech stack overview, selection tradeoffs, quick start, env vars, module overview, and interview-ready engineering stories.

Engineering stories must cover:

- mock interview
- RAG
- Redis Stream async tasks
- Redis + Lua multidimensional rate limiting
- real-time voice interview

Each story must follow: problem phenomenon, initial version, root cause, options compared, final solution, effect evaluation.

- [ ] **Step 4: Verify and commit**

```powershell
docker compose config
go test ./...
git add .
git commit -m "docs: add deployment and project documentation"
git push -u origin feature/docs-and-deployment
```

## Task 13: Integration and Final Verification

**Branch:** `integration/full-go-port`

**Files:** integrate all prior task files.

- [ ] **Step 1: Create integration branch**

```powershell
git switch master
git pull --ff-only
git switch -c integration/full-go-port
```

- [ ] **Step 2: Merge feature branches in dependency order**

```powershell
git merge --no-ff feature/bootstrap-go-gin
git merge --no-ff feature/common-infra
git merge --no-ff feature/rate-limit-stream
git merge --no-ff feature/file-export-foundation
git merge --no-ff feature/resume-module
git merge --no-ff feature/interview-evaluation-foundation
git merge --no-ff feature/interview-module
git merge --no-ff feature/knowledgebase-rag
git merge --no-ff feature/voice-interview
git merge --no-ff feature/schedule-module
git merge --no-ff feature/frontend-migration
git merge --no-ff feature/docs-and-deployment
```

- [ ] **Step 3: Run full verification**

```powershell
go fmt ./...
go test ./...
go test -race ./internal/common/... ./internal/modules/voiceinterview/...
go vet ./...
docker compose config
```

Then run frontend build and manual smoke checks:

- `GET /health`
- resume upload returns pending analysis
- knowledge base upload returns pending vectorization
- text interview session can be created
- voice interview session returns WebSocket URL
- voice WebSocket can exchange audio/text with configured APIs

- [ ] **Step 4: Merge to master and push**

```powershell
git add .
git commit -m "chore: integrate full Go port"
git push -u origin integration/full-go-port
git switch master
git merge --no-ff integration/full-go-port
git push origin master
```

## Self-Review

- [x] Approved design requirements map to tasks.
- [x] Voice interview is full real-time ASR/TTS/WebSocket, not a stub.
- [x] Redis Stream covers resume analysis, vectorization, interview evaluation, and voice evaluation.
- [x] Redis + Lua keeps GLOBAL/IP/USER multidimensional limits.
- [x] RAG includes rewrite, dynamic retrieval parameters, fallback filtering, and streaming no-result normalization.
- [x] Frontend migration keeps layout stable.
- [x] README and docs include engineering-story requirements.
- [x] Each branch has test-first steps, verification commands, commit, and push.


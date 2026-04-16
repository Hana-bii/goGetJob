# Go Eino Gin Interview Guide Design

## Goal

Build a Go version of the original Spring Boot + Java + Spring AI interview guide
project in the `goGetJob` repository. The Go version should preserve the original
product behavior, module boundaries, middleware choices, prompt content, and
engineering story while replacing the Java-specific framework layer with Go,
Eino, and Gin.

The target system must include a complete voice interview implementation with
usable ASR, TTS, WebSocket real-time dialogue, conversation persistence, timeout
handling, echo protection, and asynchronous evaluation.

## Scope

The Go version will implement these original modules:

- Resume management: upload, parsing, deduplication, storage, asynchronous AI
  analysis, history, detail, deletion, and report export.
- Mock interview: skill-driven question generation, resume-aware questions,
  historical question deduplication, follow-up questions, answer submission,
  report generation, and PDF export.
- Knowledge base: upload, parsing, deduplication, asynchronous vectorization,
  pgvector retrieval, RAG query, RAG chat history, SSE streaming, categories,
  stats, download, delete, and manual revectorization.
- Interview schedule: calendar management, invitation parsing, status updates,
  and CRUD APIs.
- Voice interview: REST session management, WebSocket audio control channel,
  DashScope-compatible ASR/TTS real-time APIs, LLM interview response generation,
  subtitle push, AI audio push, pause/resume, auto timeout, message persistence,
  and Redis Stream asynchronous evaluation.
- Common middleware and infrastructure: unified response, business error codes,
  Redis + Lua multidimensional rate limiting, Redis Stream task templates,
  structured LLM output retry, provider registry, PostgreSQL persistence,
  pgvector, S3-compatible object storage, CORS, config loading, logging, and API
  documentation.

## Non-Goals

- Do not introduce a separate vector database such as Milvus or Qdrant. Keep
  PostgreSQL + pgvector to match the original middleware and reduce operations
  complexity.
- Do not replace Redis Stream with Kafka or RabbitMQ. Redis is already required
  for cache and rate limiting, so Stream remains the best balance for this
  project scale.
- Do not redesign the frontend layout. Keep the original React page structure
  and only adjust styling and API integration where needed.
- Do not convert local prompts into new prompt wording unless a Go integration
  requires small format adaptation. The semantic content of system prompts must
  remain stable.
- Do not use local LLM inference as the primary base model. All LLM, embedding,
  ASR, and TTS model calls must go through APIs.

## Recommended Architecture

Use a layered modular monolith:

```text
goGetJob/
  cmd/server/                  # process entrypoint
  configs/                     # sample config files
  internal/common/             # shared app capabilities
    ai/                        # provider registry, structured output, prompts
    async/                     # Redis Stream producer/consumer templates
    config/                    # typed config loading
    errors/                    # business errors and error codes
    logger/                    # structured logging setup
    middleware/                # CORS, recovery, request ID, rate limit
    response/                  # Result[T] equivalent
  internal/infrastructure/
    db/                        # GORM, migrations, transaction helpers
    redis/                     # go-redis wrapper, stream, Lua scripts
    storage/                   # S3-compatible object storage
    file/                      # validation, hash, parsing, cleaning
    export/                    # PDF export
    vector/                    # pgvector repository and embedding storage
  internal/modules/
    resume/
    interview/
    knowledgebase/
    interviewschedule/
    voiceinterview/
  internal/prompts/            # copied .st prompt templates
  internal/skills/             # copied interview skills and references
  frontend/                    # copied React/Vite frontend with style changes
  docker/                      # PostgreSQL + pgvector, Redis, MinIO/RustFS
  docs/                        # project and engineering story docs
  README.md
```

Each business module keeps the original local MVC idea:

```text
handler -> service -> repository
              -> common/infrastructure dependencies
```

Handlers should only bind request data, call services, and return `Result`.
Services orchestrate business flows. Repositories own persistence details.
Infrastructure packages expose explicit interfaces so services stay testable.

## Technology Selection

### Web Framework

Use Gin.

Gin maps cleanly to the original Spring MVC controller model while giving Go a
simple middleware chain for CORS, error recovery, and rate limiting. It is more
lightweight than full-stack Go frameworks and easier to operate than adding a
custom net/http stack for every concern.

Alternatives considered:

- Echo: similar ergonomics, but Gin has broader examples and simpler onboarding.
- Fiber: fast, but uses fasthttp semantics that differ from standard net/http.
- net/http only: lowest dependency count, but would require hand-rolled routing,
  binding, validation, and middleware conventions.

Decision: Gin provides the best performance and maintainability balance.

### AI Orchestration

Use Eino for LLM, prompt, retriever, stream, and tool orchestration.

The original project depends on Spring AI concepts: ChatClient, prompt templates,
structured output, retrievers, advisors, and skill tool invocation. Eino gives Go
similar composition primitives and keeps AI workflow code from becoming raw HTTP
glue.

Alternatives considered:

- Raw OpenAI-compatible HTTP client: lowest abstraction, but RAG, streaming, and
  structured output would become duplicated plumbing.
- LangChainGo: common option, but Eino better matches the requested stack and
  CloudWeGo ecosystem.

Decision: Eino is the target framework, with a small provider registry around
OpenAI-compatible endpoints.

### Persistence

Use PostgreSQL + pgvector + GORM.

PostgreSQL + pgvector is retained from the original project. It supports both
relational entities and vector search, which keeps local deployment simple.
GORM is selected because it is closest to JPA in development style and supports
transactions, associations, migrations, and repository-style composition.

Alternatives considered:

- sqlc: excellent type safety and performance, but slower for a large feature
  migration and less similar to the original JPA style.
- Ent: strong schema modeling, but adds a heavier code generation workflow.
- Dedicated vector DB: better at large vector scale, but adds another service
  and does not fit the project's "balanced operations complexity" requirement.

Decision: GORM + pgvector-go gives enough control while preserving migration
speed and operational simplicity.

### Redis

Use Redis with go-redis.

Redis remains responsible for cache, Redis Stream async tasks, and Lua-based
rate limiting. go-redis supports Streams, EvalSha, Redis Cluster, Sentinel, and
normal Redis commands, so it can replace Redisson cleanly.

Decision: keep Redis as a shared infrastructure component and wrap go-redis in
`internal/infrastructure/redis`.

### Object Storage

Use S3-compatible storage with MinIO Go SDK.

The original project supports RustFS/S3-compatible storage. The Go version will
keep endpoint, bucket, access key, secret key, and region configuration. The SDK
can work with MinIO and RustFS locally.

Decision: preserve S3-compatible storage semantics and keep storage swappable.

### Document Parsing

Use a pragmatic parser chain:

- Plain text and Markdown: native Go reading and cleaning.
- PDF: a Go PDF text extraction library where possible.
- DOCX: unzip and parse `word/document.xml`.
- DOC: reject with a clear message or route through an optional external parser
  adapter, because robust legacy DOC parsing in pure Go is weaker than Apache
  Tika.

The original Java implementation relies on Apache Tika. Go has no equally broad,
low-friction equivalent. The first Go version should support the main interview
project formats reliably and document the DOC tradeoff. If full legacy DOC is
mandatory, add an optional Tika sidecar later without changing service APIs.

### PDF Export

Use a Go PDF library with Unicode font support and bundle a Chinese font, keeping
the original idea of stable Chinese report export. If layout requirements exceed
library limits, use an HTML-to-PDF adapter behind the same export interface.

## Common Module Design

### Unified Result and Errors

Keep the original `Result<T>` response shape:

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

Business errors should be represented by typed `BusinessError` values with an
`ErrorCode`. Handlers should not leak internal errors directly. The global Gin
error middleware converts known business errors into `Result.error` responses.

Error code domains remain aligned with the Java version:

- 1xxx common
- 2xxx resume
- 3xxx interview
- 4xxx storage
- 5xxx export
- 6xxx knowledge base
- 7xxx AI service
- 8xxx rate limit
- 9xxx schedule
- 10xxx voice interview

### Configuration

Use typed configuration structs loaded from YAML and environment variables.
Sensitive values such as API keys and database passwords must be injected through
environment variables or `.env` during local development.

Service code should depend on typed config structs, not scattered environment
lookups.

### LLM Provider Registry

Build `ProviderRegistry` equivalent to Java `LlmProviderRegistry`:

- `GetChatModel(providerID)` for general chat.
- `GetDefaultChatModel()`.
- `GetChatModelOrDefault(providerID)`.
- `GetPlainChatModel(providerID)` for flows that must avoid tool calls.
- `GetVoiceChatModel(providerID)` for voice interview streaming and skill tools.

Provider configuration:

```yaml
app:
  ai:
    default_provider: dashscope
    providers:
      dashscope:
        base_url: https://dashscope.aliyuncs.com/compatible-mode
        api_key: ${AI_BAILIAN_API_KEY}
        model: qwen-plus
```

All provider calls use API-based models.

### Structured Output Invoker

Build a generic structured output invoker:

```go
type StructuredInvoker struct {
  MaxAttempts int
  IncludeLastError bool
  UseRepairPrompt bool
  AppendStrictJSONInstruction bool
}
```

It should:

- Append schema/format instructions.
- Call the Eino chat model.
- Parse JSON into a target struct.
- Retry with a repair prompt when parsing fails.
- Include a sanitized last error when configured.
- Return `BusinessError` with the requested `ErrorCode` after all attempts fail.

The strict JSON repair instruction should keep the original Chinese semantics.

### Redis + Lua Rate Limiting

Keep the original sliding-window Lua script algorithm:

- One Redis key per method and dimension.
- Sorted set records permit usage by timestamp.
- Expired permits are reclaimed.
- Requests are rejected when available permits are insufficient.
- Key TTL is twice the interval.

Because Go has no annotation/AOP equivalent, define route metadata:

```go
type RateLimitRule struct {
  Dimension Dimension
  Count float64
  Interval time.Duration
  Fallback string
}
```

Attach rules when registering routes:

```go
rateLimited.POST(
  "/api/knowledgebase/query",
  middleware.RateLimit("KnowledgeBaseController.Query",
    RuleGlobal(10, time.Second),
    RuleIP(10, time.Second),
  ),
  handler.Query,
)
```

Dimensions:

- GLOBAL: all requests.
- IP: client IP from `X-Forwarded-For`, `X-Real-IP`, or remote address.
- USER: `X-User-Id` or context user ID; anonymous fallback.

Each rule is evaluated independently. Any failure rejects the request with
`RATE_LIMIT_EXCEEDED`.

### Redis Stream Async Template

Build generic producer and consumer templates that preserve the Java behavior:

```go
type Producer[T any] interface {
  Send(ctx context.Context, payload T) error
}

type Consumer[T any] interface {
  Start(ctx context.Context) error
}
```

The consumer template owns:

- Consumer group creation.
- Blocking XREADGROUP loop.
- Payload parsing.
- `markProcessing`.
- Business processing.
- `markCompleted`.
- ACK.
- Retry by re-enqueueing with `retry_count + 1`.
- `markFailed` after max retry count.
- ACK and discard malformed messages.

Stream channels:

- Resume analysis.
- Knowledge base vectorization.
- Text interview evaluation.
- Voice interview evaluation.

## Resume Module Design

The resume upload flow mirrors Java:

1. Validate file presence, size, and supported MIME type.
2. Hash file content.
3. Return existing analysis if duplicate.
4. Parse resume text.
5. Upload original file to S3-compatible storage.
6. Save resume metadata and text with `PENDING` status.
7. Enqueue Redis Stream analysis task.
8. Return immediately so the frontend can poll status.

The analysis consumer:

1. Loads the resume entity.
2. Marks `PROCESSING`.
3. Calls LLM with original prompt templates.
4. Parses structured output.
5. Saves analysis and marks `COMPLETED`.
6. Retries failed tasks up to the shared max retry count.

## Mock Interview Module Design

The Go implementation keeps the Java service ideas:

- Session creation generates questions before starting the session.
- Skill-driven question generation uses the copied `internal/skills`.
- When resume text exists, generate resume questions and direction questions in
  parallel using goroutines.
- Keep the Java ratio: roughly 60 percent resume questions and 40 percent
  direction questions.
- Historical question summaries are included to reduce repeat questions.
- Follow-up count is bounded.
- Empty or malformed LLM output falls back to deterministic default questions.
- Answers can be saved, submitted, completed early, and later evaluated.

The unified evaluation module is shared by text and voice interviews:

- Batch QA records to control prompt length.
- Call structured output for batch reports.
- Merge per-question evaluations.
- Run a second summary pass.
- Fall back to merged batch results if summary fails.
- Preserve reference answers and key points for report detail.

## Knowledge Base and RAG Design

The RAG implementation keeps the current optimized behavior as the final design.
The README will explain it as an engineering evolution:

Initial imagined version:

- Split documents.
- Store embeddings.
- Run one vector search with fixed topK and threshold.
- Feed retrieved chunks into LLM.

Observed problems:

- Short questions such as "索引呢" had poor recall.
- Multi-turn questions lost context.
- Fixed topK either returned too little context for vague questions or too much
  weak context for precise questions.
- Some vector-store metadata filters failed depending on query shape, causing
  cross-knowledge-base leakage or empty results.

Final implementation:

- Query normalization.
- Optional query rewrite using chat history.
- Candidate query list: rewritten query first, original query second.
- Dynamic topK and minScore:
  - short query: larger topK and lower threshold.
  - medium query: moderate topK.
  - long query: smaller topK and default threshold.
- pgvector metadata filter by `kb_id`.
- Local filtering fallback if vector-store filtering fails.
- Streaming answer normalization with an early probe window to collapse
  "no relevant info" responses into a stable fixed message.

The vectorization flow:

1. Upload and parse document.
2. Save metadata with `PENDING`.
3. Enqueue vectorization task.
4. Consumer deletes old vectors for that KB.
5. Split text into chunks.
6. Generate embeddings in batches compatible with provider limits.
7. Save chunks and vectors with `kb_id` metadata.
8. Mark vector status `COMPLETED` or `FAILED`.

## Voice Interview Design

Voice interview is a full real-time feature, not a stub.

### REST API

Keep the original API shape:

- `POST /api/voice-interview/sessions`: create session.
- `GET /api/voice-interview/sessions/{sessionId}`: get session.
- `POST /api/voice-interview/sessions/{sessionId}/end`: end session and enqueue
  evaluation.
- `PUT /api/voice-interview/sessions/{sessionId}/pause`: pause session.
- `PUT /api/voice-interview/sessions/{sessionId}/resume`: resume session and
  return WebSocket URL.
- `GET /api/voice-interview/sessions`: list sessions.
- `GET /api/voice-interview/sessions/{sessionId}/messages`: list dialogue
  history.
- `GET /api/voice-interview/sessions/{sessionId}/evaluation`: poll evaluation.
- `POST /api/voice-interview/sessions/{sessionId}/evaluation`: trigger
  evaluation.
- `DELETE /api/voice-interview/sessions/{sessionId}`: delete session.

### WebSocket API

Use:

```text
GET /ws/voice-interview/{sessionId}
```

Client-to-server messages:

```json
{"type":"audio","data":"base64 pcm audio"}
{"type":"control","action":"submit","data":{"text":"optional manual text"}}
{"type":"control","action":"end_interview"}
{"type":"control","action":"start_phase","phase":"TECH"}
```

Server-to-client messages:

```json
{"type":"control","action":"welcome","message":"连接成功，准备开始语音面试"}
{"type":"subtitle","text":"实时字幕","isFinal":false}
{"type":"subtitle","text":"最终用户回答","isFinal":true}
{"type":"text","content":"AI 面试官文本"}
{"type":"audio","data":"base64 wav audio","text":"AI 面试官文本"}
{"type":"audio_chunk","data":"base64 wav audio","index":0,"isLast":false}
{"type":"error","message":"错误信息"}
```

### Real-Time Pipeline

On WebSocket connect:

1. Extract session ID from URL.
2. Store connection in a concurrent session map.
3. Create per-session state.
4. Start ASR realtime connection.
5. Send welcome control message.
6. If the session has no history, send an opening question.
7. Synthesize and cache opening audio for common opening questions.

On audio message:

1. Drop audio if AI is currently speaking or in echo cooldown.
2. Decode base64 PCM.
3. Send PCM to ASR.
4. If ASR append fails due to missing/closed session, restart ASR and retry the
   current chunk.

On ASR partial:

1. Mark STT activity.
2. Send subtitle with `isFinal=false`.

On ASR final:

1. Append recognized segment to `mergeBuffer`.
2. Send merged preview subtitle.
3. Wait for manual `submit` control before invoking LLM.

On submit:

1. Atomically acquire the per-session processing flag.
2. Take and clear `mergeBuffer`.
3. Run LLM + TTS pipeline in a goroutine.
4. Release processing flag when done.

On LLM + TTS:

1. Set `aiSpeaking=true`.
2. Load session entity.
3. Load conversation history.
4. Build voice interview prompt from skill, resume, and history.
5. Stream LLM response through Eino.
6. Push partial AI text to frontend at a throttled interval.
7. Detect sentence boundaries.
8. Start TTS for each complete sentence with a per-session concurrency limit.
9. Save final user text and AI text to database.
10. Convert PCM TTS output to WAV for browser playback.
11. Send merged audio or ordered audio chunks.
12. Clear accumulated user text.
13. Set `aiSpeaking=false` and update `aiSpeakEndAt` for cooldown.

### ASR Provider

Use DashScope Qwen realtime ASR-compatible WebSocket API:

- model: `qwen3-asr-flash-realtime`
- audio format: PCM
- sample rate: 16000
- language: Chinese
- turn detection: server VAD

The provider wrapper should expose:

```go
type ASRService interface {
  Start(ctx context.Context, sessionID string, callbacks ASRCallbacks) error
  SendAudio(ctx context.Context, sessionID string, pcm []byte) error
  Restart(ctx context.Context, sessionID string, callbacks ASRCallbacks) error
  Stop(ctx context.Context, sessionID string) error
}
```

### TTS Provider

Use DashScope Qwen realtime TTS-compatible WebSocket API:

- model: `qwen3-tts-flash-realtime`
- audio format: PCM 24kHz mono 16-bit
- mode: commit
- voice configurable

The wrapper should expose:

```go
type TTSService interface {
  Synthesize(ctx context.Context, text string) ([]byte, error)
}
```

Each synthesis call may open a short-lived upstream WebSocket, collect audio
deltas, and close. Later optimization can pool or reuse upstream TTS sessions,
but the first implementation should favor correctness and isolation.

### Voice Session State

Per-session in-memory state:

```go
type SessionState struct {
  AccumulatedText string
  Processing atomic.Bool
  AISpeaking atomic.Bool
  AISpeakEndAt atomic.Int64
  MergeBuffer string
  MergeStartedAt int64
  LastSTTActivityAt atomic.Int64
  CancelProcessing context.CancelFunc
}
```

This state is not the source of truth. Database entities remain authoritative for
session status, phase, message history, and evaluation state.

### Pause and Resume

Track last activity time per WebSocket session.

- Send warning at 4 minutes 30 seconds of inactivity.
- Pause at 5 minutes of inactivity.
- Persist status `PAUSED`.
- Close WebSocket.
- Stop ASR.
- Allow `resume` to set status back to `IN_PROGRESS` and return a WebSocket URL.

### Conversation Persistence

Each completed AI turn saves:

- session ID
- phase
- user recognized text
- AI generated text
- sequence number
- timestamp

Opening question may save `userRecognizedText=null` and only AI text, matching
the Java behavior.

### Voice Evaluation

When a voice session ends:

1. Mark session `COMPLETED`.
2. Set evaluate status `PENDING`.
3. Enqueue Redis Stream voice evaluation task.
4. Consumer marks `PROCESSING`.
5. Load dialogue messages.
6. Convert messages to `QaRecord`.
7. Infer simple categories from AI text.
8. Reuse unified evaluation service.
9. Save evaluation JSON fields and score.
10. Mark `COMPLETED`.

If no dialogue exists, save an empty evaluation with score 0 and an explanatory
message.

## Frontend Design

Copy the original React/Vite frontend and keep page layout stable:

- Resume upload and history.
- Interview hub.
- Text mock interview.
- Interview history/detail.
- Knowledge base upload/manage/query.
- RAG chat.
- Interview schedule calendar/list.
- Voice interview page and evaluation page.

Allowed changes:

- API base URL configuration.
- WebSocket URL generation.
- Style refresh.
- Small type updates where Go JSON field names differ.

Forbidden changes:

- Replacing the major page layout.
- Removing original features.
- Rewriting user-facing prompt behavior.

## Testing Strategy

Use test-first development for new Go behavior.

Unit tests:

- Business error mapping.
- Result response helpers.
- Rate limit key generation and Lua argument construction.
- Redis Stream retry state transitions with fake Redis or integration Redis.
- Structured output retry prompt construction.
- RAG query parameter selection.
- RAG no-result normalization.
- Voice session state transitions.
- PCM-to-WAV header conversion.
- Voice prompt generation.

Integration tests:

- PostgreSQL repository CRUD and pgvector search.
- Redis Lua rate limit with real Redis.
- Redis Stream producer/consumer with real Redis.
- File upload parsing and duplicate detection.
- Knowledge base vectorization against test embeddings.
- WebSocket voice control messages with fake ASR/TTS/LLM services.

Manual verification:

- Docker Compose starts PostgreSQL, Redis, object storage, backend, and frontend.
- Upload resume and see asynchronous analysis status finish.
- Create text interview, submit answers, generate report.
- Upload knowledge base, wait for vectorization, query through streaming RAG.
- Create voice interview, speak into browser, see subtitles, hear AI audio, end
  interview, poll evaluation.

## Development and Branch Strategy

Use real feature branches and commits:

1. `feature/bootstrap-go-gin`: project scaffold, config, logging, Result,
   BusinessError, Docker base.
2. `feature/common-infra`: database, Redis, object storage, AI provider
   registry, structured output, prompt loader.
3. `feature/rate-limit-stream`: Redis + Lua rate limiter and Redis Stream
   generic templates.
4. `feature/resume-module`: resume upload, parsing, storage, async analysis.
5. `feature/interview-module`: text mock interview, skill loading, question
   generation, answer flow, evaluation.
6. `feature/knowledgebase-rag`: knowledge base upload, vectorization, RAG query,
   SSE streaming, RAG chat.
7. `feature/voice-interview`: full REST + WebSocket + ASR + TTS + LLM + async
   evaluation.
8. `feature/schedule-module`: interview schedule CRUD and invite parsing.
9. `feature/frontend-docs`: frontend migration, style changes, README and
   engineering story docs.

Each branch should be merged or rebased intentionally after tests pass, then
pushed to the remote repository.

## README Engineering Story

The README must describe the implementation as an interview-ready engineering
story. Each technical highlight should follow this structure:

1. Problem phenomenon.
2. Initial simple solution.
3. Why the initial solution failed.
4. Options compared.
5. Final implementation.
6. Effect evaluation.

Required stories:

- Mock interview: from single-shot question generation to skill-driven,
  resume-aware, history-deduplicated, follow-up-capable generation.
- RAG: from fixed vector search to rewrite + dynamic topK/threshold + fallback
  filtering + streaming normalization.
- Redis Stream async tasks: from synchronous upload analysis to resilient
  asynchronous processing with retry and status polling.
- Redis + Lua rate limiting: from single in-memory limiter to distributed
  multidimensional sliding-window limits.
- Voice interview: from blocking turn-based voice response to streaming LLM,
  sentence-level concurrent TTS, echo cooldown, ASR reconnect, and async
  evaluation.

## Open Decisions Resolved

Voice interview will be fully implemented, including ASR/TTS/WebSocket real-time
dialogue. It is not acceptable to leave only REST stubs or mock ASR/TTS in the
production path. Tests may use fake ASR/TTS/LLM implementations behind the same
interfaces.


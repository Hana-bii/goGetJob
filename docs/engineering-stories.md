# Engineering Stories

These stories describe the main implementation choices behind the current codebase. They are written to match what is actually implemented today, including the end-to-end voice interview backend.

## 1. Mock Interview

**Status:** Implemented

**Problem**

The product needed mock interview sessions that could generate different questions from the same resume while still feeling coherent and skill-aware.

**Initial Solution**

The first pass was to generate questions directly from the resume text and ask the model to "cover the important parts".

**Root Cause**

That approach produced uneven coverage. High-signal skills could be repeated too often, while supporting categories were skipped. It also made the output drift from one run to another.

**Options Compared**

- Direct prompt-only generation from the resume
- Category-driven generation with a structured skill catalog
- Fully cached question banks per skill profile

**Final Solution**

The backend now loads the skill catalog from `internal/skills`, allocates question counts by category priority, and uses prompt templates to keep generation stable. The interview module wires that into session creation and answer flow.

**Effect**

Question generation became more consistent, easier to tune, and much easier to explain when the candidate asks why a session contains a particular mix of questions.

## 2. RAG

**Status:** Implemented

**Problem**

Users needed a knowledge base that could answer questions from uploaded documents instead of only relying on keyword search.

**Initial Solution**

The simplest option was plain text search over metadata and extracted content.

**Root Cause**

Keyword search missed semantically similar questions, especially when the user phrased the query differently from the document wording.

**Options Compared**

- Plain keyword search
- Full-text search only
- Vector search with pgvector and AI embeddings
- Hybrid retrieval with query rewrite plus vector search

**Final Solution**

The current implementation uses PostgreSQL + pgvector for embeddings, a vector service for chunk indexing and retrieval, and query rewrite plus top-k tuning in the RAG service. Uploaded files are stored in MinIO so indexing and retrieval stay separate.

**Effect**

The system can answer broader questions from the same document set without requiring manual tag curation or a separate vector database service.

## 3. Redis Stream Async Tasks

**Status:** Implemented

**Problem**

Resume analysis, interview evaluation, and knowledge-base vectorization are all too slow to run inline on the HTTP request path.

**Initial Solution**

The obvious first option was to do the work synchronously in the request handler.

**Root Cause**

That would have made uploads feel sluggish, increased timeout risk, and made retries awkward when the model or storage layer was temporarily unavailable.

**Options Compared**

- Synchronous request handling
- Background goroutines inside the API process
- Redis Stream producers and consumers

**Final Solution**

The backend now uses Redis Streams for async jobs. The HTTP handlers enqueue work, and dedicated consumers process resume analysis, interview evaluation, and knowledge-base vectorization with retry-friendly loops.

**Effect**

The UI gets faster responses, the expensive work is isolated, and the job pipeline can survive temporary AI or infrastructure hiccups without blocking the user.

## 4. Redis + Lua Rate Limiting

**Status:** Implemented

**Problem**

The API needed protection around expensive or noisy endpoints such as interview creation, answer submission, knowledge-base queries, uploads, and revectorization.

**Initial Solution**

A simple in-memory counter inside the Go process looked attractive at first.

**Root Cause**

That fails as soon as the app has more than one container instance. It also becomes racy when multiple requests update the same counter at the same time.

**Options Compared**

- In-memory rate limiting
- Redis INCR-based counters
- Redis + Lua atomic evaluation

**Final Solution**

The middleware uses Redis-backed checks with Lua so the decision is atomic and consistent across processes. The handlers opt in per route, which keeps the protection focused on the expensive paths only.

**Effect**

Rate limits are stable across containers, and the code stays close to the routes that need protection instead of being spread throughout the application.

## 5. Real-Time Voice Interview

**Status:** Implemented

**Problem**

The product needed a real-time voice interview mode with speech-to-text, text-to-speech, and WebSocket-based turn handling, while keeping session state and evaluation consistent with existing text interview workflows.

**Initial Solution**

The first version focused on REST session APIs and plain text interactions, with browser pages prepared for voice but no robust real-time orchestration.

**Root Cause**

A direct request-response approach cannot support low-latency subtitles, turn control, ASR reconnect handling, and sentence-level audio streaming in one coherent flow. Without a dedicated stateful module, behavior quickly becomes inconsistent under network jitter and provider interruptions.

**Options Compared**

- Keep voice as frontend-only prototype
- Build a thin WebSocket relay around providers with minimal backend state
- Build a full backend module with persistent sessions, WS state machine, ASR/TTS adapters, and async evaluation

**Final Solution**

The backend now includes `internal/modules/voiceinterview` with:
- REST APIs for create/list/get/pause/resume/end/delete session, message history, and evaluation polling/trigger
- `/ws/voice-interview/:sessionId` real-time loop supporting `audio` and control actions (`submit`, `end_interview`, `start_phase`)
- ASR append retry via restart when upstream session is closed/missing
- sentence-level TTS synthesis, ordered `audio_chunk` streaming, and merged `audio` output
- Redis Stream evaluation tasks that transition `PENDING -> PROCESSING -> COMPLETED/FAILED` and persist final report JSON

**Effect**

Voice interview is now a production-ready backend capability instead of a placeholder contract. The UI can display live subtitles, stream AI text/audio responses, and poll asynchronous evaluation results after interview completion with the same operational model used by other async modules.

# API

This document reflects the current Go backend routes. The frontend still contains some legacy client paths, so the canonical backend paths are listed here first.

## Health

| Method | Path | Status | Notes |
| --- | --- | --- | --- |
| `GET` | `/health` | Implemented | Returns `{"status":"ok"}` |

## Resume

Canonical backend prefix: `/api/resume`

| Method | Path | Status | Notes |
| --- | --- | --- | --- |
| `POST` | `/api/resume/upload` | Implemented | Multipart upload |
| `GET` | `/api/resume/history` | Implemented | Resume list/history |
| `GET` | `/api/resume/:id` | Implemented | Resume detail |
| `DELETE` | `/api/resume/:id` | Implemented | Delete a resume |
| `POST` | `/api/resume/:id/reanalyze` | Implemented | Re-run analysis |
| `GET` | `/api/resume/:id/export` | Implemented | PDF export |

Integration note:
- Some frontend code still calls `/api/resumes/*`. The current Go handler exposes the singular `/api/resume/*` path, so keep the UI and backend aligned when working on that area.

## Text Interview

Canonical backend prefix: `/api/interview`

| Method | Path | Status | Notes |
| --- | --- | --- | --- |
| `GET` | `/api/interview/sessions` | Implemented | List sessions |
| `POST` | `/api/interview/sessions` | Implemented | Create session, rate-limited when Redis is configured |
| `GET` | `/api/interview/sessions/:sessionId` | Implemented | Session detail |
| `GET` | `/api/interview/sessions/:sessionId/question` | Implemented | Current question |
| `POST` | `/api/interview/sessions/:sessionId/answers` | Implemented | Submit answer, rate-limited when Redis is configured |
| `PUT` | `/api/interview/sessions/:sessionId/answers` | Implemented | Save answer draft |
| `POST` | `/api/interview/sessions/:sessionId/complete` | Implemented | Mark session complete |
| `GET` | `/api/interview/sessions/:sessionId/report` | Implemented | Generate evaluation report |
| `GET` | `/api/interview/sessions/:sessionId/details` | Implemented | Detailed history view |
| `GET` | `/api/interview/sessions/:sessionId/export` | Implemented | Export interview PDF |
| `DELETE` | `/api/interview/sessions/:sessionId` | Implemented | Delete session |
| `GET` | `/api/interview/sessions/unfinished/:resumeId` | Implemented | Resume an unfinished session |

## Interview Schedule

Canonical backend prefix: `/api/interview-schedule`

| Method | Path | Status | Notes |
| --- | --- | --- | --- |
| `POST` | `/api/interview-schedule/parse` | Implemented | Parse a JD into interview categories |
| `POST` | `/api/interview-schedule` | Implemented | Create a schedule |
| `GET` | `/api/interview-schedule` | Implemented | List schedules |
| `GET` | `/api/interview-schedule/:id` | Implemented | Get by ID |
| `PUT` | `/api/interview-schedule/:id` | Implemented | Update schedule |
| `DELETE` | `/api/interview-schedule/:id` | Implemented | Delete schedule |
| `PATCH` | `/api/interview-schedule/:id/status` | Implemented | Update status |
| `PUT` | `/api/interview-schedule/:id/status` | Implemented | Legacy-compatible status update path |

## Knowledge Base

Canonical backend prefix: `/api/knowledgebase`

| Method | Path | Status | Notes |
| --- | --- | --- | --- |
| `GET` | `/api/knowledgebase/list` | Implemented | List documents |
| `GET` | `/api/knowledgebase/categories` | Implemented | List categories |
| `GET` | `/api/knowledgebase/category/:category` | Implemented | Documents by category |
| `GET` | `/api/knowledgebase/uncategorized` | Implemented | Unclassified documents |
| `GET` | `/api/knowledgebase/search` | Implemented | Metadata search |
| `GET` | `/api/knowledgebase/stats` | Implemented | Document statistics |
| `GET` | `/api/knowledgebase/:id` | Implemented | Document detail |
| `DELETE` | `/api/knowledgebase/:id` | Implemented | Delete document |
| `PUT` | `/api/knowledgebase/:id/category` | Implemented | Update category |
| `GET` | `/api/knowledgebase/:id/download` | Implemented | Download file |
| `POST` | `/api/knowledgebase/query` | Implemented | RAG query, rate-limited when Redis is configured |
| `POST` | `/api/knowledgebase/query/stream` | Implemented | Streaming RAG query |
| `POST` | `/api/knowledgebase/upload` | Implemented | Upload knowledge-base file |
| `POST` | `/api/knowledgebase/:id/revectorize` | Implemented | Rebuild embeddings |

## RAG Chat

Canonical backend prefix: `/api/rag-chat`

| Method | Path | Status | Notes |
| --- | --- | --- | --- |
| `POST` | `/api/rag-chat/sessions` | Implemented | Create chat session |
| `GET` | `/api/rag-chat/sessions` | Implemented | List chat sessions |
| `GET` | `/api/rag-chat/sessions/:sessionId` | Implemented | Session detail |
| `PUT` | `/api/rag-chat/sessions/:sessionId/title` | Implemented | Update title |
| `PUT` | `/api/rag-chat/sessions/:sessionId/pin` | Implemented | Toggle pin |
| `PUT` | `/api/rag-chat/sessions/:sessionId/knowledge-bases` | Implemented | Update linked knowledge bases |
| `DELETE` | `/api/rag-chat/sessions/:sessionId` | Implemented | Delete chat session |
| `POST` | `/api/rag-chat/sessions/:sessionId/messages/stream` | Implemented | Stream chat message responses |

## Skill Catalog

The interview skill catalog is loaded from `internal/skills` and used internally by the text interview module. There is no standalone HTTP API for skill browsing in the current Go backend.

## Voice Interview

Canonical backend prefix: `/api/voice-interview`

| Method | Path | Status | Notes |
| --- | --- | --- | --- |
| `POST` | `/api/voice-interview/sessions` | Implemented | Create a voice interview session |
| `GET` | `/api/voice-interview/sessions` | Implemented | List sessions |
| `GET` | `/api/voice-interview/sessions/:sessionId` | Implemented | Session detail |
| `PUT` | `/api/voice-interview/sessions/:sessionId/pause` | Implemented | Pause session |
| `PUT` | `/api/voice-interview/sessions/:sessionId/resume` | Implemented | Resume session and return `webSocketUrl` |
| `POST` | `/api/voice-interview/sessions/:sessionId/end` | Implemented | End interview and enqueue evaluation |
| `DELETE` | `/api/voice-interview/sessions/:sessionId` | Implemented | Delete session |
| `GET` | `/api/voice-interview/sessions/:sessionId/messages` | Implemented | List dialogue history |
| `GET` | `/api/voice-interview/sessions/:sessionId/evaluation` | Implemented | Poll evaluation result |
| `POST` | `/api/voice-interview/sessions/:sessionId/evaluation` | Implemented | Trigger evaluation |

WebSocket endpoint:

- `GET /ws/voice-interview/:sessionId`

WebSocket message types:

- Client to server: `audio`, `control.submit`, `control.end_interview`, `control.start_phase`
- Server to client: `welcome`, `subtitle`, `text`, `audio_chunk`, `audio`, `error`

package voiceinterview

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResumeSessionReturnsWebSocketURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	repo := NewMemoryRepository()
	service := NewSessionService(SessionServiceOptions{Repository: repo, PromptGenerator: fakePromptGenerator{questions: []PromptQuestion{{Index: 0, Question: "Q", Category: "General"}}}})
	created, err := service.Create(ctx, CreateSessionRequest{QuestionCount: 1})
	require.NoError(t, err)
	require.NoError(t, service.Pause(ctx, created.SessionID))

	router := gin.New()
	RegisterRoutes(router, NewHandler(service, nil), nil)
	request := httptest.NewRequest(http.MethodPut, "/api/voice-interview/sessions/"+created.SessionID+"/resume", bytes.NewReader([]byte(`{}`)))
	request.Host = "api.example.test"
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var decoded struct {
		Code int `json:"code"`
		Data struct {
			SessionID    string `json:"sessionId"`
			Status       string `json:"status"`
			WebSocketURL string `json:"webSocketUrl"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &decoded))
	require.Equal(t, 0, decoded.Code)
	require.Equal(t, created.SessionID, decoded.Data.SessionID)
	require.Equal(t, string(VoiceSessionStatusInProgress), decoded.Data.Status)
	require.Equal(t, "wss://api.example.test/ws/voice-interview/"+created.SessionID, decoded.Data.WebSocketURL)
}

func TestWebSocketStateMergesFinalTranscriptBufferOnSubmit(t *testing.T) {
	ctx := context.Background()
	handler, service, asr, _, _ := newStatefulTestHandler(t)
	session := createStatefulTestSession(t, service)
	state := handler.getOrCreateState(session.SessionID)
	writer := &recordingWSWriter{}

	require.NoError(t, handler.ensureASRStarted(ctx, session.SessionID, writer))
	require.NoError(t, asr.emitPartial(session.SessionID, "hel"))
	require.NoError(t, asr.emitFinal(session.SessionID, "hello"))
	require.NoError(t, asr.emitFinal(session.SessionID, "world"))
	require.Equal(t, []string{"hello", "world"}, state.buffer)

	require.NoError(t, handler.handleControlWithWriter(ctx, writer, session.SessionID, WSInboundMessage{Type: InboundControl, Action: ControlSubmit}))

	require.Eventually(t, func() bool {
		messages, err := service.ListMessages(ctx, session.SessionID)
		return err == nil && len(messages) == 2
	}, time.Second, 10*time.Millisecond)
	messages, err := service.ListMessages(ctx, session.SessionID)
	require.NoError(t, err)
	require.Equal(t, "hello world", messages[0].Content)
	require.Empty(t, state.buffer)
	require.Contains(t, writer.textsOfType(OutboundSubtitle), "hel")
}

func TestWebSocketStateSuppressesAudioDuringAISpeakingAndCooldown(t *testing.T) {
	ctx := context.Background()
	handler, service, asr, _, _ := newStatefulTestHandler(t)
	session := createStatefulTestSession(t, service)
	writer := &recordingWSWriter{}
	require.NoError(t, handler.ensureASRStarted(ctx, session.SessionID, writer))
	state := handler.getOrCreateState(session.SessionID)

	state.aiSpeaking = true
	require.NoError(t, handler.handleAudioWithWriter(ctx, writer, session.SessionID, WSInboundMessage{Type: InboundAudio, Audio: []byte{1}}))
	state.aiSpeaking = false
	state.echoCooldownUntil = time.Now().Add(time.Second)
	require.NoError(t, handler.handleAudioWithWriter(ctx, writer, session.SessionID, WSInboundMessage{Type: InboundAudio, Audio: []byte{2}}))
	state.echoCooldownUntil = time.Now().Add(-time.Second)
	require.NoError(t, handler.handleAudioWithWriter(ctx, writer, session.SessionID, WSInboundMessage{Type: InboundAudio, Audio: []byte{3}}))

	require.Equal(t, 1, asr.sendCount(session.SessionID))
}

func TestWebSocketInactivityAutoPausesSessionAndStopsASR(t *testing.T) {
	ctx := context.Background()
	handler, service, asr, _, _ := newStatefulTestHandler(t)
	handler.inactivityTimeout = time.Second
	session := createStatefulTestSession(t, service)
	writer := &recordingWSWriter{}
	require.NoError(t, handler.ensureASRStarted(ctx, session.SessionID, writer))
	state := handler.getOrCreateState(session.SessionID)
	state.lastActivity = time.Now().Add(-2 * time.Second)

	handler.pauseInactiveSessions(ctx, time.Now())

	got, err := service.Get(ctx, session.SessionID)
	require.NoError(t, err)
	require.Equal(t, VoiceSessionStatusPaused, got.Status)
	require.Equal(t, 1, asr.stopCount(session.SessionID))
	require.Nil(t, handler.stateFor(session.SessionID))
}

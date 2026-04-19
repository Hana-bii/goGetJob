package voiceinterview

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type callbackASR struct {
	mu        sync.Mutex
	callbacks map[string]ASRCallbacks
	sends     map[string]int
	restarts  map[string]int
	stops     map[string]int
	failures  map[string]error
}

func newCallbackASR() *callbackASR {
	return &callbackASR{callbacks: map[string]ASRCallbacks{}, sends: map[string]int{}, restarts: map[string]int{}, stops: map[string]int{}, failures: map[string]error{}}
}

func (a *callbackASR) Start(_ context.Context, sessionID string, callbacks ASRCallbacks) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.callbacks[sessionID] = callbacks
	return nil
}

func (a *callbackASR) SendAudio(_ context.Context, sessionID string, _ AudioChunk) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sends[sessionID]++
	if err := a.failures[sessionID]; err != nil {
		delete(a.failures, sessionID)
		return err
	}
	return nil
}

func (a *callbackASR) Restart(_ context.Context, sessionID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.restarts[sessionID]++
	return nil
}

func (a *callbackASR) Stop(_ context.Context, sessionID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stops[sessionID]++
	delete(a.callbacks, sessionID)
	return nil
}

func (a *callbackASR) emitPartial(sessionID, text string) error {
	a.mu.Lock()
	cb := a.callbacks[sessionID]
	a.mu.Unlock()
	if cb.OnPartial != nil {
		return cb.OnPartial(Transcript{Text: text, Final: false})
	}
	return nil
}

func (a *callbackASR) emitFinal(sessionID, text string) error {
	a.mu.Lock()
	cb := a.callbacks[sessionID]
	a.mu.Unlock()
	if cb.OnFinal != nil {
		return cb.OnFinal(Transcript{Text: text, Final: true})
	}
	return nil
}

func (a *callbackASR) sendCount(sessionID string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sends[sessionID]
}

func (a *callbackASR) restartCount(sessionID string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.restarts[sessionID]
}

func (a *callbackASR) stopCount(sessionID string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stops[sessionID]
}

func (a *callbackASR) failNextSend(sessionID string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failures[sessionID] = err
}

type streamingFakeLLM struct {
	chunks []string
}

func (l streamingFakeLLM) StreamReply(_ context.Context, _ LLMReplyInput) (<-chan string, error) {
	out := make(chan string, len(l.chunks))
	go func() {
		defer close(out)
		for _, chunk := range l.chunks {
			out <- chunk
		}
	}()
	return out, nil
}

type statefulFakeTTS struct {
	*recordingTTS
}

func newStatefulFakeTTS() statefulFakeTTS {
	return statefulFakeTTS{recordingTTS: newRecordingTTS()}
}

type recordingTTS struct {
	mu           sync.Mutex
	requestsList []string
	err          error
}

func newRecordingTTS() *recordingTTS {
	return &recordingTTS{}
}

func (t *recordingTTS) Synthesize(_ context.Context, request SpeechRequest) (SpeechAudio, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.requestsList = append(t.requestsList, request.Text)
	if t.err != nil {
		return SpeechAudio{}, t.err
	}
	return SpeechAudio{Audio: []byte(request.Text), Format: "pcm", SampleRate: 24000}, nil
}

func (t *recordingTTS) requests() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.requestsList...)
}

func (t *recordingTTS) fail(err error) {
	if err == nil {
		err = errors.New("tts failed")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.err = err
}

type recordingWSWriter struct {
	mu       sync.Mutex
	messages []WSOutboundMessage
}

func (w *recordingWSWriter) WriteJSON(value any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	msg, ok := value.(WSOutboundMessage)
	if ok {
		w.messages = append(w.messages, msg)
	}
	return nil
}

func (w *recordingWSWriter) textsOfType(kind string) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := []string{}
	for _, msg := range w.messages {
		if msg.Type == kind {
			out = append(out, msg.Text)
		}
	}
	return out
}

func (w *recordingWSWriter) messagesOfType(kind string) []WSOutboundMessage {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := []WSOutboundMessage{}
	for _, msg := range w.messages {
		if msg.Type == kind {
			out = append(out, msg)
		}
	}
	return out
}

func (w *recordingWSWriter) lastOfType(kind string) WSOutboundMessage {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := len(w.messages) - 1; i >= 0; i-- {
		if w.messages[i].Type == kind {
			return w.messages[i]
		}
	}
	return WSOutboundMessage{}
}

func newStatefulTestHandler(t *testing.T) (*WebSocketHandler, *SessionService, *callbackASR, statefulFakeTTS, streamingFakeLLM) {
	t.Helper()
	repo := NewMemoryRepository()
	service := NewSessionService(SessionServiceOptions{Repository: repo, PromptGenerator: fakePromptGenerator{questions: []PromptQuestion{{Index: 0, Question: "Q", Category: "General"}}}})
	asr := newCallbackASR()
	tts := newStatefulFakeTTS()
	llm := streamingFakeLLM{chunks: []string{"chunk one", " and two"}}
	handler := NewWebSocketHandler(service, asr, tts, llm)
	return handler, service, asr, tts, llm
}

func createStatefulTestSession(t *testing.T, service *SessionService) SessionDTO {
	t.Helper()
	created, err := service.Create(context.Background(), CreateSessionRequest{QuestionCount: 1})
	require.NoError(t, err)
	return created
}

package voiceinterview

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type fakeASR struct {
	callback ASRCallbacks
}

func (f *fakeASR) Start(_ context.Context, _ string, callbacks ASRCallbacks) error {
	f.callback = callbacks
	return nil
}

func (f *fakeASR) SendAudio(_ context.Context, _ string, _ AudioChunk) error {
	if f.callback.OnFinal != nil {
		return f.callback.OnFinal(Transcript{Text: "hello interviewer", Final: true})
	}
	return nil
}

func (f *fakeASR) Restart(context.Context, string) error {
	return nil
}

func (f *fakeASR) Stop(context.Context, string) error {
	return nil
}

type fakeTTS struct{}

func (fakeTTS) Synthesize(context.Context, SpeechRequest) (SpeechAudio, error) {
	return SpeechAudio{Audio: []byte{1, 2, 3}, Format: "pcm", SampleRate: 24000}, nil
}

type fakeLLM struct{}

func (fakeLLM) StreamReply(context.Context, LLMReplyInput) (<-chan string, error) {
	return singleChunk("Thanks, please continue."), nil
}

func TestWebSocketHandlesAudioAndSubmitControl(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	repo := NewMemoryRepository()
	service := NewSessionService(SessionServiceOptions{Repository: repo, PromptGenerator: fakePromptGenerator{questions: []PromptQuestion{{Index: 0, Question: "Q", Category: "General"}}}})
	created, err := service.Create(ctx, CreateSessionRequest{QuestionCount: 1})
	require.NoError(t, err)

	wsHandler := NewWebSocketHandler(service, &fakeASR{}, fakeTTS{}, fakeLLM{})
	router := gin.New()
	router.GET("/ws/voice-interview/:sessionId", wsHandler.Handle)
	server := httptest.NewServer(router)
	defer server.Close()

	url := "ws" + server.URL[len("http"):] + "/ws/voice-interview/" + created.SessionID
	conn, _, err := websocket.DefaultDialer.Dial(url, http.Header{})
	require.NoError(t, err)
	defer conn.Close()

	var welcome WSOutboundMessage
	require.NoError(t, conn.ReadJSON(&welcome))
	require.Equal(t, OutboundWelcome, welcome.Type)

	require.NoError(t, conn.WriteJSON(WSInboundMessage{Type: InboundAudio, Audio: []byte{9, 8, 7}}))
	subtitle := readUntilType(t, conn, OutboundSubtitle)
	require.Equal(t, OutboundSubtitle, subtitle.Type)
	require.Equal(t, "hello interviewer", subtitle.Text)

	require.NoError(t, conn.WriteJSON(WSInboundMessage{Type: InboundControl, Action: ControlSubmit, Text: "final answer"}))
	text := readUntilType(t, conn, OutboundText)
	require.Equal(t, OutboundText, text.Type)
	require.Contains(t, text.Text, "continue")
	audioChunk := readUntilType(t, conn, OutboundAudioChunk)
	require.Equal(t, OutboundAudioChunk, audioChunk.Type)
	require.Equal(t, []byte("RIFF"), audioChunk.Audio[0:4])
	audio := readUntilType(t, conn, OutboundAudio)
	require.Equal(t, OutboundAudio, audio.Type)
	require.Equal(t, []byte("RIFF"), audio.Audio[0:4])

	require.Eventually(t, func() bool {
		messages, err := service.ListMessages(ctx, created.SessionID)
		return err == nil && len(messages) == 2
	}, time.Second, 10*time.Millisecond)
	messages, err := service.ListMessages(ctx, created.SessionID)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, MessageRoleUser, messages[0].Role)
	require.Equal(t, "hello interviewer final answer", messages[0].Content)
	require.Equal(t, MessageRoleAssistant, messages[1].Role)
}

func readUntilType(t *testing.T, conn *websocket.Conn, kind string) WSOutboundMessage {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		var msg WSOutboundMessage
		require.NoError(t, conn.ReadJSON(&msg))
		if msg.Type == kind {
			return msg
		}
	}
	t.Fatalf("did not receive %s", kind)
	return WSOutboundMessage{}
}

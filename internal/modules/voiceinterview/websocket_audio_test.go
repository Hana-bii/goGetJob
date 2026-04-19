package voiceinterview

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReplySynthesizesSentencesAsOrderedWAVAudioChunks(t *testing.T) {
	ctx := context.Background()
	handler, service, _, tts, _ := newStatefulTestHandler(t)
	handler.ttsConcurrency = 2
	session := createStatefulTestSession(t, service)
	writer := &recordingWSWriter{}

	require.NoError(t, handler.reply(ctx, writer, session.SessionID, "answer"))

	require.Equal(t, []string{"chunk one", " and two"}, writer.textsOfType(OutboundText))
	require.Equal(t, []string{"chunk one and two"}, tts.requests())
	audioChunks := writer.messagesOfType(OutboundAudioChunk)
	require.Len(t, audioChunks, 1)
	require.Equal(t, []byte("RIFF"), audioChunks[0].Audio[0:4])
	require.Equal(t, []byte("WAVE"), audioChunks[0].Audio[8:12])
	require.Equal(t, OutboundAudio, writer.lastOfType(OutboundAudio).Type)
}

func TestReplySplitsSentencesBeforeTTSAndPreservesAudioOrder(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	service := NewSessionService(SessionServiceOptions{Repository: repo, PromptGenerator: fakePromptGenerator{questions: []PromptQuestion{{Index: 0, Question: "Q", Category: "General"}}}})
	tts := newRecordingTTS()
	handler := NewWebSocketHandler(service, newCallbackASR(), tts, streamingFakeLLM{chunks: []string{"First sentence. Second sentence!"}})
	handler.ttsConcurrency = 2
	session := createStatefulTestSession(t, service)
	writer := &recordingWSWriter{}

	require.NoError(t, handler.reply(ctx, writer, session.SessionID, "answer"))

	require.ElementsMatch(t, []string{"First sentence.", "Second sentence!"}, tts.requests())
	chunks := writer.messagesOfType(OutboundAudioChunk)
	require.Len(t, chunks, 2)
	require.Contains(t, string(chunks[0].Audio), "First sentence.")
	require.Contains(t, string(chunks[1].Audio), "Second sentence!")
}

func TestOpeningAudioIsSynthesizedOnceAndCachedByText(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	service := NewSessionService(SessionServiceOptions{Repository: repo, PromptGenerator: fakePromptGenerator{questions: []PromptQuestion{{Index: 0, Question: "Opening question?", Category: "General"}}}})
	tts := newRecordingTTS()
	handler := NewWebSocketHandler(service, newCallbackASR(), tts, streamingFakeLLM{chunks: []string{"unused"}})
	session := createStatefulTestSession(t, service)
	stored, err := repo.FindSessionByID(ctx, session.SessionID)
	require.NoError(t, err)
	first := &recordingWSWriter{}
	second := &recordingWSWriter{}
	opening := welcomeText(*stored)

	require.NoError(t, handler.sendOpeningAudio(ctx, first, opening))
	require.NoError(t, handler.sendOpeningAudio(ctx, second, opening))

	require.ElementsMatch(t, []string{"Welcome to the voice interview.", "Opening question?"}, tts.requests())
	require.Len(t, first.messagesOfType(OutboundAudioChunk), 2)
	require.Len(t, second.messagesOfType(OutboundAudioChunk), 2)
	require.Equal(t, first.messagesOfType(OutboundAudioChunk)[0].Audio, second.messagesOfType(OutboundAudioChunk)[0].Audio)
	require.Equal(t, first.messagesOfType(OutboundAudioChunk)[1].Audio, second.messagesOfType(OutboundAudioChunk)[1].Audio)
}

func TestAudioSendClosedSessionRestartsAndRetriesCurrentChunkOnce(t *testing.T) {
	ctx := context.Background()
	handler, service, asr, _, _ := newStatefulTestHandler(t)
	session := createStatefulTestSession(t, service)
	writer := &recordingWSWriter{}
	require.NoError(t, handler.ensureASRStarted(ctx, session.SessionID, writer))
	asr.failNextSend(session.SessionID, ErrASRSessionClosed)

	require.NoError(t, handler.handleAudioWithWriter(ctx, writer, session.SessionID, WSInboundMessage{Type: InboundAudio, Audio: []byte{7}}))

	require.Equal(t, 2, asr.sendCount(session.SessionID))
	require.Equal(t, 1, asr.restartCount(session.SessionID))
}

func TestSubmitRunsPipelineAsyncAndRejectsConcurrentSubmit(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	service := NewSessionService(SessionServiceOptions{Repository: repo, PromptGenerator: fakePromptGenerator{questions: []PromptQuestion{{Index: 0, Question: "Q", Category: "General"}}}})
	llm := &blockingLLM{started: make(chan struct{}), release: make(chan struct{})}
	handler := NewWebSocketHandler(service, newCallbackASR(), newRecordingTTS(), llm)
	session := createStatefulTestSession(t, service)
	writer := &recordingWSWriter{}

	require.NoError(t, handler.handleControlWithWriter(ctx, writer, session.SessionID, WSInboundMessage{Type: InboundControl, Action: ControlSubmit, Text: "first"}))
	select {
	case <-llm.started:
	case <-time.After(time.Second):
		t.Fatal("llm pipeline did not start")
	}
	err := handler.handleControlWithWriter(ctx, writer, session.SessionID, WSInboundMessage{Type: InboundControl, Action: ControlSubmit, Text: "second"})
	require.ErrorIs(t, err, ErrSubmitProcessing)
	close(llm.release)
	require.Eventually(t, func() bool { return !handler.getOrCreateState(session.SessionID).isProcessing() }, time.Second, 10*time.Millisecond)
}

type blockingLLM struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (l *blockingLLM) StreamReply(_ context.Context, _ LLMReplyInput) (<-chan string, error) {
	out := make(chan string, 1)
	go func() {
		l.once.Do(func() { close(l.started) })
		<-l.release
		out <- "done."
		close(out)
	}()
	return out, nil
}

func TestSplitSentences(t *testing.T) {
	got := splitSentences("One. Two! Three? tail")
	require.Equal(t, []string{"One.", "Two!", "Three?", "tail"}, got)
}

func TestIsASRSessionClosed(t *testing.T) {
	require.True(t, errors.Is(ErrASRSessionClosed, ErrASRSessionClosed))
	require.True(t, isASRSessionClosed(errors.New("upstream session closed")))
	require.True(t, isASRSessionClosed(errors.New("missing upstream session")))
	require.False(t, isASRSessionClosed(errors.New("network timeout")))
}

func TestWAVPayloadContainsSentenceMarker(t *testing.T) {
	wav, err := PCMToWAV([]byte("marker"), 24000, 1, 16)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(wav), "marker"))
}

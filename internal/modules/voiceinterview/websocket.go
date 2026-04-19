package voiceinterview

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	InboundAudio   = "audio"
	InboundControl = "control"

	ControlSubmit       = "submit"
	ControlEndInterview = "end_interview"
	ControlStartPhase   = "start_phase"

	OutboundWelcome    = "welcome"
	OutboundSubtitle   = "subtitle"
	OutboundText       = "text"
	OutboundAudio      = "audio"
	OutboundAudioChunk = "audio_chunk"
	OutboundError      = "error"
)

type WSInboundMessage struct {
	Type          string `json:"type"`
	Action        string `json:"action,omitempty"`
	Text          string `json:"text,omitempty"`
	Phase         string `json:"phase,omitempty"`
	Audio         []byte `json:"audio,omitempty"`
	Format        string `json:"format,omitempty"`
	SampleRate    int    `json:"sampleRate,omitempty"`
	Channels      int    `json:"channels,omitempty"`
	BitsPerSample int    `json:"bitsPerSample,omitempty"`
}

type WSOutboundMessage struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	Audio      []byte `json:"audio,omitempty"`
	Format     string `json:"format,omitempty"`
	SampleRate int    `json:"sampleRate,omitempty"`
	Final      bool   `json:"final,omitempty"`
	Error      string `json:"error,omitempty"`
}

type WebSocketHandler struct {
	sessions          *SessionService
	asr               ASR
	tts               TTS
	llm               LLM
	upgrader          websocket.Upgrader
	mu                sync.Mutex
	states            map[string]*voiceSessionState
	openingAudio      map[string]cachedAudio
	inactivityTimeout time.Duration
	echoCooldown      time.Duration
	ttsConcurrency    int
}

type voiceSessionState struct {
	mu                sync.Mutex
	buffer            []string
	aiSpeaking        bool
	processing        bool
	echoCooldownUntil time.Time
	lastActivity      time.Time
	asrStarted        bool
}

type cachedAudio struct {
	chunks     [][]byte
	merged     []byte
	sampleRate int
}

type sentenceAudio struct {
	pcm        []byte
	wav        []byte
	sampleRate int
}

type jsonWriter interface {
	WriteJSON(any) error
}

var (
	ErrASRSessionClosed = errors.New("asr upstream session closed")
	ErrSubmitProcessing = errors.New("voice submit already processing")
)

func NewWebSocketHandler(sessions *SessionService, asr ASR, tts TTS, llm LLM) *WebSocketHandler {
	h := &WebSocketHandler{
		sessions:          sessions,
		asr:               asr,
		tts:               tts,
		llm:               llm,
		states:            map[string]*voiceSessionState{},
		openingAudio:      map[string]cachedAudio{},
		inactivityTimeout: 2 * time.Minute,
		echoCooldown:      1200 * time.Millisecond,
		ttsConcurrency:    2,
		upgrader: websocket.Upgrader{
			HandshakeTimeout: 10 * time.Second,
			CheckOrigin: func(*http.Request) bool {
				return true
			},
		},
	}
	go h.inactivityLoop()
	return h
}

func (h *WebSocketHandler) Handle(c *gin.Context) {
	if h == nil || h.sessions == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "voice websocket dependencies are required"})
		return
	}
	sessionID := c.Param("sessionId")
	session, err := h.sessions.repo.FindSessionByID(c.Request.Context(), sessionID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	defer h.cleanupSession(c.Request.Context(), sessionID)

	openingText := welcomeText(*session)
	_ = conn.WriteJSON(WSOutboundMessage{Type: OutboundWelcome, Text: openingText})
	_ = h.sendOpeningAudio(c.Request.Context(), conn, openingText)
	if err := h.ensureASRStarted(c.Request.Context(), sessionID, conn); err != nil {
		_ = conn.WriteJSON(WSOutboundMessage{Type: OutboundError, Error: err.Error()})
		return
	}
	for {
		var inbound WSInboundMessage
		if err := conn.ReadJSON(&inbound); err != nil {
			return
		}
		if err := h.handleInbound(c.Request.Context(), conn, sessionID, inbound); err != nil {
			_ = conn.WriteJSON(WSOutboundMessage{Type: OutboundError, Error: err.Error()})
		}
	}
}

func (h *WebSocketHandler) handleInbound(ctx context.Context, conn *websocket.Conn, sessionID string, inbound WSInboundMessage) error {
	switch inbound.Type {
	case InboundAudio:
		return h.handleAudioWithWriter(ctx, conn, sessionID, inbound)
	case InboundControl:
		return h.handleControlWithWriter(ctx, conn, sessionID, inbound)
	default:
		return conn.WriteJSON(WSOutboundMessage{Type: OutboundError, Error: "unsupported message type"})
	}
}

func (h *WebSocketHandler) handleAudioWithWriter(ctx context.Context, writer jsonWriter, sessionID string, inbound WSInboundMessage) error {
	if h.asr == nil {
		return errors.New("asr service is required")
	}
	state := h.getOrCreateState(sessionID)
	now := time.Now()
	if state.isAISpeakingOrCoolingDown(now) {
		return nil
	}
	state.markActivity(now)
	if err := h.ensureASRStarted(ctx, sessionID, writer); err != nil {
		return err
	}
	chunk := AudioChunk{Audio: inbound.Audio, Format: inbound.Format, SampleRate: inbound.SampleRate, Channels: inbound.Channels, BitsPerSample: inbound.BitsPerSample}
	if err := h.asr.SendAudio(ctx, sessionID, chunk); err != nil {
		if !isASRSessionClosed(err) {
			return err
		}
		if restartErr := h.asr.Restart(ctx, sessionID); restartErr != nil {
			return errors.Join(err, restartErr)
		}
		return h.asr.SendAudio(ctx, sessionID, chunk)
	}
	return nil
}

func (h *WebSocketHandler) handleControlWithWriter(ctx context.Context, writer jsonWriter, sessionID string, inbound WSInboundMessage) error {
	switch inbound.Action {
	case ControlSubmit:
		state := h.getOrCreateState(sessionID)
		if !state.tryStartProcessing() {
			return ErrSubmitProcessing
		}
		text := h.consumeBufferedText(sessionID, inbound.Text)
		if text != "" {
			if _, err := h.sessions.AppendMessage(ctx, sessionID, MessageRoleUser, text, "answer"); err != nil {
				state.finishProcessing()
				if h.asr != nil {
					_ = h.asr.Restart(ctx, sessionID)
				}
				return err
			}
		}
		go func() {
			defer state.finishProcessing()
			if err := h.reply(ctx, writer, sessionID, text); err != nil {
				_ = writer.WriteJSON(WSOutboundMessage{Type: OutboundError, Error: err.Error()})
			}
		}()
		return nil
	case ControlEndInterview:
		if err := h.sessions.End(ctx, sessionID); err != nil {
			return err
		}
		return writer.WriteJSON(WSOutboundMessage{Type: OutboundText, Text: "Interview ended. Evaluation has been queued."})
	case ControlStartPhase:
		if err := h.sessions.StartPhase(ctx, sessionID, inbound.Phase); err != nil {
			return err
		}
		return writer.WriteJSON(WSOutboundMessage{Type: OutboundText, Text: "Phase started: " + nonEmpty(inbound.Phase, DefaultPhase)})
	default:
		return errors.New("unsupported control action")
	}
}

func (h *WebSocketHandler) reply(ctx context.Context, writer jsonWriter, sessionID, text string) error {
	session, err := h.sessions.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	messages, err := h.sessions.ListMessages(ctx, sessionID)
	if err != nil {
		return err
	}
	chunks := singleChunk(fallbackReply(text))
	if h.llm != nil {
		chunks, err = h.llm.StreamReply(ctx, LLMReplyInput{Session: *session, Messages: messages, Text: text})
		if err != nil {
			return err
		}
	}
	state := h.getOrCreateState(sessionID)
	state.setAISpeaking(true)
	var builder strings.Builder
	for chunk := range chunks {
		if chunk == "" {
			continue
		}
		builder.WriteString(chunk)
		if err := writer.WriteJSON(WSOutboundMessage{Type: OutboundText, Text: chunk}); err != nil {
			state.setAISpeaking(false)
			return err
		}
	}
	reply := strings.TrimSpace(builder.String())
	if reply == "" {
		reply = fallbackReply(text)
	}
	if _, err := h.sessions.AppendMessage(ctx, sessionID, MessageRoleAssistant, reply, session.CurrentPhase); err != nil {
		state.setAISpeaking(false)
		return err
	}
	state.finishSpeaking(time.Now().Add(h.echoCooldown))
	if h.tts == nil {
		return nil
	}
	return h.writeSentenceAudio(ctx, writer, reply)
}

func (h *WebSocketHandler) writeSentenceAudio(ctx context.Context, writer jsonWriter, text string) error {
	audio, err := h.synthesizeSentenceAudio(ctx, text)
	if err != nil {
		return err
	}
	return writeSentenceAudio(writer, audio)
}

func writeAudio(writer jsonWriter, audio SpeechAudio) error {
	const chunkSize = 32 * 1024
	if len(audio.Audio) <= chunkSize {
		return writer.WriteJSON(WSOutboundMessage{Type: OutboundAudio, Audio: audio.Audio, Format: audio.Format, SampleRate: audio.SampleRate, Final: true})
	}
	for offset := 0; offset < len(audio.Audio); offset += chunkSize {
		end := offset + chunkSize
		if end > len(audio.Audio) {
			end = len(audio.Audio)
		}
		if err := writer.WriteJSON(WSOutboundMessage{Type: OutboundAudioChunk, Audio: audio.Audio[offset:end], Format: audio.Format, SampleRate: audio.SampleRate, Final: end == len(audio.Audio)}); err != nil {
			return err
		}
	}
	return nil
}

func writeSentenceAudio(writer jsonWriter, chunks []sentenceAudio) error {
	if len(chunks) == 0 {
		return nil
	}
	mergedPCM := []byte{}
	for i, chunk := range chunks {
		final := i == len(chunks)-1
		if err := writer.WriteJSON(WSOutboundMessage{Type: OutboundAudioChunk, Audio: chunk.wav, Format: "wav", SampleRate: chunk.sampleRate, Final: final}); err != nil {
			return err
		}
		mergedPCM = append(mergedPCM, chunk.pcm...)
	}
	sampleRate := chunks[0].sampleRate
	merged, err := PCMToWAV(mergedPCM, nonZero(sampleRate, 24000), 1, 16)
	if err != nil {
		return err
	}
	return writer.WriteJSON(WSOutboundMessage{Type: OutboundAudio, Audio: merged, Format: "wav", SampleRate: sampleRate, Final: true})
}

func (h *WebSocketHandler) synthesizeSentenceAudio(ctx context.Context, text string) ([]sentenceAudio, error) {
	if h.tts == nil {
		return nil, nil
	}
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return nil, nil
	}
	limit := h.ttsConcurrency
	if limit <= 0 {
		limit = 2
	}
	if limit > len(sentences) {
		limit = len(sentences)
	}
	results := make([]sentenceAudio, len(sentences))
	errs := make(chan error, len(sentences))
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, sentence := range sentences {
		wg.Add(1)
		go func(i int, sentence string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			}
			audio, err := h.tts.Synthesize(ctx, SpeechRequest{Text: sentence})
			if err != nil {
				errs <- err
				return
			}
			sampleRate := nonZero(audio.SampleRate, 24000)
			wav, err := PCMToWAV(audio.Audio, sampleRate, 1, 16)
			if err != nil {
				errs <- err
				return
			}
			results[i] = sentenceAudio{pcm: audio.Audio, wav: wav, sampleRate: sampleRate}
		}(i, sentence)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

func (h *WebSocketHandler) sendOpeningAudio(ctx context.Context, writer jsonWriter, text string) error {
	if h.tts == nil || strings.TrimSpace(text) == "" {
		return nil
	}
	h.mu.Lock()
	cached, ok := h.openingAudio[text]
	h.mu.Unlock()
	if ok {
		return writeCachedAudio(writer, cached)
	}
	audio, err := h.synthesizeSentenceAudio(ctx, text)
	if err != nil {
		return err
	}
	cached = cacheSentenceAudio(audio)
	h.mu.Lock()
	if existing, ok := h.openingAudio[text]; ok {
		cached = existing
	} else {
		h.openingAudio[text] = cached
	}
	h.mu.Unlock()
	return writeCachedAudio(writer, cached)
}

func cacheSentenceAudio(audio []sentenceAudio) cachedAudio {
	out := cachedAudio{}
	mergedPCM := []byte{}
	for _, chunk := range audio {
		out.chunks = append(out.chunks, append([]byte(nil), chunk.wav...))
		mergedPCM = append(mergedPCM, chunk.pcm...)
		if out.sampleRate == 0 {
			out.sampleRate = chunk.sampleRate
		}
	}
	if len(mergedPCM) > 0 {
		out.merged, _ = PCMToWAV(mergedPCM, nonZero(out.sampleRate, 24000), 1, 16)
	}
	return out
}

func writeCachedAudio(writer jsonWriter, cached cachedAudio) error {
	for i, chunk := range cached.chunks {
		if err := writer.WriteJSON(WSOutboundMessage{Type: OutboundAudioChunk, Audio: chunk, Format: "wav", SampleRate: cached.sampleRate, Final: i == len(cached.chunks)-1}); err != nil {
			return err
		}
	}
	if len(cached.merged) > 0 {
		return writer.WriteJSON(WSOutboundMessage{Type: OutboundAudio, Audio: cached.merged, Format: "wav", SampleRate: cached.sampleRate, Final: true})
	}
	return nil
}

func splitSentences(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	out := []string{}
	start := 0
	for i, r := range text {
		if strings.ContainsRune(".!?。！？\n", r) {
			part := strings.TrimSpace(text[start : i+len(string(r))])
			if part != "" {
				out = append(out, part)
			}
			start = i + len(string(r))
		}
	}
	if tail := strings.TrimSpace(text[start:]); tail != "" {
		out = append(out, tail)
	}
	return out
}

func (h *WebSocketHandler) ensureASRStarted(ctx context.Context, sessionID string, writer jsonWriter) error {
	if h.asr == nil {
		return nil
	}
	state := h.getOrCreateState(sessionID)
	if state.asrStarted {
		return nil
	}
	callbacks := ASRCallbacks{
		OnPartial: func(transcript Transcript) error {
			h.markActivity(sessionID)
			return writer.WriteJSON(WSOutboundMessage{Type: OutboundSubtitle, Text: transcript.Text, Final: false})
		},
		OnFinal: func(transcript Transcript) error {
			text := strings.TrimSpace(transcript.Text)
			if text != "" {
				h.appendBuffer(sessionID, text)
			}
			h.markActivity(sessionID)
			return writer.WriteJSON(WSOutboundMessage{Type: OutboundSubtitle, Text: transcript.Text, Final: true})
		},
	}
	if err := h.asr.Start(ctx, sessionID, callbacks); err != nil {
		return err
	}
	state.markASRStarted(time.Now())
	return nil
}

func (h *WebSocketHandler) getOrCreateState(sessionID string) *voiceSessionState {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.states == nil {
		h.states = map[string]*voiceSessionState{}
	}
	state := h.states[sessionID]
	if state == nil {
		state = &voiceSessionState{lastActivity: time.Now()}
		h.states[sessionID] = state
	}
	return state
}

func (h *WebSocketHandler) stateFor(sessionID string) *voiceSessionState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.states[sessionID]
}

func (h *WebSocketHandler) appendBuffer(sessionID string, text string) {
	state := h.getOrCreateState(sessionID)
	state.appendBuffer(text)
}

func (h *WebSocketHandler) consumeBufferedText(sessionID, submitted string) string {
	state := h.getOrCreateState(sessionID)
	parts := state.consumeBuffer()
	if strings.TrimSpace(submitted) != "" {
		parts = append(parts, strings.TrimSpace(submitted))
	}
	return strings.Join(parts, " ")
}

func (h *WebSocketHandler) markActivity(sessionID string) {
	state := h.getOrCreateState(sessionID)
	state.markActivity(time.Now())
}

func (h *WebSocketHandler) inactivityLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for now := range ticker.C {
		h.pauseInactiveSessions(context.Background(), now)
	}
}

func (h *WebSocketHandler) pauseInactiveSessions(ctx context.Context, now time.Time) {
	if h.inactivityTimeout <= 0 {
		return
	}
	h.mu.Lock()
	expired := []string{}
	for sessionID, state := range h.states {
		if state.inactiveSince(now, h.inactivityTimeout) {
			expired = append(expired, sessionID)
		}
	}
	h.mu.Unlock()
	for _, sessionID := range expired {
		_ = h.sessions.Pause(ctx, sessionID)
		if h.asr != nil {
			_ = h.asr.Stop(ctx, sessionID)
		}
		h.mu.Lock()
		delete(h.states, sessionID)
		h.mu.Unlock()
	}
}

func (h *WebSocketHandler) cleanupSession(ctx context.Context, sessionID string) {
	if h.asr != nil {
		_ = h.asr.Stop(ctx, sessionID)
	}
	h.mu.Lock()
	delete(h.states, sessionID)
	h.mu.Unlock()
}

func welcomeText(session VoiceSession) string {
	questions, _ := parseQuestions(session.QuestionsJSON)
	if len(questions) == 0 {
		return "Welcome to the voice interview. Please introduce yourself."
	}
	return "Welcome to the voice interview. " + questions[0].Question
}

func (s *voiceSessionState) isAISpeakingOrCoolingDown(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.aiSpeaking || now.Before(s.echoCooldownUntil)
}

func (s *voiceSessionState) setAISpeaking(value bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aiSpeaking = value
}

func (s *voiceSessionState) finishSpeaking(cooldownUntil time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aiSpeaking = false
	s.echoCooldownUntil = cooldownUntil
}

func (s *voiceSessionState) tryStartProcessing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.processing {
		return false
	}
	s.processing = true
	return true
}

func (s *voiceSessionState) finishProcessing() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processing = false
}

func (s *voiceSessionState) isProcessing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.processing
}

func (s *voiceSessionState) markActivity(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActivity = now
}

func (s *voiceSessionState) markASRStarted(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asrStarted = true
	s.lastActivity = now
}

func (s *voiceSessionState) appendBuffer(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffer = append(s.buffer, text)
}

func (s *voiceSessionState) consumeBuffer() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	parts := append([]string(nil), s.buffer...)
	s.buffer = nil
	return parts
}

func (s *voiceSessionState) inactiveSince(now time.Time, timeout time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.lastActivity.IsZero() && now.Sub(s.lastActivity) >= timeout
}

func isASRSessionClosed(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrASRSessionClosed) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "session closed") || strings.Contains(text, "session missing") || strings.Contains(text, "missing upstream session")
}

func nonZero(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

package voiceinterview

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"goGetJob/internal/common/config"
)

type DashScopeASR struct {
	options config.VoiceASRConfig
	mu      sync.Mutex
	active  map[string]ASRCallbacks
}

func NewDashScopeASR(options config.VoiceASRConfig) *DashScopeASR {
	return &DashScopeASR{options: options, active: map[string]ASRCallbacks{}}
}

func (a *DashScopeASR) Start(ctx context.Context, sessionID string, callbacks ASRCallbacks) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a == nil {
		return fmt.Errorf("dashscope asr is nil")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active == nil {
		a.active = map[string]ASRCallbacks{}
	}
	a.active[sessionID] = callbacks
	return nil
}

func (a *DashScopeASR) SendAudio(ctx context.Context, sessionID string, chunk AudioChunk) error {
	transcript, err := a.transcribeOnce(ctx, chunk)
	if err != nil {
		return err
	}
	a.mu.Lock()
	callbacks := a.active[sessionID]
	a.mu.Unlock()
	if callbacks.OnFinal != nil {
		return callbacks.OnFinal(transcript)
	}
	return nil
}

func (a *DashScopeASR) Restart(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.active[sessionID]; !ok {
		return fmt.Errorf("asr session %s is not started", sessionID)
	}
	return nil
}

func (a *DashScopeASR) Stop(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.active, sessionID)
	return nil
}

func (a *DashScopeASR) transcribeOnce(ctx context.Context, chunk AudioChunk) (Transcript, error) {
	if err := ctx.Err(); err != nil {
		return Transcript{}, err
	}
	if a == nil {
		return Transcript{}, fmt.Errorf("dashscope asr is nil")
	}
	if len(chunk.Audio) == 0 {
		return Transcript{}, fmt.Errorf("audio is required")
	}
	if strings.TrimSpace(a.options.APIKey) == "" {
		return Transcript{}, fmt.Errorf("dashscope asr api key is required")
	}
	conn, err := dialDashScopeRealtime(ctx, a.options.URL, a.options.APIKey)
	if err != nil {
		return Transcript{}, err
	}
	defer conn.Close()

	format := nonEmpty(chunk.Format, nonEmpty(a.options.Format, "pcm"))
	if format == "pcm" {
		format = "pcm16"
	}
	sampleRate := chunk.SampleRate
	if sampleRate == 0 {
		sampleRate = a.options.SampleRate
	}
	session := map[string]any{
		"modalities":         []string{"text"},
		"model":              a.options.Model,
		"input_audio_format": format,
	}
	if sampleRate > 0 {
		session["sample_rate"] = sampleRate
	}
	if a.options.EnableTurnDetection {
		session["turn_detection"] = map[string]any{
			"type":                nonEmpty(a.options.TurnDetectionType, "server_vad"),
			"threshold":           a.options.TurnDetectionThreshold,
			"silence_duration_ms": a.options.TurnDetectionSilenceMillis,
		}
	}
	if err := conn.WriteJSON(map[string]any{"type": "session.update", "session": session}); err != nil {
		return Transcript{}, fmt.Errorf("configure dashscope asr session: %w", err)
	}
	if err := conn.WriteJSON(map[string]any{"type": "input_audio_buffer.append", "audio": base64.StdEncoding.EncodeToString(chunk.Audio)}); err != nil {
		return Transcript{}, fmt.Errorf("send dashscope asr audio: %w", err)
	}
	if err := conn.WriteJSON(map[string]any{"type": "input_audio_buffer.commit"}); err != nil {
		return Transcript{}, fmt.Errorf("commit dashscope asr audio: %w", err)
	}
	_ = conn.WriteJSON(map[string]any{"type": "response.create", "response": map[string]any{"modalities": []string{"text"}}})

	deadline := time.Now().Add(30 * time.Second)
	_ = conn.SetReadDeadline(deadline)
	var partial string
	for {
		var event map[string]any
		if err := conn.ReadJSON(&event); err != nil {
			if partial != "" {
				return Transcript{Text: partial, Final: true}, nil
			}
			return Transcript{}, fmt.Errorf("read dashscope asr response: %w", err)
		}
		if errText := extractString(event, "error", "message"); errText != "" {
			return Transcript{}, fmt.Errorf("dashscope asr error: %s", errText)
		}
		if text := extractString(event, "transcript", "text", "delta"); text != "" {
			partial += text
		}
		eventType := strings.ToLower(extractString(event, "type"))
		if strings.Contains(eventType, "completed") || strings.Contains(eventType, "done") || strings.Contains(eventType, "committed") {
			if strings.TrimSpace(partial) == "" {
				return Transcript{}, fmt.Errorf("dashscope asr returned empty transcript")
			}
			return Transcript{Text: strings.TrimSpace(partial), Final: true}, nil
		}
		if time.Now().After(deadline) {
			if strings.TrimSpace(partial) != "" {
				return Transcript{Text: strings.TrimSpace(partial), Final: true}, nil
			}
			return Transcript{}, fmt.Errorf("dashscope asr timed out")
		}
	}
}

func dialDashScopeRealtime(ctx context.Context, url, apiKey string) (*websocket.Conn, error) {
	if strings.TrimSpace(url) == "" {
		return nil, fmt.Errorf("dashscope realtime url is required")
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+apiKey)
	header.Set("X-DashScope-DataInspection", "enable")
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, header)
	if err != nil {
		return nil, fmt.Errorf("connect dashscope realtime websocket: %w", err)
	}
	return conn, nil
}

func extractString(value any, keys ...string) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if found, ok := typed[key]; ok {
				if text, ok := found.(string); ok {
					return text
				}
				if text := extractString(found, keys...); text != "" {
					return text
				}
			}
		}
		for _, child := range typed {
			if text := extractString(child, keys...); text != "" {
				return text
			}
		}
	case []any:
		for _, child := range typed {
			if text := extractString(child, keys...); text != "" {
				return text
			}
		}
	case string:
		if len(keys) == 0 {
			return typed
		}
	}
	return ""
}

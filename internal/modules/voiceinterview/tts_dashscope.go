package voiceinterview

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"goGetJob/internal/common/config"
)

type DashScopeTTS struct {
	options config.VoiceTTSConfig
}

func NewDashScopeTTS(options config.VoiceTTSConfig, client *http.Client) *DashScopeTTS {
	_ = client
	return &DashScopeTTS{options: options}
}

func (t *DashScopeTTS) Synthesize(ctx context.Context, req SpeechRequest) (SpeechAudio, error) {
	if err := ctx.Err(); err != nil {
		return SpeechAudio{}, err
	}
	if t == nil {
		return SpeechAudio{}, fmt.Errorf("dashscope tts is nil")
	}
	if strings.TrimSpace(req.Text) == "" {
		return SpeechAudio{}, fmt.Errorf("text is required")
	}
	if strings.TrimSpace(t.options.APIKey) == "" {
		return SpeechAudio{}, fmt.Errorf("dashscope tts api key is required")
	}
	if strings.HasPrefix(strings.ToLower(t.options.URL), "wss://") {
		return t.synthesizeRealtime(ctx, req)
	}
	return SpeechAudio{}, fmt.Errorf("dashscope tts requires websocket realtime url")
}

func (t *DashScopeTTS) synthesizeRealtime(ctx context.Context, req SpeechRequest) (SpeechAudio, error) {
	conn, err := dialDashScopeRealtime(ctx, t.options.URL, t.options.APIKey)
	if err != nil {
		return SpeechAudio{}, err
	}
	defer conn.Close()

	format := nonEmpty(req.Format, nonEmpty(t.options.Format, "pcm"))
	if format == "pcm" {
		format = "pcm16"
	}
	sampleRate := t.options.SampleRate
	session := map[string]any{
		"modalities":          []string{"text", "audio"},
		"model":               t.options.Model,
		"voice":               nonEmpty(req.Voice, nonEmpty(t.options.Voice, t.options.DefaultVoice)),
		"output_audio_format": format,
	}
	if sampleRate > 0 {
		session["sample_rate"] = sampleRate
	}
	if err := conn.WriteJSON(map[string]any{"type": "session.update", "session": session}); err != nil {
		return SpeechAudio{}, fmt.Errorf("configure dashscope tts session: %w", err)
	}
	if err := conn.WriteJSON(map[string]any{
		"type": "response.create",
		"response": map[string]any{
			"modalities":   []string{"audio", "text"},
			"instructions": req.Text,
			"voice":        nonEmpty(req.Voice, nonEmpty(t.options.Voice, t.options.DefaultVoice)),
		},
	}); err != nil {
		return SpeechAudio{}, fmt.Errorf("send dashscope tts request: %w", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	_ = conn.SetReadDeadline(deadline)
	audio := []byte{}
	for {
		var event map[string]any
		if err := conn.ReadJSON(&event); err != nil {
			if len(audio) > 0 {
				return SpeechAudio{Audio: audio, Format: nonEmpty(req.Format, nonEmpty(t.options.Format, "pcm")), SampleRate: sampleRate}, nil
			}
			return SpeechAudio{}, fmt.Errorf("read dashscope tts response: %w", err)
		}
		if errText := extractString(event, "error", "message"); errText != "" {
			return SpeechAudio{}, fmt.Errorf("dashscope tts error: %s", errText)
		}
		if delta := extractAudioDelta(event); delta != "" {
			decoded, err := base64.StdEncoding.DecodeString(delta)
			if err == nil {
				audio = append(audio, decoded...)
			}
		}
		eventType := strings.ToLower(extractString(event, "type"))
		if strings.Contains(eventType, "completed") || strings.Contains(eventType, "done") {
			if len(audio) == 0 {
				return SpeechAudio{}, fmt.Errorf("dashscope tts returned empty audio")
			}
			return SpeechAudio{Audio: audio, Format: nonEmpty(req.Format, nonEmpty(t.options.Format, "pcm")), SampleRate: sampleRate}, nil
		}
		if time.Now().After(deadline) {
			if len(audio) > 0 {
				return SpeechAudio{Audio: audio, Format: nonEmpty(req.Format, nonEmpty(t.options.Format, "pcm")), SampleRate: sampleRate}, nil
			}
			return SpeechAudio{}, fmt.Errorf("dashscope tts timed out")
		}
	}
}

func extractAudioDelta(event map[string]any) string {
	eventType := strings.ToLower(extractString(event, "type"))
	if strings.Contains(eventType, "audio") {
		if delta := extractString(event, "delta", "audio"); delta != "" {
			return delta
		}
	}
	return ""
}

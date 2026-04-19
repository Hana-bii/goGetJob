package voiceinterview

import "context"

type AudioChunk struct {
	Audio         []byte
	Format        string
	SampleRate    int
	Channels      int
	BitsPerSample int
}

type Transcript struct {
	Text  string `json:"text"`
	Final bool   `json:"final"`
}

type ASRCallbacks struct {
	OnPartial func(Transcript) error
	OnFinal   func(Transcript) error
}

type ASR interface {
	Start(ctx context.Context, sessionID string, callbacks ASRCallbacks) error
	SendAudio(ctx context.Context, sessionID string, chunk AudioChunk) error
	Restart(ctx context.Context, sessionID string) error
	Stop(ctx context.Context, sessionID string) error
}

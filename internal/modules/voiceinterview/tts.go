package voiceinterview

import "context"

type SpeechRequest struct {
	Text   string
	Voice  string
	Format string
}

type SpeechAudio struct {
	Audio      []byte `json:"audio"`
	Format     string `json:"format"`
	SampleRate int    `json:"sampleRate"`
}

type TTS interface {
	Synthesize(context.Context, SpeechRequest) (SpeechAudio, error)
}

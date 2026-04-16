package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

func New(env string) *slog.Logger {
	level := slog.LevelInfo
	if strings.EqualFold(env, "dev") || strings.EqualFold(env, "local") || strings.EqualFold(env, "debug") {
		level = slog.LevelDebug
	}

	var handler slog.Handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	if strings.EqualFold(env, "dev") || strings.EqualFold(env, "local") || strings.EqualFold(env, "debug") {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}

	log := slog.New(handler)
	slog.SetDefault(log)
	return log
}

func NewDiscard() *slog.Logger {
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)
	return log
}

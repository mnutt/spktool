package logging

import (
	"context"
	"log/slog"
	"os"
)

type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

type Config struct {
	Format  Format
	Verbose bool
}

func New(cfg Config) *slog.Logger {
	level := slog.LevelInfo
	if cfg.Verbose {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	if cfg.Format == FormatJSON {
		return slog.New(slog.NewJSONHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}

func WithTrace(ctx context.Context, logger *slog.Logger, traceID string) *slog.Logger {
	if traceID == "" {
		return logger
	}
	return logger.With("trace_id", traceID)
}

package server

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// NewLogger builds the `mathiz serve` logger from the config's MATHIZ_LOG_*
// settings: stdout always, plus an append-mode file tee when LogFile is set.
// The returned close func releases the file (no-op without one). A log file
// that cannot be opened is a startup FAILURE — a silently dropped log path
// is worse than a crash at boot. Rotation is intentionally out of scope:
// rotate externally (logrotate, container log driver).
func NewLogger(stdout io.Writer, cfg *Config) (*slog.Logger, func() error, error) {
	var level slog.Level
	switch cfg.LogLevel {
	case "", "info":
		level = slog.LevelInfo
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return nil, nil, fmt.Errorf("unsupported MATHIZ_LOG_LEVEL %q (available: debug, info, warn, error)", cfg.LogLevel)
	}

	out := stdout
	closeFn := func() error { return nil }
	if cfg.LogFile != "" {
		f, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, fmt.Errorf("open MATHIZ_LOG_FILE: %w", err)
		}
		out = io.MultiWriter(stdout, f)
		closeFn = f.Close
	}

	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	switch cfg.LogFormat {
	case "", "text":
		h = slog.NewTextHandler(out, opts)
	case "json":
		h = slog.NewJSONHandler(out, opts)
	default:
		_ = closeFn()
		return nil, nil, fmt.Errorf("unsupported MATHIZ_LOG_FORMAT %q (available: text, json)", cfg.LogFormat)
	}
	return slog.New(h), closeFn, nil
}

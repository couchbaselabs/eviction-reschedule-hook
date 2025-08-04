package reschedule

import (
	"context"
	"log/slog"
)

// Logger is a slog.Handler that prefixes messages with a string
type Logger struct {
	slog.Handler
	prefix string
}

func (h *Logger) Handle(ctx context.Context, r slog.Record) error {
	// Prefix the message
	r.Message = h.prefix + r.Message
	return h.Handler.Handle(ctx, r)
}

func (h *Logger) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Logger{
		Handler: h.Handler.WithAttrs(attrs),
		prefix:  h.prefix,
	}
}

func (h *Logger) WithGroup(name string) slog.Handler {
	return &Logger{
		Handler: h.Handler.WithGroup(name),
		prefix:  h.prefix,
	}
}

func CreateLogger(pod, namespace string, dryRun bool) *slog.Logger {
	logger := slog.With("pod", pod, "namespace", namespace)

	prefix := ""
	if dryRun {
		prefix = "(server dry run) "
	}

	return slog.New(&Logger{
		Handler: logger.Handler(),
		prefix:  prefix,
	})
}

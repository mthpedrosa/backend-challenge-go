// Package observability provides structured logging and, in later phases,
// metrics. It must not be imported by the domain layer.
package observability

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/mthpedrosa/backend-challenge-go/internal/config"
	"github.com/mthpedrosa/backend-challenge-go/internal/platform/correlation"
)

// NewLogger builds the JSON logger used by the whole process. Every record
// carries the service name and instance id, plus the correlation id found in
// the context passed to the *Context logging methods.
func NewLogger(cfg config.Config) *slog.Logger {
	return newLogger(os.Stdout, cfg)
}

func newLogger(w io.Writer, cfg config.Config) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: cfg.Log.Level})
	return slog.New(contextHandler{Handler: handler}).With(
		slog.String("service", cfg.App.Name),
		slog.String("instanceId", cfg.App.InstanceID),
		slog.String("env", cfg.App.Env),
	)
}

// contextHandler enriches records with identifiers carried by the context.
type contextHandler struct {
	slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id, ok := correlation.FromContext(ctx); ok {
		r.AddAttrs(slog.String("correlationId", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{Handler: h.Handler.WithGroup(name)}
}

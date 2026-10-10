// Package bootstrap composes the application with Uber Fx. It is the only
// place that knows how every module fits together.
package bootstrap

import (
	"log/slog"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/mthpedrosa/backend-challenge-go/internal/adapters/httpapi"
	"github.com/mthpedrosa/backend-challenge-go/internal/config"
	"github.com/mthpedrosa/backend-challenge-go/internal/platform/observability"
	"github.com/mthpedrosa/backend-challenge-go/internal/platform/postgres"
)

const (
	startTimeout = 30 * time.Second
	stopTimeout  = 30 * time.Second
)

// Options returns the full application graph. Tests can append extra options
// (e.g. fx.Decorate) to override pieces such as the configuration.
func Options(extra ...fx.Option) fx.Option {
	return fx.Options(
		fx.StartTimeout(startTimeout),
		fx.StopTimeout(stopTimeout),
		fx.WithLogger(func(l *slog.Logger) fxevent.Logger {
			fl := &fxevent.SlogLogger{Logger: l.With(slog.String("component", "fx"))}
			fl.UseLogLevel(slog.LevelDebug)
			return fl
		}),

		fx.Provide(config.Load),
		observability.Module,
		postgres.Module,
		httpapi.Module,

		fx.Options(extra...),
	)
}

package observability

import "go.uber.org/fx"

// Module provides the process-wide *slog.Logger.
var Module = fx.Module("observability",
	fx.Provide(NewLogger),
)

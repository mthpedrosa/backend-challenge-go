package httpapi

import "go.uber.org/fx"

// Module wires the HTTP router, server and handlers.
var Module = fx.Module("httpapi",
	fx.Provide(
		NewRouter,
		NewServer,
		NewAuthMiddleware,
		AsRoutes(NewHealthHandler),
		AsRoutes(NewWalletHandler),
		AsRoutes(NewWagerHandler),
	),
	// Force the server to be constructed so its lifecycle hooks are registered.
	fx.Invoke(func(*Server) {}),
)

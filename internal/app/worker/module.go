package worker

import "go.uber.org/fx"

// Module groups all background workers.
var Module = fx.Module("worker",
	fx.Provide(
		NewOutboxPublisher,
		NewPendingReferenceWorker,
	),
	fx.Invoke(func(*OutboxPublisher, *PendingReferenceWorker) {}),
)

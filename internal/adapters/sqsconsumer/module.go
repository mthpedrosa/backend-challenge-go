package sqsconsumer

import "go.uber.org/fx"

// Module groups SQS consumers.
var Module = fx.Module("sqsconsumer",
	fx.Provide(NewConsumer),
	fx.Invoke(func(*Consumer) {}),
)

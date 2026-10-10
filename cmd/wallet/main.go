// Command wallet runs the wallet service. Which components run (HTTP API,
// SQS consumer, outbox publisher, pending-reference worker) is controlled by
// configuration, so the same binary can be scaled horizontally.
package main

import (
	"go.uber.org/fx"

	"github.com/mthpedrosa/backend-challenge-go/internal/bootstrap"
)

func main() {
	// Run blocks until SIGINT/SIGTERM, then executes OnStop hooks in reverse
	// order: entry points stop first, shared dependencies close last.
	fx.New(bootstrap.Options()).Run()
}

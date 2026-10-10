package event

import "context"

// OutboxRepository defines the persistence contract for saving domain events.
type OutboxRepository interface {
	// Save stores a domain event in the transactional outbox table.
	Save(ctx context.Context, e Envelope) error
}

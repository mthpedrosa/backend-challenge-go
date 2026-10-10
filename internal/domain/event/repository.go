package event

import "context"

// OutboxRepository defines the persistence contract for saving domain events.
type OutboxRepository interface {
	// Save stores a domain event in the transactional outbox table.
	Save(ctx context.Context, e Envelope) error

	// LeaseUnpublished acquires a batch of unpublished events for processing.
	LeaseUnpublished(ctx context.Context, batchSize int) ([]Envelope, error)

	// MarkPublished marks an event as successfully published.
	MarkPublished(ctx context.Context, eventID string) error
}

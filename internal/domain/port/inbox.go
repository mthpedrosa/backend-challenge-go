package port

import (
	"context"
	"time"
)

// InboxStatus defines the status of an inbox message.
type InboxStatus string

const (
	InboxStatusPending   InboxStatus = "PENDING"
	InboxStatusCompleted InboxStatus = "COMPLETED"
)

// InboxMessage represents an idempotency check for incoming events.
type InboxMessage struct {
	ConsumerName string
	MessageID    string
	PayloadHash  string
	ReceivedAt   time.Time
	CompletedAt  *time.Time
}

// InboxRepository handles the inbox pattern persistence for deduplication.
type InboxRepository interface {
	// TryLock attempts to insert a new message into the inbox or returns the existing one.
	// If the message already exists with a different payload hash, it should return an error.
	TryLock(ctx context.Context, msg InboxMessage) (bool, *InboxMessage, error)

	// MarkCompleted marks an inbox message as successfully processed.
	MarkCompleted(ctx context.Context, consumerName, messageID string, now time.Time) error
}

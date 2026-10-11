package wager

import "context"

// Repository defines the persistence contract for Wager Transactions.
type Repository interface {
	// Create saves a new transaction.
	Create(ctx context.Context, t *Transaction) error

	// FindByID retrieves a transaction by its internal ID.
	FindByID(ctx context.Context, id string) (*Transaction, error)

	// FindByExternalID retrieves a transaction by its provider and external ID.
	FindByExternalID(ctx context.Context, providerID, externalID string) (*Transaction, error)

	// FindByIdempotencyKey retrieves a transaction by its idempotency key.
	FindByIdempotencyKey(ctx context.Context, key string) (*Transaction, error)

	// FindPendingReferences retrieves transactions waiting for a reference.
	FindPendingReferences(ctx context.Context, limit int) ([]*Transaction, error)

	// HasProcessedReversal checks if a reference transaction has already been reversed by a PROCESSED reversal.
	HasProcessedReversal(ctx context.Context, referenceTransactionID string, excludeTxID string) (bool, error)

	// Update saves changes to an existing transaction.
	Update(ctx context.Context, t *Transaction) error
}

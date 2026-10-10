package port

import "context"

// TxManager defines the contract to execute operations atomically.
// Implementations (e.g., PostgreSQL) will inject the transaction into the
// context so that repositories can participate in the same transaction.
type TxManager interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

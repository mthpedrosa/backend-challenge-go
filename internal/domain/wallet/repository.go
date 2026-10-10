package wallet

import "context"

// Repository defines the persistence contract for Wallet aggregates.
type Repository interface {
	// Create saves a new Wallet.
	Create(ctx context.Context, w *Wallet) error

	// FindByID retrieves a Wallet by its ID.
	FindByID(ctx context.Context, id string) (*Wallet, error)

	// FindByIDForUpdate retrieves a Wallet and locks it for updates (e.g. SELECT FOR UPDATE).
	FindByIDForUpdate(ctx context.Context, id string) (*Wallet, error)

	// FindByPlayerAndCurrency retrieves a Wallet by player and currency.
	FindByPlayerAndCurrency(ctx context.Context, playerID, currency string) (*Wallet, error)

	// Update saves changes to a Wallet. The optimistic locking version must match.
	Update(ctx context.Context, w *Wallet) error
}

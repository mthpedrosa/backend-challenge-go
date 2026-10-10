package ledger

import "context"

// Repository defines the persistence contract for Wallet Ledger Entries.
type Repository interface {
	// Append saves a new entry to the append-only ledger.
	Append(ctx context.Context, entry *Entry) error

	// FindByWalletPaginated retrieves a page of ledger entries for a specific wallet.
	FindByWalletPaginated(ctx context.Context, walletID string, cursor string, limit int) ([]*Entry, error)
}

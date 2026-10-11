package ledger

import "context"

// Repository defines the persistence contract for Wallet Ledger Entries.
type Repository interface {
	// Append saves a new entry to the append-only ledger.
	Append(ctx context.Context, entry *Entry) error

	// FindByWalletPaginated retrieves a page of ledger entries for a specific wallet.
	FindByWalletPaginated(ctx context.Context, walletID string, cursor string, limit int) ([]*Entry, error)

	// CalculateSummary returns the net balance (credits - debits in minor units) and total entry count for a wallet.
	CalculateSummary(ctx context.Context, walletID string) (netBalanceMinor int64, count int64, err error)
}

package postgres

import "go.uber.org/fx"

// Module provides the PostgreSQL implementations of domain repositories and the TxManager.
var Module = fx.Provide(
	NewTxManager,
	NewWalletRepository,
	NewLedgerRepository,
	NewWagerRepository,
	NewOutboxRepository,
	NewInboxRepository,
)

package usecase

import (
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/event"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/ledger"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/port"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/wager"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/wallet"
	"go.uber.org/fx"
)

// UseCaseParams aggregates the dependencies for all use cases to simplify Fx injection.
type UseCaseParams struct {
	fx.In

	TxManager port.TxManager
	Wallets   wallet.Repository
	Wagers    wager.Repository
	Ledgers   ledger.Repository
	Outbox    event.OutboxRepository
}

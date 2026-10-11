package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/event"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/ledger"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/money"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/wager"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/wallet"
	"github.com/mthpedrosa/backend-challenge-go/internal/platform/observability"
)

type OpenWalletInput struct {
	PlayerID       string
	Currency       string
	InitialBalance string
}

type OpenWalletUseCase struct {
	params UseCaseParams
}

func NewOpenWalletUseCase(params UseCaseParams) *OpenWalletUseCase {
	return &OpenWalletUseCase{params: params}
}

func (uc *OpenWalletUseCase) Execute(ctx context.Context, input OpenWalletInput) (*wallet.Wallet, error) {
	now := time.Now().UTC()

	// Parse initial balance
	initialMoney, err := money.Parse(input.InitialBalance, input.Currency)
	if err != nil {
		return nil, err
	}

	walletID := uuid.New().String()

	// Create wallet entity
	w, err := wallet.New(walletID, input.PlayerID, initialMoney, now)
	if err != nil {
		return nil, err
	}

	var (
		openingTx   *wager.Transaction
		ledgerEntry *ledger.Entry
	)

	if initialMoney.IsPositive() {
		// Create the OPENING wager transaction
		openingTxID := uuid.New().String()
		tx, err := wager.NewOpening(openingTxID, walletID, input.PlayerID, initialMoney, now)
		if err != nil {
			return nil, err
		}

		// Opening is automatically processed
		balanceAfter := w.Balance()
		if err := tx.MarkProcessed(&balanceAfter, nil, now); err != nil {
			return nil, err
		}
		openingTx = tx

		zero, _ := money.Zero(input.Currency)
		entry, err := ledger.NewEntry(
			uuid.New().String(),
			w.ID(),
			openingTx.ID(),
			ledger.DirectionCredit,
			initialMoney,
			zero,        // balanceBefore
			w.Balance(), // balanceAfter
			now,
		)
		if err != nil {
			return nil, err
		}
		ledgerEntry = entry
	}

	err = uc.params.TxManager.RunInTx(ctx, func(txCtx context.Context) error {
		// 1. Check if wallet already exists for this player/currency to return 409
		existing, err := uc.params.Wallets.FindByPlayerAndCurrency(txCtx, input.PlayerID, input.Currency)
		if err == nil && existing != nil {
			observability.Metrics.WalletDuplicatedTotal.Inc()
			return domain.WrapDomainError("WALLET_ALREADY_EXISTS", "a wallet for this player and currency already exists", nil)
		}

		// 2. Save Wallet
		if err := uc.params.Wallets.Create(txCtx, w); err != nil {
			return err
		}

		// 3. If initial balance > 0, save OPENING transaction, ledger entry, and outbox events
		if initialMoney.IsPositive() && openingTx != nil && ledgerEntry != nil {
			if err := uc.params.Wagers.Create(txCtx, openingTx); err != nil {
				return err
			}

			if err := uc.params.Ledgers.Append(txCtx, ledgerEntry); err != nil {
				return err
			}

			// Outbox: WagerTransactionProcessed
			processedEnv := event.NewTransactionProcessedEvent(
				uuid.New().String(),
				openingTx,
				"correlation-placeholder",
				nil,
				now,
			)
			if err := uc.params.Outbox.Save(txCtx, processedEnv); err != nil {
				return err
			}

			// Outbox: WalletBalanceChanged
			zero, _ := money.Zero(input.Currency)
			changedEnv := event.NewWalletBalanceChangedEvent(
				uuid.New().String(),
				w.ID(),
				openingTx.ID(),
				ledger.DirectionCredit,
				initialMoney,
				zero, // balanceBefore
				w.Balance(),
				w.Version(),
				"correlation-placeholder",
				nil,
				now,
			)
			if err := uc.params.Outbox.Save(txCtx, changedEnv); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return w, nil
}

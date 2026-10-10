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

	// Create the OPENING wager transaction
	openingTxID := uuid.New().String()
	openingTx, err := wager.NewOpening(openingTxID, walletID, input.PlayerID, initialMoney, now)
	if err != nil {
		return nil, err
	}

	// Opening is automatically processed
	balanceAfter := w.Balance()
	if err := openingTx.MarkProcessed(&balanceAfter, nil, now); err != nil {
		return nil, err
	}

	err = uc.params.TxManager.RunInTx(ctx, func(txCtx context.Context) error {
		// 1. Check if wallet already exists for this player/currency to return 409
		existing, err := uc.params.Wallets.FindByPlayerAndCurrency(txCtx, input.PlayerID, input.Currency)
		if err == nil && existing != nil {
			return domain.WrapDomainError("WALLET_ALREADY_EXISTS", "a wallet for this player and currency already exists", nil)
		}

		// 2. Save Wallet
		if err := uc.params.Wallets.Create(txCtx, w); err != nil {
			return err
		}

		// 3. Save OPENING transaction
		if err := uc.params.Wagers.Create(txCtx, openingTx); err != nil {
			return err
		}

		// 4. If initial balance > 0, we emit an event
		if initialMoney.IsPositive() {
			zero, _ := money.Zero(input.Currency)
			env := event.NewWalletBalanceChangedEvent(
				uuid.New().String(),
				w.ID(),
				openingTxID,
				ledger.DirectionCredit,
				initialMoney,
				zero, // balanceBefore
				w.Balance(),
				w.Version(),
				"correlation-placeholder", // TODO: inject correlation
				nil,
				now,
			)
			if err := uc.params.Outbox.Save(txCtx, env); err != nil {
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

package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain/money"
	"github.com/mthpedrosa/backend-challenge-go/internal/platform/observability"
)

type ReconcileWalletOutput struct {
	WalletID          string
	StoredBalance     money.Money
	CalculatedBalance money.Money
	Difference        money.Money
	Consistent        bool
	CheckedEntries    int64
}

type ReconcileWalletUseCase struct {
	params UseCaseParams
	logger *slog.Logger
}

func NewReconcileWalletUseCase(params UseCaseParams, logger *slog.Logger) *ReconcileWalletUseCase {
	return &ReconcileWalletUseCase{
		params: params,
		logger: logger.With(slog.String("component", "reconcile_wallet_usecase")),
	}
}

func (uc *ReconcileWalletUseCase) Execute(ctx context.Context, walletID string) (*ReconcileWalletOutput, error) {
	w, err := uc.params.Wallets.FindByID(ctx, walletID)
	if err != nil {
		return nil, fmt.Errorf("failed to find wallet for reconciliation: %w", err)
	}

	netMinor, count, err := uc.params.Ledgers.CalculateSummary(ctx, walletID)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate ledger summary: %w", err)
	}

	calculatedMoney, err := money.FromMinor(netMinor, w.Currency())
	if err != nil {
		return nil, fmt.Errorf("failed to construct calculated balance: %w", err)
	}

	diffMinor := w.Balance().MinorUnits() - netMinor
	diffMoney, err := money.FromMinor(diffMinor, w.Currency())
	if err != nil {
		return nil, fmt.Errorf("failed to construct difference: %w", err)
	}

	consistent := (diffMinor == 0)

	if !consistent {
		observability.Metrics.ReconciliationMismatch.Inc()
		uc.logger.WarnContext(ctx, "wallet reconciliation mismatch detected",
			slog.String("wallet_id", walletID),
			slog.Int64("stored_minor", w.Balance().MinorUnits()),
			slog.Int64("calculated_minor", netMinor),
			slog.Int64("diff_minor", diffMinor),
			slog.Int64("checked_entries", count),
		)
	} else {
		uc.logger.InfoContext(ctx, "wallet reconciliation successful",
			slog.String("wallet_id", walletID),
			slog.Int64("balance_minor", w.Balance().MinorUnits()),
			slog.Int64("checked_entries", count),
		)
	}

	return &ReconcileWalletOutput{
		WalletID:          w.ID(),
		StoredBalance:     w.Balance(),
		CalculatedBalance: calculatedMoney,
		Difference:        diffMoney,
		Consistent:        consistent,
		CheckedEntries:    count,
	}, nil
}

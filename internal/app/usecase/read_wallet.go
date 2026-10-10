package usecase

import (
	"context"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain/ledger"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/wallet"
)

type ReadWalletUseCase struct {
	params UseCaseParams
}

func NewReadWalletUseCase(params UseCaseParams) *ReadWalletUseCase {
	return &ReadWalletUseCase{params: params}
}

func (uc *ReadWalletUseCase) Execute(ctx context.Context, walletID string) (*wallet.Wallet, error) {
	return uc.params.Wallets.FindByID(ctx, walletID)
}

type ReadLedgerUseCase struct {
	params UseCaseParams
}

func NewReadLedgerUseCase(params UseCaseParams) *ReadLedgerUseCase {
	return &ReadLedgerUseCase{params: params}
}

func (uc *ReadLedgerUseCase) Execute(ctx context.Context, walletID string, cursor string, limit int) ([]*ledger.Entry, error) {
	return uc.params.Ledgers.FindByWalletPaginated(ctx, walletID, cursor, limit)
}

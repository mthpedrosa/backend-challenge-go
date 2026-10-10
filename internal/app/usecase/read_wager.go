package usecase

import (
	"context"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain/wager"
)

type ReadWagerUseCase struct {
	params UseCaseParams
}

func NewReadWagerUseCase(params UseCaseParams) *ReadWagerUseCase {
	return &ReadWagerUseCase{params: params}
}

func (uc *ReadWagerUseCase) GetByID(ctx context.Context, id string) (*wager.Transaction, error) {
	return uc.params.Wagers.FindByID(ctx, id)
}

func (uc *ReadWagerUseCase) GetByExternalID(ctx context.Context, providerID, externalID string) (*wager.Transaction, error) {
	return uc.params.Wagers.FindByExternalID(ctx, providerID, externalID)
}

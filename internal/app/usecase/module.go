package usecase

import "go.uber.org/fx"

var Module = fx.Module("usecase",
	fx.Provide(
		NewOpenWalletUseCase,
		NewProcessWagerUseCase,
		NewReadWalletUseCase,
		NewReadLedgerUseCase,
		NewReadWagerUseCase,
		NewResolvePendingReferenceUseCase,
		NewReconcileWalletUseCase,
	),
)

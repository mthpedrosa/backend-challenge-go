package worker

import (
	"context"
	"log/slog"
	"time"

	"go.uber.org/fx"

	"github.com/mthpedrosa/backend-challenge-go/internal/app/usecase"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/wager"
)

type PendingReferenceWorker struct {
	wagers wager.Repository
	uc     *usecase.ResolvePendingReferenceUseCase
	logger *slog.Logger

	ticker *time.Ticker
	done   chan struct{}
}

type PendingReferenceWorkerParams struct {
	fx.In

	Wagers wager.Repository
	UC     *usecase.ResolvePendingReferenceUseCase
	Logger *slog.Logger
}

func NewPendingReferenceWorker(lc fx.Lifecycle, p PendingReferenceWorkerParams) *PendingReferenceWorker {
	w := &PendingReferenceWorker{
		wagers: p.Wagers,
		uc:     p.UC,
		logger: p.Logger.With(slog.String("component", "pending_reference_worker")),
		done:   make(chan struct{}),
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			w.ticker = time.NewTicker(2 * time.Second)
			go w.loop()
			w.logger.Info("pending reference worker started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if w.ticker != nil {
				w.ticker.Stop()
				close(w.done)
				w.logger.Info("pending reference worker stopped")
			}
			return nil
		},
	})

	return w
}

func (w *PendingReferenceWorker) loop() {
	for {
		select {
		case <-w.done:
			return
		case <-w.ticker.C:
			w.processBatch()
		}
	}
}

func (w *PendingReferenceWorker) processBatch() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	txs, err := w.wagers.FindPendingReferences(ctx, 20)
	if err != nil {
		w.logger.Error("failed to find pending references", slog.Any("error", err))
		return
	}

	for _, tx := range txs {
		if err := w.uc.Execute(ctx, tx.ID()); err != nil {
			w.logger.Error("failed to resolve pending reference",
				slog.String("txId", tx.ID()),
				slog.Any("error", err),
			)
		} else {
			w.logger.Debug("resolved pending reference successfully", slog.String("txId", tx.ID()))
		}
	}
}

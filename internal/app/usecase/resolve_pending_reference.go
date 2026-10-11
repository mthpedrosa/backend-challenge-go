package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/event"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/ledger"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/wager"
)

type ResolvePendingReferenceUseCase struct {
	params UseCaseParams
}

func NewResolvePendingReferenceUseCase(params UseCaseParams) *ResolvePendingReferenceUseCase {
	return &ResolvePendingReferenceUseCase{params: params}
}

func (uc *ResolvePendingReferenceUseCase) Execute(ctx context.Context, pendingTxID string) error {
	now := time.Now().UTC()

	return uc.params.TxManager.RunInTx(ctx, func(txCtx context.Context) error {
		// Fetch the transaction (could be locked, but we rely on wallet lock anyway)
		pendingTx, err := uc.params.Wagers.FindByID(txCtx, pendingTxID)
		if err != nil {
			return err // Not found or error
		}

		if pendingTx.Status() != wager.StatusPendingReference {
			// Already processed or rejected by another worker run
			return nil
		}

		if pendingTx.ReferenceExternalTransactionID() == nil {
			// Shouldn't happen based on domain rules, but let's be safe
			return fmt.Errorf("transaction %s has no reference external id", pendingTxID)
		}

		// Lock wallet to ensure sequential processing
		w, err := uc.params.Wallets.FindByIDForUpdate(txCtx, pendingTx.WalletID())
		if err != nil {
			return err
		}

		// Look for the reference transaction
		refTx, err := uc.params.Wagers.FindByExternalID(txCtx, *pendingTx.ProviderID(), *pendingTx.ReferenceExternalTransactionID())
		if err != nil {
			if errors.Is(err, domain.NewDomainError("TRANSACTION_NOT_FOUND", "")) {
				// Still not found, update attempts and next_attempt_at
				nextAttempt := now.Add(5 * time.Second) // basic backoff could be exponential

				if pendingTx.ExpiresAt() != nil && now.After(*pendingTx.ExpiresAt()) {
					// Expired, mark as REJECTED with REFERENCE_NOT_FOUND per specification
					if err := pendingTx.MarkRejected(domain.CodeReferenceNotFound, now); err != nil {
						return err
					}
					if err := uc.params.Wagers.Update(txCtx, pendingTx); err != nil {
						return err
					}
					env := event.NewTransactionRejectedEvent(
						uuid.New().String(), pendingTx, *pendingTx.FailureCode(), "worker-correlation", nil, now,
					)
					return uc.params.Outbox.Save(txCtx, env)
				}
				pendingTx.IncrementAttempts(nextAttempt, now)
				return uc.params.Wagers.Update(txCtx, pendingTx)
			}
			return err
		}

		// If the reference is not PROCESSED, it might be REJECTED, FAILED, or PENDING/PENDING_REFERENCE
		if refTx.Status() == wager.StatusPending || refTx.Status() == wager.StatusPendingReference {
			// Reference is still pending itself, we can just backoff
			nextAttempt := now.Add(5 * time.Second)
			if pendingTx.ExpiresAt() != nil && now.After(*pendingTx.ExpiresAt()) {
				if err := pendingTx.MarkRejected(domain.CodeReferenceNotFound, now); err != nil {
					return err
				}
				if err := uc.params.Wagers.Update(txCtx, pendingTx); err != nil {
					return err
				}
				env := event.NewTransactionRejectedEvent(
					uuid.New().String(), pendingTx, *pendingTx.FailureCode(), "worker-correlation", nil, now,
				)
				return uc.params.Outbox.Save(txCtx, env)
			}
			pendingTx.IncrementAttempts(nextAttempt, now)
			return uc.params.Wagers.Update(txCtx, pendingTx)
		}

		// At this point, the reference is terminal (PROCESSED, REJECTED, FAILED).
		// We can now validate the reversal against it.
		if err := pendingTx.ValidateReversalReference(refTx); err != nil {
			pendingTx.MarkRejected(err.(*domain.DomainError).Code, now)
		} else {
			// Check if reference transaction has already been reversed by another transaction
			alreadyReversed, err := uc.params.Wagers.HasProcessedReversal(txCtx, refTx.ID(), pendingTx.ID())
			if err != nil {
				return err
			}
			if alreadyReversed {
				pendingTx.MarkRejected(domain.CodeAlreadyReversed, now)
			} else {
				amt := pendingTx.Amount()
				balanceBefore := w.Balance()
				var ledgerEntry *ledger.Entry

				switch pendingTx.Kind() {
				case wager.KindRefund:
					if err := w.Credit(amt, now); err != nil {
						pendingTx.MarkRejected(err.(*domain.DomainError).Code, now)
					} else {
						balanceAfter := w.Balance()
						refID := refTx.ID()
						pendingTx.MarkProcessed(&balanceAfter, &refID, now)
						ledgerEntry, _ = ledger.NewEntry(
							uuid.New().String(), w.ID(), pendingTx.ID(), ledger.DirectionCredit,
							amt, balanceBefore, w.Balance(), now,
						)
					}
				case wager.KindRollback:
					if refTx.Kind() == wager.KindBet {
						// Undoing a BET (debit) means CREDITING the wallet
						if err := w.Credit(amt, now); err != nil {
							pendingTx.MarkRejected(err.(*domain.DomainError).Code, now)
						} else {
							balanceAfter := w.Balance()
							refID := refTx.ID()
							pendingTx.MarkProcessed(&balanceAfter, &refID, now)
							ledgerEntry, _ = ledger.NewEntry(
								uuid.New().String(), w.ID(), pendingTx.ID(), ledger.DirectionCredit,
								amt, balanceBefore, w.Balance(), now,
							)
						}
					} else if refTx.Kind() == wager.KindWin || refTx.Kind() == wager.KindRefund {
						// Undoing a WIN or REFUND (credit) means DEBITING the wallet
						if err := w.Debit(amt, now); err != nil {
							code := err.(*domain.DomainError).Code
							if code == domain.CodeInsufficientFunds {
								code = domain.CodeReversalInsufficientFunds
							}
							pendingTx.MarkRejected(code, now)
						} else {
							balanceAfter := w.Balance()
							refID := refTx.ID()
							pendingTx.MarkProcessed(&balanceAfter, &refID, now)
							ledgerEntry, _ = ledger.NewEntry(
								uuid.New().String(), w.ID(), pendingTx.ID(), ledger.DirectionDebit,
								amt, balanceBefore, w.Balance(), now,
							)
						}
					} else {
						pendingTx.MarkRejected(domain.CodeInvalidReferenceKind, now)
					}
				default:
					pendingTx.MarkRejected(domain.CodeInvalidKind, now)
				}

				// If it was processed, update wallet and ledger
				if pendingTx.Status() == wager.StatusProcessed && ledgerEntry != nil {
					if err := uc.params.Wallets.Update(txCtx, w); err != nil {
						return err
					}
					if err := uc.params.Ledgers.Append(txCtx, ledgerEntry); err != nil {
						return err
					}

					env := event.NewWalletBalanceChangedEvent(
						uuid.New().String(),
						w.ID(),
						pendingTx.ID(),
						ledgerEntry.Direction(),
						amt,
						balanceBefore,
						w.Balance(),
						w.Version(),
						"worker-correlation", // In a real scenario, preserve correlation id
						nil,
						now,
					)
					if err := uc.params.Outbox.Save(txCtx, env); err != nil {
						return err
					}
				}
			}
		}

		// Update transaction status (PROCESSED, REJECTED, or FAILED)
		if err := uc.params.Wagers.Update(txCtx, pendingTx); err != nil {
			return err
		}

		// Outbox Event
		if pendingTx.Status() == wager.StatusProcessed || pendingTx.Status() == wager.StatusRejected {
			var env event.Envelope
			if pendingTx.Status() == wager.StatusProcessed {
				env = event.NewTransactionProcessedEvent(
					uuid.New().String(), pendingTx, "worker-correlation", nil, now,
				)
			} else {
				env = event.NewTransactionRejectedEvent(
					uuid.New().String(), pendingTx, *pendingTx.FailureCode(), "worker-correlation", nil, now,
				)
			}
			if err := uc.params.Outbox.Save(txCtx, env); err != nil {
				return err
			}
		}

		return nil
	})
}

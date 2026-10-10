package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/event"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/ledger"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/money"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/wager"
	"github.com/mthpedrosa/backend-challenge-go/internal/platform/observability"
)

type ProcessWagerInput struct {
	ProviderID            string
	ExternalTransactionID string
	IdempotencyKey        string
	PlayerID              string
	WalletID              string
	Currency              string
	Amount                string
	Kind                  string
	RoundID               string
	GameID                string
	ReferenceExternalID   *string
}

// GeneratePayloadHash creates a SHA-256 hash of the JSON representation of the input.
func (i *ProcessWagerInput) GeneratePayloadHash() string {
	b, _ := json.Marshal(i)
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:])
}

type ProcessWagerUseCase struct {
	params UseCaseParams
}

func NewProcessWagerUseCase(params UseCaseParams) *ProcessWagerUseCase {
	return &ProcessWagerUseCase{params: params}
}

func (uc *ProcessWagerUseCase) Execute(ctx context.Context, input ProcessWagerInput) (*wager.Transaction, error) {
	now := time.Now().UTC()
	payloadHash := input.GeneratePayloadHash()

	var resultTx *wager.Transaction

	err := uc.params.TxManager.RunInTx(ctx, func(txCtx context.Context) error {
		// 1. Idempotency Check by IdempotencyKey
		existingByIdem, err := uc.params.Wagers.FindByIdempotencyKey(txCtx, input.IdempotencyKey)
		if err != nil && !errors.Is(err, domain.NewDomainError("TRANSACTION_NOT_FOUND", "")) {
			return err
		}
		if existingByIdem != nil {
			if existingByIdem.ProviderID() != nil && *existingByIdem.ProviderID() == input.ProviderID {
				if existingByIdem.PayloadHash() != nil && *existingByIdem.PayloadHash() != payloadHash {
					observability.Metrics.IdempotencyConflictTotal.Inc()
					return domain.WrapDomainError("IDEMPOTENCY_CONFLICT", "idempotency key reused with different payload", nil)
				}
				resultTx = existingByIdem
				return nil
			}
		}

		// 2. Idempotency Check by ExternalTransactionID
		existingByExt, err := uc.params.Wagers.FindByExternalID(txCtx, input.ProviderID, input.ExternalTransactionID)
		if err != nil && !errors.Is(err, domain.NewDomainError("TRANSACTION_NOT_FOUND", "")) {
			return err
		}
		if existingByExt != nil {
			if existingByExt.PayloadHash() != nil && *existingByExt.PayloadHash() != payloadHash {
				observability.Metrics.IdempotencyConflictTotal.Inc()
				return domain.WrapDomainError("IDEMPOTENCY_CONFLICT", "external transaction id reused with different payload", nil)
			}
			resultTx = existingByExt
			return nil
		}

		// 3. Lock Wallet
		w, err := uc.params.Wallets.FindByIDForUpdate(txCtx, input.WalletID)
		if err != nil {
			return err
		}

		if w.PlayerID() != input.PlayerID {
			return domain.WrapDomainError("PLAYER_WALLET_MISMATCH", "wallet does not belong to the player", nil)
		}

		amt, err := money.Parse(input.Amount, input.Currency)
		if err != nil {
			return err
		}

		// 4. Resolve reference transaction if needed (Refund, Rollback)
		var refTxID *string
		if input.ReferenceExternalID != nil {
			refTx, err := uc.params.Wagers.FindByExternalID(txCtx, input.ProviderID, *input.ReferenceExternalID)
			if err != nil {
				if errors.Is(err, domain.NewDomainError("TRANSACTION_NOT_FOUND", "")) {
					// Pending reference flow
					return uc.handlePendingReference(txCtx, input, amt, w.ID(), payloadHash, now)
				}
				return err
			}

			if refTx.Status() == wager.StatusRejected || refTx.Status() == wager.StatusFailed {
				return domain.WrapDomainError("REFERENCE_NOT_PROCESSED", "reference transaction was not processed successfully", nil)
			}

			if refTx.Status() == wager.StatusPending || refTx.Status() == wager.StatusPendingReference {
				return uc.handlePendingReference(txCtx, input, amt, w.ID(), payloadHash, now)
			}

			id := refTx.ID()
			refTxID = &id
		}

		// 5. Create new Transaction entity
		txID := uuid.New().String()
		tx, err := wager.NewExternal(wager.NewExternalParams{
			ID:                             txID,
			WalletID:                       w.ID(),
			PlayerID:                       w.PlayerID(),
			Kind:                           wager.Kind(input.Kind),
			Amount:                         amt,
			ProviderID:                     input.ProviderID,
			ExternalTransactionID:          input.ExternalTransactionID,
			IdempotencyKey:                 input.IdempotencyKey,
			PayloadHash:                    payloadHash,
			RoundID:                        input.RoundID,
			GameID:                         input.GameID,
			ReferenceExternalTransactionID: input.ReferenceExternalID,
			Now:                            now,
		})
		if err != nil {
			return err
		}

		// Validate reversal if applicable
		if tx.Kind() == wager.KindRefund || tx.Kind() == wager.KindRollback {
			// Actually we need the referenced transaction entity
			refTx, err := uc.params.Wagers.FindByExternalID(txCtx, input.ProviderID, *input.ReferenceExternalID)
			if err == nil {
				if err := tx.ValidateReversalReference(refTx); err != nil {
					tx.MarkRejected(err.(*domain.DomainError).Code, now)
				}
			}
		}

		// 6. Apply Domain Logic (Debit / Credit)
		var ledgerEntry *ledger.Entry
		balanceBefore := w.Balance()

		if tx.Status() == wager.StatusPending { // Only process if not already rejected
			switch tx.Kind() {
			case wager.KindBet, wager.KindRollback:
				if err := w.Debit(amt, now); err != nil {
					code := err.(*domain.DomainError).Code
					// If rollback fails due to funds, it's a specific code
					if code == domain.CodeInsufficientFunds && tx.Kind() == wager.KindRollback {
						code = domain.CodeReversalInsufficientFunds
					}
					tx.MarkRejected(code, now)
				} else {
					balanceAfter := w.Balance()
					tx.MarkProcessed(&balanceAfter, refTxID, now)
					ledgerEntry, _ = ledger.NewEntry(
						uuid.New().String(), w.ID(), tx.ID(), ledger.DirectionDebit,
						amt, balanceBefore, w.Balance(), now,
					)
				}
			case wager.KindWin, wager.KindRefund:
				if err := w.Credit(amt, now); err != nil {
					tx.MarkRejected(err.(*domain.DomainError).Code, now)
				} else {
					balanceAfter := w.Balance()
					tx.MarkProcessed(&balanceAfter, refTxID, now)
					ledgerEntry, _ = ledger.NewEntry(
						uuid.New().String(), w.ID(), tx.ID(), ledger.DirectionCredit,
						amt, balanceBefore, w.Balance(), now,
					)
				}
			case wager.KindLoss:
				balanceAfter := w.Balance()
				tx.MarkProcessed(&balanceAfter, refTxID, now)
			}
		}

		// 7. Persist Transaction
		if err := uc.params.Wagers.Create(txCtx, tx); err != nil {
			return err // If unique constraints fail here (e.g. unique reversal), it will rollback
		}

		// 8. If processed and wallet changed, persist Wallet + Ledger + Event
		if tx.Status() == wager.StatusProcessed && ledgerEntry != nil {
			if err := uc.params.Wallets.Update(txCtx, w); err != nil {
				return err
			}
			if err := uc.params.Ledgers.Append(txCtx, ledgerEntry); err != nil {
				return err
			}

			env := event.NewWalletBalanceChangedEvent(
				uuid.New().String(),
				w.ID(),
				tx.ID(),
				ledgerEntry.Direction(),
				amt,
				balanceBefore,
				w.Balance(),
				w.Version(),
				"correlation-placeholder",
				nil,
				now,
			)
			if err := uc.params.Outbox.Save(txCtx, env); err != nil {
				return err
			}
		}

		// Outbox: Transaction Processed or Rejected
		var env event.Envelope
		if tx.Status() == wager.StatusProcessed {
			env = event.NewTransactionProcessedEvent(
				uuid.New().String(), tx, "correlation-placeholder", nil, now,
			)
		} else {
			env = event.NewTransactionRejectedEvent(
				uuid.New().String(), tx, *tx.FailureCode(), "correlation-placeholder", nil, now,
			)
		}
		if err := uc.params.Outbox.Save(txCtx, env); err != nil {
			return err
		}

		resultTx = tx
		return nil
	})

	if err != nil {
		return nil, err
	}

	if resultTx != nil {
		if resultTx.Status() == wager.StatusProcessed {
			observability.Metrics.TransactionProcessedTotal.WithLabelValues(string(resultTx.Kind()), "EXTERNAL").Inc()
		} else if resultTx.Status() == wager.StatusRejected {
			code := ""
			if resultTx.FailureCode() != nil {
				code = string(*resultTx.FailureCode())
			}
			observability.Metrics.TransactionRejectedTotal.WithLabelValues(string(resultTx.Kind()), code).Inc()
		}
	}

	return resultTx, nil
}

func (uc *ProcessWagerUseCase) handlePendingReference(txCtx context.Context, input ProcessWagerInput, amt money.Money, walletID, payloadHash string, now time.Time) error {
	txID := uuid.New().String()
	tx, err := wager.NewExternal(wager.NewExternalParams{
		ID:                             txID,
		WalletID:                       walletID,
		PlayerID:                       input.PlayerID,
		Kind:                           wager.Kind(input.Kind),
		Amount:                         amt,
		ProviderID:                     input.ProviderID,
		ExternalTransactionID:          input.ExternalTransactionID,
		IdempotencyKey:                 input.IdempotencyKey,
		PayloadHash:                    payloadHash,
		RoundID:                        input.RoundID,
		GameID:                         input.GameID,
		ReferenceExternalTransactionID: input.ReferenceExternalID,
		Now:                            now,
	})
	if err != nil {
		return err
	}

	// Will set nextAttemptAt and expiresAt logic
	nextAttempt := now.Add(1 * time.Second)
	expiresAt := now.Add(30 * time.Minute)
	tx.MarkPendingReference(nextAttempt, expiresAt, now)

	if err := uc.params.Wagers.Create(txCtx, tx); err != nil {
		return err
	}

	env := event.NewTransactionPendingReferenceEvent(
		uuid.New().String(), tx, "correlation-placeholder", nil, now,
	)
	if err := uc.params.Outbox.Save(txCtx, env); err != nil {
		return err
	}

	return nil
}

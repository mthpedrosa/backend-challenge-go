package wager

import (
	"errors"
	"testing"
	"time"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/money"
)

func validExternalParams(kind Kind, amount money.Money) NewExternalParams {
	ref := "ref-ext-1"
	return NewExternalParams{
		ID:                             "tx-1",
		WalletID:                       "w-1",
		PlayerID:                       "p-1",
		Kind:                           kind,
		Amount:                         amount,
		ProviderID:                     "provider-a",
		ExternalTransactionID:          "ext-1",
		IdempotencyKey:                 "provider-a:ext-1",
		PayloadHash:                    "hash-123",
		RoundID:                        "round-1",
		GameID:                         "game-1",
		ReferenceExternalTransactionID: &ref,
		Now:                            time.Now(),
	}
}

func TestNewOpening(t *testing.T) {
	now := time.Now()
	amount, _ := money.Parse("100.00", money.CurrencyBRL)

	tx, err := NewOpening("tx-open", "w-1", "p-1", amount, now)
	if err != nil {
		t.Fatalf("unexpected error creating opening tx: %v", err)
	}

	if tx.Origin() != OriginInternal || tx.Kind() != KindOpening || tx.Status() != StatusPending {
		t.Errorf("opening state mismatch: %+v", tx)
	}
	if tx.ProviderID() != nil || tx.IdempotencyKey() != nil {
		t.Errorf("internal opening must not carry external metadata")
	}
}

func TestNewExternal_KindsValidation(t *testing.T) {
	m25, _ := money.Parse("25.00", money.CurrencyBRL)
	m0, _ := money.Zero(money.CurrencyBRL)

	// OPENING as external is rejected
	pOpening := validExternalParams(KindOpening, m25)
	if _, err := NewExternal(pOpening); err == nil {
		t.Error("expected error for external OPENING")
	}

	// LOSS with amount > 0 is rejected
	pLossInvalid := validExternalParams(KindLoss, m25)
	if _, err := NewExternal(pLossInvalid); err == nil {
		t.Error("expected error for LOSS with amount > 0")
	}

	// LOSS with amount == 0 succeeds
	pLossValid := validExternalParams(KindLoss, m0)
	txLoss, err := NewExternal(pLossValid)
	if err != nil {
		t.Fatalf("unexpected error creating valid LOSS: %v", err)
	}
	if !txLoss.Amount().IsZero() {
		t.Errorf("expected zero amount for LOSS")
	}

	// BET with positive amount succeeds
	pBet := validExternalParams(KindBet, m25)
	pBet.ReferenceExternalTransactionID = nil
	if _, err := NewExternal(pBet); err != nil {
		t.Fatalf("unexpected error creating BET: %v", err)
	}

	// REFUND without reference fails
	pRefundNoRef := validExternalParams(KindRefund, m25)
	pRefundNoRef.ReferenceExternalTransactionID = nil
	if _, err := NewExternal(pRefundNoRef); err == nil {
		t.Error("expected error for REFUND without reference")
	}

	// ROLLBACK without reference fails
	pRollbackNoRef := validExternalParams(KindRollback, m25)
	pRollbackNoRef.ReferenceExternalTransactionID = nil
	if _, err := NewExternal(pRollbackNoRef); err == nil {
		t.Error("expected error for ROLLBACK without reference")
	}
}

func TestStateTransitions(t *testing.T) {
	m25, _ := money.Parse("25.00", money.CurrencyBRL)
	p := validExternalParams(KindBet, m25)
	tx, _ := NewExternal(p)

	now := time.Now()
	next := now.Add(time.Minute)
	exp := now.Add(30 * time.Minute)

	// PENDING -> PENDING_REFERENCE
	if err := tx.MarkPendingReference(next, exp, now); err != nil {
		t.Fatalf("failed transition to PENDING_REFERENCE: %v", err)
	}
	if tx.Status() != StatusPendingReference {
		t.Errorf("status = %s, want PENDING_REFERENCE", tx.Status())
	}

	// PENDING_REFERENCE -> PROCESSED
	balAfter, _ := money.Parse("75.00", money.CurrencyBRL)
	if err := tx.MarkProcessed(&balAfter, nil, now); err != nil {
		t.Fatalf("failed transition to PROCESSED: %v", err)
	}
	if tx.Status() != StatusProcessed {
		t.Errorf("status = %s, want PROCESSED", tx.Status())
	}

	// Terminal state cannot transition
	if err := tx.MarkRejected("SOME_CODE", now); err == nil {
		t.Error("expected error transitioning from PROCESSED to REJECTED")
	}
	if err := tx.MarkFailed(now); err == nil {
		t.Error("expected error transitioning from PROCESSED to FAILED")
	}
}

func TestValidateReversalReference(t *testing.T) {
	now := time.Now()
	m50, _ := money.Parse("50.00", money.CurrencyBRL)
	m30, _ := money.Parse("30.00", money.CurrencyBRL)

	// Create processed BET as reference
	pBet := validExternalParams(KindBet, m50)
	pBet.ID = "tx-bet"
	pBet.ExternalTransactionID = "ext-bet"
	pBet.ReferenceExternalTransactionID = nil
	betTx, _ := NewExternal(pBet)
	_ = betTx.MarkProcessed(&m50, nil, now)

	// Valid REFUND referencing BET
	pRefund := validExternalParams(KindRefund, m50)
	pRefund.ID = "tx-refund"
	pRefund.ReferenceExternalTransactionID = &pBet.ExternalTransactionID
	refundTx, _ := NewExternal(pRefund)

	if err := refundTx.ValidateReversalReference(betTx); err != nil {
		t.Fatalf("valid refund rejected reference: %v", err)
	}

	// Valid ROLLBACK referencing BET
	pRollback := validExternalParams(KindRollback, m50)
	pRollback.ID = "tx-rb"
	pRollback.ReferenceExternalTransactionID = &pBet.ExternalTransactionID
	rbTx, _ := NewExternal(pRollback)

	if err := rbTx.ValidateReversalReference(betTx); err != nil {
		t.Fatalf("valid rollback rejected reference: %v", err)
	}

	// Failure: Amount mismatch
	pRefundWrongAmt := validExternalParams(KindRefund, m30)
	refundWrongAmt, _ := NewExternal(pRefundWrongAmt)
	err := refundWrongAmt.ValidateReversalReference(betTx)
	if err == nil {
		t.Fatal("expected error on amount mismatch")
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) || domErr.Code != domain.CodeAmountMismatch {
		t.Errorf("expected AMOUNT_MISMATCH, got %v", err)
	}

	// Failure: Reference not PROCESSED
	pendingBet, _ := NewExternal(pBet)
	err = refundTx.ValidateReversalReference(pendingBet)
	if err == nil || !errors.As(err, &domErr) || domErr.Code != domain.CodeReferenceNotProcessed {
		t.Errorf("expected REFERENCE_NOT_PROCESSED, got %v", err)
	}

	// Failure: Player mismatch
	pBetDiffPlayer := validExternalParams(KindBet, m50)
	pBetDiffPlayer.PlayerID = "other-player"
	betDiffPlayer, _ := NewExternal(pBetDiffPlayer)
	_ = betDiffPlayer.MarkProcessed(&m50, nil, now)
	err = refundTx.ValidateReversalReference(betDiffPlayer)
	if err == nil || !errors.As(err, &domErr) || domErr.Code != domain.CodeReferenceMismatch {
		t.Errorf("expected REFERENCE_MISMATCH for player, got %v", err)
	}

	// Failure: REFUND on a WIN transaction
	pWin := validExternalParams(KindWin, m50)
	winTx, _ := NewExternal(pWin)
	_ = winTx.MarkProcessed(&m50, nil, now)
	err = refundTx.ValidateReversalReference(winTx)
	if err == nil || !errors.As(err, &domErr) || domErr.Code != domain.CodeInvalidReferenceKind {
		t.Errorf("expected INVALID_REFERENCE_KIND when refunding a WIN, got %v", err)
	}
}

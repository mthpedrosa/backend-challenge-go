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

func TestRehydrateAndGetters(t *testing.T) {
	now := time.Now()
	next := now.Add(time.Minute)
	exp := now.Add(10 * time.Minute)
	refExt := "ref-ext-1"
	m100, _ := money.Parse("100.00", money.CurrencyBRL)
	m50, _ := money.Parse("50.00", money.CurrencyBRL)

	tx := Rehydrate(
		"tx-1", OriginExternal, KindBet, StatusPendingReference,
		"w-1", "p-1", m100,
		func() *string { p := "provider-a"; return &p }(),
		func() *string { e := "ext-1"; return &e }(),
		func() *string { i := "idemp-1"; return &i }(),
		func() *string { h := "hash-1"; return &h }(),
		func() *string { r := "round-1"; return &r }(),
		func() *string { g := "game-1"; return &g }(),
		&refExt,
		func() *string { rt := "ref-tx-1"; return &rt }(),
		func() *string { fc := "FAIL_01"; return &fc }(),
		&m50,
		3, &next, &exp, now, now,
	)

	// Test all getters
	if tx.ID() != "tx-1" {
		t.Errorf("ID mismatch")
	}
	if tx.Origin() != OriginExternal {
		t.Errorf("Origin mismatch")
	}
	if tx.Kind() != KindBet {
		t.Errorf("Kind mismatch")
	}
	if tx.Status() != StatusPendingReference {
		t.Errorf("Status mismatch")
	}
	if tx.WalletID() != "w-1" {
		t.Errorf("WalletID mismatch")
	}
	if tx.PlayerID() != "p-1" {
		t.Errorf("PlayerID mismatch")
	}
	if tx.Amount() != m100 {
		t.Errorf("Amount mismatch")
	}
	if *tx.ProviderID() != "provider-a" {
		t.Errorf("ProviderID mismatch")
	}
	if *tx.ExternalTransactionID() != "ext-1" {
		t.Errorf("ExternalTransactionID mismatch")
	}
	if *tx.IdempotencyKey() != "idemp-1" {
		t.Errorf("IdempotencyKey mismatch")
	}
	if *tx.PayloadHash() != "hash-1" {
		t.Errorf("PayloadHash mismatch")
	}
	if *tx.RoundID() != "round-1" {
		t.Errorf("RoundID mismatch")
	}
	if *tx.GameID() != "game-1" {
		t.Errorf("GameID mismatch")
	}
	if *tx.ReferenceExternalTransactionID() != "ref-ext-1" {
		t.Errorf("ReferenceExternalTransactionID mismatch")
	}
	if *tx.ReferenceTransactionID() != "ref-tx-1" {
		t.Errorf("ReferenceTransactionID mismatch")
	}
	if *tx.FailureCode() != "FAIL_01" {
		t.Errorf("FailureCode mismatch")
	}
	if tx.BalanceAfter() == nil || *tx.BalanceAfter() != m50 {
		t.Errorf("BalanceAfter mismatch")
	}
	if tx.Attempts() != 3 {
		t.Errorf("Attempts mismatch")
	}
	if tx.NextAttemptAt() == nil || !tx.NextAttemptAt().Equal(next) {
		t.Errorf("NextAttemptAt mismatch")
	}
	if tx.ExpiresAt() == nil || !tx.ExpiresAt().Equal(exp) {
		t.Errorf("ExpiresAt mismatch")
	}
	if !tx.CreatedAt().Equal(now) {
		t.Errorf("CreatedAt mismatch")
	}
	if !tx.UpdatedAt().Equal(now) {
		t.Errorf("UpdatedAt mismatch")
	}
	if tx.Status().IsTerminal() {
		t.Errorf("Should not be terminal")
	}
}

func TestRemainingTransitions(t *testing.T) {
	now := time.Now()
	m25, _ := money.Parse("25.00", money.CurrencyBRL)

	t.Run("MarkRejected", func(t *testing.T) {
		p := validExternalParams(KindBet, m25)
		tx, _ := NewExternal(p)

		err := tx.MarkRejected("INSUFFICIENT_FUNDS", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tx.Status() != StatusRejected {
			t.Errorf("want REJECTED")
		}
		if *tx.FailureCode() != "INSUFFICIENT_FUNDS" {
			t.Errorf("want failure code")
		}
		if !tx.Status().IsTerminal() {
			t.Errorf("want terminal")
		}
	})

	t.Run("MarkFailed", func(t *testing.T) {
		p := validExternalParams(KindBet, m25)
		tx, _ := NewExternal(p)

		err := tx.MarkFailed(now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tx.Status() != StatusFailed {
			t.Errorf("want FAILED")
		}
		if !tx.Status().IsTerminal() {
			t.Errorf("want terminal")
		}
	})

	t.Run("IncrementAttempts", func(t *testing.T) {
		p := validExternalParams(KindBet, m25)
		tx, _ := NewExternal(p)

		next := now.Add(5 * time.Minute)
		tx.IncrementAttempts(next, now)

		if tx.Attempts() != 1 {
			t.Errorf("want 1 attempt")
		}
		if !tx.NextAttemptAt().Equal(next) {
			t.Errorf("want next attempt")
		}
	})
}

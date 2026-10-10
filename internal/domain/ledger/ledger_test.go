package ledger

import (
	"errors"
	"testing"
	"time"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/money"
)

func TestNewEntry_CreditSuccess(t *testing.T) {
	now := time.Now()
	amount, _ := money.Parse("25.00", money.CurrencyBRL)
	before, _ := money.Parse("100.00", money.CurrencyBRL)
	after, _ := money.Parse("125.00", money.CurrencyBRL)

	entry, err := NewEntry("e-1", "w-1", "tx-1", DirectionCredit, amount, before, after, now)
	if err != nil {
		t.Fatalf("unexpected error creating credit entry: %v", err)
	}

	if entry.Direction() != DirectionCredit {
		t.Errorf("direction = %s, want CREDIT", entry.Direction())
	}
	if !entry.Amount().Equal(amount) || !entry.BalanceBefore().Equal(before) || !entry.BalanceAfter().Equal(after) {
		t.Errorf("values mismatch in entry: %+v", entry)
	}
}

func TestNewEntry_DebitSuccess(t *testing.T) {
	now := time.Now()
	amount, _ := money.Parse("25.00", money.CurrencyBRL)
	before, _ := money.Parse("100.00", money.CurrencyBRL)
	after, _ := money.Parse("75.00", money.CurrencyBRL)

	entry, err := NewEntry("e-1", "w-1", "tx-1", DirectionDebit, amount, before, after, now)
	if err != nil {
		t.Fatalf("unexpected error creating debit entry: %v", err)
	}

	if entry.Direction() != DirectionDebit {
		t.Errorf("direction = %s, want DEBIT", entry.Direction())
	}
}

func TestNewEntry_InvariantViolations(t *testing.T) {
	now := time.Now()
	amount, _ := money.Parse("25.00", money.CurrencyBRL)
	before, _ := money.Parse("100.00", money.CurrencyBRL)
	wrongAfter, _ := money.Parse("150.00", money.CurrencyBRL) // should be 125 for credit, 75 for debit

	// Credit invariant violation
	_, err := NewEntry("e-1", "w-1", "tx-1", DirectionCredit, amount, before, wrongAfter, now)
	if err == nil {
		t.Fatal("expected error on credit invariant violation, got nil")
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) || domErr.Code != domain.CodeInvalidLedgerEntry {
		t.Errorf("expected INVALID_LEDGER_ENTRY, got %v", err)
	}

	// Debit invariant violation
	_, err = NewEntry("e-1", "w-1", "tx-1", DirectionDebit, amount, before, wrongAfter, now)
	if err == nil {
		t.Fatal("expected error on debit invariant violation, got nil")
	}
	if !errors.As(err, &domErr) || domErr.Code != domain.CodeInvalidLedgerEntry {
		t.Errorf("expected INVALID_LEDGER_ENTRY, got %v", err)
	}
}

func TestNewEntry_Validation(t *testing.T) {
	now := time.Now()
	amount, _ := money.Parse("25.00", money.CurrencyBRL)
	before, _ := money.Parse("100.00", money.CurrencyBRL)
	after, _ := money.Parse("125.00", money.CurrencyBRL)

	// Empty IDs
	if _, err := NewEntry("", "w-1", "tx-1", DirectionCredit, amount, before, after, now); err == nil {
		t.Error("expected error for empty id")
	}
	if _, err := NewEntry("e-1", "", "tx-1", DirectionCredit, amount, before, after, now); err == nil {
		t.Error("expected error for empty walletId")
	}
	if _, err := NewEntry("e-1", "w-1", "", DirectionCredit, amount, before, after, now); err == nil {
		t.Error("expected error for empty transactionId")
	}

	// Invalid direction
	if _, err := NewEntry("e-1", "w-1", "tx-1", "TRANSFER", amount, before, after, now); err == nil {
		t.Error("expected error for invalid direction")
	}

	// Currency mismatch
	usdBefore, _ := money.Parse("100.00", money.CurrencyUSD)
	if _, err := NewEntry("e-1", "w-1", "tx-1", DirectionCredit, amount, usdBefore, after, now); err == nil {
		t.Error("expected error for currency mismatch")
	}
}

func TestRehydrateEntry(t *testing.T) {
	now := time.Now()
	amount, _ := money.Parse("25.00", money.CurrencyBRL)
	before, _ := money.Parse("100.00", money.CurrencyBRL)
	after, _ := money.Parse("125.00", money.CurrencyBRL)

	entry, err := RehydrateEntry("e-1", 42, "w-1", "tx-1", DirectionCredit, amount, before, after, now)
	if err != nil {
		t.Fatalf("unexpected error rehydrating: %v", err)
	}
	if entry.Seq() != 42 {
		t.Errorf("seq = %d, want 42", entry.Seq())
	}
}

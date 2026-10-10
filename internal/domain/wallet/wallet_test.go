package wallet

import (
	"errors"
	"testing"
	"time"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/money"
)

func TestNewWallet_Success(t *testing.T) {
	now := time.Now()
	initBal, _ := money.Parse("100.00", money.CurrencyBRL)

	w, err := New("w-1", "p-1", initBal, now)
	if err != nil {
		t.Fatalf("unexpected error creating wallet: %v", err)
	}

	if w.ID() != "w-1" || w.PlayerID() != "p-1" || w.Currency() != money.CurrencyBRL {
		t.Errorf("incorrect wallet identity: %+v", w)
	}
	if w.Version() != 1 {
		t.Errorf("initial version = %d, want 1", w.Version())
	}
	if !w.Balance().Equal(initBal) {
		t.Errorf("balance = %v, want %v", w.Balance(), initBal)
	}
}

func TestNewWallet_Validation(t *testing.T) {
	now := time.Now()
	bal, _ := money.Parse("10.00", money.CurrencyBRL)
	negBal, _ := money.FromMinor(-100, money.CurrencyBRL)

	// Empty wallet id
	if _, err := New("", "p-1", bal, now); err == nil {
		t.Error("expected error for empty wallet ID")
	}

	// Empty player id
	if _, err := New("w-1", "", bal, now); err == nil {
		t.Error("expected error for empty player ID")
	}

	// Negative balance
	if _, err := New("w-1", "p-1", negBal, now); err == nil {
		t.Error("expected error for negative initial balance")
	}
}

func TestWallet_Debit(t *testing.T) {
	now := time.Now()
	initBal, _ := money.Parse("100.00", money.CurrencyBRL)
	w, _ := New("w-1", "p-1", initBal, now)

	// Valid debit
	debitAmount, _ := money.Parse("40.00", money.CurrencyBRL)
	later := now.Add(time.Minute)
	if err := w.Debit(debitAmount, later); err != nil {
		t.Fatalf("unexpected error on debit: %v", err)
	}

	if w.Balance().FormatDecimal() != "60.00" {
		t.Errorf("balance after debit = %s, want 60.00", w.Balance().FormatDecimal())
	}
	if w.Version() != 2 {
		t.Errorf("version = %d, want 2", w.Version())
	}
	if !w.UpdatedAt().Equal(later.UTC()) {
		t.Errorf("updatedAt not updated")
	}

	// Insufficient funds
	excessDebit, _ := money.Parse("80.00", money.CurrencyBRL)
	err := w.Debit(excessDebit, later)
	if err == nil {
		t.Fatal("expected error on debit with insufficient funds, got nil")
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) || domErr.Code != domain.CodeInsufficientFunds {
		t.Errorf("expected code INSUFFICIENT_FUNDS, got %v", err)
	}
	// Balance and version must remain unchanged
	if w.Balance().FormatDecimal() != "60.00" || w.Version() != 2 {
		t.Errorf("state modified on failed debit")
	}

	// Currency mismatch
	usdAmount, _ := money.Parse("10.00", money.CurrencyUSD)
	if err := w.Debit(usdAmount, later); err == nil {
		t.Error("expected currency mismatch error on debit")
	}

	// Zero amount debit
	zeroAmount, _ := money.Zero(money.CurrencyBRL)
	if err := w.Debit(zeroAmount, later); err == nil {
		t.Error("expected error for zero debit amount")
	}
}

func TestWallet_Credit(t *testing.T) {
	now := time.Now()
	initBal, _ := money.Parse("50.00", money.CurrencyBRL)
	w, _ := New("w-1", "p-1", initBal, now)

	creditAmount, _ := money.Parse("25.00", money.CurrencyBRL)
	later := now.Add(time.Minute)
	if err := w.Credit(creditAmount, later); err != nil {
		t.Fatalf("unexpected error on credit: %v", err)
	}

	if w.Balance().FormatDecimal() != "75.00" {
		t.Errorf("balance after credit = %s, want 75.00", w.Balance().FormatDecimal())
	}
	if w.Version() != 2 {
		t.Errorf("version = %d, want 2", w.Version())
	}

	// Currency mismatch
	usdAmount, _ := money.Parse("10.00", money.CurrencyUSD)
	if err := w.Credit(usdAmount, later); err == nil {
		t.Error("expected currency mismatch error on credit")
	}

	// Zero amount credit
	zeroAmount, _ := money.Zero(money.CurrencyBRL)
	if err := w.Credit(zeroAmount, later); err == nil {
		t.Error("expected error for zero credit amount")
	}
}

func TestRehydrate_SuccessAndFailure(t *testing.T) {
	now := time.Now()
	bal, _ := money.Parse("50.00", money.CurrencyBRL)

	w, err := Rehydrate("w-1", "p-1", money.CurrencyBRL, bal, 5, now, now)
	if err != nil {
		t.Fatalf("unexpected error on rehydrate: %v", err)
	}
	if w.Version() != 5 || w.Balance().FormatDecimal() != "50.00" {
		t.Errorf("rehydrated wallet fields incorrect: %+v", w)
	}

	// Version < 1
	if _, err := Rehydrate("w-1", "p-1", money.CurrencyBRL, bal, 0, now, now); err == nil {
		t.Error("expected error for rehydrating with version < 1")
	}

	// Currency mismatch
	if _, err := Rehydrate("w-1", "p-1", money.CurrencyUSD, bal, 1, now, now); err == nil {
		t.Error("expected error for rehydrating with mismatched currency")
	}
}

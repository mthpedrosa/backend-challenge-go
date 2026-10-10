package wallet

import (
	"fmt"
	"strings"
	"time"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/money"
)

// Wallet is the financial aggregate root.
// It encapsulates player balance, optimistic concurrency version, and mutation invariants.
type Wallet struct {
	id        string
	playerID  string
	currency  string
	balance   money.Money
	version   int64
	createdAt time.Time
	updatedAt time.Time
}

// New creates a new wallet with initial version 1 and invariant enforcement.
// Rejects empty identifiers or negative initial balances.
func New(id, playerID string, initialBalance money.Money, now time.Time) (*Wallet, error) {
	if strings.TrimSpace(id) == "" {
		return nil, domain.WrapDomainError(
			domain.CodeInvalidWalletState,
			"wallet id must not be empty",
			nil,
		)
	}
	if strings.TrimSpace(playerID) == "" {
		return nil, domain.WrapDomainError(
			domain.CodeInvalidWalletState,
			"player id must not be empty",
			nil,
		)
	}
	if initialBalance.IsNegative() {
		return nil, domain.WrapDomainError(
			domain.CodeInvalidAmount,
			"initial balance must not be negative",
			nil,
		)
	}

	return &Wallet{
		id:        id,
		playerID:  playerID,
		currency:  initialBalance.Currency(),
		balance:   initialBalance,
		version:   1,
		createdAt: now.UTC(),
		updatedAt: now.UTC(),
	}, nil
}

// Rehydrate reconstructs an existing wallet entity from persistence.
// It does not re-apply business rules or emit domain events.
func Rehydrate(id, playerID, currency string, balance money.Money, version int64, createdAt, updatedAt time.Time) (*Wallet, error) {
	if strings.TrimSpace(id) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidWalletState, "wallet id must not be empty", nil)
	}
	if strings.TrimSpace(playerID) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidWalletState, "player id must not be empty", nil)
	}
	if balance.Currency() != currency {
		return nil, domain.WrapDomainError(
			domain.CodeCurrencyMismatch,
			fmt.Sprintf("balance currency %s does not match wallet currency %s", balance.Currency(), currency),
			nil,
		)
	}
	if version < 1 {
		return nil, domain.WrapDomainError(domain.CodeInvalidWalletState, "wallet version must be >= 1", nil)
	}
	if balance.IsNegative() {
		return nil, domain.WrapDomainError(domain.CodeInvalidAmount, "wallet balance cannot be negative", nil)
	}

	return &Wallet{
		id:        id,
		playerID:  playerID,
		currency:  currency,
		balance:   balance,
		version:   version,
		createdAt: createdAt.UTC(),
		updatedAt: updatedAt.UTC(),
	}, nil
}

// ID returns the unique identifier of the wallet.
func (w *Wallet) ID() string {
	return w.id
}

// PlayerID returns the owning player's identifier.
func (w *Wallet) PlayerID() string {
	return w.playerID
}

// Currency returns the wallet's ISO 4217 currency.
func (w *Wallet) Currency() string {
	return w.currency
}

// Balance returns the current monetary balance.
func (w *Wallet) Balance() money.Money {
	return w.balance
}

// Version returns the optimistic locking version.
func (w *Wallet) Version() int64 {
	return w.version
}

// CreatedAt returns the creation timestamp in UTC.
func (w *Wallet) CreatedAt() time.Time {
	return w.createdAt
}

// UpdatedAt returns the last modification timestamp in UTC.
func (w *Wallet) UpdatedAt() time.Time {
	return w.updatedAt
}

// Debit applies a debit to the wallet balance.
// Ensures balance never drops below zero and increments version.
func (w *Wallet) Debit(amount money.Money, now time.Time) error {
	if amount.Currency() != w.currency {
		return domain.WrapDomainError(
			domain.CodeCurrencyMismatch,
			fmt.Sprintf("movement currency %s does not match wallet currency %s", amount.Currency(), w.currency),
			nil,
		)
	}
	if !amount.IsPositive() {
		return domain.WrapDomainError(
			domain.CodeInvalidAmount,
			"debit amount must be strictly positive",
			nil,
		)
	}

	cmp, err := w.balance.Compare(amount)
	if err != nil {
		return err
	}
	if cmp < 0 {
		return domain.WrapDomainError(
			domain.CodeInsufficientFunds,
			fmt.Sprintf("insufficient funds: available %s, required %s", w.balance.FormatDecimal(), amount.FormatDecimal()),
			nil,
		)
	}

	newBalance, err := w.balance.Sub(amount)
	if err != nil {
		return err
	}

	w.balance = newBalance
	w.version++
	w.updatedAt = now.UTC()
	return nil
}

// Credit applies a credit to the wallet balance and increments version.
func (w *Wallet) Credit(amount money.Money, now time.Time) error {
	if amount.Currency() != w.currency {
		return domain.WrapDomainError(
			domain.CodeCurrencyMismatch,
			fmt.Sprintf("movement currency %s does not match wallet currency %s", amount.Currency(), w.currency),
			nil,
		)
	}
	if !amount.IsPositive() {
		return domain.WrapDomainError(
			domain.CodeInvalidAmount,
			"credit amount must be strictly positive",
			nil,
		)
	}

	newBalance, err := w.balance.Add(amount)
	if err != nil {
		return err
	}

	w.balance = newBalance
	w.version++
	w.updatedAt = now.UTC()
	return nil
}

package ledger

import (
	"fmt"
	"strings"
	"time"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/money"
)

// Direction defines the accounting direction of a ledger entry.
type Direction string

const (
	DirectionDebit  Direction = "DEBIT"
	DirectionCredit Direction = "CREDIT"
)

// Entry represents an immutable financial entry in the wallet ledger.
type Entry struct {
	id            string
	seq           int64 // persistent monotonic sequence for stable cursor pagination
	walletID      string
	transactionID string
	direction     Direction
	amount        money.Money
	balanceBefore money.Money
	balanceAfter  money.Money
	createdAt     time.Time
}

// NewEntry constructs and validates an immutable ledger entry.
// Enforces the strict invariant: balanceAfter = balanceBefore ± amount.
func NewEntry(
	id string,
	walletID string,
	transactionID string,
	direction Direction,
	amount money.Money,
	balanceBefore money.Money,
	balanceAfter money.Money,
	createdAt time.Time,
) (*Entry, error) {
	if strings.TrimSpace(id) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidLedgerEntry, "entry id must not be empty", nil)
	}
	if strings.TrimSpace(walletID) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidLedgerEntry, "wallet id must not be empty", nil)
	}
	if strings.TrimSpace(transactionID) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidLedgerEntry, "transaction id must not be empty", nil)
	}
	if direction != DirectionDebit && direction != DirectionCredit {
		return nil, domain.WrapDomainError(
			domain.CodeInvalidLedgerEntry,
			fmt.Sprintf("invalid direction %q: must be DEBIT or CREDIT", direction),
			nil,
		)
	}
	if !amount.IsPositive() {
		return nil, domain.WrapDomainError(
			domain.CodeInvalidAmount,
			"ledger entry amount must be strictly positive",
			nil,
		)
	}
	if balanceBefore.IsNegative() || balanceAfter.IsNegative() {
		return nil, domain.WrapDomainError(
			domain.CodeInvalidAmount,
			"ledger balance before and after must not be negative",
			nil,
		)
	}

	// Currencies must match
	currency := amount.Currency()
	if balanceBefore.Currency() != currency || balanceAfter.Currency() != currency {
		return nil, domain.WrapDomainError(
			domain.CodeCurrencyMismatch,
			"amount and balances currency mismatch in ledger entry",
			nil,
		)
	}

	// Invariant validation: balanceAfter = balanceBefore ± amount
	switch direction {
	case DirectionCredit:
		expectedAfter, err := balanceBefore.Add(amount)
		if err != nil {
			return nil, err
		}
		if !balanceAfter.Equal(expectedAfter) {
			return nil, domain.WrapDomainError(
				domain.CodeInvalidLedgerEntry,
				fmt.Sprintf("credit invariant violated: balanceBefore (%s) + amount (%s) != balanceAfter (%s)",
					balanceBefore.FormatDecimal(), amount.FormatDecimal(), balanceAfter.FormatDecimal()),
				nil,
			)
		}
	case DirectionDebit:
		expectedAfter, err := balanceBefore.Sub(amount)
		if err != nil {
			return nil, err
		}
		if !balanceAfter.Equal(expectedAfter) {
			return nil, domain.WrapDomainError(
				domain.CodeInvalidLedgerEntry,
				fmt.Sprintf("debit invariant violated: balanceBefore (%s) - amount (%s) != balanceAfter (%s)",
					balanceBefore.FormatDecimal(), amount.FormatDecimal(), balanceAfter.FormatDecimal()),
				nil,
			)
		}
	}

	return &Entry{
		id:            id,
		seq:           0,
		walletID:      walletID,
		transactionID: transactionID,
		direction:     direction,
		amount:        amount,
		balanceBefore: balanceBefore,
		balanceAfter:  balanceAfter,
		createdAt:     createdAt.UTC(),
	}, nil
}

// RehydrateEntry reconstructs an existing ledger entry from persistence.
func RehydrateEntry(
	id string,
	seq int64,
	walletID string,
	transactionID string,
	direction Direction,
	amount money.Money,
	balanceBefore money.Money,
	balanceAfter money.Money,
	createdAt time.Time,
) (*Entry, error) {
	entry, err := NewEntry(id, walletID, transactionID, direction, amount, balanceBefore, balanceAfter, createdAt)
	if err != nil {
		return nil, err
	}
	entry.seq = seq
	return entry, nil
}

// ID returns the entry identifier.
func (e *Entry) ID() string { return e.id }

// Seq returns the sequential ordering number from database.
func (e *Entry) Seq() int64 { return e.seq }

// WalletID returns the associated wallet identifier.
func (e *Entry) WalletID() string { return e.walletID }

// TransactionID returns the associated transaction identifier.
func (e *Entry) TransactionID() string { return e.transactionID }

// Direction returns DEBIT or CREDIT.
func (e *Entry) Direction() Direction { return e.direction }

// Amount returns the transferred amount.
func (e *Entry) Amount() money.Money { return e.amount }

// BalanceBefore returns the balance prior to this movement.
func (e *Entry) BalanceBefore() money.Money { return e.balanceBefore }

// BalanceAfter returns the resulting balance after this movement.
func (e *Entry) BalanceAfter() money.Money { return e.balanceAfter }

// CreatedAt returns the creation timestamp in UTC.
func (e *Entry) CreatedAt() time.Time { return e.createdAt }

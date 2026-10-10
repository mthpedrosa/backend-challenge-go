package wager

import (
	"fmt"
	"strings"
	"time"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/money"
)

// Origin defines transaction origin: internal (system) or external (game provider).
type Origin string

const (
	OriginInternal Origin = "INTERNAL"
	OriginExternal Origin = "EXTERNAL"
)

// Kind defines the financial and operational wager transaction kinds.
type Kind string

const (
	KindOpening  Kind = "OPENING"
	KindBet      Kind = "BET"
	KindWin      Kind = "WIN"
	KindLoss     Kind = "LOSS"
	KindRefund   Kind = "REFUND"
	KindRollback Kind = "ROLLBACK"
)

// Status defines the lifecycle status of a transaction.
type Status string

const (
	StatusPending          Status = "PENDING"
	StatusPendingReference Status = "PENDING_REFERENCE"
	StatusProcessed        Status = "PROCESSED"
	StatusRejected         Status = "REJECTED"
	StatusFailed           Status = "FAILED"
)

// IsTerminal reports whether the status is terminal and immutable.
func (s Status) IsTerminal() bool {
	return s == StatusProcessed || s == StatusRejected || s == StatusFailed
}

// NewExternalParams holds the input parameters for creating an external wager transaction.
type NewExternalParams struct {
	ID                             string
	WalletID                       string
	PlayerID                       string
	Kind                           Kind
	Amount                         money.Money
	ProviderID                     string
	ExternalTransactionID          string
	IdempotencyKey                 string
	PayloadHash                    string
	RoundID                        string
	GameID                         string
	ReferenceExternalTransactionID *string
	Now                            time.Time
}

// Transaction represents a financial or auditing wagering operation.
type Transaction struct {
	id                             string
	origin                         Origin
	kind                           Kind
	status                         Status
	walletID                       string
	playerID                       string
	amount                         money.Money
	providerID                     *string
	externalTransactionID          *string
	idempotencyKey                 *string
	payloadHash                    *string
	roundID                        *string
	gameID                         *string
	referenceExternalTransactionID *string
	referenceTransactionID         *string
	failureCode                    *string
	balanceAfter                   *money.Money
	attempts                       int
	nextAttemptAt                  *time.Time
	expiresAt                      *time.Time
	createdAt                      time.Time
	updatedAt                      time.Time
}

// NewOpening creates an internal wallet opening transaction (OPENING).
// Exclusively for internal origin; rejects external provider metadata.
func NewOpening(id, walletID, playerID string, initialBalance money.Money, now time.Time) (*Transaction, error) {
	if strings.TrimSpace(id) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidStateTransition, "transaction id must not be empty", nil)
	}
	if strings.TrimSpace(walletID) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidStateTransition, "wallet id must not be empty", nil)
	}
	if strings.TrimSpace(playerID) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidStateTransition, "player id must not be empty", nil)
	}
	if initialBalance.IsNegative() {
		return nil, domain.WrapDomainError(domain.CodeInvalidAmount, "opening balance must not be negative", nil)
	}

	return &Transaction{
		id:        id,
		origin:    OriginInternal,
		kind:      KindOpening,
		status:    StatusPending,
		walletID:  walletID,
		playerID:  playerID,
		amount:    initialBalance,
		createdAt: now.UTC(),
		updatedAt: now.UTC(),
	}, nil
}

// NewExternal validates and constructs an external wager transaction initiated by a provider.
func NewExternal(p NewExternalParams) (*Transaction, error) {
	if strings.TrimSpace(p.ID) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidStateTransition, "transaction id must not be empty", nil)
	}
	if strings.TrimSpace(p.WalletID) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidStateTransition, "wallet id must not be empty", nil)
	}
	if strings.TrimSpace(p.PlayerID) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidStateTransition, "player id must not be empty", nil)
	}
	if strings.TrimSpace(p.ProviderID) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidStateTransition, "provider id must not be empty", nil)
	}
	if strings.TrimSpace(p.ExternalTransactionID) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidStateTransition, "external transaction id must not be empty", nil)
	}
	if strings.TrimSpace(p.IdempotencyKey) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidStateTransition, "idempotency key must not be empty", nil)
	}
	if strings.TrimSpace(p.PayloadHash) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidStateTransition, "payload hash must not be empty", nil)
	}
	if strings.TrimSpace(p.RoundID) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidStateTransition, "round id must not be empty", nil)
	}
	if strings.TrimSpace(p.GameID) == "" {
		return nil, domain.WrapDomainError(domain.CodeInvalidStateTransition, "game id must not be empty", nil)
	}

	// Strict kind validation
	switch p.Kind {
	case KindOpening:
		return nil, domain.WrapDomainError(
			domain.CodeInvalidKind,
			"OPENING is an internal transaction type and cannot be requested externally",
			nil,
		)
	case KindLoss:
		if !p.Amount.IsZero() {
			return nil, domain.WrapDomainError(
				domain.CodeInvalidAmount,
				fmt.Sprintf("LOSS transaction amount must be exactly 0.00, got %s", p.Amount.FormatDecimal()),
				nil,
			)
		}
	case KindBet, KindWin:
		if !p.Amount.IsPositive() {
			return nil, domain.WrapDomainError(
				domain.CodeInvalidAmount,
				fmt.Sprintf("%s transaction amount must be strictly positive, got %s", p.Kind, p.Amount.FormatDecimal()),
				nil,
			)
		}
	case KindRefund, KindRollback:
		if !p.Amount.IsPositive() {
			return nil, domain.WrapDomainError(
				domain.CodeInvalidAmount,
				fmt.Sprintf("%s transaction amount must be strictly positive, got %s", p.Kind, p.Amount.FormatDecimal()),
				nil,
			)
		}
		if p.ReferenceExternalTransactionID == nil || strings.TrimSpace(*p.ReferenceExternalTransactionID) == "" {
			return nil, domain.WrapDomainError(
				domain.CodeInvalidStateTransition,
				fmt.Sprintf("%s requires referenceExternalTransactionId", p.Kind),
				nil,
			)
		}
	default:
		return nil, domain.WrapDomainError(
			domain.CodeInvalidKind,
			fmt.Sprintf("unknown transaction kind: %q", p.Kind),
			nil,
		)
	}

	return &Transaction{
		id:                             p.ID,
		origin:                         OriginExternal,
		kind:                           p.Kind,
		status:                         StatusPending,
		walletID:                       p.WalletID,
		playerID:                       p.PlayerID,
		amount:                         p.Amount,
		providerID:                     &p.ProviderID,
		externalTransactionID:          &p.ExternalTransactionID,
		idempotencyKey:                 &p.IdempotencyKey,
		payloadHash:                    &p.PayloadHash,
		roundID:                        &p.RoundID,
		gameID:                         &p.GameID,
		referenceExternalTransactionID: p.ReferenceExternalTransactionID,
		createdAt:                      p.Now.UTC(),
		updatedAt:                      p.Now.UTC(),
	}, nil
}

// Rehydrate reconstructs a transaction from persistence without triggering transitions.
func Rehydrate(
	id string,
	origin Origin,
	kind Kind,
	status Status,
	walletID string,
	playerID string,
	amount money.Money,
	providerID *string,
	externalTransactionID *string,
	idempotencyKey *string,
	payloadHash *string,
	roundID *string,
	gameID *string,
	referenceExternalTransactionID *string,
	referenceTransactionID *string,
	failureCode *string,
	balanceAfter *money.Money,
	attempts int,
	nextAttemptAt *time.Time,
	expiresAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) *Transaction {
	return &Transaction{
		id:                             id,
		origin:                         origin,
		kind:                           kind,
		status:                         status,
		walletID:                       walletID,
		playerID:                       playerID,
		amount:                         amount,
		providerID:                     providerID,
		externalTransactionID:          externalTransactionID,
		idempotencyKey:                 idempotencyKey,
		payloadHash:                    payloadHash,
		roundID:                        roundID,
		gameID:                         gameID,
		referenceExternalTransactionID: referenceExternalTransactionID,
		referenceTransactionID:         referenceTransactionID,
		failureCode:                    failureCode,
		balanceAfter:                   balanceAfter,
		attempts:                       attempts,
		nextAttemptAt:                  nextAttemptAt,
		expiresAt:                      expiresAt,
		createdAt:                      createdAt.UTC(),
		updatedAt:                      updatedAt.UTC(),
	}
}

// Getters
func (t *Transaction) ID() string                     { return t.id }
func (t *Transaction) Origin() Origin                 { return t.origin }
func (t *Transaction) Kind() Kind                     { return t.kind }
func (t *Transaction) Status() Status                 { return t.status }
func (t *Transaction) WalletID() string               { return t.walletID }
func (t *Transaction) PlayerID() string               { return t.playerID }
func (t *Transaction) Amount() money.Money            { return t.amount }
func (t *Transaction) ProviderID() *string            { return t.providerID }
func (t *Transaction) ExternalTransactionID() *string { return t.externalTransactionID }
func (t *Transaction) IdempotencyKey() *string        { return t.idempotencyKey }
func (t *Transaction) PayloadHash() *string           { return t.payloadHash }
func (t *Transaction) RoundID() *string               { return t.roundID }
func (t *Transaction) GameID() *string                { return t.gameID }
func (t *Transaction) ReferenceExternalTransactionID() *string {
	return t.referenceExternalTransactionID
}
func (t *Transaction) ReferenceTransactionID() *string { return t.referenceTransactionID }
func (t *Transaction) FailureCode() *string            { return t.failureCode }
func (t *Transaction) BalanceAfter() *money.Money      { return t.balanceAfter }
func (t *Transaction) Attempts() int                   { return t.attempts }
func (t *Transaction) NextAttemptAt() *time.Time       { return t.nextAttemptAt }
func (t *Transaction) ExpiresAt() *time.Time           { return t.expiresAt }
func (t *Transaction) CreatedAt() time.Time            { return t.createdAt }
func (t *Transaction) UpdatedAt() time.Time            { return t.updatedAt }

// State machine transition methods

// MarkProcessed transitions transaction to PROCESSED with resulting balance and resolved reference.
func (t *Transaction) MarkProcessed(balanceAfter *money.Money, resolvedRefID *string, now time.Time) error {
	if t.status.IsTerminal() {
		return domain.WrapDomainError(
			domain.CodeInvalidStateTransition,
			fmt.Sprintf("cannot transition from terminal state %s to PROCESSED", t.status),
			nil,
		)
	}

	t.status = StatusProcessed
	t.balanceAfter = balanceAfter
	if resolvedRefID != nil {
		t.referenceTransactionID = resolvedRefID
	}
	t.updatedAt = now.UTC()
	return nil
}

// MarkRejected transitions transaction to REJECTED with a persistent business failure code.
func (t *Transaction) MarkRejected(failureCode string, now time.Time) error {
	if t.status.IsTerminal() {
		return domain.WrapDomainError(
			domain.CodeInvalidStateTransition,
			fmt.Sprintf("cannot transition from terminal state %s to REJECTED", t.status),
			nil,
		)
	}

	t.status = StatusRejected
	t.failureCode = &failureCode
	t.updatedAt = now.UTC()
	return nil
}

// MarkPendingReference transitions transaction to PENDING_REFERENCE scheduling future retry.
func (t *Transaction) MarkPendingReference(nextAttemptAt, expiresAt time.Time, now time.Time) error {
	if t.status != StatusPending && t.status != StatusPendingReference {
		return domain.WrapDomainError(
			domain.CodeInvalidStateTransition,
			fmt.Sprintf("cannot transition from state %s to PENDING_REFERENCE", t.status),
			nil,
		)
	}

	t.status = StatusPendingReference
	t.nextAttemptAt = &nextAttemptAt
	t.expiresAt = &expiresAt
	t.updatedAt = now.UTC()
	return nil
}

// MarkFailed transitions transaction to FAILED for permanent infrastructure audit records.
func (t *Transaction) MarkFailed(now time.Time) error {
	if t.status.IsTerminal() {
		return domain.WrapDomainError(
			domain.CodeInvalidStateTransition,
			fmt.Sprintf("cannot transition from terminal state %s to FAILED", t.status),
			nil,
		)
	}

	t.status = StatusFailed
	t.updatedAt = now.UTC()
	return nil
}

// IncrementAttempts registers a retry attempt by the pending-reference worker.
func (t *Transaction) IncrementAttempts(nextAttemptAt time.Time, now time.Time) {
	t.attempts++
	t.nextAttemptAt = &nextAttemptAt
	t.updatedAt = now.UTC()
}

// ValidateReversalReference enforces compatibility invariants between this transaction
// (which must be REFUND or ROLLBACK) and the referenced transaction (ref).
func (t *Transaction) ValidateReversalReference(ref *Transaction) error {
	if ref == nil {
		return domain.WrapDomainError(domain.CodeReferenceNotFound, "referenced transaction not found", nil)
	}

	// Reference must be in PROCESSED status
	if ref.Status() != StatusProcessed {
		return domain.WrapDomainError(
			domain.CodeReferenceNotProcessed,
			fmt.Sprintf("referenced transaction has status %s, expected PROCESSED", ref.Status()),
			nil,
		)
	}

	// Provider match
	if t.providerID == nil || ref.providerID == nil || *t.providerID != *ref.providerID {
		return domain.WrapDomainError(
			domain.CodeReferenceMismatch,
			"provider mismatch between reversal and reference",
			nil,
		)
	}

	// Player match
	if t.playerID != ref.playerID {
		return domain.WrapDomainError(
			domain.CodeReferenceMismatch,
			"player mismatch between reversal and reference",
			nil,
		)
	}

	// Wallet match
	if t.walletID != ref.walletID {
		return domain.WrapDomainError(
			domain.CodeReferenceMismatch,
			"wallet mismatch between reversal and reference",
			nil,
		)
	}

	// Currency match
	if t.amount.Currency() != ref.amount.Currency() {
		return domain.WrapDomainError(
			domain.CodeReferenceMismatch,
			"currency mismatch between reversal and reference",
			nil,
		)
	}

	// Round match
	if t.roundID == nil || ref.roundID == nil || *t.roundID != *ref.roundID {
		return domain.WrapDomainError(
			domain.CodeReferenceMismatch,
			"round mismatch between reversal and reference",
			nil,
		)
	}

	// Exact amount match (partial reversals are out of challenge scope)
	if !t.amount.Equal(ref.amount) {
		return domain.WrapDomainError(
			domain.CodeAmountMismatch,
			fmt.Sprintf("reversal amount %s must match reference amount %s", t.amount.FormatDecimal(), ref.amount.FormatDecimal()),
			nil,
		)
	}

	// Kind-specific reversal rules
	switch t.kind {
	case KindRefund:
		// REFUND can only reverse a BET
		if ref.Kind() != KindBet {
			return domain.WrapDomainError(
				domain.CodeInvalidReferenceKind,
				fmt.Sprintf("REFUND can only reference a BET transaction, referenced kind is %s", ref.Kind()),
				nil,
			)
		}
	case KindRollback:
		// ROLLBACK can reverse BET, WIN, or REFUND
		if ref.Kind() != KindBet && ref.Kind() != KindWin && ref.Kind() != KindRefund {
			return domain.WrapDomainError(
				domain.CodeInvalidReferenceKind,
				fmt.Sprintf("ROLLBACK cannot reference kind %s: allowed kinds are BET, WIN, REFUND", ref.Kind()),
				nil,
			)
		}
	default:
		return domain.WrapDomainError(
			domain.CodeInvalidKind,
			fmt.Sprintf("transaction of kind %s is not a reversal", t.kind),
			nil,
		)
	}

	return nil
}

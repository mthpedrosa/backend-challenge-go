package event

import (
	"encoding/json"
	"time"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain/ledger"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/money"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/wager"
)

// Mandatory domain event types required by the challenge specification.
const (
	TypeWagerTransactionProcessed        = "WagerTransactionProcessed"
	TypeWagerTransactionRejected         = "WagerTransactionRejected"
	TypeWalletBalanceChanged             = "WalletBalanceChanged"
	TypeWagerTransactionPendingReference = "WagerTransactionPendingReference"
)

// Envelope represents the standardized envelope for outbox publishing.
type Envelope struct {
	EventID       string    `json:"eventId"`
	EventType     string    `json:"eventType"`
	AggregateType string    `json:"aggregateType"`
	AggregateID   string    `json:"aggregateId"`
	CorrelationID string    `json:"correlationId"`
	CausationID   *string   `json:"causationId,omitempty"`
	OccurredAt    time.Time `json:"occurredAt"`
	Version       int       `json:"version"`
	Data          any       `json:"data"`
}

// TransactionProcessedData represents WagerTransactionProcessed payload.
type TransactionProcessedData struct {
	TransactionID string      `json:"transactionId"`
	WalletID      string      `json:"walletId"`
	PlayerID      string      `json:"playerId"`
	Kind          wager.Kind  `json:"kind"`
	Money         money.Money `json:"money"`
	Status        string      `json:"status"`
	BalanceAfter  *string     `json:"balanceAfter,omitempty"`
	ProviderID    *string     `json:"providerId,omitempty"`
	RoundID       *string     `json:"roundId,omitempty"`
}

// TransactionRejectedData represents WagerTransactionRejected payload.
type TransactionRejectedData struct {
	TransactionID string      `json:"transactionId"`
	WalletID      string      `json:"walletId"`
	PlayerID      string      `json:"playerId"`
	Kind          wager.Kind  `json:"kind"`
	Money         money.Money `json:"money"`
	Status        string      `json:"status"`
	FailureCode   string      `json:"failureCode"`
	ProviderID    *string     `json:"providerId,omitempty"`
	RoundID       *string     `json:"roundId,omitempty"`
}

// TransactionPendingReferenceData represents WagerTransactionPendingReference payload.
type TransactionPendingReferenceData struct {
	TransactionID                  string      `json:"transactionId"`
	WalletID                       string      `json:"walletId"`
	PlayerID                       string      `json:"playerId"`
	Kind                           wager.Kind  `json:"kind"`
	Money                          money.Money `json:"money"`
	Status                         string      `json:"status"`
	ReferenceExternalTransactionID *string     `json:"referenceExternalTransactionId,omitempty"`
	ProviderID                     *string     `json:"providerId,omitempty"`
	RoundID                        *string     `json:"roundId,omitempty"`
}

// WalletBalanceChangedData represents WalletBalanceChanged payload matching Section 11 specifications.
type WalletBalanceChangedData struct {
	WalletID      string           `json:"walletId"`
	TransactionID string           `json:"transactionId"`
	Direction     ledger.Direction `json:"direction"`
	Money         money.Money      `json:"money"`
	BalanceBefore string           `json:"balanceBefore"`
	BalanceAfter  string           `json:"balanceAfter"`
	WalletVersion int64            `json:"walletVersion"`
}

// NewTransactionProcessedEvent constructs a typed WagerTransactionProcessed event envelope.
func NewTransactionProcessedEvent(
	eventID string,
	tx *wager.Transaction,
	correlationID string,
	causationID *string,
	occurredAt time.Time,
) Envelope {
	var balAfterStr *string
	if tx.BalanceAfter() != nil {
		s := tx.BalanceAfter().FormatDecimal()
		balAfterStr = &s
	}

	return Envelope{
		EventID:       eventID,
		EventType:     TypeWagerTransactionProcessed,
		AggregateType: "WagerTransaction",
		AggregateID:   tx.ID(),
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    occurredAt.UTC(),
		Version:       1,
		Data: TransactionProcessedData{
			TransactionID: tx.ID(),
			WalletID:      tx.WalletID(),
			PlayerID:      tx.PlayerID(),
			Kind:          tx.Kind(),
			Money:         tx.Amount(),
			Status:        string(tx.Status()),
			BalanceAfter:  balAfterStr,
			ProviderID:    tx.ProviderID(),
			RoundID:       tx.RoundID(),
		},
	}
}

// NewTransactionRejectedEvent constructs a typed WagerTransactionRejected event envelope.
func NewTransactionRejectedEvent(
	eventID string,
	tx *wager.Transaction,
	failureCode string,
	correlationID string,
	causationID *string,
	occurredAt time.Time,
) Envelope {
	return Envelope{
		EventID:       eventID,
		EventType:     TypeWagerTransactionRejected,
		AggregateType: "WagerTransaction",
		AggregateID:   tx.ID(),
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    occurredAt.UTC(),
		Version:       1,
		Data: TransactionRejectedData{
			TransactionID: tx.ID(),
			WalletID:      tx.WalletID(),
			PlayerID:      tx.PlayerID(),
			Kind:          tx.Kind(),
			Money:         tx.Amount(),
			Status:        string(tx.Status()),
			FailureCode:   failureCode,
			ProviderID:    tx.ProviderID(),
			RoundID:       tx.RoundID(),
		},
	}
}

// NewTransactionPendingReferenceEvent constructs a typed WagerTransactionPendingReference event envelope.
func NewTransactionPendingReferenceEvent(
	eventID string,
	tx *wager.Transaction,
	correlationID string,
	causationID *string,
	occurredAt time.Time,
) Envelope {
	return Envelope{
		EventID:       eventID,
		EventType:     TypeWagerTransactionPendingReference,
		AggregateType: "WagerTransaction",
		AggregateID:   tx.ID(),
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    occurredAt.UTC(),
		Version:       1,
		Data: TransactionPendingReferenceData{
			TransactionID:                  tx.ID(),
			WalletID:                       tx.WalletID(),
			PlayerID:                       tx.PlayerID(),
			Kind:                           tx.Kind(),
			Money:                          tx.Amount(),
			Status:                         string(tx.Status()),
			ReferenceExternalTransactionID: tx.ReferenceExternalTransactionID(),
			ProviderID:                     tx.ProviderID(),
			RoundID:                        tx.RoundID(),
		},
	}
}

// NewWalletBalanceChangedEvent constructs a typed WalletBalanceChanged event envelope.
func NewWalletBalanceChangedEvent(
	eventID string,
	walletID string,
	transactionID string,
	direction ledger.Direction,
	movement money.Money,
	balanceBefore money.Money,
	balanceAfter money.Money,
	walletVersion int64,
	correlationID string,
	causationID *string,
	occurredAt time.Time,
) Envelope {
	return Envelope{
		EventID:       eventID,
		EventType:     TypeWalletBalanceChanged,
		AggregateType: "Wallet",
		AggregateID:   walletID,
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    occurredAt.UTC(),
		Version:       1,
		Data: WalletBalanceChangedData{
			WalletID:      walletID,
			TransactionID: transactionID,
			Direction:     direction,
			Money:         movement,
			BalanceBefore: balanceBefore.FormatDecimal(),
			BalanceAfter:  balanceAfter.FormatDecimal(),
			WalletVersion: walletVersion,
		},
	}
}

// MarshalPayload serializes the event Data into an immutable JSON payload.
func (e Envelope) MarshalPayload() ([]byte, error) {
	return json.Marshal(e.Data)
}

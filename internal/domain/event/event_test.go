package event

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain/ledger"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/money"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/wager"
)

func TestWalletBalanceChangedEvent(t *testing.T) {
	now := time.Now()
	amount, _ := money.Parse("25.00", money.CurrencyBRL)
	before, _ := money.Parse("100.00", money.CurrencyBRL)
	after, _ := money.Parse("75.00", money.CurrencyBRL)

	env := NewWalletBalanceChangedEvent(
		"evt-1",
		"w-1",
		"tx-1",
		ledger.DirectionDebit,
		amount,
		before,
		after,
		2,
		"corr-1",
		nil,
		now,
	)

	if env.EventType != TypeWalletBalanceChanged || env.AggregateID != "w-1" {
		t.Errorf("envelope header mismatch: %+v", env)
	}

	payload, err := env.MarshalPayload()
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	var data WalletBalanceChangedData
	if err := json.Unmarshal(payload, &data); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if data.WalletID != "w-1" || data.TransactionID != "tx-1" || data.Direction != ledger.DirectionDebit {
		t.Errorf("payload fields mismatch: %+v", data)
	}
	if data.BalanceBefore != "100.00" || data.BalanceAfter != "75.00" || data.WalletVersion != 2 {
		t.Errorf("balances or version mismatch: %+v", data)
	}
}

func TestWagerTransactionProcessedEvent(t *testing.T) {
	now := time.Now()
	amount, _ := money.Parse("50.00", money.CurrencyBRL)
	ref := "ref-1"
	p := wager.NewExternalParams{
		ID:                             "tx-1",
		WalletID:                       "w-1",
		PlayerID:                       "p-1",
		Kind:                           wager.KindWin,
		Amount:                         amount,
		ProviderID:                     "provider-a",
		ExternalTransactionID:          "ext-1",
		IdempotencyKey:                 "provider-a:ext-1",
		PayloadHash:                    "hash-123",
		RoundID:                        "round-1",
		GameID:                         "game-1",
		ReferenceExternalTransactionID: &ref,
		Now:                            now,
	}
	tx, _ := wager.NewExternal(p)
	balAfter, _ := money.Parse("150.00", money.CurrencyBRL)
	_ = tx.MarkProcessed(&balAfter, nil, now)

	env := NewTransactionProcessedEvent("evt-2", tx, "corr-2", nil, now)
	if env.EventType != TypeWagerTransactionProcessed || env.AggregateID != "tx-1" {
		t.Errorf("envelope mismatch: %+v", env)
	}

	payload, err := env.MarshalPayload()
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	var data TransactionProcessedData
	if err := json.Unmarshal(payload, &data); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if data.TransactionID != "tx-1" || data.Status != "PROCESSED" || *data.BalanceAfter != "150.00" {
		t.Errorf("processed data mismatch: %+v", data)
	}
}

func TestWagerTransactionRejectedEvent(t *testing.T) {
	now := time.Now()
	amount, _ := money.Parse("50.00", money.CurrencyBRL)
	p := wager.NewExternalParams{
		ID:                    "tx-1",
		WalletID:              "w-1",
		PlayerID:              "p-1",
		Kind:                  wager.KindBet,
		Amount:                amount,
		ProviderID:            "provider-a",
		ExternalTransactionID: "ext-1",
		IdempotencyKey:        "provider-a:ext-1",
		PayloadHash:           "hash-123",
		RoundID:               "round-1",
		GameID:                "game-1",
		Now:                   now,
	}
	tx, _ := wager.NewExternal(p)
	_ = tx.MarkRejected("INSUFFICIENT_FUNDS", now)

	env := NewTransactionRejectedEvent("evt-3", tx, "INSUFFICIENT_FUNDS", "corr-3", nil, now)
	if env.EventType != TypeWagerTransactionRejected {
		t.Errorf("eventType = %s, want WagerTransactionRejected", env.EventType)
	}

	payload, _ := env.MarshalPayload()
	var data TransactionRejectedData
	_ = json.Unmarshal(payload, &data)
	if data.FailureCode != "INSUFFICIENT_FUNDS" || data.Status != "REJECTED" {
		t.Errorf("rejected data mismatch: %+v", data)
	}
}

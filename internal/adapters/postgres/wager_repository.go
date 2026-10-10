package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/money"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/wager"
)

type WagerRepository struct {
	pool *pgxpool.Pool
}

func NewWagerRepository(pool *pgxpool.Pool) wager.Repository {
	return &WagerRepository{pool: pool}
}

func (r *WagerRepository) Create(ctx context.Context, t *wager.Transaction) error {
	runner := getRunner(ctx, r.pool)

	query := `
		INSERT INTO wager_transactions (
			id, origin, kind, status, wallet_id, player_id, amount, currency,
			provider_id, external_transaction_id, idempotency_key, payload_hash,
			round_id, game_id, reference_external_transaction_id, reference_transaction_id,
			failure_code, balance_after, attempts, next_attempt_at, expires_at,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23
		)
	`

	var balAfter *int64
	if t.BalanceAfter() != nil {
		c := t.BalanceAfter().MinorUnits()
		balAfter = &c
	}

	_, err := runner.Exec(ctx, query,
		t.ID(),
		string(t.Origin()),
		string(t.Kind()),
		string(t.Status()),
		t.WalletID(),
		t.PlayerID(),
		t.Amount().MinorUnits(),
		t.Amount().Currency(),
		t.ProviderID(),
		t.ExternalTransactionID(),
		t.IdempotencyKey(),
		t.PayloadHash(),
		t.RoundID(),
		t.GameID(),
		t.ReferenceExternalTransactionID(),
		t.ReferenceTransactionID(),
		t.FailureCode(),
		balAfter,
		t.Attempts(),
		t.NextAttemptAt(),
		t.ExpiresAt(),
		t.CreatedAt(),
		t.UpdatedAt(),
	)
	if err != nil {
		return fmt.Errorf("failed to create wager transaction: %w", err)
	}
	return nil
}

func (r *WagerRepository) Update(ctx context.Context, t *wager.Transaction) error {
	runner := getRunner(ctx, r.pool)

	query := `
		UPDATE wager_transactions SET 
			status = $1,
			reference_transaction_id = $2,
			failure_code = $3,
			balance_after = $4,
			attempts = $5,
			next_attempt_at = $6,
			updated_at = $7
		WHERE id = $8
	`

	var balAfter *int64
	if t.BalanceAfter() != nil {
		c := t.BalanceAfter().MinorUnits()
		balAfter = &c
	}

	tag, err := runner.Exec(ctx, query,
		string(t.Status()),
		t.ReferenceTransactionID(),
		t.FailureCode(),
		balAfter,
		t.Attempts(),
		t.NextAttemptAt(),
		t.UpdatedAt(),
		t.ID(),
	)
	if err != nil {
		return fmt.Errorf("failed to update wager transaction: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return domain.WrapDomainError("TRANSACTION_NOT_FOUND", "wager transaction not found for update", nil)
	}

	return nil
}

func (r *WagerRepository) FindByID(ctx context.Context, id string) (*wager.Transaction, error) {
	query := r.baseSelectQuery() + ` WHERE id = $1`
	return r.scanOne(ctx, query, id)
}

func (r *WagerRepository) FindByExternalID(ctx context.Context, providerID, externalID string) (*wager.Transaction, error) {
	query := r.baseSelectQuery() + ` WHERE provider_id = $1 AND external_transaction_id = $2`
	return r.scanOne(ctx, query, providerID, externalID)
}

func (r *WagerRepository) FindByIdempotencyKey(ctx context.Context, key string) (*wager.Transaction, error) {
	query := r.baseSelectQuery() + ` WHERE idempotency_key = $1`
	return r.scanOne(ctx, query, key)
}

func (r *WagerRepository) baseSelectQuery() string {
	return `
		SELECT 
			id, origin, kind, status, wallet_id, player_id, amount, currency,
			provider_id, external_transaction_id, idempotency_key, payload_hash,
			round_id, game_id, reference_external_transaction_id, reference_transaction_id,
			failure_code, balance_after, attempts, next_attempt_at, expires_at,
			created_at, updated_at
		FROM wager_transactions
	`
}

func (r *WagerRepository) scanOne(ctx context.Context, query string, args ...any) (*wager.Transaction, error) {
	runner := getRunner(ctx, r.pool)
	row := runner.QueryRow(ctx, query, args...)

	var (
		id, origin, kind, status, walletID, playerID                                      string
		amountCents                                                                       int64
		currency                                                                          string
		providerID, extTxID, idempKey, hash, roundID, gameID, refExtID, refTxID, failCode *string
		balAfter                                                                          *int64
		attempts                                                                          int
		nextAttempt, expiresAt                                                            *time.Time
		createdAt, updatedAt                                                              time.Time
	)

	err := row.Scan(
		&id, &origin, &kind, &status, &walletID, &playerID, &amountCents, &currency,
		&providerID, &extTxID, &idempKey, &hash,
		&roundID, &gameID, &refExtID, &refTxID,
		&failCode, &balAfter, &attempts, &nextAttempt, &expiresAt,
		&createdAt, &updatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.WrapDomainError("TRANSACTION_NOT_FOUND", "transaction not found", nil)
		}
		return nil, fmt.Errorf("failed to scan transaction: %w", err)
	}

	amt, err := money.FromMinor(amountCents, currency)
	if err != nil {
		return nil, fmt.Errorf("invalid amount money in database: %w", err)
	}

	var balAfterMoney *money.Money
	if balAfter != nil {
		m, err := money.FromMinor(*balAfter, currency)
		if err != nil {
			return nil, fmt.Errorf("invalid balance_after money in database: %w", err)
		}
		balAfterMoney = &m
	}

	t := wager.Rehydrate(
		id, wager.Origin(origin), wager.Kind(kind), wager.Status(status),
		walletID, playerID, amt,
		providerID, extTxID, idempKey, hash, roundID, gameID,
		refExtID, refTxID, failCode, balAfterMoney,
		attempts, nextAttempt, expiresAt,
		createdAt, updatedAt,
	)

	return t, nil
}

func (r *WagerRepository) FindPendingReferences(ctx context.Context, limit int) ([]*wager.Transaction, error) {
	query := r.baseSelectQuery() + `
		WHERE status = 'PENDING_REFERENCE' AND (next_attempt_at IS NULL OR next_attempt_at <= NOW())
		ORDER BY next_attempt_at ASC NULLS FIRST
		LIMIT $1
	`
	runner := getRunner(ctx, r.pool)
	rows, err := runner.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending references: %w", err)
	}
	defer rows.Close()

	var txs []*wager.Transaction
	for rows.Next() {
		var (
			id, origin, kind, status, walletID, playerID                                      string
			amountCents                                                                       int64
			currency                                                                          string
			providerID, extTxID, idempKey, hash, roundID, gameID, refExtID, refTxID, failCode *string
			balAfter                                                                          *int64
			attempts                                                                          int
			nextAttempt, expiresAt                                                            *time.Time
			createdAt, updatedAt                                                              time.Time
		)

		err := rows.Scan(
			&id, &origin, &kind, &status, &walletID, &playerID, &amountCents, &currency,
			&providerID, &extTxID, &idempKey, &hash,
			&roundID, &gameID, &refExtID, &refTxID,
			&failCode, &balAfter, &attempts, &nextAttempt, &expiresAt,
			&createdAt, &updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan pending reference transaction: %w", err)
		}

		amt, err := money.FromMinor(amountCents, currency)
		if err != nil {
			return nil, fmt.Errorf("invalid amount money in database: %w", err)
		}

		var balAfterMoney *money.Money
		if balAfter != nil {
			m, err := money.FromMinor(*balAfter, currency)
			if err != nil {
				return nil, fmt.Errorf("invalid balance_after money in database: %w", err)
			}
			balAfterMoney = &m
		}

		t := wager.Rehydrate(
			id, wager.Origin(origin), wager.Kind(kind), wager.Status(status),
			walletID, playerID, amt,
			providerID, extTxID, idempKey, hash, roundID, gameID,
			refExtID, refTxID, failCode, balAfterMoney,
			attempts, nextAttempt, expiresAt,
			createdAt, updatedAt,
		)
		txs = append(txs, t)
	}

	return txs, rows.Err()
}

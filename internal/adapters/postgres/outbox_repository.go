package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain/event"
)

type OutboxRepository struct {
	pool *pgxpool.Pool
}

func NewOutboxRepository(pool *pgxpool.Pool) event.OutboxRepository {
	return &OutboxRepository{pool: pool}
}

func (r *OutboxRepository) Save(ctx context.Context, e event.Envelope) error {
	runner := getRunner(ctx, r.pool)

	payloadBytes, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	query := `
		INSERT INTO outbox (
			event_id, aggregate_id, event_type, payload, occurred_at, 
			attempts, next_attempt_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, 0, $6, $7, $8)
	`
	// Using occurredAt as next_attempt_at for immediate processing
	_, err = runner.Exec(ctx, query,
		e.EventID,
		e.AggregateID,
		e.EventType,
		payloadBytes,
		e.OccurredAt,
		e.OccurredAt, // next_attempt_at
		e.OccurredAt, // created_at
		e.OccurredAt, // updated_at
	)
	if err != nil {
		return fmt.Errorf("failed to save outbox event: %w", err)
	}

	return nil
}

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

	payloadBytes, err := json.Marshal(e.Data)
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

// LeaseUnpublished acquires a batch of unpublished events, incrementing attempts and setting the next attempt.
func (r *OutboxRepository) LeaseUnpublished(ctx context.Context, batchSize int) ([]event.Envelope, error) {
	query := `
		UPDATE outbox
		SET 
			attempts = attempts + 1,
			next_attempt_at = NOW() + INTERVAL '30 seconds',
			updated_at = NOW()
		WHERE event_id IN (
			SELECT event_id
			FROM outbox
			WHERE published_at IS NULL AND next_attempt_at <= NOW()
			ORDER BY created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING event_id, event_type, aggregate_id, payload, occurred_at, attempts
	`

	rows, err := r.pool.Query(ctx, query, batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to lease outbox events: %w", err)
	}
	defer rows.Close()

	var events []event.Envelope
	for rows.Next() {
		var e event.Envelope
		var payload []byte
		var attempts int
		if err := rows.Scan(
			&e.EventID,
			&e.EventType,
			&e.AggregateID,
			&payload,
			&e.OccurredAt,
			&attempts,
		); err != nil {
			return nil, fmt.Errorf("failed to scan outbox event: %w", err)
		}
		e.Version = 1

		// If legacy payload contains a full envelope, extract inner data
		var rawMap map[string]json.RawMessage
		if err := json.Unmarshal(payload, &rawMap); err == nil {
			if innerData, ok := rawMap["data"]; ok && rawMap["eventId"] != nil {
				e.Data = innerData
			} else {
				e.Data = json.RawMessage(payload)
			}
		} else {
			e.Data = json.RawMessage(payload)
		}

		events = append(events, e)
	}

	return events, rows.Err()
}

// MarkPublished marks an event as successfully published.
func (r *OutboxRepository) MarkPublished(ctx context.Context, eventID string) error {
	query := `
		UPDATE outbox
		SET published_at = NOW(), updated_at = NOW()
		WHERE event_id = $1
	`
	_, err := r.pool.Exec(ctx, query, eventID)
	if err != nil {
		return fmt.Errorf("failed to mark outbox event %q as published: %w", eventID, err)
	}
	return nil
}

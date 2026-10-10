package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/port"
)

type InboxRepository struct {
	pool *pgxpool.Pool
}

func NewInboxRepository(pool *pgxpool.Pool) port.InboxRepository {
	return &InboxRepository{pool: pool}
}

func (r *InboxRepository) TryLock(ctx context.Context, msg port.InboxMessage) (bool, *port.InboxMessage, error) {
	runner := getRunner(ctx, r.pool)

	// Try to insert
	insertQuery := `
		INSERT INTO inbox (consumer_name, message_id, payload_hash, received_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (consumer_name, message_id) DO NOTHING
	`

	tag, err := runner.Exec(ctx, insertQuery, msg.ConsumerName, msg.MessageID, msg.PayloadHash, msg.ReceivedAt)
	if err != nil {
		return false, nil, fmt.Errorf("failed to insert into inbox: %w", err)
	}

	inserted := tag.RowsAffected() > 0
	if inserted {
		return true, &msg, nil
	}

	// If not inserted, fetch the existing one
	selectQuery := `
		SELECT consumer_name, message_id, payload_hash, received_at, completed_at
		FROM inbox
		WHERE consumer_name = $1 AND message_id = $2
	`
	row := runner.QueryRow(ctx, selectQuery, msg.ConsumerName, msg.MessageID)

	var existing port.InboxMessage
	if err := row.Scan(&existing.ConsumerName, &existing.MessageID, &existing.PayloadHash, &existing.ReceivedAt, &existing.CompletedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil, fmt.Errorf("inbox row not found after conflict") // Should not happen
		}
		return false, nil, fmt.Errorf("failed to scan existing inbox message: %w", err)
	}

	return false, &existing, nil
}

func (r *InboxRepository) MarkCompleted(ctx context.Context, consumerName, messageID string, now time.Time) error {
	runner := getRunner(ctx, r.pool)

	query := `
		UPDATE inbox
		SET completed_at = $1
		WHERE consumer_name = $2 AND message_id = $3
	`

	tag, err := runner.Exec(ctx, query, now, consumerName, messageID)
	if err != nil {
		return fmt.Errorf("failed to mark inbox as completed: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return domain.WrapDomainError("INBOX_NOT_FOUND", "inbox message not found", nil)
	}

	return nil
}

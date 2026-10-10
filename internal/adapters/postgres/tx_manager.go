package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/port"
)

type txKey struct{}

// TxManagerImpl implements port.TxManager using pgxpool.
type TxManagerImpl struct {
	pool *pgxpool.Pool
}

// NewTxManager creates a new Postgres transaction manager.
func NewTxManager(pool *pgxpool.Pool) port.TxManager {
	return &TxManagerImpl{pool: pool}
}

// RunInTx executes the given function inside a database transaction.
func (m *TxManagerImpl) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	// If there's already a transaction in the context, just reuse it.
	if _, ok := extractTx(ctx); ok {
		return fn(ctx)
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	ctxWithTx := context.WithValue(ctx, txKey{}, tx)

	if err := fn(ctxWithTx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// extractTx retrieves the pgx.Tx from the context if it exists.
func extractTx(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

// getRunner returns a pgx.Tx if available in context, otherwise returns the pgxpool.Pool.
// This allows repositories to work both inside and outside of a RunInTx block.
func getRunner(ctx context.Context, pool *pgxpool.Pool) DBRunner {
	if tx, ok := extractTx(ctx); ok {
		return tx
	}
	return pool
}

// DBRunner is an interface satisfied by both pgxpool.Pool and pgx.Tx.
type DBRunner interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

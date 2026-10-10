package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain/ledger"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/money"
)

type LedgerRepository struct {
	pool *pgxpool.Pool
}

func NewLedgerRepository(pool *pgxpool.Pool) ledger.Repository {
	return &LedgerRepository{pool: pool}
}

func (r *LedgerRepository) Append(ctx context.Context, entry *ledger.Entry) error {
	runner := getRunner(ctx, r.pool)

	query := `
		INSERT INTO wallet_ledger (
			id, wallet_id, transaction_id, direction, amount, 
			balance_before, balance_after, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := runner.Exec(ctx, query,
		entry.ID(),
		entry.WalletID(),
		entry.TransactionID(),
		string(entry.Direction()),
		entry.Amount().MinorUnits(),
		entry.BalanceBefore().MinorUnits(),
		entry.BalanceAfter().MinorUnits(),
		entry.CreatedAt(),
	)
	if err != nil {
		return fmt.Errorf("failed to append ledger entry: %w", err)
	}
	return nil
}

func (r *LedgerRepository) FindByWalletPaginated(ctx context.Context, walletID string, cursor string, limit int) ([]*ledger.Entry, error) {
	runner := getRunner(ctx, r.pool)

	query := `
		SELECT l.id, l.wallet_id, l.transaction_id, l.direction, l.amount, l.balance_before, l.balance_after, l.created_at, w.currency
		FROM wallet_ledger l
		JOIN wallets w ON w.id = l.wallet_id
		WHERE l.wallet_id = $1
	`
	args := []any{walletID}

	if cursor != "" {
		query += ` AND l.created_at < (SELECT created_at FROM wallet_ledger WHERE id = $2) `
		args = append(args, cursor)
	}

	query += ` ORDER BY l.created_at DESC LIMIT $` + fmt.Sprint(len(args)+1)
	args = append(args, limit)

	rows, err := runner.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query ledger entries: %w", err)
	}
	defer rows.Close()

	var entries []*ledger.Entry
	for rows.Next() {
		var (
			id, wID, txID, direction, currency string
			amount, balBefore, balAfter        int64
			createdAt                          time.Time
		)
		if err := rows.Scan(&id, &wID, &txID, &direction, &amount, &balBefore, &balAfter, &createdAt, &currency); err != nil {
			return nil, fmt.Errorf("failed to scan ledger entry: %w", err)
		}

		amtMoney, err := money.FromMinor(amount, currency)
		if err != nil {
			return nil, fmt.Errorf("failed to parse amount: %w", err)
		}
		bbMoney, err := money.FromMinor(balBefore, currency)
		if err != nil {
			return nil, fmt.Errorf("failed to parse balance before: %w", err)
		}
		baMoney, err := money.FromMinor(balAfter, currency)
		if err != nil {
			return nil, fmt.Errorf("failed to parse balance after: %w", err)
		}

		entry, err := ledger.RehydrateEntry(
			id, 0, wID, txID, ledger.Direction(direction),
			amtMoney, bbMoney, baMoney, createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to rehydrate ledger entry: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

package postgres

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
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
		var parsedTime time.Time
		var parsedID string
		decoded, err := base64.URLEncoding.DecodeString(cursor)
		if err == nil {
			parts := strings.SplitN(string(decoded), "|", 2)
			if len(parts) == 2 {
				if t, err := time.Parse("2006-01-02T15:04:05.999999999Z07:00", parts[0]); err == nil {
					parsedTime = t
					parsedID = parts[1]
				}
			}
		}

		if !parsedTime.IsZero() && parsedID != "" {
			query += fmt.Sprintf(` AND (l.created_at < $%d OR (l.created_at = $%d AND l.id < $%d)) `, len(args)+1, len(args)+1, len(args)+2)
			args = append(args, parsedTime, parsedID)
		} else {
			// Legacy fallback for plain UUID cursor
			query += fmt.Sprintf(` AND (l.created_at < (SELECT created_at FROM wallet_ledger WHERE id = $%d) OR (l.created_at = (SELECT created_at FROM wallet_ledger WHERE id = $%d) AND l.id < $%d)) `, len(args)+1, len(args)+1, len(args)+1)
			args = append(args, cursor)
		}
	}

	query += ` ORDER BY l.created_at DESC, l.id DESC LIMIT $` + fmt.Sprint(len(args)+1)
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

func (r *LedgerRepository) CalculateSummary(ctx context.Context, walletID string) (int64, int64, error) {
	runner := getRunner(ctx, r.pool)

	query := `
		SELECT 
			COALESCE(SUM(CASE WHEN direction = 'CREDIT' THEN amount ELSE -amount END), 0),
			COUNT(*)
		FROM wallet_ledger
		WHERE wallet_id = $1
	`
	var (
		netBalance int64
		count      int64
	)
	if err := runner.QueryRow(ctx, query, walletID).Scan(&netBalance, &count); err != nil {
		return 0, 0, fmt.Errorf("failed to calculate ledger summary: %w", err)
	}

	return netBalance, count, nil
}

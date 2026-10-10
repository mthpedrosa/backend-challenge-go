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
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/wallet"
)

type WalletRepository struct {
	pool *pgxpool.Pool
}

func NewWalletRepository(pool *pgxpool.Pool) wallet.Repository {
	return &WalletRepository{pool: pool}
}

func (r *WalletRepository) Create(ctx context.Context, w *wallet.Wallet) error {
	runner := getRunner(ctx, r.pool)

	query := `
		INSERT INTO wallets (id, player_id, currency, balance, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := runner.Exec(ctx, query,
		w.ID(),
		w.PlayerID(),
		w.Currency(),
		w.Balance().MinorUnits(),
		w.Version(),
		w.CreatedAt(),
		w.UpdatedAt(),
	)
	if err != nil {
		return fmt.Errorf("failed to create wallet: %w", err)
	}
	return nil
}

func (r *WalletRepository) FindByID(ctx context.Context, id string) (*wallet.Wallet, error) {
	return r.findByID(ctx, id, false)
}

func (r *WalletRepository) FindByIDForUpdate(ctx context.Context, id string) (*wallet.Wallet, error) {
	return r.findByID(ctx, id, true)
}

func (r *WalletRepository) findByID(ctx context.Context, id string, forUpdate bool) (*wallet.Wallet, error) {
	runner := getRunner(ctx, r.pool)

	query := `SELECT id, player_id, currency, balance, version, created_at, updated_at FROM wallets WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}

	row := runner.QueryRow(ctx, query, id)
	return r.scanWallet(row)
}

func (r *WalletRepository) FindByPlayerAndCurrency(ctx context.Context, playerID, currency string) (*wallet.Wallet, error) {
	runner := getRunner(ctx, r.pool)

	query := `SELECT id, player_id, currency, balance, version, created_at, updated_at FROM wallets WHERE player_id = $1 AND currency = $2`
	row := runner.QueryRow(ctx, query, playerID, currency)
	return r.scanWallet(row)
}

func (r *WalletRepository) scanWallet(row pgx.Row) (*wallet.Wallet, error) {
	var id, playerID, currency string
	var balanceCents, version int64
	var createdAt, updatedAt time.Time // wait, time needs import. I'll fix imports with goimports.

	err := row.Scan(&id, &playerID, &currency, &balanceCents, &version, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.WrapDomainError("WALLET_NOT_FOUND", "wallet not found", nil)
		}
		return nil, fmt.Errorf("failed to scan wallet: %w", err)
	}

	m, err := money.FromMinor(balanceCents, currency)
	if err != nil {
		return nil, fmt.Errorf("invalid money in database: %w", err)
	}

	return wallet.Rehydrate(id, playerID, currency, m, version, createdAt, updatedAt)
}

func (r *WalletRepository) Update(ctx context.Context, w *wallet.Wallet) error {
	runner := getRunner(ctx, r.pool)

	query := `
		UPDATE wallets 
		SET balance = $1, version = $2, updated_at = $3
		WHERE id = $4 AND version = $5
	`
	// Optimistic locking: we only update if the version matches the one we started with (version - 1 since w.Version() is the new one)
	// Wait, the aggregate increments version on Debit/Credit. So w.Version() is the NEW version. The old one is w.Version() - 1.
	// We check against old version to detect concurrent modifications.
	oldVersion := w.Version() - 1

	tag, err := runner.Exec(ctx, query,
		w.Balance().MinorUnits(),
		w.Version(),
		w.UpdatedAt(),
		w.ID(),
		oldVersion,
	)
	if err != nil {
		return fmt.Errorf("failed to update wallet: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return domain.WrapDomainError("CONCURRENT_MODIFICATION", "wallet was modified concurrently", nil)
	}

	return nil
}

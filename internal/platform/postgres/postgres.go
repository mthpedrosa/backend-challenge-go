package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"

	"github.com/mthpedrosa/backend-challenge-go/internal/config"
)

// Module provides the PostgreSQL connection pool to the Fx application.
var Module = fx.Provide(NewPool)

// NewPool creates a new pgxpool.Pool from the configuration and registers
// its lifecycle with Fx.
func NewPool(lc fx.Lifecycle, cfg config.Config, logger *slog.Logger) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DB.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database url: %w", err)
	}

	// Adjust pool settings if needed
	poolConfig.MaxConns = 20
	poolConfig.MinConns = 2

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := pool.Ping(ctx); err != nil {
				if cfg.App.Env == "test" {
					logger.Warn("failed to ping database, ignoring because env=test", slog.Any("err", err))
					return nil
				}
				return fmt.Errorf("failed to ping database: %w", err)
			}
			logger.Info("connected to postgres", slog.Int("maxConns", int(poolConfig.MaxConns)))
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("closing postgres connection pool")
			pool.Close()
			return nil
		},
	})

	return pool, nil
}

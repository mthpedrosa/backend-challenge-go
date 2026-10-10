-- +goose Up
CREATE TABLE wallets (
    id UUID PRIMARY KEY,
    player_id VARCHAR(255) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    balance BIGINT NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT wallets_balance_check CHECK (balance >= 0),
    CONSTRAINT wallets_player_currency_unique UNIQUE (player_id, currency)
);

-- +goose Down
DROP TABLE wallets;

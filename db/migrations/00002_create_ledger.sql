-- +goose Up
CREATE TABLE wallet_ledger (
    id UUID PRIMARY KEY,
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    transaction_id VARCHAR(255) NOT NULL,
    direction VARCHAR(10) NOT NULL,
    amount BIGINT NOT NULL,
    balance_before BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT wallet_ledger_unique_tx UNIQUE (wallet_id, transaction_id),
    CONSTRAINT wallet_ledger_direction_check CHECK (direction IN ('CREDIT', 'DEBIT')),
    CONSTRAINT wallet_ledger_amount_check CHECK (amount > 0),
    CONSTRAINT wallet_ledger_math_check CHECK (
        (direction = 'CREDIT' AND balance_after = balance_before + amount) OR
        (direction = 'DEBIT' AND balance_after = balance_before - amount)
    )
);

CREATE INDEX idx_wallet_ledger_wallet_id ON wallet_ledger(wallet_id, created_at DESC);

-- +goose Down
DROP TABLE wallet_ledger;

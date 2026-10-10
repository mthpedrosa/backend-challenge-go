-- +goose Up
CREATE TABLE wager_transactions (
    id VARCHAR(255) PRIMARY KEY,
    origin VARCHAR(20) NOT NULL,
    kind VARCHAR(20) NOT NULL,
    status VARCHAR(30) NOT NULL,
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    player_id VARCHAR(255) NOT NULL,
    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL,
    provider_id VARCHAR(255),
    external_transaction_id VARCHAR(255),
    idempotency_key VARCHAR(512),
    payload_hash VARCHAR(255),
    round_id VARCHAR(255),
    game_id VARCHAR(255),
    reference_external_transaction_id VARCHAR(255),
    reference_transaction_id VARCHAR(255) REFERENCES wager_transactions(id),
    failure_code VARCHAR(100),
    balance_after BIGINT,
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT wager_transactions_amount_check CHECK (amount >= 0)
);

CREATE UNIQUE INDEX idx_wager_transactions_idempotency 
ON wager_transactions(idempotency_key) 
WHERE idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX idx_wager_transactions_provider_ext 
ON wager_transactions(provider_id, external_transaction_id) 
WHERE provider_id IS NOT NULL AND external_transaction_id IS NOT NULL;

CREATE INDEX idx_wager_transactions_wallet ON wager_transactions(wallet_id);
CREATE INDEX idx_wager_transactions_pending_refs ON wager_transactions(status, next_attempt_at) WHERE status = 'PENDING_REFERENCE';

-- +goose Down
DROP TABLE wager_transactions;

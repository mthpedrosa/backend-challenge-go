-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION prevent_ledger_update_or_delete()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'wallet_ledger entries are append-only and cannot be updated or deleted';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trigger_prevent_ledger_update_or_delete
BEFORE UPDATE OR DELETE ON wallet_ledger
FOR EACH ROW
EXECUTE FUNCTION prevent_ledger_update_or_delete();

-- Single-slot reversal: a referenced transaction cannot receive two processed reversals
CREATE UNIQUE INDEX ux_tx_one_reversal 
ON wager_transactions (reference_transaction_id) 
WHERE kind IN ('REFUND', 'ROLLBACK') AND status = 'PROCESSED';

-- Multi-tenant composite idempotency index scoped to provider
DROP INDEX IF EXISTS idx_wager_transactions_idempotency;
CREATE UNIQUE INDEX idx_wager_transactions_provider_idempotency
ON wager_transactions (provider_id, idempotency_key)
WHERE provider_id IS NOT NULL AND idempotency_key IS NOT NULL;

-- +goose Down
DROP TRIGGER IF EXISTS trigger_prevent_ledger_update_or_delete ON wallet_ledger;
DROP FUNCTION IF EXISTS prevent_ledger_update_or_delete();
DROP INDEX IF EXISTS ux_tx_one_reversal;
DROP INDEX IF EXISTS idx_wager_transactions_provider_idempotency;
CREATE UNIQUE INDEX idx_wager_transactions_idempotency 
ON wager_transactions (idempotency_key) 
WHERE idempotency_key IS NOT NULL;

-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'transfer_status') THEN
        CREATE TYPE transfer_status AS ENUM ('pending', 'completed', 'failed', 'declined');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS transfers (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    from_wallet_id  UUID            NOT NULL REFERENCES wallets(id),
    to_wallet_id    UUID            NOT NULL REFERENCES wallets(id),
    amount          BIGINT          NOT NULL CHECK (amount > 0),
    status          transfer_status NOT NULL DEFAULT 'pending',
    idempotency_key TEXT            NOT NULL,
    initiated_by    UUID            NOT NULL REFERENCES users(id),
    failure_reason  TEXT,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_different_wallets CHECK (from_wallet_id <> to_wallet_id)
);

CREATE INDEX IF NOT EXISTS idx_transfers_from_wallet  ON transfers(from_wallet_id);
CREATE INDEX IF NOT EXISTS idx_transfers_to_wallet    ON transfers(to_wallet_id);
CREATE INDEX IF NOT EXISTS idx_transfers_idem_key     ON transfers(idempotency_key, initiated_by);
CREATE INDEX IF NOT EXISTS idx_transfers_initiated_by ON transfers(initiated_by);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS transfers CASCADE;
DROP TYPE IF EXISTS transfer_status;
-- +goose StatementEnd

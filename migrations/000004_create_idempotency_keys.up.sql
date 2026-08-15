CREATE TABLE IF NOT EXISTS idempotency_keys (
    idempotency_key TEXT        NOT NULL,
    user_id         UUID        NOT NULL REFERENCES users(id),
    request_hash    TEXT        NOT NULL,
    transfer_id     UUID        REFERENCES transfers(id),
    response_status INT         NOT NULL,
    response_body   JSONB       NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours',

    PRIMARY KEY (idempotency_key, user_id)
);

CREATE INDEX IF NOT EXISTS idx_idem_keys_expires ON idempotency_keys(expires_at);

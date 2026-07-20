CREATE TABLE IF NOT EXISTS payment_integration (
    provider VARCHAR(32) PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    encrypted_config TEXT NOT NULL DEFAULT '',
    webhook_token VARCHAR(96) NOT NULL,
    updated_by BIGINT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE purchase
    ADD COLUMN IF NOT EXISTS external_payment_id VARCHAR(160),
    ADD COLUMN IF NOT EXISTS external_payment_url TEXT;

CREATE INDEX IF NOT EXISTS purchase_external_payment_lookup_idx
    ON purchase (invoice_type, external_payment_id)
    WHERE external_payment_id IS NOT NULL;


CREATE TABLE IF NOT EXISTS moynalog_receipt (
    purchase_id BIGINT PRIMARY KEY REFERENCES purchase(id) ON DELETE CASCADE,
    status VARCHAR(16) NOT NULL DEFAULT 'processing'
        CHECK (status IN ('processing', 'succeeded', 'failed', 'uncertain')),
    receipt_uuid TEXT NOT NULL DEFAULT '',
    item_name TEXT NOT NULL DEFAULT '',
    amount NUMERIC(12, 2) NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    attempt_count INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    succeeded_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS moynalog_receipt_updated_at_idx
    ON moynalog_receipt (updated_at DESC);

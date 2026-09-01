ALTER TABLE customer
    ADD COLUMN IF NOT EXISTS is_blocked BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS blocked_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS customer_admin_search_idx
    ON customer (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS customer_blocked_idx
    ON customer (is_blocked)
    WHERE is_blocked;

ALTER TABLE customer
    ADD COLUMN IF NOT EXISTS autopay_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS autopay_plan_months INTEGER,
    ADD COLUMN IF NOT EXISTS yookasa_payment_method_id UUID,
    ADD COLUMN IF NOT EXISTS yookasa_payment_method_type VARCHAR(32),
    ADD COLUMN IF NOT EXISTS yookasa_payment_method_title TEXT,
    ADD COLUMN IF NOT EXISTS yookasa_payment_method_saved_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS yookasa_last_charge_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS yookasa_last_charge_status VARCHAR(20),
    ADD COLUMN IF NOT EXISTS yookasa_last_charge_error TEXT;

ALTER TABLE purchase
    ADD COLUMN IF NOT EXISTS agreement_accepted BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS is_auto_payment BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS parent_purchase_id BIGINT REFERENCES purchase (id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS yookasa_payment_method_id UUID,
    ADD COLUMN IF NOT EXISTS yookasa_payment_method_type VARCHAR(32),
    ADD COLUMN IF NOT EXISTS yookasa_payment_method_title TEXT,
    ADD COLUMN IF NOT EXISTS yookasa_payment_method_saved BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_customer_autopay_enabled
    ON customer (autopay_enabled, expire_at);

CREATE INDEX IF NOT EXISTS idx_purchase_customer_created_at
    ON purchase (customer_id, created_at DESC);

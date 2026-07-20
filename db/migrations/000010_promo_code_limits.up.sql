ALTER TABLE promo_code
    ADD COLUMN IF NOT EXISTS max_redemptions INTEGER CHECK (max_redemptions IS NULL OR max_redemptions > 0),
    ADD COLUMN IF NOT EXISTS redemption_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP WITH TIME ZONE;

ALTER TABLE purchase
    ADD COLUMN IF NOT EXISTS promo_code_id BIGINT REFERENCES promo_code (id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS promo_code_snapshot VARCHAR(64),
    ADD COLUMN IF NOT EXISTS promo_code_discount_percent INTEGER;

CREATE TABLE IF NOT EXISTS promo_code_redemption
(
    id            BIGSERIAL PRIMARY KEY,
    promo_code_id BIGINT                   NOT NULL REFERENCES promo_code (id) ON DELETE CASCADE,
    customer_id   BIGINT                   NOT NULL REFERENCES customer (id) ON DELETE CASCADE,
    purchase_id   BIGINT                   NOT NULL UNIQUE REFERENCES purchase (id) ON DELETE CASCADE,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (promo_code_id, customer_id)
);

CREATE INDEX IF NOT EXISTS idx_purchase_customer_promo_status
    ON purchase (customer_id, promo_code_id, status);

CREATE INDEX IF NOT EXISTS idx_promo_code_redemption_customer
    ON promo_code_redemption (customer_id, promo_code_id);

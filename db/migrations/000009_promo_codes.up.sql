CREATE TABLE IF NOT EXISTS promo_code
(
    id                     BIGSERIAL PRIMARY KEY,
    code                   VARCHAR(64)              NOT NULL UNIQUE,
    discount_percent       INTEGER                  NOT NULL CHECK (discount_percent > 0 AND discount_percent < 100),
    is_active              BOOLEAN                  NOT NULL DEFAULT TRUE,
    expires_at             TIMESTAMP WITH TIME ZONE,
    created_by_telegram_id BIGINT                   NOT NULL,
    created_at             TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_promo_code_active_expires
    ON promo_code (is_active, expires_at);

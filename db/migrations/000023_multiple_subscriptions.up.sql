CREATE TABLE IF NOT EXISTS customer_subscription (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL REFERENCES customer(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL DEFAULT 'Основная',
    position SMALLINT NOT NULL,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    panel_user_id BIGINT,
    panel_user_uuid UUID,
    subscription_link TEXT,
    expire_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT customer_subscription_position_range CHECK (position BETWEEN 1 AND 3),
    CONSTRAINT customer_subscription_display_name_length CHECK (char_length(btrim(display_name)) BETWEEN 1 AND 40),
    CONSTRAINT customer_subscription_customer_position UNIQUE (customer_id, position)
);

CREATE UNIQUE INDEX IF NOT EXISTS customer_subscription_primary_unique
    ON customer_subscription(customer_id)
    WHERE is_primary;

CREATE INDEX IF NOT EXISTS customer_subscription_customer_idx
    ON customer_subscription(customer_id, position);

INSERT INTO customer_subscription (
    customer_id,
    display_name,
    position,
    is_primary,
    subscription_link,
    expire_at
)
SELECT
    id,
    'Основная',
    1,
    TRUE,
    subscription_link,
    expire_at
FROM customer
ON CONFLICT (customer_id, position) DO NOTHING;

CREATE TABLE IF NOT EXISTS customer_subscription_selection (
    customer_id BIGINT PRIMARY KEY REFERENCES customer(id) ON DELETE CASCADE,
    subscription_id BIGINT NOT NULL REFERENCES customer_subscription(id) ON DELETE CASCADE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE purchase
    ADD COLUMN IF NOT EXISTS subscription_id BIGINT;

UPDATE purchase AS p
SET subscription_id = s.id
FROM customer_subscription AS s
WHERE p.subscription_id IS NULL
  AND s.customer_id = p.customer_id
  AND s.is_primary;

CREATE INDEX IF NOT EXISTS purchase_subscription_idx
    ON purchase(subscription_id, status, paid_at DESC);

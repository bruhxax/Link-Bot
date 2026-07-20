CREATE TABLE review (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL REFERENCES customer(id) ON DELETE CASCADE,
    telegram_id BIGINT NOT NULL,
    telegram_username TEXT NOT NULL DEFAULT '',
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment TEXT NOT NULL,
    reward_granted BOOLEAN NOT NULL DEFAULT FALSE,
    reward_days INTEGER NOT NULL DEFAULT 0,
    reward_traffic_bytes BIGINT NOT NULL DEFAULT 0,
    reward_granted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT review_customer_id_key UNIQUE (customer_id),
    CONSTRAINT review_telegram_id_key UNIQUE (telegram_id)
);

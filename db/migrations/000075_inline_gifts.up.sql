CREATE TABLE inline_gift (
    id UUID PRIMARY KEY,
    sender_telegram_id BIGINT NOT NULL,
    plan_id TEXT NOT NULL,
    inline_message_id TEXT NOT NULL UNIQUE,
    has_banner BOOLEAN NOT NULL DEFAULT FALSE,
    purchase_id BIGINT UNIQUE REFERENCES purchase(id),
    checkout_started_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX inline_gift_sender_idx ON inline_gift (sender_telegram_id, created_at DESC);

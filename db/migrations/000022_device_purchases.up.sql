ALTER TABLE purchase
    ADD COLUMN IF NOT EXISTS purchase_kind TEXT NOT NULL DEFAULT 'subscription',
    ADD COLUMN IF NOT EXISTS extra_devices INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS device_notification_state (
    telegram_id BIGINT PRIMARY KEY,
    device_count INTEGER NOT NULL DEFAULT 0,
    limit_reached BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

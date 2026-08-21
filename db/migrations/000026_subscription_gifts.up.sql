ALTER TABLE customer
    ADD COLUMN IF NOT EXISTS telegram_username TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS customer_telegram_username_unique_idx
    ON customer (lower(telegram_username))
    WHERE telegram_username IS NOT NULL AND btrim(telegram_username) <> '';

ALTER TABLE purchase
    ADD COLUMN IF NOT EXISTS gift_recipient_username TEXT,
    ADD COLUMN IF NOT EXISTS gift_recipient_customer_id BIGINT REFERENCES customer(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS gift_token UUID,
    ADD COLUMN IF NOT EXISTS gift_delivered_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS gift_sender_seen_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS purchase_gift_token_unique_idx
    ON purchase (gift_token)
    WHERE gift_token IS NOT NULL;

CREATE INDEX IF NOT EXISTS purchase_pending_gift_username_idx
    ON purchase (lower(gift_recipient_username), paid_at, id)
    WHERE purchase_kind = 'gift' AND status = 'paid' AND gift_delivered_at IS NULL;

CREATE INDEX IF NOT EXISTS purchase_unseen_gift_sender_idx
    ON purchase (customer_id, paid_at DESC, id DESC)
    WHERE purchase_kind = 'gift' AND status = 'paid' AND gift_sender_seen_at IS NULL;

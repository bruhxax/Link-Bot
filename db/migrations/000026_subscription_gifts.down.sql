DROP INDEX IF EXISTS purchase_unseen_gift_sender_idx;
DROP INDEX IF EXISTS purchase_pending_gift_username_idx;
DROP INDEX IF EXISTS purchase_gift_token_unique_idx;

ALTER TABLE purchase
    DROP COLUMN IF EXISTS gift_sender_seen_at,
    DROP COLUMN IF EXISTS gift_delivered_at,
    DROP COLUMN IF EXISTS gift_token,
    DROP COLUMN IF EXISTS gift_recipient_customer_id,
    DROP COLUMN IF EXISTS gift_recipient_username;

DROP INDEX IF EXISTS customer_telegram_username_unique_idx;

ALTER TABLE customer
    DROP COLUMN IF EXISTS telegram_username;

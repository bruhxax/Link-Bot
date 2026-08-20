DROP INDEX IF EXISTS purchase_subscription_idx;

ALTER TABLE purchase
    DROP COLUMN IF EXISTS subscription_id;

DROP TABLE IF EXISTS customer_subscription_selection;
DROP TABLE IF EXISTS customer_subscription;

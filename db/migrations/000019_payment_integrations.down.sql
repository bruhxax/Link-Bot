DROP INDEX IF EXISTS purchase_external_payment_lookup_idx;

ALTER TABLE purchase
    DROP COLUMN IF EXISTS external_payment_url,
    DROP COLUMN IF EXISTS external_payment_id;

DROP TABLE IF EXISTS payment_integration;


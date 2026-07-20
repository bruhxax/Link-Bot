DROP INDEX IF EXISTS idx_promo_code_redemption_customer;
DROP INDEX IF EXISTS idx_purchase_customer_promo_status;

DROP TABLE IF EXISTS promo_code_redemption;

ALTER TABLE purchase
    DROP COLUMN IF EXISTS promo_code_discount_percent,
    DROP COLUMN IF EXISTS promo_code_snapshot,
    DROP COLUMN IF EXISTS promo_code_id;

ALTER TABLE promo_code
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS redemption_count,
    DROP COLUMN IF EXISTS max_redemptions;

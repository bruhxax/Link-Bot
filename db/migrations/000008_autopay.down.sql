DROP INDEX IF EXISTS idx_purchase_customer_created_at;
DROP INDEX IF EXISTS idx_customer_autopay_enabled;

ALTER TABLE purchase
    DROP COLUMN IF EXISTS yookasa_payment_method_saved,
    DROP COLUMN IF EXISTS yookasa_payment_method_title,
    DROP COLUMN IF EXISTS yookasa_payment_method_type,
    DROP COLUMN IF EXISTS yookasa_payment_method_id,
    DROP COLUMN IF EXISTS parent_purchase_id,
    DROP COLUMN IF EXISTS is_auto_payment,
    DROP COLUMN IF EXISTS agreement_accepted;

ALTER TABLE customer
    DROP COLUMN IF EXISTS yookasa_last_charge_error,
    DROP COLUMN IF EXISTS yookasa_last_charge_status,
    DROP COLUMN IF EXISTS yookasa_last_charge_at,
    DROP COLUMN IF EXISTS yookasa_payment_method_saved_at,
    DROP COLUMN IF EXISTS yookasa_payment_method_title,
    DROP COLUMN IF EXISTS yookasa_payment_method_type,
    DROP COLUMN IF EXISTS yookasa_payment_method_id,
    DROP COLUMN IF EXISTS autopay_plan_months,
    DROP COLUMN IF EXISTS autopay_enabled;

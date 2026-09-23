DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM promo_code WHERE reward_type <> 'discount')
       OR EXISTS (SELECT 1 FROM promo_code_redemption WHERE purchase_id IS NULL) THEN
        RAISE EXCEPTION 'Cannot roll back promo rewards while reward codes or redemptions exist';
    END IF;
END $$;

ALTER TABLE promo_code_redemption
    DROP CONSTRAINT promo_code_redemption_status_check,
    DROP COLUMN target_traffic_bytes,
    DROP COLUMN target_expires_at,
    DROP COLUMN subscription_id,
    DROP COLUMN status,
    ALTER COLUMN purchase_id SET NOT NULL;

ALTER TABLE promo_code
    DROP CONSTRAINT promo_code_reward_check,
    DROP COLUMN reward_traffic_gb,
    DROP COLUMN reward_value,
    DROP COLUMN reward_type,
    ADD CONSTRAINT promo_code_discount_percent_check CHECK (discount_percent > 0 AND discount_percent < 100);

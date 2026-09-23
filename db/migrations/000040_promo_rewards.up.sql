ALTER TABLE promo_code
    DROP CONSTRAINT IF EXISTS promo_code_discount_percent_check;

ALTER TABLE promo_code
    ADD COLUMN reward_type VARCHAR(16) NOT NULL DEFAULT 'discount',
    ADD COLUMN reward_value INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN reward_traffic_gb INTEGER NOT NULL DEFAULT 0,
    ADD CONSTRAINT promo_code_reward_check CHECK (
        (reward_type = 'discount' AND discount_percent BETWEEN 1 AND 99 AND reward_value = 0 AND reward_traffic_gb = 0)
        OR (reward_type = 'balance' AND discount_percent = 0 AND reward_value BETWEEN 1 AND 1000000 AND reward_traffic_gb = 0)
        OR (reward_type = 'days' AND discount_percent = 0 AND reward_value BETWEEN 1 AND 3650 AND reward_traffic_gb = 0)
        OR (reward_type = 'traffic' AND discount_percent = 0 AND reward_value BETWEEN 1 AND 1000000 AND reward_traffic_gb = 0)
        OR (reward_type = 'days_traffic' AND discount_percent = 0 AND reward_value BETWEEN 1 AND 3650 AND reward_traffic_gb BETWEEN 1 AND 1000000)
    );

ALTER TABLE promo_code_redemption
    ALTER COLUMN purchase_id DROP NOT NULL,
    ADD COLUMN status VARCHAR(16) NOT NULL DEFAULT 'applied',
    ADD COLUMN subscription_id BIGINT REFERENCES customer_subscription (id) ON DELETE SET NULL,
    ADD COLUMN target_expires_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN target_traffic_bytes BIGINT,
    ADD CONSTRAINT promo_code_redemption_status_check CHECK (status IN ('pending', 'applied'));

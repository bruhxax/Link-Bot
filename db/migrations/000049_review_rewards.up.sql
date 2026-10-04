ALTER TABLE review ADD COLUMN reward_settings JSONB NOT NULL DEFAULT '{"days":2,"trafficGb":20,"balanceRub":0,"promo":{"enabled":false}}';

CREATE TABLE review_reward_claim (
    review_id BIGINT PRIMARY KEY REFERENCES review(id) ON DELETE CASCADE,
    target_initialized BOOLEAN NOT NULL DEFAULT FALSE,
    subscription_id BIGINT REFERENCES customer_subscription(id) ON DELETE SET NULL,
    panel_user_id BIGINT NOT NULL DEFAULT 0,
    panel_user_uuid UUID,
    target_expires_at TIMESTAMPTZ,
    target_traffic_bytes BIGINT,
    access_applied BOOLEAN NOT NULL DEFAULT FALSE
);

ALTER TABLE promo_code ADD COLUMN owner_customer_id BIGINT REFERENCES customer(id) ON DELETE CASCADE;
ALTER TABLE promo_code ADD COLUMN review_id BIGINT UNIQUE REFERENCES review(id) ON DELETE CASCADE;
CREATE INDEX promo_code_owner_idx ON promo_code(owner_customer_id) WHERE owner_customer_id IS NOT NULL;
ALTER TABLE promo_code ADD CONSTRAINT promo_code_personal_limit CHECK (owner_customer_id IS NULL OR (max_redemptions IS NOT NULL AND max_redemptions = 1));

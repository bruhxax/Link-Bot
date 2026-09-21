ALTER TABLE customer_subscription
    DROP CONSTRAINT IF EXISTS customer_subscription_position_range;

ALTER TABLE customer_subscription
    ADD CONSTRAINT customer_subscription_position_range
        CHECK (position BETWEEN 1 AND 10);

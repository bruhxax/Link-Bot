DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM customer_subscription WHERE position > 3) THEN
        RAISE EXCEPTION 'cannot roll back subscription limit while positions 4-10 are in use';
    END IF;
END $$;

ALTER TABLE customer_subscription
    DROP CONSTRAINT IF EXISTS customer_subscription_position_range;

ALTER TABLE customer_subscription
    ADD CONSTRAINT customer_subscription_position_range
        CHECK (position BETWEEN 1 AND 3);

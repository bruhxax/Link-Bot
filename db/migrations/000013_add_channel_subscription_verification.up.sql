ALTER TABLE customer
    ADD COLUMN channel_subscription_verified_at TIMESTAMP WITH TIME ZONE;

UPDATE customer
SET channel_subscription_verified_at = COALESCE(created_at, CURRENT_TIMESTAMP)
WHERE channel_subscription_verified_at IS NULL;

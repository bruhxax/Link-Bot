ALTER TABLE device_notification_state
    ADD COLUMN IF NOT EXISTS subscription_id BIGINT;

UPDATE device_notification_state AS state
SET subscription_id = subscription.id
FROM customer
JOIN customer_subscription AS subscription
  ON subscription.customer_id = customer.id
 AND subscription.is_primary
WHERE state.telegram_id = customer.telegram_id
  AND state.subscription_id IS NULL;

DELETE FROM device_notification_state
WHERE subscription_id IS NULL;

ALTER TABLE device_notification_state
    DROP CONSTRAINT IF EXISTS device_notification_state_pkey;

ALTER TABLE device_notification_state
    ALTER COLUMN subscription_id SET NOT NULL,
    ADD CONSTRAINT device_notification_state_subscription_fk
        FOREIGN KEY (subscription_id) REFERENCES customer_subscription(id) ON DELETE CASCADE,
    ADD PRIMARY KEY (telegram_id, subscription_id);

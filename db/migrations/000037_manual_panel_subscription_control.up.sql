-- A panel profile attached by an administrator may be managed outside Link-Bot.
-- Keep it out of tariff reconciliation until the customer makes a new bot purchase.
CREATE TABLE IF NOT EXISTS customer_subscription_manual_control (
    subscription_id BIGINT PRIMARY KEY REFERENCES customer_subscription(id) ON DELETE CASCADE,
    marked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Existing attached profiles without a successful tariff purchase are manual
-- subscriptions as well. This prevents the first Mini App refresh after the
-- upgrade from overwriting their panel traffic or device limits.
INSERT INTO customer_subscription_manual_control (subscription_id)
SELECT s.id
FROM customer_subscription AS s
WHERE (s.panel_user_id IS NOT NULL OR s.panel_user_uuid IS NOT NULL)
  AND NOT EXISTS (
      SELECT 1
      FROM purchase AS p
      WHERE p.subscription_id = s.id
        AND p.status = 'paid'
        AND p.month > 0
  )
ON CONFLICT (subscription_id) DO NOTHING;

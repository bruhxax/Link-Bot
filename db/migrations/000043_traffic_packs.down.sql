DROP TRIGGER IF EXISTS miniapp_traffic_limit_changed ON subscription_traffic_limit;
DROP TABLE IF EXISTS subscription_traffic_limit;
DROP INDEX IF EXISTS purchase_traffic_grants_idx;
ALTER TABLE purchase DROP COLUMN IF EXISTS extra_traffic_unlimited;
ALTER TABLE purchase DROP COLUMN IF EXISTS extra_traffic_bytes;

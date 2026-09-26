DROP TABLE IF EXISTS subscription_device_grant;
DROP TABLE IF EXISTS subscription_device_limit;
ALTER TABLE purchase DROP COLUMN IF EXISTS device_expires_at;

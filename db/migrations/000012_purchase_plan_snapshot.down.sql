ALTER TABLE purchase
    DROP COLUMN IF EXISTS device_limit_count,
    DROP COLUMN IF EXISTS traffic_limit_bytes,
    DROP COLUMN IF EXISTS plan_id;

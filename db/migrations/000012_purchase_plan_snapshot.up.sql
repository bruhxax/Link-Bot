ALTER TABLE purchase
    ADD COLUMN IF NOT EXISTS plan_id VARCHAR(64),
    ADD COLUMN IF NOT EXISTS traffic_limit_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS device_limit_count INTEGER;

CREATE TABLE IF NOT EXISTS subscription_grace_delivery (
    customer_id BIGINT NOT NULL REFERENCES customer(id) ON DELETE CASCADE,
    source_expire_at TIMESTAMPTZ NOT NULL,
    grace_expire_at TIMESTAMPTZ,
    applied_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (customer_id, source_expire_at)
);

CREATE INDEX IF NOT EXISTS subscription_grace_delivery_grace_expire_idx
    ON subscription_grace_delivery (customer_id, grace_expire_at)
    WHERE grace_expire_at IS NOT NULL;

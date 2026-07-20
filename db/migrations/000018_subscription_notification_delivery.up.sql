CREATE TABLE IF NOT EXISTS subscription_notification_delivery (
    customer_id BIGINT NOT NULL REFERENCES customer (id) ON DELETE CASCADE,
    expire_at TIMESTAMPTZ NOT NULL,
    kind TEXT NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (customer_id, expire_at, kind),
    CONSTRAINT subscription_notification_delivery_kind_check
        CHECK (kind IN ('expiring', 'expired'))
);

CREATE INDEX IF NOT EXISTS idx_subscription_notification_delivery_sent_at
    ON subscription_notification_delivery (sent_at);

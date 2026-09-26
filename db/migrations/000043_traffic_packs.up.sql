ALTER TABLE purchase ADD COLUMN extra_traffic_bytes BIGINT NOT NULL DEFAULT 0 CHECK (extra_traffic_bytes >= 0);
ALTER TABLE purchase ADD COLUMN extra_traffic_unlimited BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE subscription_traffic_limit (
    subscription_id BIGINT PRIMARY KEY REFERENCES customer_subscription(id) ON DELETE CASCADE,
    base_limit BIGINT NOT NULL CHECK (base_limit >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX purchase_traffic_grants_idx ON purchase(subscription_id)
    WHERE extra_traffic_bytes > 0 OR extra_traffic_unlimited;

CREATE TRIGGER miniapp_traffic_limit_changed AFTER INSERT OR UPDATE OR DELETE ON subscription_traffic_limit
    FOR EACH STATEMENT EXECUTE FUNCTION miniapp_notify_change();

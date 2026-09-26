ALTER TABLE purchase ADD COLUMN IF NOT EXISTS device_expires_at TIMESTAMPTZ;

CREATE TABLE subscription_device_limit (
    subscription_id BIGINT PRIMARY KEY REFERENCES customer_subscription(id) ON DELETE CASCADE,
    base_limit INTEGER NOT NULL CHECK (base_limit >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE subscription_device_grant (
    purchase_id BIGINT PRIMARY KEY REFERENCES purchase(id) ON DELETE CASCADE,
    subscription_id BIGINT NOT NULL REFERENCES customer_subscription(id) ON DELETE CASCADE,
    devices INTEGER NOT NULL CHECK (devices > 0),
    expires_at TIMESTAMPTZ NOT NULL,
    expired_applied_at TIMESTAMPTZ,
    purchase_notified_at TIMESTAMPTZ,
    reminder_notified_at TIMESTAMPTZ,
    expiry_notified_at TIMESTAMPTZ
);
CREATE INDEX subscription_device_grant_subscription_idx ON subscription_device_grant(subscription_id);
CREATE INDEX subscription_device_grant_expiry_idx ON subscription_device_grant(expires_at) WHERE expired_applied_at IS NULL;

CREATE TRIGGER miniapp_device_limit_changed AFTER INSERT OR UPDATE OR DELETE ON subscription_device_limit
    FOR EACH STATEMENT EXECUTE FUNCTION miniapp_notify_change();
CREATE TRIGGER miniapp_device_grant_changed AFTER INSERT OR UPDATE OR DELETE ON subscription_device_grant
    FOR EACH STATEMENT EXECUTE FUNCTION miniapp_notify_change();

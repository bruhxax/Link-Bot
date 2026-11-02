CREATE TABLE IF NOT EXISTS recurring_method (
    customer_id BIGINT PRIMARY KEY REFERENCES customer(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    reference TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    payer_phone TEXT,
    plan_id TEXT,
    source_purchase_id BIGINT NOT NULL DEFAULT 0,
    months INTEGER NOT NULL CHECK (months > 0),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_attempt_at TIMESTAMPTZ,
    last_charge_status TEXT,
    last_charge_error TEXT,
    saved_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO recurring_method (customer_id, provider, reference, title, months, enabled, saved_at)
SELECT id, 'yookassa', yookasa_payment_method_id::TEXT,
       COALESCE(yookasa_payment_method_title, 'YooKassa'),
       autopay_plan_months, autopay_enabled,
       COALESCE(yookasa_payment_method_saved_at, NOW())
FROM customer
WHERE yookasa_payment_method_id IS NOT NULL AND autopay_plan_months > 0
ON CONFLICT (customer_id) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_recurring_method_due ON recurring_method (enabled, provider) WHERE enabled;

CREATE TABLE IF NOT EXISTS recurring_enrollment (
    purchase_id BIGINT PRIMARY KEY REFERENCES purchase(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    reference TEXT,
    payer_phone TEXT,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS recurring_subscription (
    provider TEXT NOT NULL,
    external_id TEXT NOT NULL,
    customer_id BIGINT NOT NULL REFERENCES customer(id) ON DELETE CASCADE,
    source_purchase_id BIGINT NOT NULL REFERENCES purchase(id) ON DELETE CASCADE,
    plan_id TEXT,
    months INTEGER NOT NULL CHECK (months > 0),
    amount NUMERIC(12,2) NOT NULL CHECK (amount > 0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_checked_at TIMESTAMPTZ,
    stopped_at TIMESTAMPTZ,
    PRIMARY KEY (provider, external_id),
    UNIQUE (provider, source_purchase_id)
);
CREATE INDEX IF NOT EXISTS idx_recurring_subscription_customer ON recurring_subscription (customer_id, active);
CREATE INDEX IF NOT EXISTS idx_recurring_subscription_lava_due ON recurring_subscription (last_checked_at) WHERE active AND provider='lava';

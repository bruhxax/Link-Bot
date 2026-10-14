-- No cascading owner FK: technical profiles must be disabled even after a
-- customer/subscription is deleted. The reconciler handles orphan records.
CREATE TABLE device_traffic_binding (
 id BIGSERIAL PRIMARY KEY,
 parent_key TEXT NOT NULL UNIQUE,
 parent_id BIGINT NOT NULL,
 parent_uuid UUID NOT NULL,
 subscription_id BIGINT REFERENCES customer_subscription(id) ON DELETE SET NULL,
 token UUID NOT NULL UNIQUE,
 squads JSONB NOT NULL,
 slots JSONB NOT NULL DEFAULT '[]'::jsonb,
 isolated BOOLEAN NOT NULL DEFAULT FALSE,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Keep actor/target snapshots without foreign keys: removing a role or account
-- must not erase the history of actions already performed.
CREATE TABLE admin_activity (
    id BIGSERIAL PRIMARY KEY,
    actor_telegram_id BIGINT NOT NULL,
    actor_name TEXT NOT NULL DEFAULT '',
    actor_role TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    category TEXT NOT NULL,
    title TEXT NOT NULL,
    target_customer_id BIGINT NOT NULL DEFAULT 0,
    target_telegram_id BIGINT NOT NULL DEFAULT 0,
    target_name TEXT NOT NULL DEFAULT '',
    details JSONB NOT NULL DEFAULT '[]',
    status TEXT NOT NULL CHECK (status IN ('pending', 'success', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);
CREATE INDEX admin_activity_actor_cursor ON admin_activity(actor_telegram_id, id DESC);

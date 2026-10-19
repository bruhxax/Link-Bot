CREATE TABLE app_update_notification_state (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    revision TEXT NOT NULL DEFAULT '',
    notified_at TIMESTAMPTZ,
    lease_until TIMESTAMPTZ
);

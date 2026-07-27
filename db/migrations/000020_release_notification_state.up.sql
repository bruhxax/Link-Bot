CREATE TABLE IF NOT EXISTS app_release_notification_state (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    revision TEXT NOT NULL,
    notified_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

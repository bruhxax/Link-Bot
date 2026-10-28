ALTER TABLE user_activity DROP CONSTRAINT user_activity_source_check;
ALTER TABLE user_activity ADD CONSTRAINT user_activity_source_check
    CHECK (source IN ('miniapp', 'telegram', 'payment', 'remnawave'));

-- Store only delivery state, never the raw panel webhook (which contains keys).
CREATE TABLE torrent_alert (
    event_key TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notified_at TIMESTAMPTZ,
    notification_token UUID,
    notification_lease_until TIMESTAMPTZ
);

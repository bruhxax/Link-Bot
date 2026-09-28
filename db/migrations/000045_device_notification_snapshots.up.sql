ALTER TABLE device_notification_state
    ADD COLUMN device_hwids TEXT[],
    ADD COLUMN panel_user_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN observed_at TIMESTAMPTZ;

-- Existing counts may have been reset by a failed panel request. A NULL HWID
-- list establishes a fresh baseline on the first successful read, silently.

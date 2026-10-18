CREATE TABLE bot_pinned_message (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    revision BIGINT NOT NULL DEFAULT 1,
    source_version BIGINT NOT NULL DEFAULT 0,
    source_chat_id BIGINT,
    source_message_id INTEGER,
    source_preview TEXT NOT NULL DEFAULT '',
    source_kind TEXT NOT NULL DEFAULT '',
    capture_admin_id BIGINT,
    capture_started_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO bot_pinned_message(id) VALUES(1);

-- Keep the exact copied message even after unpinning: re-enabling must not
-- duplicate announcements or touch other pinned messages in the user's chat.
CREATE TABLE bot_pinned_delivery (
    chat_id BIGINT PRIMARY KEY,
    message_id INTEGER,
    source_version BIGINT NOT NULL DEFAULT 0,
    applied_revision BIGINT NOT NULL DEFAULT 0,
    attempt_revision BIGINT NOT NULL DEFAULT 0,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE bot_broadcast_draft ADD COLUMN delete_after_minutes INTEGER NOT NULL DEFAULT 0
    CHECK (delete_after_minutes BETWEEN 0 AND 2820);

CREATE TABLE broadcast_message_deletion (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    message_id INTEGER NOT NULL CHECK (message_id > 0),
    sent_at TIMESTAMPTZ NOT NULL,
    delete_at TIMESTAMPTZ NOT NULL,
    next_attempt_at TIMESTAMPTZ NOT NULL,
    lease_until TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'deleted', 'failed')),
    last_error TEXT NOT NULL DEFAULT '',
    finished_at TIMESTAMPTZ,
    UNIQUE (chat_id, message_id)
);
CREATE INDEX broadcast_message_deletion_due ON broadcast_message_deletion (next_attempt_at, id) WHERE status = 'pending';

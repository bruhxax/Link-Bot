CREATE TABLE IF NOT EXISTS bot_broadcast_draft (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    status TEXT NOT NULL DEFAULT 'idle',
    source_chat_id BIGINT,
    source_message_id INTEGER,
    source_kind TEXT NOT NULL DEFAULT '',
    source_preview TEXT NOT NULL DEFAULT '',
    buttons JSONB NOT NULL DEFAULT '[]'::jsonb,
    recipient_count INTEGER NOT NULL DEFAULT 0,
    sent_count INTEGER NOT NULL DEFAULT 0,
    failed_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    updated_by BIGINT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT bot_broadcast_draft_status_check
        CHECK (status IN ('idle', 'awaiting_message', 'draft', 'running', 'finished', 'failed'))
);

INSERT INTO bot_broadcast_draft (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;

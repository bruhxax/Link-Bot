CREATE TABLE IF NOT EXISTS bot_direct_message_drafts (
    admin_telegram_id BIGINT PRIMARY KEY,
    customer_id BIGINT NOT NULL REFERENCES customer(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'awaiting_message' CHECK (status IN ('awaiting_message', 'draft', 'sending', 'sent', 'interrupted')),
    source_chat_id BIGINT,
    source_message_id INTEGER,
    source_kind TEXT NOT NULL DEFAULT '',
    source_preview TEXT NOT NULL DEFAULT '',
    source_html TEXT NOT NULL DEFAULT '',
    previewed_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

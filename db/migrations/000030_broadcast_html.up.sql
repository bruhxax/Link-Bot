ALTER TABLE bot_broadcast_draft
    ADD COLUMN IF NOT EXISTS source_html TEXT NOT NULL DEFAULT '';

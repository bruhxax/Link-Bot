DROP TABLE IF EXISTS broadcast_message_deletion;
ALTER TABLE bot_broadcast_draft DROP COLUMN IF EXISTS delete_after_minutes;

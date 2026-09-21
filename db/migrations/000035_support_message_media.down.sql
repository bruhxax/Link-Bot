ALTER TABLE support_message
    DROP CONSTRAINT IF EXISTS support_message_media_size_check,
    DROP CONSTRAINT IF EXISTS support_message_media_type_check,
    DROP COLUMN IF EXISTS media_size_bytes,
    DROP COLUMN IF EXISTS media_original_name,
    DROP COLUMN IF EXISTS media_storage_name,
    DROP COLUMN IF EXISTS media_mime,
    DROP COLUMN IF EXISTS media_type;

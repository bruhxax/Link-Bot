ALTER TABLE support_message
    ADD COLUMN media_type          VARCHAR(16) NOT NULL DEFAULT '',
    ADD COLUMN media_mime          VARCHAR(80) NOT NULL DEFAULT '',
    ADD COLUMN media_storage_name  TEXT        NOT NULL DEFAULT '',
    ADD COLUMN media_original_name TEXT        NOT NULL DEFAULT '',
    ADD COLUMN media_size_bytes    BIGINT      NOT NULL DEFAULT 0;

ALTER TABLE support_message
    ADD CONSTRAINT support_message_media_type_check
        CHECK (media_type IN ('', 'image', 'video')),
    ADD CONSTRAINT support_message_media_size_check
        CHECK (media_size_bytes >= 0);

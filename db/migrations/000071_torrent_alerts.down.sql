DROP TABLE torrent_alert;
DELETE FROM user_activity WHERE source = 'remnawave';
ALTER TABLE user_activity DROP CONSTRAINT user_activity_source_check;
ALTER TABLE user_activity ADD CONSTRAINT user_activity_source_check
    CHECK (source IN ('miniapp', 'telegram', 'payment'));

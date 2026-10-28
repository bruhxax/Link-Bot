CREATE TABLE torrent_report (
    id BIGSERIAL PRIMARY KEY,
    event_key TEXT UNIQUE,
    panel_user_uuid UUID NOT NULL,
    panel_user_id BIGINT NOT NULL DEFAULT 0,
    username TEXT NOT NULL DEFAULT '',
    node_name TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL,
    blocked BOOLEAN NOT NULL,
    block_duration BIGINT NOT NULL DEFAULT 0,
    unblock_at TIMESTAMPTZ,
    occurred_at TIMESTAMPTZ NOT NULL,
    protocol TEXT NOT NULL DEFAULT '',
    network TEXT NOT NULL DEFAULT '',
    UNIQUE(panel_user_uuid,node_name,ip,occurred_at)
);
CREATE INDEX torrent_report_occurred_idx ON torrent_report(occurred_at DESC);

-- Recover the safe fields retained by 2.8.2 for linked accounts. The full
-- webhook was intentionally never stored, so unlinked old reports are absent.
WITH history AS (
    SELECT created_at,
      (SELECT value->>'value' FROM jsonb_array_elements(details) value WHERE value->>'label'='UUID пользователя' LIMIT 1) AS user_uuid,
      (SELECT value->>'value' FROM jsonb_array_elements(details) value WHERE value->>'label'='Пользователь панели' LIMIT 1) AS username,
      (SELECT value->>'value' FROM jsonb_array_elements(details) value WHERE value->>'label'='Нода' LIMIT 1) AS node_name,
      (SELECT value->>'value' FROM jsonb_array_elements(details) value WHERE value->>'label'='IP' LIMIT 1) AS ip,
      (SELECT value->>'value' FROM jsonb_array_elements(details) value WHERE value->>'label'='Результат' LIMIT 1) AS result,
      (SELECT value->>'value' FROM jsonb_array_elements(details) value WHERE value->>'label'='Срок блокировки' LIMIT 1) AS duration
    FROM user_activity WHERE source='remnawave' AND action='torrent_blocker.report'
)
INSERT INTO torrent_report(panel_user_uuid,username,node_name,ip,blocked,block_duration,occurred_at,unblock_at)
SELECT user_uuid::uuid,COALESCE(username,''),COALESCE(node_name,''),host(ip::inet),result='IP заблокирован на ноде',
       COALESCE(NULLIF(split_part(duration,' ',1),''),'0')::bigint,created_at,
       CASE WHEN result='IP заблокирован на ноде' THEN created_at+COALESCE(NULLIF(split_part(duration,' ',1),''),'0')::bigint*INTERVAL '1 second' END
FROM history WHERE user_uuid IS NOT NULL AND ip IS NOT NULL
ON CONFLICT DO NOTHING;

CREATE TABLE torrent_user_alert (
    event_key TEXT NOT NULL REFERENCES torrent_alert(event_key) ON DELETE CASCADE,
    telegram_id BIGINT NOT NULL,
    delivered_at TIMESTAMPTZ,
    notification_token UUID,
    lease_until TIMESTAMPTZ,
    PRIMARY KEY(event_key,telegram_id)
);
CREATE TRIGGER miniapp_torrent_report_changed AFTER INSERT OR UPDATE ON torrent_report
    FOR EACH ROW EXECUTE FUNCTION miniapp_notify_change();

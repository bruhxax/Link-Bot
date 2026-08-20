DELETE FROM device_notification_state AS state
USING (
    SELECT ctid
    FROM (
        SELECT
            ctid,
            ROW_NUMBER() OVER (
                PARTITION BY telegram_id
                ORDER BY updated_at DESC, subscription_id ASC
            ) AS row_number
        FROM device_notification_state
    ) AS ranked
    WHERE ranked.row_number > 1
) AS duplicates
WHERE state.ctid = duplicates.ctid;

ALTER TABLE device_notification_state
    DROP CONSTRAINT IF EXISTS device_notification_state_pkey,
    DROP CONSTRAINT IF EXISTS device_notification_state_subscription_fk,
    DROP COLUMN IF EXISTS subscription_id,
    ADD PRIMARY KEY (telegram_id);

DROP TABLE IF EXISTS device_notification_state;

ALTER TABLE purchase
    DROP COLUMN IF EXISTS extra_devices,
    DROP COLUMN IF EXISTS purchase_kind;

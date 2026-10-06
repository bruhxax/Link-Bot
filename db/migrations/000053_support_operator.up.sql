ALTER TABLE support_ticket ADD COLUMN operator_telegram_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE support_ticket ADD COLUMN operator_name TEXT NOT NULL DEFAULT '';

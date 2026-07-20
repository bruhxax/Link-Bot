BEGIN;

DROP INDEX IF EXISTS idx_referral_referrer_id;
DROP INDEX IF EXISTS idx_referral_referee_id;

DROP TABLE IF EXISTS referral;

ALTER TABLE customer
DROP CONSTRAINT IF EXISTS customer_telegram_id_unique;

COMMIT;

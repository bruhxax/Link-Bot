DROP TABLE IF EXISTS trial_activation_claim;
DROP SEQUENCE IF EXISTS google_customer_telegram_id_seq;

ALTER TABLE customer
  DROP COLUMN IF EXISTS telegram_id_is_synthetic;

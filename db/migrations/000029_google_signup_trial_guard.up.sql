ALTER TABLE customer
  ADD COLUMN IF NOT EXISTS telegram_id_is_synthetic BOOLEAN NOT NULL DEFAULT FALSE;

CREATE SEQUENCE IF NOT EXISTS google_customer_telegram_id_seq
  AS BIGINT
  START WITH 9000000000000000
  INCREMENT BY -1
  MINVALUE 8000000000000000
  MAXVALUE 9000000000000000
  NO CYCLE;

CREATE TABLE trial_activation_claim (
  id BIGSERIAL PRIMARY KEY,
  customer_id BIGINT NOT NULL UNIQUE REFERENCES customer(id) ON DELETE CASCADE,
  google_subject TEXT,
  browser_device_hash TEXT,
  status VARCHAR(20) NOT NULL DEFAULT 'processing' CHECK (status IN ('processing', 'granted')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  granted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX trial_activation_google_subject_idx
  ON trial_activation_claim (google_subject)
  WHERE google_subject IS NOT NULL AND google_subject <> '';

CREATE UNIQUE INDEX trial_activation_browser_device_idx
  ON trial_activation_claim (browser_device_hash)
  WHERE browser_device_hash IS NOT NULL AND browser_device_hash <> '';

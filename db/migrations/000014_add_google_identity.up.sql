ALTER TABLE customer
  ADD COLUMN IF NOT EXISTS google_subject TEXT,
  ADD COLUMN IF NOT EXISTS google_email TEXT,
  ADD COLUMN IF NOT EXISTS google_email_verified BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS google_linked_at TIMESTAMP WITH TIME ZONE;

CREATE UNIQUE INDEX IF NOT EXISTS idx_customer_google_subject
  ON customer (google_subject)
  WHERE google_subject IS NOT NULL AND google_subject <> '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_customer_google_email
  ON customer (lower(google_email))
  WHERE google_email IS NOT NULL AND google_email <> '';

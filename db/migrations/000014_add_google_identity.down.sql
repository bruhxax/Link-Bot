DROP INDEX IF EXISTS idx_customer_google_email;
DROP INDEX IF EXISTS idx_customer_google_subject;

ALTER TABLE customer
  DROP COLUMN IF EXISTS google_linked_at,
  DROP COLUMN IF EXISTS google_email_verified,
  DROP COLUMN IF EXISTS google_email,
  DROP COLUMN IF EXISTS google_subject;

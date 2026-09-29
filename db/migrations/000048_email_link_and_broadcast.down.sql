DROP TABLE IF EXISTS email_broadcast;
DELETE FROM email_auth_challenge WHERE mode = 'link';
ALTER TABLE email_auth_challenge DROP CONSTRAINT email_auth_challenge_mode_check;
ALTER TABLE email_auth_challenge
  ADD CONSTRAINT email_auth_challenge_mode_check CHECK (mode IN ('login', 'register'));
ALTER TABLE email_auth_challenge DROP COLUMN customer_id;

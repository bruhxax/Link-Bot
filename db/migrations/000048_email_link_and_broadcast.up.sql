ALTER TABLE email_auth_challenge
  ADD COLUMN customer_id BIGINT REFERENCES customer(id) ON DELETE CASCADE;

ALTER TABLE email_auth_challenge DROP CONSTRAINT email_auth_challenge_mode_check;
ALTER TABLE email_auth_challenge
  ADD CONSTRAINT email_auth_challenge_mode_check CHECK (mode IN ('login', 'register', 'link'));

CREATE TABLE email_broadcast (
  id SMALLINT PRIMARY KEY CHECK (id = 1),
  status TEXT NOT NULL DEFAULT 'idle' CHECK (status IN ('idle', 'draft', 'running', 'finished', 'failed')),
  subject TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL DEFAULT '',
  recipient_count INTEGER NOT NULL DEFAULT 0,
  sent_count INTEGER NOT NULL DEFAULT 0,
  failed_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  updated_by BIGINT,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO email_broadcast (id) VALUES (1);
